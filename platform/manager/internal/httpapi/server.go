// Package httpapi is the manager's HTTP surface:
//
//	GET  /healthz
//	GET  /work?convo=&wait=&pod=   runtime long-poll dispatch
//	POST /work/done                runtime completion report
//	POST /work/context             context-sync sidecar operation report
//
// The manager hosts NO signal transports — alert/webhook ingestion lives in
// signal adapters feeding POST /signal/inbound. That endpoint is also how work
// ORIGINATES: there is no route that names a Pipeline, because a caller
// choosing its own wiring is what the Pipeline model exists to prevent.
//
// plus the channel adapter contract (bearer-token auth, see ADAPTER_TOKEN):
//
//	GET  /channel/ops?adapter=&wait=    outbound op long-poll (204 on timeout)
//	POST /channel/ops/{id}/done         async op completion
//	POST /channel/inbound               {"channel","threadId","text"} -> router
//	GET  /channel/channels?adapter=     channels served by an adapter (+config)
//	POST /channel/conversations/{name}/reopen         return a Closed one to Idle
//	POST /channel/conversations/{name}/delete         remove an already-Closed one
//	POST /channel/conversations/{name}/reset-context  clear its context handle
//	GET  /channel/vocabulary            what may be typed, + its revision
//	GET  /channel/state/{channel}/{key} adapter cursor state (Stores.AdapterState)
//	PUT  /channel/state/{channel}/{key}
//	POST /channel/channels/{name}/status  {"ready","reason"?,"message"?}
//	POST /channel/read                  {"channel","reads":[{"threadId","readAt"}]}
//
// plus the state read surface (state.go; design state-store-generic-contract
// D-5/D-6), guarded by stateAuth rather than adapterAuth:
//
//	GET /state/conversations               {"revision","items":[record]}
//	GET /state/conversations/{name}        one record
//	GET /state/conversations/stream?since= SSE: put|delete|bookmark|resync
//	GET /state/signals
//	GET /state/signals/stream?since=
//
// /channel/read is OPTIONAL: an adapter that never calls it stays fully
// conformant and its threads simply carry no watermark. The manager owns the
// write — the adapter reports how far a thread has been seen and issues no
// Kubernetes write of its own.
package httpapi

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	controlplane "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/control-plane"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/convtask"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/dispatch"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/ingest"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/storagebreaker"
)

const errInvalidJSON = "invalid JSON"

// Server carries dependencies for the HTTP surface.
type Server struct {
	Client client.Client // cached
	Reader client.Reader // APIReader: strong reads for dispatch state
	// Conversations, Runs and Blobs are the only way this surface reaches a
	// Conversation, its run history or a queued input's payload (state-store
	// design D-2).
	Conversations state.Collection[state.ConversationRecord]
	Runs          state.Appended[state.ConversationRecord, state.Run]
	Blobs         state.Blobs
	// Signals is the only way publication stores a Signal.
	Signals state.Expiring[state.SignalRecord]
	// AdapterState is the only way /channel/state and /signal/state read or
	// write an adapter's cursor (design D-4) — keyed by state.AdapterStateKey,
	// never a raw annotation patch.
	AdapterState state.Collection[string]
	Namespace    string
	Addr         string

	// Runtime is the bootstrap execution config, the same fallback the
	// reconciler builds pods from. Dispatch needs it to answer ONE question
	// before promising continuity: does this deployment give the runtime
	// anywhere to keep a conversation's context?
	Runtime runtimepod.Config

	// Channel adapter contract. AdapterToken guards /channel/*; empty
	// disables the surface (503) — the manager reads it from env, never from
	// the Secret API.
	Ops          *chat.OpQueue
	Router       *chat.Router
	AdapterToken string

	// Bus is the command bus (async-command-bus design D-9) every accept
	// handler durably enqueues onto instead of doing the work inline, and
	// the registered consumer.Pool(s) (cmd/manager) claim from to run
	// HandleConvTask. Required by every handler in accept.go/convtasks.go —
	// a zero-valued Server built for a test that does not exercise one of
	// them never dereferences it.
	Bus commandbus.Bus

	// ControlPlaneToken guards /control-plane/* (internal/control-plane). A
	// single flat shared secret, its OWN env var (CONTROL_PLANE_TOKEN) —
	// deliberately isolated from AdapterToken and the coordinator token, so
	// a caller holding one can never reach the other's surface. Empty
	// disables the surface (503), the same unconfigured-is-unauthorized
	// rule AdapterToken follows.
	ControlPlaneToken string

	// StateReadToken guards /state/* (design D-6) for a caller that is not an
	// adapter — today housekeeping alone. Read from STATE_READ_TOKEN by
	// cmd/manager/main.go only, never from a Secret. Empty disables this ONE
	// credential; the master token and a stateAccess-granted adapter token
	// still open the surface.
	StateReadToken string

	// Activity is the per-hop telemetry log served by /activity*. A nil log is
	// inert, so every emission site can call it unguarded.
	Activity *activity.Log

	// Version is the manager build reported by GET /status.
	Version string
	// MaxActiveConversations is the runtime-slot ceiling reported by
	// GET /status alongside the slots actually in use.
	MaxActiveConversations int

	// MaxQueuedConversations bounds the PENDING backlog. An unbounded queue
	// reproduces the capacity complaint one level down — no pods, but unbounded
	// Conversation CRs — so ingest declines to create beyond it. This is the one
	// capacity check that cannot live in the reconciler: the point is not to
	// create the object at all. It is a count, not a scheduling decision, so a
	// stale read at worst admits one over the bound. <=0 means the default.
	MaxQueuedConversations int

	// Cooldowns is the leader's per-source cooldown map the consumer pool
	// shares, read here only to report on it (GET /status, the metrics
	// sample). Nil is usable and reports nothing.
	Cooldowns *ingest.CooldownBook

	// StorageBreaker judges whether unreachable context is one conversation's
	// loss or an install-wide outage. SHARED with the reconciler, which feeds
	// it the provisioning edge — a pod that cannot attach its volume is the
	// same incident as a run that cannot reach its context, and two breakers
	// disagreeing about that would surface as a bug report far from either.
	// Nil is usable: one is built lazily so a zero-valued Server works.
	StorageBreaker *storagebreaker.Breaker

	// ReplicaIdentity is this process's own identity, the same string the
	// reconciler's claims carry (durable-chat-ops-broker). Compared against
	// the leader-election Lease's holder to decide whether THIS replica may
	// safely claim a channel op on /channel/ops. Empty disables the check
	// (tests, and any Server built with no leader election behind it at
	// all), so a zero-valued Server keeps answering as it always has.
	ReplicaIdentity string

	breakerOnce sync.Once
}

// defaultMaxQueuedConversations is the pending-backlog bound when unset.
const defaultMaxQueuedConversations = 50

const (
	errAdapterAuthUnset = "adapter auth not configured"
	bearerPrefix        = "Bearer "
)

func (s *Server) maxQueued() int {
	if s.MaxQueuedConversations > 0 {
		return s.MaxQueuedConversations
	}
	return defaultMaxQueuedConversations
}

