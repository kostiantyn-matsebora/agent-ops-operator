// state.go: the console's client for the manager's state read surface
// (openspec/changes/state-store-generic-contract, design D-5/D-6/D-8) — the
// ONE door this module reaches a Conversation or a Signal through. Every
// other agentops.dev kind stays on the Kubernetes watch in kube.go.
//
// stateSource is shaped as an apiSource (cache.go) so it drops straight into
// the same Cache that watches configuration kinds: List/Get/Watch, the same
// ADDED/MODIFIED/DELETED/BOOKMARK vocabulary, the same resume-by-revision,
// relist-on-expiry contract kube.go's own implementation already honours.
// compositeSource below is what lets one Cache hold both origins at once —
// see main.go for where it is built and handed to NewCache.
//
// The console authenticates with its own derived adapter token, the same
// ADAPTER_TOKEN it already carries as a ChannelAdapter (config.go) — the
// manager admits it on /state/* once the console's CR sets
// spec.stateAccess: true (a parallel chart change). No new credential.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// StateKinds are the two kinds the manager's state read surface serves —
// disjoint from Kinds (kube.go) by construction. This module's own
// TestKindsAreDisjointFromStateKinds and the chart's Role render test both
// assert that split, so the two cannot drift apart (design D-8).
var StateKinds = []string{"conversations", "signals"}

// ConfigurableKinds is every kind the console's generic configuration
// surface — handleKinds, knownKind, the Overview rollup — treats as its
// own: Kinds plus StateKinds. Kinds itself stays scoped to what kube.go
// actually requests from the Kubernetes API, since it is also what the
// chart derives its read-only Role from; a caller asking "what kind does
// this console know how to show" wants this list instead.
var ConfigurableKinds = append(append([]string{}, Kinds...), StateKinds...)

// stateRecord is the wire record design D-5 publishes: a kind's published
// field names, no metadata, no labels, no annotations. Spec/status are kept
// as raw JSON, exactly as kube.go's Object does — the console never
// needs more out of them than the small declared shapes it already
// decodes on demand (configapi.go's decodeSpec).
type stateRecord struct {
	Name                string          `json:"name"`
	ID                  string          `json:"id"`
	Revision            string          `json:"revision"`
	CreatedAt           string          `json:"createdAt"`
	DeletionRequestedAt *string         `json:"deletionRequestedAt"`
	Spec                json.RawMessage `json:"spec"`
	Status              json.RawMessage `json:"status"`
}

// toObject maps the wire record onto the same Object kube.go's client
// builds, so every reader downstream of the Cache — topology, convapi,
// configapi, adapter — stays unaware of which source answered it.
// Namespace, Labels and Annotations are left zero: the wire record carries
// none of the three (design D-5's "no metadata, labels or annotations
// cross the wire"), an accepted loss on these two kinds alone
// (design.md's Risks: "the Configuration page loses namespace, labels,
// annotations on two kinds").
func (r stateRecord) toObject(kind string) *Object {
	o := &Object{Kind: kind, Spec: r.Spec, Status: r.Status}
	o.Metadata.Name = r.Name
	o.Metadata.UID = r.ID
	o.Metadata.ResourceVersion = r.Revision
	o.Metadata.CreationTimestamp = r.CreatedAt
	if r.DeletionRequestedAt != nil {
		o.Metadata.DeletionTimestamp = *r.DeletionRequestedAt
	}
	return o
}

// stateListing is the envelope GET /state/conversations and
// GET /state/signals both answer with: {"revision","items":[record]}.
type stateListing struct {
	Revision string        `json:"revision"`
	Items    []stateRecord `json:"items"`
}

