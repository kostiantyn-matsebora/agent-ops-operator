// Package httpapi is the manager's HTTP surface:
//
//	GET  /healthz
//	GET  /work?convo=&wait=&pod=   worker long-poll dispatch
//	POST /work/done                worker completion report
//
// The manager hosts NO signal transports — alert/webhook ingestion lives in
// signal adapters feeding POST /signal/inbound. That endpoint is also how work
// ORIGINATES: there is no route that names a Pipeline, because a caller
// choosing its own wiring is what the Pipeline model exists to prevent.
//
// plus the channel adapter contract (bearer-token auth, see ADAPTER_TOKEN):
//
//	GET  /channel/ops?type=&wait=       outbound op long-poll (204 on timeout)
//	POST /channel/ops/{id}/done         async op completion
//	POST /channel/inbound               {"channel","threadId"?,"text"} -> router
//	GET  /channel/channels?type=        channels served by an adapter (+config)
//	GET  /channel/state/{channel}/{key} adapter cursor state (annotation-backed)
//	PUT  /channel/state/{channel}/{key}
//	POST /channel/channels/{name}/status  {"ready","reason"?,"message"?}
package httpapi

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/dispatch"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/ingest"
)

// StateAnnotationPrefix namespaces adapter cursor state kept as Channel
// annotations (e.g. agentops.dev/adapter-state-offset).
const StateAnnotationPrefix = "agentops.dev/adapter-state-"

// Server carries dependencies for the HTTP surface.
type Server struct {
	Client    client.Client // cached
	Reader    client.Reader // APIReader: strong reads for dispatch state
	Namespace string
	Addr      string

	// Channel adapter contract. AdapterToken guards /channel/*; empty
	// disables the surface (503) — the manager reads it from env, never from
	// the Secret API.
	Ops          *chat.OpQueue
	Router       *chat.Router
	AdapterToken string

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

	cooldowns map[string]*ingest.Cooldown
}

// defaultMaxQueuedConversations is the pending-backlog bound when unset.
const defaultMaxQueuedConversations = 50

func (s *Server) maxQueued() int {
	if s.MaxQueuedConversations > 0 {
		return s.MaxQueuedConversations
	}
	return defaultMaxQueuedConversations
}

