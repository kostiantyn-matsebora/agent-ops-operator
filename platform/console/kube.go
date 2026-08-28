package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// In-cluster Kubernetes access over net/http — no client-go, keeping this
// module dependency-free like every other adapter (the technique
// signal-k8s-events already uses). Only what a read-only console needs: list
// and watch on agentops.dev custom resources in its own namespace.
//
// Nothing here writes. There is no POST/PUT/PATCH/DELETE path in this file at
// all, so the console cannot mutate cluster state even if a later change
// wanted it to — a UI that renders CRs must not be one edit away from
// applying them.

const (
	defaultSADir = "/var/run/secrets/kubernetes.io/serviceaccount"
	tokenMaxAge  = 5 * time.Minute

	// APIGroup is the only group the console ever reads.
	APIGroup   = "agentops.dev"
	APIVersion = "v1alpha1"
)

// Kinds are the agentops.dev resource plurals the console watches — the wiring
// graph plus live conversations and the capability objects. Nothing outside
// this list and InstallKinds is ever requested, which is what makes the chart's
// read-only Role a complete description of the console's reach.
var Kinds = []string{
	"agentprofiles",
	"agentruntimes",
	"channels",
	"channeladapters",
	"conversations",
	"mcpconfigs",
	"mcptoolsets",
	"pipelines",
	"signaladapters",
	"signalsources",
}

// InstallKinds are the workload resources that carry INSTALL FACTS — image
// references and digests, readiness, restart counts, pod phase and failure
// reasons. None of it exists in any CR, so an operations console that cannot
// read them cannot see a CrashLoopBackOff.
//
// This is a deliberate widening past agentops.dev, and it stays read-only and
// namespaced: `get/list/watch` on these two, granted by the chart against the
// console's own ServiceAccount.
var InstallKinds = []string{
	"deployments",
	"pods",
}

// groupVersion locates a resource plural in the API. Everything not listed is
// an agentops.dev custom resource — the console's own group is the default
// precisely because it is nearly everything the console reads.
var groupVersion = map[string]struct{ Group, Version string }{
	"deployments": {"apps", "v1"},
	"pods":        {"", "v1"}, // core group: /api/v1, no /apis prefix
}

// Singular renders a plural resource name for display.
var Singular = map[string]string{
	"agentprofiles":   "AgentProfile",
	"agentruntimes":   "AgentRuntime",
	"channels":        "Channel",
	"channeladapters": "ChannelAdapter",
	"conversations":   "Conversation",
	"mcpconfigs":      "MCPConfig",
	"mcptoolsets":     "MCPToolset",
	"pipelines":       "Pipeline",
	"signaladapters":  "SignalAdapter",
	"signalsources":   "SignalSource",
	"deployments":     "Deployment",
	"pods":            "Pod",
}

// AgentOpsKind reports whether a plural is one of ours — the console's own
// kinds are what the Configuration and Topology views enumerate; the install
// kinds are facts about the deployment, not objects a user wires.
func AgentOpsKind(kind string) bool {
	_, isInstall := groupVersion[kind]
	return !isInstall
}

// Kube is a minimal in-cluster API client.
type Kube struct {
	BaseURL   string
	Namespace string
	HTTP      *http.Client

	// TokenPath is re-read rather than cached forever: projected SA tokens are
	// short-lived and rotated in place, so a long-running watcher that read the
	// file once would start 401ing after the first rotation.
	TokenPath string

	mu          sync.Mutex
	token       string
	tokenLoaded time.Time
}

// saDir is where the kubelet mounts the ServiceAccount — the default path in
// every real pod. KUBERNETES_SERVICEACCOUNT_DIR overrides it so the built
// binary can be driven black-box against a fake API server by the contract
// conformance suite; nothing in a chart sets it.
func saDir() string {
	if d := os.Getenv("KUBERNETES_SERVICEACCOUNT_DIR"); d != "" {
		return d
	}
	return defaultSADir
}