// stateSource is the apiSource backing StateKinds: it talks to the
// manager's read-only /state/* surface over plain net/http, never the
// Kubernetes API.
type stateSource struct {
	baseURL string
	token   string
	http    *http.Client

	// known is every name this source has handed the Cache, per kind: the
	// feed's one `put` frame stands for create and update alike, and the
	// browser inserts a row only on ADDED, so the first sighting of a name is
	// reported as one.
	mu    sync.Mutex
	known map[string]map[string]bool

	// idleTimeout is how long a watch may be silent before it is dropped.
	idleTimeout time.Duration
}

// newStateSource builds the client from the console's own config: the
// manager's Service URL and the derived adapter token it already holds as
// a ChannelAdapter — no credential this module does not already have.
func newStateSource(cfg *Config) *stateSource {
	return &stateSource{
		baseURL: strings.TrimRight(cfg.ManagerURL, "/"),
		token:   cfg.AdapterToken,
		http:    &http.Client{},
		known:   map[string]map[string]bool{},

		idleTimeout: defaultStreamIdle,
	}
}

// defaultStreamIdle is how long a stream may say nothing, keep-alives
// included, before it is taken for dead. The manager writes one every 20s, so
// this spans several lost ones.
const defaultStreamIdle = 90 * time.Second

// errStreamIdle ends a Watch whose connection went silent. The cache treats it
// like any dropped stream: reconnect, and relist.
var errStreamIdle = errors.New("state stream went silent")

// idleReader resets a timer on every read that returns bytes, and cancels the
// stream when the timer fires before the next.
type idleReader struct {
	r     io.Reader
	timer *time.Timer
	limit time.Duration
}

func (i *idleReader) Read(p []byte) (int, error) {
	n, err := i.r.Read(p)
	if n > 0 {
		i.timer.Reset(i.limit)
	}
	return n, err
}

// seen records name as known for kind and reports whether it already was.
func (s *stateSource) seen(kind, name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.known[kind] == nil {
		s.known[kind] = map[string]bool{}
	}
	was := s.known[kind][name]
	s.known[kind][name] = true
	return was
}

func (s *stateSource) forget(kind, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.known[kind], name)
}

// statePath is the collection path for a kind this source serves, or an
// error naming the kind — a stateSource must never silently answer for a
// kind it was not built to serve.
func statePath(kind string) (string, error) {
	for _, k := range StateKinds {
		if kind == k {
			return "/state/" + kind, nil
		}
	}
	return "", fmt.Errorf("state source: %q is not a kind the manager's state read surface serves (only %v)", kind, StateKinds)
}

// do issues one authenticated GET and returns the response body for the
// caller to decode, or an *apiError carrying the status — the same error
// shape kube.go's own client uses, so a caller need not know which source
// answered to handle a failure.
//
// timeout bounds this one call, mirroring kube.go's own get(): a List or Get
// that dials a backend mid-rollout and never answers must fail and retry,
// not block runKind's goroutine forever with no error to log and no further
// reconnect attempt. 0 leaves it unbounded — Watch's own contract, since a
// resumable feed is meant to sit open until the caller's ctx ends.
func (s *stateSource) do(ctx context.Context, path, accept string, timeout time.Duration) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	req.Header.Set("Accept", accept)
	client := s.http
	if timeout > 0 {
		c := *s.http
		c.Timeout = timeout
		client = &c
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		resp.Body.Close()
		return nil, &apiError{Code: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	return resp, nil
}

// List reads the whole set in one call. Design D-5's listing already
// carries each record's appended items (a Conversation's runs and their
// inputs) flattened server-side, so there is no per-record fetch to make
// here — the contract's own reason for growing once (design.md) is exactly
// to avoid that.
func (s *stateSource) List(ctx context.Context, kind string) ([]*Object, string, error) {
	path, err := statePath(kind)
	if err != nil {
		return nil, "", err
	}
	resp, err := s.do(ctx, path, "application/json", 60*time.Second)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	var listing stateListing
	if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
		return nil, "", fmt.Errorf("decoding %s listing: %w", kind, err)
	}
	objs := make([]*Object, 0, len(listing.Items))
	s.mu.Lock()
	s.known[kind] = make(map[string]bool, len(listing.Items))
	for _, item := range listing.Items {
		objs = append(objs, item.toObject(kind))
		s.known[kind][item.Name] = true
	}
	s.mu.Unlock()
	return objs, listing.Revision, nil
}