// Handler builds the HTTP mux (shared by Start and tests).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /work", s.handleWork)
	mux.HandleFunc("POST /work/done", s.handleWorkDone)
	mux.HandleFunc("POST /work/context", s.handleContextReport)
	mux.HandleFunc("GET /channel/ops", s.adapterAuth(s.handleChannelOps))
	mux.HandleFunc("POST /channel/ops/{id}/done", s.adapterAuth(s.handleChannelOpDone))
	mux.HandleFunc("POST /channel/inbound", s.adapterAuth(s.handleChannelInbound))
	mux.HandleFunc("POST /channel/conversations/{name}/reopen", s.adapterAuth(s.handleConversationReopen))
	mux.HandleFunc("POST /channel/conversations/{name}/delete", s.adapterAuth(s.handleConversationDelete))
	mux.HandleFunc("POST /channel/conversations/{name}/reset-context", s.adapterAuth(s.handleContextReset))
	mux.HandleFunc("GET /channel/channels", s.adapterAuth(s.handleChannelList))
	mux.HandleFunc("GET /channel/vocabulary", s.adapterAuth(s.handleVocabulary))
	mux.HandleFunc("GET /channel/state/{channel}/{key}", s.adapterAuth(s.handleStateGet))
	mux.HandleFunc("PUT /channel/state/{channel}/{key}", s.adapterAuth(s.handleStatePut))
	mux.HandleFunc("POST /channel/channels/{name}/status", s.adapterAuth(s.handleChannelStatus))
	mux.HandleFunc("POST /channel/read", s.adapterAuth(s.handleChannelRead))
	mux.HandleFunc("POST /signal/inbound", s.signalAuth(s.handleSignalInbound))
	mux.HandleFunc("GET /signal/sources", s.signalAuth(s.handleSignalSources))
	mux.HandleFunc("GET /signal/state/{source}/{key}", s.signalAuth(s.handleSignalStateGet))
	mux.HandleFunc("PUT /signal/state/{source}/{key}", s.signalAuth(s.handleSignalStatePut))
	mux.HandleFunc("POST /signal/sources/{name}/status", s.signalAuth(s.handleSignalStatus))
	mux.HandleFunc("GET /state/conversations", s.stateAuth(s.handleStateConversationsList))
	mux.HandleFunc("GET /state/conversations/stream", s.stateAuth(s.handleStateConversationsStream))
	mux.HandleFunc("GET /state/conversations/{name}", s.stateAuth(s.handleStateConversationGet))
	mux.HandleFunc("GET /state/signals", s.stateAuth(s.handleStateSignalsList))
	mux.HandleFunc("GET /state/signals/stream", s.stateAuth(s.handleStateSignalsStream))
	// async-command-bus task 5.8: read-only, same /state/* token scheme.
	mux.HandleFunc("GET /state/tasks", s.stateAuth(s.handleStateTasksList))
	mux.HandleFunc("GET /state/tasks/stream", s.stateAuth(s.handleStateTasksStream))
	mux.HandleFunc("GET /state/tasks/{id}", s.stateAuth(s.handleStateTaskGet))
	mux.HandleFunc("GET /state/dead-letters", s.stateAuth(s.handleStateDeadLetters))
	mux.HandleFunc("GET /activity", s.anyAdapterAuth(s.handleActivity))
	mux.HandleFunc("GET /activity/stream", s.anyAdapterAuth(s.handleActivityStream))
	mux.HandleFunc("POST /activity", s.anyAdapterAuth(s.handleActivityReport))
	mux.HandleFunc("GET /status", s.anyAdapterAuth(s.handleStatus))
	mux.HandleFunc("GET /pipelines/{name}/resolved", s.anyAdapterAuth(s.handlePipelineResolved))
	s.RegisterCoordinateRoutes(mux)
	// /control-plane/*: its own package, its own dedicated token — see
	// internal/control-plane's doc comment for why this is not folded into
	// adapterAuth/the coordinator token.
	controlplane.RegisterRoutes(mux, s.ControlPlaneToken, s.Client, s.Reader, s.Namespace)
	return s.leaderProxyMiddleware(mux)
}

// leaderProxyMiddleware is the ONE place every route becomes a dumb proxy on
// a non-leader (hardcode-manager-replicas, generalized from the single
// /channel/ops exception chat-ops-broker-telegram-gateway shipped first).
//
// Every endpoint here ultimately reads or writes the Kubernetes API, which is
// consistent regardless of which replica answers — so in principle any
// replica could serve any of them correctly. In practice, repeatedly fixing
// ONE more endpoint at a time (first /channel/ops, then /activity*) was
// exactly the whack-a-mole this exists to stop: a replica this request
// happens to land on is an implementation detail no caller should have to
// route around, and the next endpoint nobody thought to check is the one
// that breaks next.
//
// /healthz is the one exemption — it answers liveness/readiness for THIS
// pod, and proxying it would report on a different pod's health entirely.
// /state/ joins it (design D-7): a store read is backend-consistent, not
// leader-affine — Get/List/Watch answer the same from any replica, so
// proxying would only drop the console's and housekeeping's feeds during a
// leader rollout for no correctness gain.
//
// A handler reached despite this (the proxy attempt failed, or none applied)
// falls through to its OWN local behavior unchanged — for `/channel/ops`
// that is still the leader-only refusal `durable-chat-ops-broker` requires;
// for everything else it is ordinary, replica-safe handling that needed no
// leader at all.
func (s *Server) leaderProxyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, "/state/") || s.isLeader(r.Context()) {
			next.ServeHTTP(w, r)
			return
		}
		// The proxy reads the body to forward it, and a failed attempt leaves
		// nothing to read. A request that falls through to the local handler
		// must still carry its body, or a POST reaches its handler empty and
		// is refused as the caller's mistake while the real cause was a leader
		// mid-rollout. Bodies on this surface are small JSON, so hold a copy.
		held, restore := holdBody(r)
		if s.proxyToLeader(w, r) {
			return
		}
		restore(held)
		next.ServeHTTP(w, r)
	})
}

// maxHeldBody bounds the copy kept for a fall-through. A body past it cannot
// be replayed, and is forwarded as it comes.
const maxHeldBody = 1 << 20

// noRestore is the restore function for a body that was not held.
func noRestore([]byte) {
	// Nothing was held, so there is nothing to put back.
}

// holdBody reads r's body into memory so it can be forwarded and, if the
// forward fails, handed to the local handler. It returns the bytes and a
// function that restores them onto r. A request with no body, or one too large
// to hold, is left alone.
func holdBody(r *http.Request) ([]byte, func([]byte)) {
	if r.Body == nil || r.Body == http.NoBody {
		return nil, noRestore
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxHeldBody+1))
	if err != nil || len(data) > maxHeldBody {
		// Put back what was read ahead of the rest, so the stream is whole.
		r.Body = struct {
			io.Reader
			io.Closer
		}{io.MultiReader(bytes.NewReader(data), r.Body), r.Body}
		return nil, noRestore
	}
	r.Body = io.NopCloser(bytes.NewReader(data))
	return data, func(b []byte) { r.Body = io.NopCloser(bytes.NewReader(b)) }
}

// anyAdapterAuth accepts the master token or ANY derived adapter token, channel
// or signal. The introspection surfaces are cross-cutting — the console holds
// both identities and reads them with whichever it has — so scoping them to one
// adapter kind would only force a caller to pick a token arbitrarily.
func (s *Server) anyAdapterAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.AdapterToken == "" {
			writeJSON(w, 503, map[string]string{"error": errAdapterAuthUnset})
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), bearerPrefix)
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.AdapterToken)) == 1 {
			next(w, r)
			return
		}
		var channels agentopsv1alpha1.ChannelAdapterList
		if err := s.Client.List(r.Context(), &channels, client.InNamespace(s.Namespace)); err == nil {
			for i := range channels.Items {
				want := chat.DeriveAdapterToken(s.AdapterToken, channels.Items[i].Name)
				if subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1 {
					next(w, r.WithContext(context.WithValue(r.Context(), adapterScopeKey{}, channels.Items[i].Name)))
					return
				}
			}
		}
		var signals agentopsv1alpha1.SignalAdapterList
		if err := s.Client.List(r.Context(), &signals, client.InNamespace(s.Namespace)); err == nil {
			for i := range signals.Items {
				want := chat.DeriveSignalAdapterToken(s.AdapterToken, signals.Items[i].Name)
				if subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1 {
					next(w, r.WithContext(context.WithValue(r.Context(), adapterScopeKey{}, signals.Items[i].Name)))
					return
				}
			}
		}
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
	}
}

// NeedLeaderElection: the HTTP surface (webhooks, worker dispatch) must serve
// on every replica immediately — only reconcilers require the lease. Without
// this, Alertmanager webhooks are refused for the leader-election window
// (~30s) after every rollout.
func (s *Server) NeedLeaderElection() bool { return false }

