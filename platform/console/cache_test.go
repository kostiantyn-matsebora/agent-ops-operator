package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// The cache's whole job is convergence: after any sequence of disconnects,
// expiries and reconnects the store must equal what the API server holds.
// These tests drive it with recorded frames — no cluster, no envtest.

// fakeSource replays scripted list results and watch frames.
type fakeSource struct {
	mu sync.Mutex
	// lists is consumed one entry per List call (last entry repeats).
	lists []listResult
	// watches is consumed one entry per Watch call (last entry repeats).
	watches   []watchScript
	listCalls int
	watchArgs []string // resourceVersion passed to each Watch
}

type listResult struct {
	objs []*Object
	rv   string
	err  error
}

type watchScript struct {
	frames []frame
	err    error
	// block holds the watch open until the context ends (a healthy idle watch).
	block bool
}

type frame struct {
	eventType string
	obj       *Object
}

func (f *fakeSource) List(ctx context.Context, kind string) ([]*Object, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.listCalls
	if i >= len(f.lists) {
		i = len(f.lists) - 1
	}
	f.listCalls++
	r := f.lists[i]
	return r.objs, r.rv, r.err
}

func (f *fakeSource) Watch(ctx context.Context, kind, rv string, fn func(string, *Object)) error {
	f.mu.Lock()
	i := len(f.watchArgs)
	f.watchArgs = append(f.watchArgs, rv)
	if i >= len(f.watches) {
		i = len(f.watches) - 1
	}
	w := f.watches[i]
	f.mu.Unlock()
	for _, fr := range w.frames {
		fn(fr.eventType, fr.obj)
	}
	if w.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return w.err
}

func (f *fakeSource) watchStarts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.watchArgs))
	copy(out, f.watchArgs)
	return out
}

func obj(kind, name, rv string, spec, status string) *Object {
	o := &Object{Kind: kind}
	o.Metadata.Name = name
	o.Metadata.ResourceVersion = rv
	o.Metadata.UID = "uid-" + name
	if spec != "" {
		o.Spec = json.RawMessage(spec)
	}
	if status != "" {
		o.Status = json.RawMessage(status)
	}
	return o
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not reached in time")
}

// The real client must follow `continue` tokens: a namespace with thousands
// of Conversations is served in pages, and stopping at the first one would
// silently show a fraction of the cluster.
func TestKubeListFollowsContinueTokens(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("continue") == "" {
			_, _ = io.WriteString(w, `{"metadata":{"resourceVersion":"100","continue":"tok-2"},
				"items":[{"metadata":{"name":"a","resourceVersion":"1"}}]}`)
			return
		}
		// continuation pages carry no resourceVersion of their own
		_, _ = io.WriteString(w, `{"metadata":{"resourceVersion":"","continue":""},
			"items":[{"metadata":{"name":"b","resourceVersion":"2"}}]}`)
	}))
	defer srv.Close()

	k := &Kube{BaseURL: srv.URL, Namespace: "agent-ops", HTTP: srv.Client(), TokenPath: ""}
	objs, rv, err := k.List(context.Background(), "conversations")
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 2 || objs[0].Metadata.Name != "a" || objs[1].Metadata.Name != "b" {
		t.Fatalf("both pages must be collected: %+v", objs)
	}
	// the FIRST page's resourceVersion is the consistent watch start point
	if rv != "100" {
		t.Fatalf("watch start point must come from the first page: %q", rv)
	}
	if len(paths) != 2 || !strings.Contains(paths[0], "limit=500") || !strings.Contains(paths[1], "continue=tok-2") {
		t.Fatalf("pagination requests wrong: %v", paths)
	}
}