// Get resolves one Conversation by name — the Cache's "seen the name, not
// yet the object" path (adapter.go's threadID, deriving a thread id from a
// conversation's UID before the watch has caught up). Signals carry no
// such endpoint: design D-5 serves no GET /state/signals/{name}, "not
// served, because nothing reads it". Asking for one here is a programming
// error this reports clearly rather than answering with an empty or wrong
// object.
func (s *stateSource) Get(ctx context.Context, kind, name string) (*Object, error) {
	if kind != "conversations" {
		return nil, fmt.Errorf("state source: GET /state/%s/{name} does not exist (design D-5 serves a single-record read for conversations only)", kind)
	}
	resp, err := s.do(ctx, "/state/conversations/"+url.PathEscape(name), "application/json", 10*time.Second)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var rec stateRecord
	if err := json.NewDecoder(resp.Body).Decode(&rec); err != nil {
		return nil, fmt.Errorf("decoding conversation %s: %w", name, err)
	}
	return rec.toObject(kind), nil
}

// Watch opens the resumable feed and translates its frames into the same
// ADDED/MODIFIED/DELETED/BOOKMARK vocabulary kube.go's own Watch emits —
// the Cache cannot tell, and must not need to, which source answered.
//
// A `resync` frame (design D-5: an unknown or stale `since` is relisted
// rather than errored) is surfaced as ErrWatchExpired, the same signal
// kube.go's 410 path already uses. cache.go's runKind sees that error and
// drops its resourceVersion so the NEXT loop iteration relists before
// resuming — the "existing backoff" the task calls for, not a second one
// invented here. The cost is one extra round trip versus consuming the
// rest of the same connection the server kept open after its own resync
// frame: correct either way, since a relist always reads the live set.
func (s *stateSource) Watch(ctx context.Context, kind, revision string, fn func(eventType string, obj *Object)) error {
	path, err := statePath(kind)
	if err != nil {
		return err
	}
	streamPath := path + "/stream"
	if revision != "" {
		streamPath += "?since=" + url.QueryEscape(revision)
	}
	resp, err := s.do(ctx, streamPath, "text/event-stream", 0)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// A connection that dies without closing leaves Read blocked for ever.
	// Silence past the limit cancels the read, and is reported as such.
	idleCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	limit := s.idleTimeout
	if limit <= 0 {
		limit = defaultStreamIdle
	}
	var idled atomic.Bool
	timer := time.AfterFunc(limit, func() { idled.Store(true); cancel() })
	defer timer.Stop()
	go func() { <-idleCtx.Done(); _ = resp.Body.Close() }() // unblocks the pending Read
	err = scanSSE(idleCtx, &idleReader{r: resp.Body, timer: timer, limit: limit}, func(f sseFrame) error {
		return applySSEFrame(kind, f, s, fn)
	})
	if idled.Load() && ctx.Err() == nil {
		return errStreamIdle
	}
	return err
}

// applySSEFrame decodes one dispatched SSE event and calls fn in the
// Cache's own vocabulary, or returns ErrWatchExpired for a resync.
func applySSEFrame(kind string, f sseFrame, s *stateSource, fn func(eventType string, obj *Object)) error {
	switch f.event {
	case "put":
		var rec stateRecord
		if err := json.Unmarshal(f.data, &rec); err != nil {
			return fmt.Errorf("decoding %s put frame: %w", kind, err)
		}
		obj := rec.toObject(kind)
		if f.id != "" {
			obj.Metadata.ResourceVersion = f.id
		}
		// A `put` frame does not say which this was; a name the source has
		// not handed over yet is an addition.
		if s.seen(kind, obj.Metadata.Name) {
			fn("MODIFIED", obj)
		} else {
			fn("ADDED", obj)
		}
	case "delete":
		var del struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(f.data, &del); err != nil {
			return fmt.Errorf("decoding %s delete frame: %w", kind, err)
		}
		s.forget(kind, del.Name)
		obj := &Object{Kind: kind}
		obj.Metadata.Name = del.Name
		obj.Metadata.ResourceVersion = f.id
		fn("DELETED", obj)
	case "bookmark":
		obj := &Object{Kind: kind}
		obj.Metadata.ResourceVersion = f.id
		fn("BOOKMARK", obj)
	case "resync":
		return ErrWatchExpired
	}
	return nil
}