// Handler builds the HTTP mux (shared by Start and tests).
func (s *Server) Handler() http.Handler {
	if s.cooldowns == nil {
		s.cooldowns = map[string]*ingest.Cooldown{}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /work", s.handleWork)
	mux.HandleFunc("POST /work/done", s.handleWorkDone)
	mux.HandleFunc("GET /channel/ops", s.adapterAuth(s.handleChannelOps))
	mux.HandleFunc("POST /channel/ops/{id}/done", s.adapterAuth(s.handleChannelOpDone))
	mux.HandleFunc("POST /channel/inbound", s.adapterAuth(s.handleChannelInbound))
	mux.HandleFunc("GET /channel/channels", s.adapterAuth(s.handleChannelList))
	mux.HandleFunc("GET /channel/state/{channel}/{key}", s.adapterAuth(s.handleStateGet))
	mux.HandleFunc("PUT /channel/state/{channel}/{key}", s.adapterAuth(s.handleStatePut))
	mux.HandleFunc("POST /channel/channels/{name}/status", s.adapterAuth(s.handleChannelStatus))
	mux.HandleFunc("POST /signal/inbound", s.signalAuth(s.handleSignalInbound))
	mux.HandleFunc("GET /signal/sources", s.signalAuth(s.handleSignalSources))
	mux.HandleFunc("GET /signal/state/{source}/{key}", s.signalAuth(s.handleSignalStateGet))
	mux.HandleFunc("PUT /signal/state/{source}/{key}", s.signalAuth(s.handleSignalStatePut))
	mux.HandleFunc("POST /signal/sources/{name}/status", s.signalAuth(s.handleSignalStatus))
	mux.HandleFunc("GET /activity", s.anyAdapterAuth(s.handleActivity))
	mux.HandleFunc("GET /activity/stream", s.anyAdapterAuth(s.handleActivityStream))
	mux.HandleFunc("POST /activity", s.anyAdapterAuth(s.handleActivityReport))
	mux.HandleFunc("GET /status", s.anyAdapterAuth(s.handleStatus))
	mux.HandleFunc("GET /pipelines/{name}/resolved", s.anyAdapterAuth(s.handlePipelineResolved))
	return mux
}

// anyAdapterAuth accepts the master token or ANY derived adapter token, channel
// or signal. The introspection surfaces are cross-cutting — the console holds
// both identities and reads them with whichever it has — so scoping them to one
// adapter kind would only force a caller to pick a token arbitrarily.
func (s *Server) anyAdapterAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.AdapterToken == "" {
			writeJSON(w, 503, map[string]string{"error": "adapter auth not configured"})
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
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

func (s *Server) resolvePayload(ctx context.Context, ns string) dispatch.PayloadResolver {
	return func(item agentopsv1alpha1.InputItem) (string, error) {
		if item.PayloadRef == nil {
			return item.Payload, nil
		}
		var ci agentopsv1alpha1.ConversationInput
		if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: ns, Name: item.PayloadRef.Name}, &ci); err != nil {
			return "", err
		}
		return ci.Spec.Payload, nil
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
func (s *Server) effectiveTooling(ctx context.Context, conv *agentopsv1alpha1.Conversation) (dispatch.Tooling, error) {
	byRef, err := s.resolveToolsets(ctx, conv, conv.Spec.Toolsets)
	if err != nil {
		return dispatch.Tooling{}, err
	}
	return dispatch.Tooling{
		AllowedTools: dispatch.EffectiveAllowedTools(byRef),
		Mode:         dispatch.ToolsModeOf(conv.Spec.Toolsets),
	}, nil
}

// resolveToolsets reads a binding's MCPToolsets in ref order.
func (s *Server) resolveToolsets(ctx context.Context, conv *agentopsv1alpha1.Conversation,
	binding *agentopsv1alpha1.ToolsetBinding) ([][]string, error) {

	if binding == nil {
		return nil, nil
	}
	byRef := make([][]string, 0, len(binding.Refs))
	for _, ref := range binding.Refs {
		var ts agentopsv1alpha1.MCPToolset
		if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: ref.Name}, &ts); err != nil {
			err = fmt.Errorf("bound MCPToolset %q: %w", ref.Name, err)
			s.setToolingCondition(ctx, conv, "ToolsetUnresolved", err.Error())
			return nil, err
		}
		byRef = append(byRef, ts.Spec.Tools)
	}
	return byRef, nil
}

// setToolingCondition surfaces a binding failure on the conversation so it is
// visible in `kubectl describe` rather than only in manager logs.
func (s *Server) setToolingCondition(ctx context.Context, conv *agentopsv1alpha1.Conversation, reason, message string) {
	patch := client.MergeFrom(conv.DeepCopy())
	apimeta.SetStatusCondition(&conv.Status.Conditions, metav1.Condition{
		Type: controller.ConditionToolingResolved, Status: metav1.ConditionFalse,
		Reason: reason, Message: message,
	})
	_ = s.Client.Status().Patch(ctx, conv, patch)
}