// Start implements manager.Runnable.
func (s *Server) Start(ctx context.Context) error {
	srv := &http.Server{Addr: s.Addr, Handler: s.Handler()}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

// ---- work dispatch ----------------------------------------------------------

func (s *Server) resolvePayload(ctx context.Context) dispatch.PayloadResolver {
	return func(item state.InputItem) (string, error) {
		if item.PayloadRef == "" {
			return item.Payload, nil
		}
		blob, err := s.Blobs.Get(ctx, item.PayloadRef)
		if err != nil {
			return "", err
		}
		return blob.Payload, nil
	}
}

func (s *Server) handleWork(w http.ResponseWriter, r *http.Request) {
	convoName := r.URL.Query().Get("convo")
	podName := r.URL.Query().Get("pod")
	wait, _ := strconv.Atoi(r.URL.Query().Get("wait"))
	if wait > 30 {
		wait = 30
	}
	if convoName == "" {
		writeJSON(w, 400, map[string]string{"error": "missing convo"})
		return
	}
	deadline := time.Now().Add(time.Duration(wait) * time.Second)
	for {
		unit, served, err := s.tryDispatch(r.Context(), convoName, podName)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		if served {
			writeJSON(w, 200, unit)
			return
		}
		if time.Now().After(deadline) {
			w.WriteHeader(204)
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// effectiveTooling resolves the wiring's half of the conversation's tool
// access from the MCPToolsets it bound, together with the mode that composes it
// with the agent definition's own tools in the runtime. The profile contributes
// nothing — capabilities live only on the Pipeline. A ref that no longer
// resolves fails the dispatch and is recorded on the conversation, never
// degraded to partial tooling.
func (s *Server) effectiveTooling(ctx context.Context, conv *state.ConversationRecord) (dispatch.Tooling, error) {
	byRef, err := s.resolveToolsets(ctx, conv, conv.Toolsets)
	if err != nil {
		return dispatch.Tooling{}, err
	}
	return dispatch.Tooling{
		AllowedTools: dispatch.EffectiveAllowedTools(byRef),
		Mode:         dispatch.ToolsModeOfRecord(conv.Toolsets),
	}, nil
}

// resolveToolsets reads a binding's MCPToolsets in ref order.
func (s *Server) resolveToolsets(ctx context.Context, conv *state.ConversationRecord,
	binding *state.ToolsetBinding) ([][]string, error) {

	if binding == nil {
		return nil, nil
	}
	byRef := make([][]string, 0, len(binding.Refs))
	for _, ref := range binding.Refs {
		var ts agentopsv1alpha1.MCPToolset
		if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: ref}, &ts); err != nil {
			err = fmt.Errorf("bound MCPToolset %q: %w", ref, err)
			s.setToolingCondition(ctx, conv, "ToolsetUnresolved", err.Error())
			return nil, err
		}
		byRef = append(byRef, ts.Spec.Tools)
	}
	return byRef, nil
}

// setToolingCondition surfaces a binding failure on the conversation so it is
// visible in `kubectl describe` rather than only in manager logs.
func (s *Server) setToolingCondition(ctx context.Context, conv *state.ConversationRecord, reason, message string) {
	_ = state.Patch(ctx, s.Conversations, conv, func(r *state.ConversationRecord) {
		r.SetCondition(state.Condition{
			Type: controller.ConditionToolingResolved, Status: state.ConditionFalse,
			Reason: reason, Message: message,
		})
	})
}

func (s *Server) tryDispatch(ctx context.Context, convoName, podName string) (dispatch.WorkUnit, bool, error) {
	conv, rev, err := s.Conversations.Get(ctx, state.Key(convoName))
	if err != nil {
		return dispatch.WorkUnit{}, false, err
	}
	// pending-topic gate: a channel-bound conversation waits until AT LEAST ONE
	// thread binding exists before its first unit dispatches (waiting for all
	// would deadlock on one broken channel; late topics catch up). Dangling
	// channelRefs stay chat-less. Delivery itself needs no channel lookup — the
	// operator routes every result to the bound channels after /work/done.
	if len(conv.ChannelRefs) > 0 {
		chatBound := false
		for _, ref := range conv.ChannelRefs {
			var ch agentopsv1alpha1.Channel
			if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: ref}, &ch); err != nil || ch.Spec.Adapter == "" {
				continue
			}
			chatBound = true
		}
		if chatBound && len(conv.Threads) == 0 {
			return dispatch.WorkUnit{}, false, nil
		}
	}
	var profile agentopsv1alpha1.AgentProfile
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: conv.ProfileRef}, &profile); err != nil {
		return dispatch.WorkUnit{}, false, err
	}
	tools, err := s.effectiveTooling(ctx, &conv)
	if err != nil {
		return dispatch.WorkUnit{}, false, err
	}
	// Continuity is PROMISED ONLY WHERE IT IS POSSIBLE. A runtime that keeps
	// context on its context volume, in a deployment providing none, can never
	// continue anything — the pod exits on its idle timeout and takes the context
	// with it. Withholding the handle makes that conversation single-run by
	// declaration: it answers each message fresh and says so, instead of failing
	// every follow-up for a configuration the operator deliberately chose.
	contextID := ""
	resolved, err := runtimepod.ResolveFor(ctx, s.Reader, s.Conversations, s.Namespace, &conv, s.Runtime)
	if err != nil {
		return dispatch.WorkUnit{}, false, err
	}
	if resolved.ContinuityPossible() {
		contextID = conv.ContextID()
	}
	// While contexts are unreachable across the install, dispatching a
	// continuation only reproduces the failure and spends the message. Hold it:
	// the input stays pending and goes out with its context once the store
	// answers again. A conversation with nothing to continue is unaffected.
	if contextID != "" {
		if open, _ := s.breaker().Open(); open {
			return dispatch.WorkUnit{}, false, nil
		}
	}
	unit, ids, ok, err := dispatch.Next(&conv, &profile, tools, s.resolvePayload(ctx), time.Now(), contextID)
	if err != nil || !ok {
		return dispatch.WorkUnit{}, false, err
	}
	now := time.Now().UTC()
	conv.Inflight = &state.InflightRun{RunID: unit.RunID, InputIDs: ids, DispatchedAt: now}
	conv.Phase = state.PhaseWorking
	conv.LastActivity = &now
	if podName != "" {
		conv.RuntimePod = podName
	}
	// Compare-and-swap on the revision read above: a racing update is not
	// merged over, the caller retries next tick.
	if _, err := s.Conversations.Update(ctx, state.Key(conv.Name), rev, conv); err != nil {
		if errors.Is(err, state.ErrConflict) {
			return dispatch.WorkUnit{}, false, nil
		}
		return dispatch.WorkUnit{}, false, err
	}
	pipeline := s.pipelineName(ctx, &conv)
	s.Activity.Emit(activity.Event{
		Kind:     activity.KindRunDispatched,
		From:     s.originNode(pipeline, conv.Name),
		To:       activity.Node(activity.NodeRuntime, s.runtimeName(ctx, &conv)),
		Pipeline: pipeline, Conversation: conv.Name, RunID: unit.RunID,
		Detail: fmt.Sprintf("%d input(s) to %s", len(ids), profile.Name),
	})
	return unit, true, nil
}

// pipelineName attributes a conversation to its originating pipeline, or "" when
// the wiring is ambiguous. INFERRED from materialized bindings — a Conversation
// records no pipelineRef — so a blank answer is the honest one and never a
// guess. Telemetry labelling only; nothing routes on it.
func (s *Server) pipelineName(ctx context.Context, conv *state.ConversationRecord) string {
	if p := chat.PipelineForConversation(ctx, s.Client, s.Namespace, conv); p != nil {
		return p.Name
	}
	return ""
}

// originNode names the graph node a conversation's work flows from: its
// pipeline when attribution succeeded, the conversation itself when it did not.
// Falling back to the conversation keeps the hop renderable without inventing an
// edge from a pipeline that may not be the one that ran it.
func (s *Server) originNode(pipeline, conversation string) *activity.NodeRef {
	if pipeline != "" {
		return activity.Node(activity.NodePipeline, pipeline)
	}
	return activity.Node(activity.NodeConversation, conversation)
}

// runtimeName names the AgentRuntime a CONVERSATION executes on, for telemetry
// attribution. It follows the pod builder's own chain — the conversation's
// snapshot, then the deprecated profile ref, then "default" — because a graph
// edge naming a different runtime than the pod ran on is worse than no edge.
func (s *Server) runtimeName(ctx context.Context, conv *state.ConversationRecord) string {
	if conv != nil && conv.RuntimeRef != "" {
		return conv.RuntimeRef
	}
	if conv != nil && conv.ProfileRef != "" {
		var profile agentopsv1alpha1.AgentProfile
		if err := s.Client.Get(ctx, types.NamespacedName{
			Namespace: s.Namespace, Name: conv.ProfileRef}, &profile); err == nil {
			if profile.Spec.RuntimeRef != nil && profile.Spec.RuntimeRef.Name != "" {
				return profile.Spec.RuntimeRef.Name // DEPRECATED, one release
			}
		}
	}
	return s.defaultRuntimeName(ctx)
}

// pipelineRuntimeName answers what a capability would run on, for the resolved
// endpoint the console reads before any conversation exists. Same chain, minus
// the snapshot there is nothing to snapshot from yet.
func (s *Server) pipelineRuntimeName(ctx context.Context, capability agentopsv1alpha1.AgentCapabilitySpec,
	profile *agentopsv1alpha1.AgentProfile) string {

	if capability.RuntimeRef != nil && capability.RuntimeRef.Name != "" {
		return capability.RuntimeRef.Name
	}
	if profile != nil && profile.Spec.RuntimeRef != nil && profile.Spec.RuntimeRef.Name != "" {
		return profile.Spec.RuntimeRef.Name // DEPRECATED, one release
	}
	return s.defaultRuntimeName(ctx)
}

func (s *Server) defaultRuntimeName(ctx context.Context) string {
	var rt agentopsv1alpha1.AgentRuntime
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: "default"}, &rt); err == nil {
		return rt.Name
	}
	return "default"
}