func TestCacheListThenWatchApplies(t *testing.T) {
	src := &fakeSource{
		lists: []listResult{{objs: []*Object{obj("pipelines", "a", "1", "{}", "")}, rv: "10"}},
		watches: []watchScript{{frames: []frame{
			{"ADDED", obj("pipelines", "b", "11", "{}", "")},
			{"MODIFIED", obj("pipelines", "a", "12", `{"profileRef":{"name":"p"}}`, "")},
			{"DELETED", obj("pipelines", "b", "13", "{}", "")},
		}, block: true}},
	}
	c := NewCache(src, []string{"pipelines"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)

	if !c.WaitForSync(ctx) {
		t.Fatal("cache never synced")
	}
	waitFor(t, func() bool { return len(c.List("pipelines")) == 1 && c.Get("pipelines", "b") == nil })
	got := c.Get("pipelines", "a")
	if got == nil || decodeSpec[pipelineSpec](got.Spec).ProfileRef.Name != "p" {
		t.Fatalf("MODIFIED not applied: %+v", got)
	}
	// the watch must have resumed from the list's resourceVersion
	if starts := src.watchStarts(); len(starts) == 0 || starts[0] != "10" {
		t.Fatalf("watch did not resume from the list rv: %v", starts)
	}
}

func TestCacheRelistsOnExpiry(t *testing.T) {
	// first watch expires (410); the cache must relist and NOT resume from the
	// stale resourceVersion — the recovery path browsers never see.
	src := &fakeSource{
		lists: []listResult{
			{objs: []*Object{obj("channels", "old", "1", "{}", "")}, rv: "10"},
			{objs: []*Object{obj("channels", "fresh", "2", "{}", "")}, rv: "20"},
		},
		watches: []watchScript{{err: ErrWatchExpired}, {block: true}},
	}
	c := NewCache(src, []string{"channels"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)

	waitFor(t, func() bool { return c.Get("channels", "fresh") != nil })
	if c.Get("channels", "old") != nil {
		t.Fatal("relist must replace the store, not merge into it")
	}
	waitFor(t, func() bool {
		starts := src.watchStarts()
		return len(starts) >= 2 && starts[1] == "20"
	})
}

func TestCacheSubscribersGetDeltasAndResync(t *testing.T) {
	src := &fakeSource{
		lists: []listResult{{objs: nil, rv: "1"}},
		watches: []watchScript{{frames: []frame{
			{"ADDED", obj("conversations", "c1", "2", "{}", "")},
		}, block: true}},
	}
	c := NewCache(src, []string{"conversations"})
	deltas, cancelSub := c.Subscribe()
	defer cancelSub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)

	var kinds []string
	timeout := time.After(3 * time.Second)
	for len(kinds) < 2 {
		select {
		case d := <-deltas:
			kinds = append(kinds, d.Type)
		case <-timeout:
			t.Fatalf("expected a RESYNC then an ADDED, got %v", kinds)
		}
	}
	if kinds[0] != DeltaResync || kinds[1] != "ADDED" {
		t.Fatalf("unexpected delta sequence: %v", kinds)
	}
}

// A slow subscriber must never wedge the watch loop: it is told to resync
// instead of accumulating a backlog, because CR state — not the event
// sequence — is what the console renders from.
func TestCacheSlowSubscriberIsResyncedNotBlocked(t *testing.T) {
	c := NewCache(&fakeSource{lists: []listResult{{rv: "1"}}, watches: []watchScript{{block: true}}}, []string{"pipelines"})
	ch, cancel := c.Subscribe()
	defer cancel()
	for i := 0; i < 1000; i++ { // far past the 256 buffer
		c.apply("ADDED", obj("pipelines", "p", "1", "{}", ""))
	}
	drained := 0
	sawResync := false
	for {
		select {
		case d := <-ch:
			drained++
			if d.Type == DeltaResync {
				sawResync = true
			}
			continue
		default:
		}
		break
	}
	if drained == 0 {
		t.Fatal("subscriber received nothing")
	}
	// publishing never blocked (the loop above returned), and the overflow was
	// converted into a resync rather than lost silently
	c.apply("ADDED", obj("pipelines", "q", "1", "{}", ""))
	select {
	case d := <-ch:
		sawResync = sawResync || d.Type == DeltaResync
	case <-time.After(time.Second):
		t.Fatal("no delta after draining")
	}
	if !sawResync {
		t.Fatal("overflow must surface as RESYNC")
	}
}