func (s *Server) tryDispatch(ctx context.Context, convoName, podName string) (dispatch.WorkUnit, bool, error) {
	var conv agentopsv1alpha1.Conversation
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: convoName}, &conv); err != nil {
		return dispatch.WorkUnit{}, false, err
	}
	// pending-topic gate: a channel-bound conversation waits until AT LEAST ONE
	// thread binding exists before its first unit dispatches (waiting for all
	// would deadlock on one broken channel; late topics catch up). Dangling
	// channelRefs stay chat-less. Delivery itself needs no channel lookup — the
	// operator routes every result to the bound channels after /work/done.
	if len(conv.Spec.ChannelRefs) > 0 {
		chatBound := false
		for _, ref := range conv.Spec.ChannelRefs {
			var ch agentopsv1alpha1.Channel
			if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: ref.Name}, &ch); err != nil || ch.Spec.Adapter == "" {
				continue
			}
			chatBound = true
		}
		if chatBound && len(conv.Status.Threads) == 0 {
			return dispatch.WorkUnit{}, false, nil
		}
	}
	var profile agentopsv1alpha1.AgentProfile
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: conv.Spec.ProfileRef.Name}, &profile); err != nil {
		return dispatch.WorkUnit{}, false, err
	}
	tools, err := s.effectiveTooling(ctx, &conv)
	if err != nil {
		return dispatch.WorkUnit{}, false, err
	}
	unit, ids, ok, err := dispatch.Next(&conv, &profile, tools, s.resolvePayload(ctx, s.Namespace), time.Now())
	if err != nil || !ok {
		return dispatch.WorkUnit{}, false, err
	}
	patch := client.MergeFrom(conv.DeepCopy())
	now := metav1.Now()
	conv.Status.Inflight = &agentopsv1alpha1.InflightRun{RunID: unit.RunID, InputIDs: ids, DispatchedAt: now}
	conv.Status.Phase = agentopsv1alpha1.ConversationWorking
	conv.Status.LastActivity = &now
	if podName != "" {
		conv.Status.RuntimePod = podName
	}
	if err := s.Client.Status().Patch(ctx, &conv, patch); err != nil {
		if apierrors.IsConflict(err) { // racing update — caller retries next tick
			return dispatch.WorkUnit{}, false, nil
		}
		return dispatch.WorkUnit{}, false, err
	}
	pipeline := s.pipelineName(ctx, &conv)
	s.Activity.Emit(activity.Event{
		Kind:     activity.KindRunDispatched,
		From:     s.originNode(pipeline, conv.Name),
		To:       activity.Node(activity.NodeRuntime, s.runtimeName(ctx, &profile)),
		Pipeline: pipeline, Conversation: conv.Name, RunID: unit.RunID,
		Detail: fmt.Sprintf("%d input(s) to %s", len(ids), profile.Name),
	})
	return unit, true, nil
}

// pipelineName attributes a conversation to its originating pipeline, or "" when
// the wiring is ambiguous. INFERRED from materialized bindings — a Conversation
// records no pipelineRef — so a blank answer is the honest one and never a
// guess. Telemetry labelling only; nothing routes on it.
func (s *Server) pipelineName(ctx context.Context, conv *agentopsv1alpha1.Conversation) string {
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

// runtimeName resolves the AgentRuntime a profile executes on, matching the
// runtime pod builder's own fallback (profile ref, then "default").
func (s *Server) runtimeName(ctx context.Context, profile *agentopsv1alpha1.AgentProfile) string {
	if profile.Spec.RuntimeRef != nil && profile.Spec.RuntimeRef.Name != "" {
		return profile.Spec.RuntimeRef.Name
	}
	var rt agentopsv1alpha1.AgentRuntime
	if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: "default"}, &rt); err == nil {
		return rt.Name
	}
	return "default"
}