type workDone struct {
	// Convo is the Conversation this report is for.
	Convo string `json:"convo"`
	// RunID is the unit's own id, round-tripped.
	RunID string `json:"runId"`
	// Status is "succeeded" or "failed". The manager treats any other
	// value as a failed run.
	Status string `json:"status"`
	// ExitCode is the process exit code, when the runtime has one to
	// report.
	ExitCode *int32 `json:"exitCode,omitempty"`
	// RuntimeContextID is the handle for the context this run leaves behind —
	// the one to continue next time. Runtimes may report it on a FAILED run too:
	// a crash after the context was established should not strand it.
	RuntimeContextID string `json:"runtimeContextId,omitempty"`
	// SessionID is the retired spelling, accepted for one release so an older
	// runtime image keeps working. DEPRECATED.
	SessionID string `json:"sessionId,omitempty"`
	// Continuity says what happened to the context this run was asked to
	// continue: "continued", "new", or "unavailable" (a new one was needed
	// because the named context could not be reached).
	//
	// The manager cannot infer this. It sends a handle and receives a
	// handle, and when they differ it has no way to tell an agent that
	// branched deliberately from one that was forced to start over. The
	// runtime knows, because it is the component that tried.
	//
	// ABSENT MEANS NO CLAIM: a runtime that does not implement it keeps today's
	// behaviour, because an addition to the contract must not make an existing
	// runtime look broken.
	Continuity string `json:"continuity,omitempty"`
	// ContinuityReason is free text from the runtime explaining why a context
	// could not be continued. Recorded VERBATIM — the manager does not know
	// where any given runtime keeps its context, so any explanation it invented
	// would be confident and sometimes wrong.
	ContinuityReason string `json:"continuityReason,omitempty"`
	// Result is the run's answer. Truncated to 2000 bytes by the manager,
	// never refused for length.
	Result string `json:"result,omitempty"`
	// Brief is what the runtime extracted as this conversation's one- or
	// two-sentence description (design D-I) — a contract field, never a
	// parsed section of `result`. LATEST-WINS, the same rule as
	// `runtimeContextId`: absent leaves the stored one untouched.
	Brief string `json:"brief,omitempty"`
	// Turns is one entry per model call this run made, bounded in count and
	// field size (workreport.go). It becomes one activity hop and nothing
	// else — telemetry, never written to the Conversation.
	Turns []Turn `json:"turns,omitempty"`
	// ToolCalls is one entry per tool call this run made, bounded in count
	// and field size (workreport.go). It becomes one activity hop and
	// nothing else — telemetry, never written to the Conversation.
	ToolCalls []ToolCall `json:"toolCalls,omitempty"`
}

// Continuity report values. Runtime-agnostic by construction: they describe what
// happened to a CONTEXT, naming no session, no flag and no storage medium.
const (
	// ContinuityContinued: the run continued the context it was given.
	ContinuityContinued = "continued"
	// ContinuityNew: the run started a new context, having been given none.
	ContinuityNew = "new"
	// ContinuityUnavailable: the run was given a handle and could not reach the
	// context it named. This is the one that means context was LOST.
	ContinuityUnavailable = "unavailable"
)