// sseFrame is one dispatched server-sent event: the fields of one or more
// `field: value` lines, grouped by the blank line that ends them.
type sseFrame struct {
	event string
	id    string
	data  []byte
}

// scanSSE reads the text/event-stream format the manager's
// /state/.../stream endpoints write (httpapi.writeSSEFrame): `event:`,
// `id:` and `data:` lines terminated by a blank line, calling fn once per
// dispatched event. A line starting with `:` is a comment — the server's
// keep-alive (design D-5's Feed) — and carries no field. Returns when the
// stream ends cleanly, when fn errors, or when ctx ends.
func scanSSE(ctx context.Context, body io.Reader, fn func(sseFrame) error) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var cur sseFrame
	var data []string
	dispatch := func() error {
		defer func() { cur, data = sseFrame{}, nil }()
		if cur.event == "" && len(data) == 0 {
			return nil // a bare blank line, or only a comment, dispatches nothing
		}
		cur.data = []byte(strings.Join(data, "\n"))
		return fn(cur)
	}
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if err := dispatch(); err != nil {
				return err
			}
		case strings.HasPrefix(line, ":"):
			// keep-alive comment: no field, resets nothing
		case strings.HasPrefix(line, "event:"):
			cur.event = sseField(line, "event:")
		case strings.HasPrefix(line, "id:"):
			cur.id = sseField(line, "id:")
		case strings.HasPrefix(line, "data:"):
			data = append(data, sseField(line, "data:"))
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("reading SSE stream: %w", err)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil // server closed the stream cleanly — caller re-watches
}

// sseField strips an SSE field's prefix and the one optional leading space
// the wire format (and the manager's own writer) uses after the colon.
func sseField(line, prefix string) string {
	return strings.TrimPrefix(strings.TrimPrefix(line, prefix), " ")
}

// compositeSource routes StateKinds to a stateSource and every other kind
// to the Kubernetes-backed apiSource untouched — one Cache, two origins,
// the same apiSource shape either way (design D-5/D-8). This is the one
// place the two sources join; main.go builds it and hands it to NewCache
// in place of the bare Kube client this module used before.
type compositeSource struct {
	state apiSource
	kube  apiSource
}

// newCompositeSource builds the router. kube serves Kinds and InstallKinds
// as it always has; state serves StateKinds over the manager instead.
func newCompositeSource(kube, state apiSource) *compositeSource {
	return &compositeSource{kube: kube, state: state}
}

// route picks the backing source for one kind — the ONLY place this file
// decides which of the two a request goes to.
func (c *compositeSource) route(kind string) apiSource {
	for _, k := range StateKinds {
		if kind == k {
			return c.state
		}
	}
	return c.kube
}

func (c *compositeSource) List(ctx context.Context, kind string) ([]*Object, string, error) {
	return c.route(kind).List(ctx, kind)
}

func (c *compositeSource) Get(ctx context.Context, kind, name string) (*Object, error) {
	return c.route(kind).Get(ctx, kind, name)
}

func (c *compositeSource) Watch(ctx context.Context, kind, resourceVersion string, fn func(eventType string, obj *Object)) error {
	return c.route(kind).Watch(ctx, kind, resourceVersion, fn)
}