type workDone struct {
	Convo     string `json:"convo"`
	RunID     string `json:"runId"`
	Status    string `json:"status"`
	ExitCode  *int32 `json:"exitCode,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	Result    string `json:"result,omitempty"`
}

func (s *Server) handleWorkDone(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var d workDone
	if err := json.Unmarshal(body, &d); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	ctx := r.Context()
	var conv agentopsv1alpha1.Conversation
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: d.Convo}, &conv); err != nil {
		writeJSON(w, 404, map[string]string{"error": err.Error()})
		return
	}
	patch := client.MergeFrom(conv.DeepCopy())
	now := metav1.Now()
	// Latency is DERIVED from the dispatch stamp the manager itself wrote, not
	// reported by the runtime: a runtime that lies or has a skewed clock would
	// otherwise put a fiction on the graph.
	var latencyMs int64
	if conv.Status.Inflight != nil && conv.Status.Inflight.RunID == d.RunID {
		latencyMs = now.Sub(conv.Status.Inflight.DispatchedAt.Time).Milliseconds()
		conv.Status.ProcessedInputIDs = bound(append(conv.Status.ProcessedInputIDs, conv.Status.Inflight.InputIDs...), 50)
		conv.Status.Inflight = nil
	}
	if d.SessionID != "" && conv.Status.SessionID == "" {
		conv.Status.SessionID = d.SessionID
	}
	result := d.Result
	if len(result) > 2000 {
		result = result[:2000]
	}
	conv.Status.Runs = append(conv.Status.Runs, agentopsv1alpha1.RunStatus{
		RunID: d.RunID, Status: d.Status, ExitCode: d.ExitCode, Result: result, FinishedAt: &now,
	})
	if len(conv.Status.Runs) > 10 {
		conv.Status.Runs = conv.Status.Runs[len(conv.Status.Runs)-10:]
	}
	if len(dispatch.PendingInputs(&conv)) > 0 {
		conv.Status.Phase = agentopsv1alpha1.ConversationQueued
	} else {
		conv.Status.Phase = agentopsv1alpha1.ConversationIdle
	}
	conv.Status.LastActivity = &now
	if err := s.Client.Status().Patch(ctx, &conv, patch); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	pipeline := s.pipelineName(ctx, &conv)
	var doneProfile agentopsv1alpha1.AgentProfile
	_ = s.Client.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: conv.Spec.ProfileRef.Name}, &doneProfile)
	runEvent := activity.Event{
		Kind:     activity.KindRunCompleted,
		From:     activity.Node(activity.NodeRuntime, s.runtimeName(ctx, &doneProfile)),
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
	if len(conv.Spec.ChannelRefs) > 0 && s.Router != nil {
		text := strings.TrimSpace(result)
		// A run that produced nothing is a FAILURE to report, not an answer to
		// render: it goes out as a notice so an adapter styles it as one rather
		// than presenting an empty agent reply.
		switch {
		case d.Status != "succeeded":
			s.Router.FanOutSend(ctx, &conv, chat.Warn("❌ run failed ("+d.Status+")"))
		case text == "":
			s.Router.FanOutSend(ctx, &conv, chat.Warn("❌ run finished without output"))
		default:
			s.Router.FanOutSend(ctx, &conv, chat.AnswerMessage(text, d.Status))
		}
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
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
			writeJSON(w, 503, map[string]string{"error": "adapter auth not configured"})
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
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
	deadline := time.Now().Add(time.Duration(wait) * time.Second)
	for {
		if op := s.Ops.Claim(channelType); op != nil {
			writeJSON(w, 200, op)
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

func (s *Server) handleChannelOpDone(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var res chat.OpResult
	// An EMPTY body is a valid completion: it is how close-topic reports
	// success, and how any op says "done, nothing to report".
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &res); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
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
	if err := s.Router.HandleMessage(ctx, &ch, chat.InboundMessage{ThreadID: in.ThreadID, Text: in.Text, Sender: in.Sender}); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 202, map[string]bool{"ok": true})
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

func (s *Server) handleStateGet(w http.ResponseWriter, r *http.Request) {
	ch := s.stateChannel(r, w, r.PathValue("channel"))
	if ch == nil {
		return
	}
	writeJSON(w, 200, map[string]string{"value": ch.Annotations[StateAnnotationPrefix+r.PathValue("key")]})
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
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	patch := client.MergeFrom(ch.DeepCopy())
	if ch.Annotations == nil {
		ch.Annotations = map[string]string{}
	}
	ch.Annotations[StateAnnotationPrefix+r.PathValue("key")] = in.Value
	if err := s.Client.Patch(r.Context(), ch, patch); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
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
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
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