func (s *Server) handleWorkDone(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var d workDone
	if err := json.Unmarshal(body, &d); err != nil {
		writeJSON(w, 400, map[string]string{"error": errInvalidJSON})
		return
	}
	if err := validateReport(&d); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	ctx := r.Context()
	key := state.Key(d.Convo)
	conv, _, err := s.Conversations.Get(ctx, key)
	if err != nil {
		code := 500
		if errors.Is(err, state.ErrNotFound) {
			code = 404
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}
	now := time.Now().UTC()
	// An unavailable context during an OUTAGE holds its work instead of losing
	// it: the input stays pending so it can be dispatched once contexts are
	// reachable again, with the context it was always meant to continue.
	// Consuming it here would spend the message on a failure the infrastructure
	// caused.
	hold := false
	if d.Continuity == ContinuityUnavailable {
		hold = s.breaker().Report()
	}
	// What this run consumed and when it started, read off the inflight entry
	// BEFORE it is cleared below — the only place this run's true start time is
	// ever recorded. A coordinator's member is created mid-run, well before the
	// run that invoked it writes its own entry; the console anchors the member's
	// transcript position to this run's window (item #15 QA) and needs a real
	// StartedAt for it, not the finish time standing in for both ends.
	//
	// The messages this run consumed are recorded WITH the run: the record must
	// exist before the reconciler is allowed to prune the queue entry, or a
	// crash between the two would lose what a person said, silently.
	var (
		recorded  []state.RecordedInput
		startedAt *time.Time
		latencyMs int64
		turn      turnEvidence
	)
	// The move stamp (design D-A) is consumed with the rest of the inflight
	// entry — read here, judged below, surviving nowhere else.
	if inf := conv.Inflight; inf != nil && inf.RunID == d.RunID {
		latencyMs = now.Sub(inf.DispatchedAt).Milliseconds()
		started := inf.DispatchedAt
		startedAt = &started
		if !hold {
			recorded = s.recordInputs(ctx, &conv, inf.InputIDs)
		}
		turn = turnEvidence{Dispatched: !hold && d.Continuity != ContinuityUnavailable, Moved: inf.Moved}
	}
	result := d.Result
	if len(result) > maxRecordedResult {
		result = result[:maxRecordedResult]
	}
	run := state.Run{
		RunID: d.RunID, Status: d.Status, ExitCode: d.ExitCode, Result: result,
		StartedAt: startedAt, FinishedAt: &now,
		// This manager owes the reply to every bound thread. Recorded on the run
		// itself so the obligation survives the process that took it on.
		DeliveryTracked: true,
		// The questions, beside the answer. Without them a conversation records
		// half a thread: durable results linked by inputIds to text the queue
		// already deleted.
		Inputs: recorded,
	}
	// A Coordinator root's turn is judged BEFORE the run is recorded and its
	// inflight entry cleared (design D-B, moveperturn.go). That order is what
	// makes the hand-back durable: a manager that stops between the append and
	// the record below leaves the inflight entry in place, so the runtime's
	// retried report is judged again and the append, idempotent per run, lands
	// once. The corrective input is pending, so the phase lands Queued. A run
	// that reaches maxTurns is not judged: the root closes on that edge, and a
	// hand-back into a thread about to close helps nobody.
	missed := false
	if !turnsReachMax(&conv) && s.Router != nil {
		missed = s.actOnMiss(ctx, &conv, d.RunID, result, judgeRootTurn(&conv, d.Status, turn, recorded))
	}
	if err := s.recordRun(ctx, key, run); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	var (
		maxTurnsExceeded bool
		desiredPhase     = state.PhaseIdle
	)
	reported := d.RuntimeContextID
	if reported == "" {
		reported = d.SessionID // the retired spelling, accepted for one release
	}
	if d.Continuity == ContinuityContinued {
		s.breaker().Continued()
	}
	updated, err := state.Mutate(ctx, s.Conversations, key, func(c *state.ConversationRecord) error {
		if inf := c.Inflight; inf != nil && inf.RunID == d.RunID && !hold {
			c.ProcessedInputIDs = bound(append(c.ProcessedInputIDs, inf.InputIDs...), 50)
		}
		if c.Inflight != nil && c.Inflight.RunID == d.RunID {
			c.Inflight = nil
		}
		applyReport(c, &d, reported, hold)
		// Turns count on the conversation WHOSE OWN status this is, when ITS OWN
		// run is recorded — never when a member's result arrives as an input, so N
		// members finishing at once is one turn (design D-C, D-E). Absent Budget
		// means this conversation is not itself a Coordinator's root: nothing to
		// count.
		maxTurnsExceeded = false
		if b := c.Budget; b != nil {
			b.Turns++
			maxTurnsExceeded = b.MaxTurns > 0 && b.Turns >= b.MaxTurns
		}
		desiredPhase = state.PhaseIdle
		if missed || len(dispatch.PendingInputs(c)) > 0 {
			desiredPhase = state.PhaseQueued
		}
		c.LastActivity = &now
		return nil
	})
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	conv = updated
	// Phase moves in its OWN write, never folded into the one above: a run that
	// was still in flight when /close landed reports its completion here, and
	// this write must not un-close the conversation when that belated report
	// lands — Closed is sticky until an explicit reopen. Folded in, a write
	// landing after close's own silently reverted phase to Idle/Queued while
	// leaving ClosedAt stamped — measured live on the chart-v14.0.0 release
	// smoke (TestConsoleLifecycle/close_then_delete).
	if err := s.setPhaseUnlessClosed(ctx, d.Convo, desiredPhase); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	pipeline := s.pipelineName(ctx, &conv)
	s.emitRunCalls(ctx, &conv, pipeline, d.RunID, &d)
	runEvent := activity.Event{
		Kind:     activity.KindRunCompleted,
		From:     activity.Node(activity.NodeRuntime, s.runtimeName(ctx, &conv)),
		To:       s.originNode(pipeline, conv.Name),
		Pipeline: pipeline, Conversation: conv.Name, RunID: d.RunID,
		LatencyMs: latencyMs, Detail: d.Status,
	}
	if d.Status != "succeeded" {
		runEvent.Status = activity.StatusError
	}
	if d.ExitCode != nil {
		runEvent.Detail = fmt.Sprintf("%s (exit %d)", d.Status, *d.ExitCode)
	}
	s.Activity.Emit(runEvent)
	// The operator delivers: agent output goes to EVERY bound thread, through
	// the serving adapters. This is the only delivery path — agents never post
	// to a transport themselves, so no runtime holds channel credentials and
	// no surface can mistake silence for success.
	//
	// This is the FAST PATH, not the only one: the reply's op id is stable per
	// conversation x channel x run and delivery is marked on the run, so a manager
	// restart in the window between this write and an adapter claiming the op
	// re-derives the answer from reconciliation instead of losing it. The message
	// is composed from the RECORDED run so both paths say the same thing.
	if len(conv.ChannelRefs) > 0 && s.Router != nil {
		s.Router.FanOutRunReply(ctx, &conv, d.RunID, chat.RunReplyMessage(&run))
	}
	// A member's result becomes an input on its PARENT, one hop
	// (coordination-loop) — the FAST PATH; a conversation with no causedBy has
	// nothing to route. The reconciler backstop re-derives a missed append from
	// `causedBy and !RoutedToParent`.
	if conv.CausedBy != nil && s.Router != nil {
		dedup := "member:" + conv.Name + ":" + d.RunID
		if err := s.Router.AppendMemberResult(ctx, conv.CausedBy, conv.Name, dedup, result); err != nil {
			log.FromContext(ctx).Info("routing member result to parent (backstop will retry)",
				"conversation", conv.Name, "parent", conv.CausedBy.Parent, "error", err.Error())
		} else if err := s.Runs.Grow(ctx, key, d.RunID, state.Run{RoutedToParent: true}); err != nil &&
			!errors.Is(err, state.ErrNotFound) {
			log.FromContext(ctx).Info("marking run routed to parent (backstop will retry)",
				"conversation", conv.Name, "runId", d.RunID, "error", err.Error())
		}
	}
	// Budget edges are evaluated PER COORDINATOR LEVEL (design D-E): only a
	// conversation that is itself a Coordinator's root carries a Budget, and
	// only ITS OWN maxTurns bounds ITS OWN turns.
	//
	// async-command-bus task 5.4: this call lands AFTER s.recordRun above —
	// confirmed, not just assumed, by reading the lines between them — so
	// enqueuing it as a "conv" task here (when s.Bus is set) never races the
	// run being recorded; a retried /work/done redelivery re-enters this
	// same block, recordRun's own de-duplication already makes THAT
	// idempotent, and the close task is submitted under a CommandID stable
	// per conversation, so a redelivery resubmits the SAME suppressed id
	// rather than a second one.
	if maxTurnsExceeded {
		digest := fmt.Sprintf("budget-exceeded: maxTurns (%d) reached", conv.Budget.MaxTurns)
		if err := s.closeBudgetExceeded(ctx, &conv, digest); err != nil {
			log.FromContext(ctx).Info("closing conversation on maxTurns",
				"conversation", conv.Name, "error", err.Error())
		}
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// maxRecordedResult bounds the result kept on a run; a longer one is cut, never
// refused.
const maxRecordedResult = 2000

// turnsReachMax reports whether recording one more turn reaches the root's
// maxTurns, judged on the record read before the write.
func turnsReachMax(c *state.ConversationRecord) bool {
	b := c.Budget
	return b != nil && b.MaxTurns > 0 && b.Turns+1 >= b.MaxTurns
}

// recordRun appends the run unless the history already holds it: a retried
// report whose first attempt landed the run but not the inflight clear must not
// record it twice.
func (s *Server) recordRun(ctx context.Context, key state.Key, run state.Run) error {
	runs, err := s.Runs.List(ctx, key)
	if err != nil {
		return err
	}
	for i := range runs {
		if runs[i].RunID == run.RunID {
			return nil
		}
	}
	return s.Runs.Append(ctx, key, run)
}

// applyReport records what a work report says about the context onto the
// conversation: the handle, the brief and the continuity condition.
func applyReport(c *state.ConversationRecord, d *workDone, reported string, hold bool) {
	// LATEST-WINS, and recorded on a FAILED run too.
	//
	// This was write-once, which was unsound: a run may legitimately end in a
	// different context than it was asked to continue, and keeping the first
	// handle then named something that no longer existed. Because dispatch and
	// ingest both key off it, every later message repeated the same failed
	// continuation — one recoverable loss became permanent. Recording on failure
	// matters for the same reason: a crash after the context was established
	// otherwise strands it, and the next message starts over.
	if reported != "" {
		c.SetContextID(reported)
	}
	// LATEST-WINS, same rule as the context handle: the agent restates what the
	// conversation is about having read the whole context, so a report omitting
	// it leaves the stored one untouched rather than blanking it.
	if d.Brief != "" {
		brief := d.Brief
		if len(brief) > agentopsv1alpha1.MaxBrief {
			brief = brief[:agentopsv1alpha1.MaxBrief]
		}
		c.Brief = brief
	}
	// Context continuity, recorded where anyone can see it. This is the one
	// failure that leaves a conversation looking entirely healthy — same phase,
	// same run history, answers still arriving — while every answer is given
	// without memory. Every other health fact about a conversation is a
	// condition, so this is one too.
	//
	// ABSENT report = no claim: a runtime that does not implement it leaves the
	// condition untouched rather than appearing to have lost something.
	switch d.Continuity {
	case ContinuityUnavailable:
		reason := d.ContinuityReason
		if reason == "" {
			reason = "the runtime could not reach the context it was given"
		}
		condReason := "ContextUnavailable"
		if hold {
			// Not this conversation's loss: contexts are unreachable across the
			// install, so the work waits rather than being written off.
			condReason = "ContextStoreUnavailable"
			reason = "held: contexts are unreachable across this install, so this " +
				"message waits rather than being answered without its history — " + reason
		}
		c.SetCondition(state.Condition{
			Type: controller.ConditionContextContinuity, Status: state.ConditionFalse,
			Reason: condReason, Message: reason,
		})
	case ContinuityContinued:
		c.SetCondition(state.Condition{
			Type: controller.ConditionContextContinuity, Status: state.ConditionTrue,
			Reason: "Continued", Message: "the run continued this conversation's context",
		})
	}
}

// setPhaseUnlessClosed moves a conversation to Queued or Idle after a run
// completes, UNLESS a concurrent /close already moved it to Closed — Closed
// is sticky until an explicit reopen, so a work report that was merely slow
// to arrive must never resurrect the phase close already retired.
//
// Phase is the one field here that must be re-decided against the LATEST state
// rather than the state this request started with, so the decision is made
// inside the compare-and-swap loop and a loser re-reads.
func (s *Server) setPhaseUnlessClosed(ctx context.Context, name string, desired string) error {
	_, err := state.Mutate(ctx, s.Conversations, state.Key(name), func(c *state.ConversationRecord) error {
		if c.Phase != state.PhaseClosed {
			c.Phase = desired
		}
		return nil
	})
	if err = state.IgnoreNotFound(err); err != nil {
		return fmt.Errorf("setting phase on %q: %w", name, err)
	}
	return nil
}

// recordInputs renders the queued inputs a run consumed as the record kept on
// that run.
//
// Reads the out-of-line payload where there is one, so the record holds the
// MESSAGE rather than a reference to an object that is about to be pruned with
// the queue entry. The inline cap lives on the record type, applied once, in
// InputItem.Record.
//
// An id with no queue entry left is skipped rather than recorded empty: a
// conversation reconciled between the dispatch and this report has already
// pruned nothing, so this only happens when the input was never there — and an
// entry with no text is a message nobody sent.
func (s *Server) recordInputs(ctx context.Context, conv *state.ConversationRecord,
	ids []string) []state.RecordedInput {

	if len(ids) == 0 {
		return nil
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []state.RecordedInput
	for i := range conv.Inputs {
		item := &conv.Inputs[i]
		if !want[item.ID] {
			continue
		}
		text, labels := item.Payload, map[string]string(nil)
		if item.PayloadRef != "" {
			if blob, err := s.Blobs.Get(ctx, item.PayloadRef); err == nil {
				text, labels = blob.Payload, blob.Labels
			}
		}
		out = append(out, item.Record(text, labels))
	}
	return out
}

func bound(s []string, n int) []string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

// ---- channel adapter contract ----------------------------------------------

// adapterScopeKey carries the authenticated adapter's scope — its CR name,
// which IS the type key Channels/SignalSources select — through the request
// context ("" = master token, full scope).
type adapterScopeKey struct{}

// adapterAuth guards /channel/* (constant-time): the master token (ADAPTER_TOKEN
// env) has full scope; a per-adapter token — HMAC-derived from the master key
// and the ChannelAdapter name, validated by re-derivation against the adapter
// list (stateless, survives restarts, zero Secret reads) — is scoped to that
// adapter's name.
func (s *Server) adapterAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.AdapterToken == "" {
			writeJSON(w, 503, map[string]string{"error": errAdapterAuthUnset})
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), bearerPrefix)
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.AdapterToken)) == 1 {
			next(w, r)
			return
		}
		var adapters agentopsv1alpha1.ChannelAdapterList
		if err := s.Client.List(r.Context(), &adapters, client.InNamespace(s.Namespace)); err == nil {
			for i := range adapters.Items {
				want := chat.DeriveAdapterToken(s.AdapterToken, adapters.Items[i].Name)
				if subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1 {
					ctx := context.WithValue(r.Context(), adapterScopeKey{}, adapters.Items[i].Name)
					next(w, r.WithContext(ctx))
					return
				}
			}
		}
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
	}
}