// NewInClusterKube builds a client from the pod's ServiceAccount mount and the
// API server address the kubelet injects. Requires ChannelAdapter
// spec.kubernetesAccess — without it there is no token to read.
func NewInClusterKube(namespace string) (*Kube, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, fmt.Errorf("not running in a cluster: KUBERNETES_SERVICE_HOST/PORT unset")
	}
	caPEM, err := os.ReadFile(saDir() + "/ca.crt")
	if err != nil {
		return nil, fmt.Errorf("reading API server CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("API server CA at %s/ca.crt is not valid PEM", saDir())
	}
	k := &Kube{
		BaseURL:   "https://" + net.JoinHostPort(host, port),
		Namespace: namespace,
		TokenPath: saDir() + "/token",
		HTTP: &http.Client{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
		},
	}
	if _, err := k.bearer(); err != nil {
		return nil, err
	}
	return k, nil
}

// bearer returns the SA token, re-reading it when the cached copy is stale.
func (k *Kube) bearer() (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.TokenPath == "" {
		// no projected token (tests, or an authenticating proxy in front of the
		// API server) — send no Authorization header rather than failing
		return "", nil
	}
	if k.token != "" && time.Since(k.tokenLoaded) < tokenMaxAge {
		return k.token, nil
	}
	b, err := os.ReadFile(k.TokenPath)
	if err != nil {
		if k.token != "" {
			return k.token, nil // transient read failure — keep serving with the old token
		}
		return "", fmt.Errorf("reading ServiceAccount token: %w", err)
	}
	k.token = strings.TrimSpace(string(b))
	k.tokenLoaded = time.Now()
	return k.token, nil
}

