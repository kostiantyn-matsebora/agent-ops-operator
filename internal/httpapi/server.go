// Package httpapi is the manager's HTTP surface:
//
//	GET  /healthz
//	GET  /work?convo=&wait=&pod=   worker long-poll dispatch
//	POST /work/done                worker completion report
//	POST /task                     {"profile","agent"?,"task","channel"?}
//
// The manager hosts NO signal transports — alert/webhook ingestion lives in
// signal adapters feeding POST /signal/inbound.
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

	cooldowns map[string]*ingest.Cooldown
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
	mux.HandleFunc("POST /task", s.handleTask)
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
	return mux
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

func (s *Server) tryDispatch(ctx context.Context, convoName, podName string) (dispatch.WorkUnit, bool, error) {
	var conv agentopsv1alpha1.Conversation
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: convoName}, &conv); err != nil {
		return dispatch.WorkUnit{}, false, err
	}
	// channel metadata: delivery instructions, and the pending-topic gate — a
	// channel-bound conversation waits until AT LEAST ONE thread binding
	// exists before its first unit dispatches (waiting for all would deadlock
	// on one broken channel; late topics catch up). Dangling channelRefs stay
	// chat-less. Multi-channel conversations force result delivery — the
	// manager owns distribution on mirrored conversations.
	var delivery dispatch.Delivery
	if len(conv.Spec.ChannelRefs) > 0 {
		chatBound := false
		for _, ref := range conv.Spec.ChannelRefs {
			var ch agentopsv1alpha1.Channel
			if err := s.Client.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: ref.Name}, &ch); err != nil || ch.Spec.Type == "" {
				continue
			}
			chatBound = true
			if len(conv.Spec.ChannelRefs) == 1 && ch.Spec.Delivery != nil {
				delivery = dispatch.Delivery{Mode: string(ch.Spec.Delivery.Mode), AgentInstructions: ch.Spec.Delivery.AgentInstructions}
			}
		}
		if chatBound && len(conv.Status.Threads) == 0 {
			return dispatch.WorkUnit{}, false, nil
		}
	}
	var profile agentopsv1alpha1.AgentProfile
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: conv.Spec.ProfileRef.Name}, &profile); err != nil {
		return dispatch.WorkUnit{}, false, err
	}
	unit, ids, ok, err := dispatch.Next(&conv, &profile, s.resolvePayload(ctx, s.Namespace), delivery, time.Now())
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
	return unit, true, nil
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
	if conv.Status.Inflight != nil && conv.Status.Inflight.RunID == d.RunID {
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
	// multi-channel conversations: the manager fans the reply out to every
	// bound thread (mirrored surfaces never mistake silence for success)
	if len(conv.Spec.ChannelRefs) > 1 && s.Router != nil {
		text := strings.TrimSpace(result)
		if d.Status != "succeeded" {
			text = "❌ run failed (" + d.Status + ")"
		} else if text == "" {
			text = "❌ run finished without output"
		}
		s.Router.FanOutSend(ctx, &conv, text)
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func bound(s []string, n int) []string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

// ---- task lane --------------------------------------------------------------

type taskReq struct {
	Profile string `json:"profile"`
	Agent   string `json:"agent,omitempty"`
	Task    string `json:"task"`
	Channel string `json:"channel,omitempty"`
	// Pipeline binds the conversation to the named Pipeline's channel set
	// (mirrored surfaces). Mutually additive with Channel.
	Pipeline string `json:"pipeline,omitempty"`
	Title    string `json:"title,omitempty"`
}

func (s *Server) handleTask(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var t taskReq
	if err := json.Unmarshal(body, &t); err != nil || t.Task == "" || t.Profile == "" {
		writeJSON(w, 400, map[string]string{"error": `need {"profile","task"}`})
		return
	}
	ctx := r.Context()
	var profile agentopsv1alpha1.AgentProfile
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: t.Profile}, &profile); err != nil {
		writeJSON(w, 404, map[string]string{"error": fmt.Sprintf("unknown profile %q", t.Profile)})
		return
	}
	title := t.Title
	if title == "" {
		title = "🛠 " + strings.Join(strings.Fields(t.Task), " ")
		if len(title) > 60 {
			title = title[:60]
		}
	}
	conv := &agentopsv1alpha1.Conversation{}
	conv.Namespace = s.Namespace
	conv.GenerateName = "task-"
	conv.Spec = agentopsv1alpha1.ConversationSpec{
		ProfileRef: agentopsv1alpha1.ObjectRef{Name: t.Profile},
		Title:      title,
		Inputs: []agentopsv1alpha1.InputItem{{
			ID: "in-" + strconv.FormatInt(time.Now().UnixNano(), 36), Type: agentopsv1alpha1.InputTask,
			Payload: t.Task, Agent: t.Agent, ReceivedAt: metav1.Now(),
		}},
	}
	if t.Pipeline != "" {
		var p agentopsv1alpha1.Pipeline
		if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: t.Pipeline}, &p); err != nil {
			writeJSON(w, 404, map[string]string{"error": fmt.Sprintf("unknown pipeline %q", t.Pipeline)})
			return
		}
		conv.Spec.ChannelRefs = append(conv.Spec.ChannelRefs, p.Spec.ChannelRefs...)
	}
	if t.Channel != "" {
		already := false
		for _, ref := range conv.Spec.ChannelRefs {
			if ref.Name == t.Channel {
				already = true
			}
		}
		if !already {
			conv.Spec.ChannelRefs = append(conv.Spec.ChannelRefs, agentopsv1alpha1.ObjectRef{Name: t.Channel})
		}
	}
	if err := s.Client.Create(ctx, conv); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 202, map[string]string{"conversation": conv.Name})
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
	channelType := r.URL.Query().Get("type")
	if channelType == "" {
		writeJSON(w, 400, map[string]string{"error": "missing type"})
		return
	}
	if !scopeAllows(r, channelType) {
		forbidScope(w)
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
	if err := json.Unmarshal(body, &res); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
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

func (s *Server) handleChannelInbound(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var in inboundReq
	if err := json.Unmarshal(body, &in); err != nil || in.Channel == "" || in.Text == "" {
		writeJSON(w, 400, map[string]string{"error": `need {"channel","text"}`})
		return
	}
	ctx := r.Context()
	var ch agentopsv1alpha1.Channel
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: in.Channel}, &ch); err != nil {
		writeJSON(w, 404, map[string]string{"error": fmt.Sprintf("unknown channel %q", in.Channel)})
		return
	}
	if !scopeAllows(r, ch.Spec.Type) {
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
	channelType := r.URL.Query().Get("type")
	if channelType == "" {
		writeJSON(w, 400, map[string]string{"error": "missing type"})
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
		if ch.Spec.Type != channelType {
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
	if !scopeAllows(r, ch.Spec.Type) {
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