// ---- state read surface auth ------------------------------------------------

// stateAuth guards /state/* (design D-6, constant-time): the master token
// (full reach, same as adapterAuth's), STATE_READ_TOKEN (a non-adapter
// reader — housekeeping, scoped to this surface alone), or a derived
// ChannelAdapter/SignalAdapter token whose CR sets spec.stateAccess. It is
// deliberately NOT adapterAuth with an extra check bolted on: adapterAuth's
// master-token/derived-token halves grant every adapter the SAME reach,
// while here an adapter's own derived token must additionally be GRANTED by
// its CR, so the two cannot be folded into one without the grant becoming
// implicit everywhere adapterAuth is reused.
//
// 401 (token does not derive from anything) vs 403 (it derives, but the CR
// does not declare stateAccess) is the distinction design D-6 draws: a
// caller presenting an adapter's real identity is told exactly what field to
// set, where a caller presenting nothing recognisable is told nothing about
// what exists.
func (s *Server) stateAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.AdapterToken == "" {
			writeJSON(w, 503, map[string]string{"error": errAdapterAuthUnset})
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), bearerPrefix)
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.AdapterToken)) == 1 {
			next(w, r)
			return
		}
		if s.StateReadToken != "" && subtle.ConstantTimeCompare([]byte(got), []byte(s.StateReadToken)) == 1 {
			next(w, r)
			return
		}
		name, granted, found := s.stateAdapterFor(r.Context(), got)
		if !found {
			writeJSON(w, 401, map[string]string{"error": "unauthorized"})
			return
		}
		if !granted {
			writeJSON(w, 403, map[string]string{"error": fmt.Sprintf(
				"adapter %q does not declare spec.stateAccess", name)})
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), adapterScopeKey{}, name)))
	}
}

// stateAdapterFor finds the ChannelAdapter or SignalAdapter whose derived
// token is got. found is false when none derives it, granted reports whether
// that CR declares spec.stateAccess.
func (s *Server) stateAdapterFor(ctx context.Context, got string) (name string, granted, found bool) {
	var channels agentopsv1alpha1.ChannelAdapterList
	if err := s.Client.List(ctx, &channels, client.InNamespace(s.Namespace)); err == nil {
		for i := range channels.Items {
			a := &channels.Items[i]
			if subtle.ConstantTimeCompare([]byte(got), []byte(chat.DeriveAdapterToken(s.AdapterToken, a.Name))) == 1 {
				return a.Name, a.Spec.StateAccess, true
			}
		}
	}
	var sources agentopsv1alpha1.SignalAdapterList
	if err := s.Client.List(ctx, &sources, client.InNamespace(s.Namespace)); err == nil {
		for i := range sources.Items {
			a := &sources.Items[i]
			if subtle.ConstantTimeCompare([]byte(got), []byte(chat.DeriveSignalAdapterToken(s.AdapterToken, a.Name))) == 1 {
				return a.Name, a.Spec.StateAccess, true
			}
		}
	}
	return "", false, false
}

// adapterParam reads the contract's adapter selector. The retired `type`
// parameter is refused loudly rather than treated as absent: an outdated
// adapter must fail visibly instead of being served an empty list and looking
// healthy while delivering nothing.
func adapterParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	if name := r.URL.Query().Get("adapter"); name != "" {
		return name, true
	}
	if r.URL.Query().Get("type") != "" {
		writeJSON(w, 400, map[string]string{"error": "the 'type' parameter was renamed to 'adapter' (it names the adapter CR)"})
		return "", false
	}
	writeJSON(w, 400, map[string]string{"error": "missing adapter"})
	return "", false
}

// contractOK enforces the outbound message contract handshake on the ops
// long-poll: an adapter declares `contract=<version>` and is refused otherwise.
//
// Without it the failure is silent and baffling. An adapter built against the
// string-valued contract reads `op.text`, finds nothing, and posts empty
// messages forever — healthy-looking, delivering nothing. So the check is at the
// door, and the 400 names the version to build against, exactly as the retired
// `?type=` parameter names its replacement.
func contractOK(w http.ResponseWriter, r *http.Request) bool {
	got := r.URL.Query().Get("contract")
	if got == chat.ContractVersion {
		return true
	}
	detail := "this adapter declared no outbound contract version"
	if got != "" {
		detail = fmt.Sprintf("this adapter speaks contract %q", got)
	}
	writeJSON(w, 400, map[string]string{"error": fmt.Sprintf(
		"%s; the manager serves %q. Ops carry a TYPED message (op.message{kind,body,…}) and a topic "+
			"descriptor (op.topic{…}), never rendered text — an adapter reading op.text would post empty "+
			"messages. Render the message kinds and request /channel/ops?adapter=…&contract=%s",
		detail, chat.ContractVersion, chat.ContractVersion)})
	return false
}

// scopeAllows enforces the per-adapter type scope; master-token requests
// (empty scope) pass everything.
func scopeAllows(r *http.Request, channelType string) bool {
	scope, _ := r.Context().Value(adapterScopeKey{}).(string)
	return scope == "" || scope == channelType
}

func forbidScope(w http.ResponseWriter) {
	writeJSON(w, 403, map[string]string{"error": "token is scoped to another channel type"})
}

// claimedOp is one claimed operation plus how long the adapter holds the claim.
//
// An adapter that rides out transport backpressure in-process — sleeping a
// Telegram `retry_after`, say — must finish INSIDE the claim window, or the
// manager reclaims the op, a second claimant takes it, and the message posts
// twice. That makes the adapter's retry budget and this constant an inequality
// spanning two dependency-free modules, which is a drift bug waiting for the
// first person to tune ReclaimAfter down.
//
// So the manager states it rather than the adapter guessing. Additive and
// optional: an older adapter ignores the field, and a newer one against an
// older manager falls back to its own conservative default.
type claimedOp struct {
	*chat.Op
	ReclaimAfterSeconds int `json:"reclaimAfterSeconds"`
}

func (s *Server) handleChannelOps(w http.ResponseWriter, r *http.Request) {
	channelType, ok := adapterParam(w, r)
	if !ok {
		return
	}
	if !scopeAllows(r, channelType) {
		forbidScope(w)
		return
	}
	if !contractOK(w, r) {
		return
	}
	wait, _ := strconv.Atoi(r.URL.Query().Get("wait"))
	if wait > 30 {
		wait = 30
	}
	// THE CHANGE RIDES A CONNECTION THE ADAPTER ALREADY HOLDS.
	//
	// The manager cannot dial an adapter — a ChannelAdapter's port is optional
	// and the contract is pull-only — so a vocabulary change reaches an
	// otherwise idle adapter here, on the poll it is already blocked in. A
	// header rather than a body field, because the empty case is a bodyless 204
	// and giving it a body would change what every existing adapter parses.
	//
	// Computed ONCE per request, before the wait: it is a read of the informer
	// cache, and recomputing it every second of a 30-second poll would buy
	// nothing an adapter can act on any sooner. Set before the leader check
	// below too — the revision rides EVERY poll response, a rejected one
	// included, since the manager cannot dial an otherwise-idle adapter any
	// other way.
	s.setVocabularyRevision(r.Context(), w)

	// durable-chat-ops-broker: a claim is written only by the current leader
	// (the Conversation reconciler, leader-gated by construction), so a
	// non-leader has no safe way to answer this poll from its own state —
	// its in-memory OpQueue was never populated for ops this process did not
	// claim.
	//
	// A non-leader is a DUMB PROXY to whoever the Lease says is leader, not a
	// second decision-maker: it does no claiming, no deriving, it just
	// forwards the request and relays the response byte for byte. The leader
	// keeps being the ONLY process that ever touches OpQueue or writes a
	// claim — this changes who answers the HTTP request, not who does the
	// work.
	//
	// Falls back to the 503-and-retry a conforming adapter already tolerates
	// only when the leader is unknown or unreachable from here (a handoff in
	// progress, a network blip) — never by silently serving from this
	// replica's own, unpopulated state. The proxy ATTEMPT itself now happens
	// one layer up, in leaderProxyMiddleware, uniformly for every route —
	// reaching this check at all already means that attempt failed.
	if !s.isLeader(r.Context()) {
		w.Header().Set("Retry-After", "0")
		writeJSON(w, 503, map[string]string{"error": "this replica is not the current leader, and the leader could not be reached"})
		return
	}

	deadline := time.Now().Add(time.Duration(wait) * time.Second)
	for {
		if op := s.Ops.Claim(channelType); op != nil {
			writeJSON(w, 200, claimedOp{Op: op, ReclaimAfterSeconds: int(chat.ReclaimAfter.Seconds())})
			return
		}
		if time.Now().After(deadline) {
			w.WriteHeader(204)
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(time.Second):
		}
	}
}

