// Package httpapi is the manager's HTTP surface:
//
//	GET  /healthz
//	GET  /work?convo=&wait=&pod=   worker long-poll dispatch
//	POST /work/done                worker completion report
//	POST /task                     {"profile","agent"?,"task","channel"?}
//	POST /ingest/alertmanager/{source}  Alertmanager webhook
package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/dispatch"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/ingest"
)

// Server carries dependencies for the HTTP surface.
type Server struct {
	Client    client.Client // cached
	Reader    client.Reader // APIReader: strong reads for dispatch state
	Namespace string
	Addr      string

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
	mux.HandleFunc("POST /ingest/alertmanager/{source}", s.handleAlertmanager)
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
	var profile agentopsv1alpha1.AgentProfile
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: conv.Spec.ProfileRef.Name}, &profile); err != nil {
		return dispatch.WorkUnit{}, false, err
	}
	unit, ids, ok, err := dispatch.Next(&conv, &profile, s.resolvePayload(ctx, s.Namespace), time.Now())
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
	Title   string `json:"title,omitempty"`
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
	if t.Channel != "" {
		conv.Spec.ChannelRef = &agentopsv1alpha1.ObjectRef{Name: t.Channel}
	}
	if err := s.Client.Create(ctx, conv); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 202, map[string]string{"conversation": conv.Name})
}

// ---- alertmanager ingest ----------------------------------------------------

type amAlert struct {
	Status       string            `json:"status"`
	Fingerprint  string            `json:"fingerprint"`
	StartsAt     string            `json:"startsAt"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	GeneratorURL string            `json:"generatorURL"`
}

type amPayload struct {
	Alerts []amAlert `json:"alerts"`
}

func (s *Server) handleAlertmanager(w http.ResponseWriter, r *http.Request) {
	sourceName := r.PathValue("source")
	ctx := r.Context()
	var source agentopsv1alpha1.SignalSource
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: sourceName}, &source); err != nil {
		writeJSON(w, 404, map[string]string{"error": fmt.Sprintf("unknown source %q", sourceName)})
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var payload amPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	var firing []amAlert
	for _, a := range payload.Alerts {
		if a.Status == "firing" {
			firing = append(firing, a)
		}
	}
	if len(firing) == 0 {
		writeJSON(w, 200, map[string]any{"queued": 0, "reason": "no firing alerts"})
		return
	}

	cd := s.cooldowns[sourceName]
	if cd == nil {
		hours := source.Spec.Grouping.CooldownHours
		if hours <= 0 {
			hours = 6
		}
		cd = ingest.NewCooldown(time.Duration(hours) * time.Hour)
		s.cooldowns[sourceName] = cd
	}
	var fps []string
	byFP := map[string]amAlert{}
	for _, a := range firing {
		fps = append(fps, a.Fingerprint)
		byFP[a.Fingerprint] = a
	}
	fresh := cd.Fresh(fps)
	if len(fresh) == 0 {
		writeJSON(w, 200, map[string]any{"queued": 0, "reason": "all within cooldown"})
		return
	}

	groups := map[string][]amAlert{}
	for _, fp := range fresh {
		a := byFP[fp]
		sig := ingest.Signature(a.Labels, source.Spec.Grouping.SignatureLabels)
		groups[sig] = append(groups[sig], a)
	}

	created := 0
	for sig, alerts := range groups {
		if err := s.routeGroup(ctx, &source, sig, alerts); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		created++
	}
	// bookkeeping
	patch := client.MergeFrom(source.DeepCopy())
	now := metav1.Now()
	source.Status.LastReceived = &now
	source.Status.ReceivedTotal += int64(len(fresh))
	_ = s.Client.Status().Patch(ctx, &source, patch)

	writeJSON(w, 200, map[string]any{"queued": len(fresh), "conversations": created})
}

func (s *Server) routeGroup(ctx context.Context, source *agentopsv1alpha1.SignalSource, sig string, alerts []amAlert) error {
	payloadDoc := map[string]any{"receivedAt": time.Now().UTC().Format(time.RFC3339), "alerts": alerts}
	payloadJSON, _ := json.MarshalIndent(payloadDoc, "", "  ")

	// existing conversation with the same signature within the window?
	windowDays := source.Spec.Grouping.WindowDays
	if windowDays <= 0 {
		windowDays = 7
	}
	var list agentopsv1alpha1.ConversationList
	if err := s.Reader.List(ctx, &list, client.InNamespace(s.Namespace),
		client.MatchingLabels{controller.LabelSignatureHash: ingest.SignatureHash(sig)}); err != nil {
		return err
	}
	var conv *agentopsv1alpha1.Conversation
	cutoff := time.Now().Add(-time.Duration(windowDays) * 24 * time.Hour)
	for i := range list.Items {
		c := &list.Items[i]
		last := c.CreationTimestamp.Time
		if c.Status.LastActivity != nil {
			last = c.Status.LastActivity.Time
		}
		if last.After(cutoff) {
			conv = c
			break
		}
	}

	inputType := agentopsv1alpha1.InputAlert
	if conv == nil {
		labels := alerts[0].Labels
		title := "🔍 " + labels["alertname"]
		if ns := labels["namespace"]; ns != "" {
			title += " — " + ns
		}
		conv = &agentopsv1alpha1.Conversation{}
		conv.Namespace = s.Namespace
		conv.GenerateName = "alert-"
		conv.Labels = map[string]string{controller.LabelSignatureHash: ingest.SignatureHash(sig)}
		conv.Spec = agentopsv1alpha1.ConversationSpec{
			ProfileRef: source.Spec.ProfileRef,
			Title:      title,
			Signature:  sig,
		}
		if source.Spec.ChannelRef != nil {
			conv.Spec.ChannelRef = source.Spec.ChannelRef
		}
		if err := s.Client.Create(ctx, conv); err != nil {
			return err
		}
	} else if conv.Status.SessionID != "" {
		inputType = agentopsv1alpha1.InputRecurrence // same problem, resume with context
	}

	ci := &agentopsv1alpha1.ConversationInput{}
	ci.Namespace = s.Namespace
	ci.GenerateName = conv.Name + "-in-"
	ci.Spec = agentopsv1alpha1.ConversationInputSpec{
		ConversationRef: agentopsv1alpha1.ObjectRef{Name: conv.Name},
		Type:            inputType,
		Payload:         string(payloadJSON),
	}
	if err := s.Client.Create(ctx, ci); err != nil {
		return err
	}

	// append the input item with optimistic retry
	for attempt := 0; attempt < 5; attempt++ {
		var fresh agentopsv1alpha1.Conversation
		if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: conv.Name}, &fresh); err != nil {
			return err
		}
		patch := client.MergeFrom(fresh.DeepCopy())
		fresh.Spec.Inputs = append(fresh.Spec.Inputs, agentopsv1alpha1.InputItem{
			ID:         "in-" + strconv.FormatInt(time.Now().UnixNano(), 36),
			Type:       inputType,
			PayloadRef: &agentopsv1alpha1.ObjectRef{Name: ci.Name},
			ReceivedAt: metav1.Now(),
		})
		if err := s.Client.Patch(ctx, &fresh, patch); err != nil {
			if apierrors.IsConflict(err) {
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("conflict appending input to %s", conv.Name)
}