func (k *Kube) get(ctx context.Context, path string, timeout time.Duration) (*http.Response, error) {
	token, err := k.bearer()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", k.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")
	client := k.HTTP
	if timeout > 0 {
		c := *k.HTTP
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

// apiError carries the API server's status code so callers can distinguish
// "the watch expired" (410) from "we lost our permissions" (403).
type apiError struct {
	Code int
	Body string
}

func (e *apiError) Error() string { return fmt.Sprintf("kubernetes API %d: %s", e.Code, e.Body) }

// ErrWatchExpired signals that the resourceVersion is too old (410 Gone) and
// the caller must relist rather than resume.
var ErrWatchExpired = errors.New("watch expired, relist required")

// Object is one custom resource, kept as parsed metadata plus the untouched
// spec/status. The console interprets as little as it can get away with:
// opaque `config` blocks pass through to the browser verbatim, exactly as the
// manager treats them.
type Object struct {
	Kind     string          `json:"kind"` // resource plural
	Metadata Metadata        `json:"metadata"`
	Spec     json.RawMessage `json:"spec,omitempty"`
	Status   json.RawMessage `json:"status,omitempty"`
}

// Metadata is the object metadata the console displays or keys on.
type Metadata struct {
	Name              string `json:"name"`
	Namespace         string `json:"namespace,omitempty"`
	UID               string `json:"uid,omitempty"`
	ResourceVersion   string `json:"resourceVersion,omitempty"`
	CreationTimestamp string `json:"creationTimestamp,omitempty"`
	// DeletionTimestamp is set once an object is being deleted and a finalizer
	// still holds it. For a Conversation that is the close-topics finalizer
	// draining, which can hold the object for up to two minutes — long enough
	// for a closed conversation to keep reading as an open one.
	DeletionTimestamp string            `json:"deletionTimestamp,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
}

// Condition is the standard metav1.Condition shape, read generically from
// status.conditions so every kind drills down the same way.
type Condition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
}

// Conditions parses status.conditions. A kind that reports none yields nil —
// "no conditions" is a real answer, never an error.
func (o *Object) Conditions() []Condition {
	if len(o.Status) == 0 {
		return nil
	}
	var st struct {
		Conditions []Condition `json:"conditions"`
	}
	if err := json.Unmarshal(o.Status, &st); err != nil {
		return nil
	}
	return st.Conditions
}

// Condition returns one condition by type, or nil.
func (o *Object) Condition(condType string) *Condition {
	for _, c := range o.Conditions() {
		if c.Type == condType {
			cp := c
			return &cp
		}
	}
	return nil
}

type objectList struct {
	Metadata struct {
		ResourceVersion string `json:"resourceVersion"`
		Continue        string `json:"continue"`
	} `json:"metadata"`
	Items []json.RawMessage `json:"items"`
}

// listPageSize bounds one list response. A busy namespace can hold thousands
// of Conversations, and asking for all of them in a single response is how a
// read-only viewer turns into an API-server problem.
const listPageSize = 500

// resourcePath builds the namespaced collection path for a plural. The core
// group has no group segment and lives under /api rather than /apis — the one
// irregularity in the REST layout, and the reason this is a lookup rather than
// string concatenation at every call site.
func (k *Kube) resourcePath(kind string) string {
	gv, ok := groupVersion[kind]
	if !ok {
		gv.Group, gv.Version = APIGroup, APIVersion
	}
	prefix := "/apis/" + gv.Group + "/" + gv.Version
	if gv.Group == "" {
		prefix = "/api/" + gv.Version
	}
	return prefix + "/namespaces/" + url.PathEscape(k.Namespace) + "/" + url.PathEscape(kind)
}

// List returns every object of a kind in the console's namespace plus the
// resourceVersion to start watching from.
//
// Paginated: the API server is asked for pages of listPageSize and the
// `continue` token is followed to the end. The resourceVersion of the FIRST
// page is the one returned — continuation pages are served from that same
// snapshot, so it is the consistent point to start watching from.
func (k *Kube) List(ctx context.Context, kind string) ([]*Object, string, error) {
	var out []*Object
	resourceVersion, cont := "", ""
	for {
		query := fmt.Sprintf("?limit=%d", listPageSize)
		if cont != "" {
			query += "&continue=" + url.QueryEscape(cont)
		}
		resp, err := k.get(ctx, k.resourcePath(kind)+query, 60*time.Second)
		if err != nil {
			return nil, "", err
		}
		var list objectList
		err = json.NewDecoder(resp.Body).Decode(&list)
		resp.Body.Close()
		if err != nil {
			return nil, "", fmt.Errorf("decoding %s list: %w", kind, err)
		}
		for _, raw := range list.Items {
			if obj := decodeObject(kind, raw); obj != nil {
				out = append(out, obj)
			}
		}
		if resourceVersion == "" {
			resourceVersion = list.Metadata.ResourceVersion
		}
		cont = list.Metadata.Continue
		if cont == "" {
			return out, resourceVersion, nil
		}
	}
}

func decodeObject(kind string, raw json.RawMessage) *Object {
	var obj Object
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	obj.Kind = kind
	return &obj
}

// watchFrame is one line of a streaming watch response.
type watchFrame struct {
	Type   string          `json:"type"`
	Object json.RawMessage `json:"object"`
}

// Watch streams changes to a kind from resourceVersion, calling fn for each
// ADDED/MODIFIED/DELETED frame. It returns when the stream ends, the context is
// cancelled, or the watch expires (ErrWatchExpired — the caller relists).
func (k *Kube) Watch(ctx context.Context, kind, resourceVersion string, fn func(eventType string, obj *Object)) error {
	q := "?watch=1&allowWatchBookmarks=true&timeoutSeconds=300"
	if resourceVersion != "" {
		q += "&resourceVersion=" + url.QueryEscape(resourceVersion)
	}
	// no client timeout: a watch is a long-lived stream, bounded by
	// timeoutSeconds server-side and by ctx here
	resp, err := k.get(ctx, k.resourcePath(kind)+q, 0)
	if err != nil {
		var apiErr *apiError
		if errors.As(err, &apiErr) && apiErr.Code == http.StatusGone {
			return ErrWatchExpired
		}
		return err
	}
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	for {
		var frame watchFrame
		if err := dec.Decode(&frame); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, io.EOF) {
				return nil // server closed the stream — caller re-watches
			}
			return fmt.Errorf("decoding %s watch stream: %w", kind, err)
		}
		switch frame.Type {
		case "ADDED", "MODIFIED", "DELETED":
			if obj := decodeObject(kind, frame.Object); obj != nil {
				fn(frame.Type, obj)
			}
		case "BOOKMARK":
			// carries only a resourceVersion — surface it as a no-op delta so
			// the cache can advance its resume point without a relist
			if obj := decodeObject(kind, frame.Object); obj != nil {
				fn("BOOKMARK", obj)
			}
		case "ERROR":
			var status struct {
				Code   int32  `json:"code"`
				Reason string `json:"reason"`
			}
			_ = json.Unmarshal(frame.Object, &status)
			if status.Code == http.StatusGone || status.Reason == "Expired" {
				return ErrWatchExpired
			}
			return fmt.Errorf("%s watch error frame: %s (%d)", kind, status.Reason, status.Code)
		}
	}
}