// proxyToLeader reverse-proxies the request to whichever pod the Lease says
// is leader, reporting whether it managed to. A failed proxy (leader
// unknown, dial refused, a handoff mid-flight) is reported false so the
// caller falls back to the ordinary 503-and-retry — never serves a half
// answer from this replica's own state.
func (s *Server) proxyToLeader(w http.ResponseWriter, r *http.Request) bool {
	target := s.leaderProxyTarget(r.Context())
	if target == "" {
		return false
	}
	u := &url.URL{Scheme: "http", Host: target}
	proxied := false
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(u)
			pr.Out.Host = u.Host
		},
		ErrorHandler: func(_ http.ResponseWriter, req *http.Request, err error) {
			log.FromContext(req.Context()).Info("leader proxy failed, falling back", "target", target, "error", err.Error())
			// No response written: the caller falls back to 503-and-retry on a false
			// return, so nothing more needs writing here. Writing a second
			// response on top of one ReverseProxy may have started would
			// panic on a non-empty body already flushed.
		},
		ModifyResponse: func(resp *http.Response) error {
			proxied = true
			return nil
		},
	}
	proxy.ServeHTTP(w, r)
	return proxied
}

func (s *Server) handleChannelOpDone(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var res chat.OpResult
	// An EMPTY body is a valid completion: it is how close-topic reports
	// success, and how any op says "done, nothing to report".
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &res); err != nil {
			writeJSON(w, 400, map[string]string{"error": errInvalidJSON})
			return
		}
	}
	// duplicate completions are legal (at-least-once) — always 200
	s.Ops.Complete(r.Context(), r.PathValue("id"), res)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

type inboundReq struct {
	Channel  string  `json:"channel"`
	ThreadID *string `json:"threadId,omitempty"`
	Text     string  `json:"text"`
	Sender   string  `json:"sender,omitempty"`
}

// handleChannelInbound is REPLY-ONLY. A channel carries conversations; it does
// not start them, so every inbound message must name the thread it continues.
// Origination goes through the signal path: a general-surface message is a
// chat signal from a chat SignalSource, claimed by a Pipeline that declares who
// answers — instead of a channel picking whichever pipeline was created first.
// reachConversation resolves the conversation a surface is asking to act on,
// and decides whether it MAY.
//
// Reach is the BINDING: a surface may act on a conversation whose
// spec.channelRefs names its channel, and the binding is read from the
// CONVERSATION, never taken from the request. This does not reopen the door
// that "no remote close verb exists" shut — that rule protected a property, not
// a syntax: you may only end a conversation you are part of, and reach had to
// be PROVEN rather than asserted by naming one. Holding a live thread was the
// proof, because posting a command on a thread is only possible for a surface
// that has one. A CLOSED conversation has no thread, so that proof is
// unavailable, and the binding that put the thread there is the next-strongest.
func (s *Server) reachConversation(w http.ResponseWriter, r *http.Request) (*state.ConversationRecord, bool) {
	name := r.PathValue("name")
	var in struct {
		Channel string `json:"channel"`
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err := json.Unmarshal(body, &in); err != nil || in.Channel == "" {
		writeJSON(w, 400, map[string]string{"error": `need {"channel"}`})
		return nil, false
	}
	ctx := r.Context()
	var ch agentopsv1alpha1.Channel
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: in.Channel}, &ch); err != nil {
		writeJSON(w, 404, map[string]string{"error": fmt.Sprintf("unknown channel %q", in.Channel)})
		return nil, false
	}
	if !scopeAllows(r, ch.Spec.Adapter) {
		forbidScope(w)
		return nil, false
	}
	conv, _, err := s.Conversations.Get(ctx, state.Key(name))
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": fmt.Sprintf("unknown conversation %q", name)})
		return nil, false
	}
	for _, ref := range conv.ChannelRefs {
		if ref == ch.Name {
			return &conv, true
		}
	}
	writeJSON(w, 403, map[string]string{"error": fmt.Sprintf(
		"conversation %q is not bound to channel %q — a surface may act only on a conversation it is bound to; "+
			"bind the channel on the Pipeline that originates these conversations", name, ch.Name)})
	return nil, false
}

// handleConversationReopen accepts and enqueues (async-command-bus design
// D-9): the binding check (reachConversation) is the one cheap read accept
// keeps. "Not Closed", "a successor is already open" and "a dangling ref"
// all used to answer 409 here; they are all PRECONDITION_FAILED now (design
// D-6: terminal, never retried) and surface on /state/tasks rather than in
// this response, which is the breaking change task 5.1/proposal.md names.
func (s *Server) handleConversationReopen(w http.ResponseWriter, r *http.Request) {
	conv, ok := s.reachConversation(w, r)
	if !ok {
		return
	}
	resp, err := s.acceptAndEnqueue(r.Context(), commandbus.SubmitRequest{
		Queue:     ConvQueue,
		CommandID: commandbus.CommandID("reopen:" + conv.Name),
		Key:       commandbus.Key(conv.Name),
		Payload:   convtask.Payload{Verb: convtask.VerbReopen, Conversation: conv.Name}.Encode(),
	})
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 202, map[string]any{"ok": true, "invocationId": resp.InvocationID, "taskIds": resp.TaskIDs})
}

// handleConversationDelete reclaims a CLOSED conversation for good.
//
// It refuses anything not already closed rather than closing it first: a
// close-then-delete verb would let one call do the irreversible thing to a
// conversation that was still working, with the confirmation naming only the
// delete. Refusing makes the destructive step something ordered twice — and
// per design D-9 this refusal "depends only on a cheap read", so it STAYS
// IMMEDIATE (409), unlike every other outcome below it.
//
// async-command-bus task 5.7/D-6: a conversation already gone is the outcome
// a delete wanted, so an ABSENT target succeeds too — checked before the
// binding lookup, which has nothing to authorise a delete against once the
// record itself is gone.
//
// async-command-bus task 5.10/D-7: deleting now ENQUEUES a cascade over
// every already-closed descendant reachable by causedBy, one task per
// conversation, DEEPEST-FIRST, ending with the target itself — all sharing
// the target's own name as Key, so the bus serialises them and the target
// is never claimed before its own descendants have settled.
func (s *Server) handleConversationDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, _, err := s.Conversations.Get(r.Context(), state.Key(name)); err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeJSON(w, 202, map[string]any{"ok": true, "invocationId": "", "taskIds": []string{}})
			return
		}
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	conv, ok := s.reachConversation(w, r)
	if !ok {
		return
	}
	if conv.Phase != state.PhaseClosed {
		writeJSON(w, 409, map[string]string{"error": fmt.Sprintf(
			"conversation %q is %s, not Closed — close it first. Deleting removes its recorded results, "+
				"which are the only durable copy of what the agent answered", conv.Name, phaseOrOpen(conv))})
		return
	}
	reqs, err := s.deleteCascadeRequests(r.Context(), conv.Name)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	resp, err := s.acceptAndEnqueue(r.Context(), reqs...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 202, map[string]any{"ok": true, "invocationId": resp.InvocationID, "taskIds": resp.TaskIDs})
}

// deleteCascadeRequests lists rootName's own causedBy subtree (chat.Router.
// Descendants: current state, any depth) and turns it into one
// commandbus.SubmitRequest per conversation, DEEPEST-FIRST post-order,
// rootName itself last — task 5.10's exact ordering. Every descendant this
// reaches is already Closed by the time its ancestor is (the close cascade
// closes a whole subtree before any of it can be deleted), so there is no
// "not yet closed" case to refuse here.
func (s *Server) deleteCascadeRequests(ctx context.Context, rootName string) ([]commandbus.SubmitRequest, error) {
	descendants, err := s.Router.Descendants(ctx, rootName)
	if err != nil {
		return nil, err
	}
	names := deleteCascadeOrder(rootName, descendants)
	reqs := make([]commandbus.SubmitRequest, 0, len(names))
	for _, name := range names {
		reqs = append(reqs, commandbus.SubmitRequest{
			Queue:     ConvQueue,
			CommandID: commandbus.CommandID("delete:" + name),
			Key:       commandbus.Key(rootName),
			Payload:   convtask.Payload{Verb: convtask.VerbDelete, Conversation: name}.Encode(),
		})
	}
	return reqs, nil
}

// deleteCascadeOrder turns a flat descendant list into deepest-first
// post-order, rootName last — a child's own children are visited (and so
// ordered) before it, and rootName is visited only once every other name
// has been. Built from causedBy.Parent alone (the index chat.Router.
// Descendants already resolved it against is a hint, never trusted twice
// here).
func deleteCascadeOrder(rootName string, descendants []state.ConversationRecord) []string {
	children := map[string][]string{}
	for i := range descendants {
		d := &descendants[i]
		if d.CausedBy != nil {
			children[d.CausedBy.Parent] = append(children[d.CausedBy.Parent], d.Name)
		}
	}
	var out []string
	var visit func(name string)
	visit = func(name string) {
		for _, child := range children[name] {
			visit(child)
		}
		out = append(out, name)
	}
	visit(rootName)
	return out
}

func phaseOrOpen(conv *state.ConversationRecord) string {
	if conv.Phase == "" {
		return "open"
	}
	return conv.Phase
}

// handleChannelInbound is the accept half of an inbound chat reply
// (async-command-bus design D-9): the thread-to-conversation lookup step 6
// names is chat.Router.ConversationByThread, run HERE rather than inside
// the eventual task, because it IS the task's own Key. Three outcomes need
// no task at all — a cheap read already answers them completely:
//
//   - empty text, or an unbound thread: dropped, exactly as before, 202.
//   - the self-input loop guard (coordination-loop): refused immediately,
//     400 — unchanged from the synchronous error this endpoint always gave.
//   - a CLOSED conversation: the read-only notice is enqueued right here
//     (chat.ClosedConversationNotice, the same text HandleMessage's own
//     closed branch would send on a race), 202, no task — nothing about the
//     conversation changes, so there is nothing to defer.
//
// Anything else enqueues ONE "conv" task (taskInboundMessage,
// convtasks.go), which replays chat.Router.HandleMessage wholesale — see
// that function's own doc comment for why it is not split further.
func (s *Server) handleChannelInbound(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var in inboundReq
	if err := json.Unmarshal(body, &in); err != nil || in.Channel == "" || in.Text == "" {
		writeJSON(w, 400, map[string]string{"error": `need {"channel","text","threadId"}`})
		return
	}
	if in.ThreadID == nil || *in.ThreadID == "" {
		writeJSON(w, 400, map[string]string{"error": "threadId is required: /channel/inbound continues an existing " +
			"conversation thread and never starts one. A message on the channel's general surface is an ORIGINATION — " +
			"post it as a chat signal to POST /signal/inbound from a chat SignalSource a Ready Pipeline claims"})
		return
	}
	ctx := r.Context()
	var ch agentopsv1alpha1.Channel
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: in.Channel}, &ch); err != nil {
		writeJSON(w, 404, map[string]string{"error": fmt.Sprintf("unknown channel %q", in.Channel)})
		return
	}
	if !scopeAllows(r, ch.Spec.Adapter) {
		forbidScope(w)
		return
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		writeJSON(w, 202, map[string]bool{"ok": true}) // nothing to act on — same silent no-op as before
		return
	}
	conv, err := s.Router.ConversationByThread(ctx, &ch, *in.ThreadID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if conv == nil {
		writeJSON(w, 202, map[string]bool{"ok": true}) // dropped by contract (IN-2): no conversation bound to this thread
		return
	}
	if ch.Name == conv.Name {
		writeJSON(w, 400, map[string]string{"error": chat.ErrSelfInput.Error()})
		return
	}
	if conv.Phase == state.PhaseClosed {
		s.Ops.EnqueueMessage(ctx, &ch, conv, in.ThreadID, chat.ClosedConversationNotice())
		writeJSON(w, 202, map[string]bool{"ok": true})
		return
	}
	resp, err := s.acceptAndEnqueue(ctx, commandbus.SubmitRequest{
		Queue:     ConvQueue,
		CommandID: inboundMessageCommandID(r.Header.Get("Idempotency-Key"), ch.Name, *in.ThreadID, text, in.Sender),
		Key:       commandbus.Key(conv.Name),
		Payload: convtask.Payload{
			Verb: convtask.VerbInboundMessage, Conversation: conv.Name,
			Channel: ch.Name, ThreadID: in.ThreadID, Text: in.Text, Sender: in.Sender,
		}.Encode(),
	})
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 202, map[string]any{"ok": true, "invocationId": resp.InvocationID, "taskIds": resp.TaskIDs})
}

func (s *Server) handleChannelList(w http.ResponseWriter, r *http.Request) {
	channelType, ok := adapterParam(w, r)
	if !ok {
		return
	}
	if !scopeAllows(r, channelType) {
		forbidScope(w)
		return
	}
	var list agentopsv1alpha1.ChannelList
	if err := s.Client.List(r.Context(), &list, client.InNamespace(s.Namespace)); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	type chanOut struct {
		Name   string          `json:"name"`
		Config json.RawMessage `json:"config,omitempty"`
		// CredentialEnvPrefix locates this channel's projected credentials in
		// the adapter's own environment: Secret key K is env <prefix>K. Derived
		// from projection metadata (the secret NAME on the Channel), never from
		// Secret values.
		CredentialEnvPrefix string `json:"credentialEnvPrefix,omitempty"`
	}
	out := []chanOut{}
	for i := range list.Items {
		ch := &list.Items[i]
		if ch.Spec.Adapter != channelType {
			continue
		}
		c := chanOut{Name: ch.Name}
		if ch.Spec.Config != nil {
			c.Config = ch.Spec.Config.Raw
		}
		if ch.Spec.CredentialsSecretRef != nil {
			c.CredentialEnvPrefix = controller.CredentialEnvPrefix(ch.Name)
		}
		out = append(out, c)
	}
	writeJSON(w, 200, out)
}

func (s *Server) stateChannel(r *http.Request, w http.ResponseWriter, name string) *agentopsv1alpha1.Channel {
	var ch agentopsv1alpha1.Channel
	if err := s.Reader.Get(r.Context(), types.NamespacedName{Namespace: s.Namespace, Name: name}, &ch); err != nil {
		writeJSON(w, 404, map[string]string{"error": fmt.Sprintf("unknown channel %q", name)})
		return nil
	}
	if !scopeAllows(r, ch.Spec.Adapter) {
		forbidScope(w)
		return nil
	}
	return &ch
}

// handleStateGet and handleStatePut are the only callers of AdapterState for
// the channel surface (design D-4) — the channel resolves and scopes exactly
// as it always has, and the value moves through Stores.AdapterState instead
// of a raw annotation patch. Under kube that collection IS the same
// agentops.dev/adapter-state-<key> annotation on this same Channel (task
// 3.3), so this is a call-site change, not a wire change.
func (s *Server) handleStateGet(w http.ResponseWriter, r *http.Request) {
	ch := s.stateChannel(r, w, r.PathValue("channel"))
	if ch == nil {
		return
	}
	v, err := getAdapterState(r.Context(), s.AdapterState, state.AdapterStateKey(state.SurfaceChannel, ch.Name, r.PathValue("key")))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"value": v})
}

func (s *Server) handleStatePut(w http.ResponseWriter, r *http.Request) {
	ch := s.stateChannel(r, w, r.PathValue("channel"))
	if ch == nil {
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<10))
	var in struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		writeJSON(w, 400, map[string]string{"error": errInvalidJSON})
		return
	}
	key := state.AdapterStateKey(state.SurfaceChannel, ch.Name, r.PathValue("key"))
	if err := putAdapterState(r.Context(), s.AdapterState, key, in.Value); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// getAdapterState reads one adapter cursor, reporting "" for a key never
// written — the same absent-reads-empty rule the old annotation read gave,
// since a value the handler wrote empty is still present (kube/adapterstate_test.go).
func getAdapterState(ctx context.Context, coll state.Collection[string], key state.Key) (string, error) {
	v, _, err := coll.Get(ctx, key)
	if errors.Is(err, state.ErrNotFound) {
		return "", nil
	}
	return v, err
}

// putAdapterState writes v under key: Mutate's get-modify-CAS-retry for a key
// already written, falling back to Create on its first write (Mutate cannot
// get-modify a record that is not there yet) — the same last-write-wins
// outcome the old merge patch gave, now expressed through the contract
// (state-store-generic-contract design D-4, task 3.3's "handlers write
// through Mutate").
func putAdapterState(ctx context.Context, coll state.Collection[string], key state.Key, v string) error {
	_, err := state.Mutate(ctx, coll, key, func(cur *string) error { *cur = v; return nil })
	if !errors.Is(err, state.ErrNotFound) {
		return err
	}
	_, err = coll.Create(ctx, key, v)
	if errors.Is(err, state.ErrAlreadyExists) {
		// Lost a race against another writer's first Create: it exists now,
		// so Mutate can get-modify-CAS it.
		_, err = state.Mutate(ctx, coll, key, func(cur *string) error { *cur = v; return nil })
	}
	return err
}

func (s *Server) handleChannelStatus(w http.ResponseWriter, r *http.Request) {
	ch := s.stateChannel(r, w, r.PathValue("name"))
	if ch == nil {
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var in struct {
		Ready   bool   `json:"ready"`
		Reason  string `json:"reason,omitempty"`
		Message string `json:"message,omitempty"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		writeJSON(w, 400, map[string]string{"error": errInvalidJSON})
		return
	}
	cond := metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AdapterReady"}
	if !in.Ready {
		cond.Status = metav1.ConditionFalse
		cond.Reason = "AdapterError"
	}
	if in.Reason != "" {
		cond.Reason = in.Reason
	}
	cond.Message = in.Message
	patch := client.MergeFrom(ch.DeepCopy())
	apimeta.SetStatusCondition(&ch.Status.Conditions, cond)
	if err := s.Client.Status().Patch(r.Context(), ch, patch); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
