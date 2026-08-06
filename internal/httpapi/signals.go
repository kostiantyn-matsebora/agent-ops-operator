// Normalized-signal routing: the single core every signal source feeds —
// the built-in Alertmanager webhook and external signal adapters alike.
// Adapters normalize; the manager applies the source's grouping policy
// (fingerprint cooldown, signature grouping, window reuse, recurrence).
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
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/ingest"
)

// NormalizedSignal is one signal in the contract's normalized shape.
type NormalizedSignal struct {
	// Fingerprint identifies this event for cooldown dedup (at-least-once
	// delivery collapses on it).
	Fingerprint string `json:"fingerprint"`
	// Labels feed the source's grouping.signatureLabels.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// Title overrides the conversation title when this signal opens one.
	Title string `json:"title,omitempty"`
	// Payload is the raw signal content handed to the agent (stored out of
	// line as a ConversationInput).
	Payload string `json:"payload,omitempty"`
	// Kind selects the input lane: "alert" (default; read-only investigation
	// prompt) or "job" (task-lane prompt).
	Kind string `json:"kind,omitempty"`
}

// combineFunc renders one input payload for a signature group of fresh
// signals (callers control multi-signal payload shape).
type combineFunc func(group []NormalizedSignal) string

// combineJoined is the generic combiner: single payload verbatim, several
// joined with a separator.
func combineJoined(group []NormalizedSignal) string {
	if len(group) == 1 {
		return group[0].Payload
	}
	parts := make([]string, 0, len(group))
	for _, s := range group {
		parts = append(parts, s.Payload)
	}
	return strings.Join(parts, "\n---\n")
}

// routeSignals applies the source's grouping policy to a batch of normalized
// signals: cooldown by fingerprint, signature grouping, window-based
// conversation reuse with recurrence-on-session, out-of-line payloads, and
// source status bookkeeping (the single place lastReceived/receivedTotal are
// updated). Returns the number of fresh signals and conversations touched.
func (s *Server) routeSignals(ctx context.Context, source *agentopsv1alpha1.SignalSource, signals []NormalizedSignal, combine combineFunc) (int, int, error) {
	cd := s.cooldowns[source.Name]
	if cd == nil {
		hours := source.Spec.Grouping.CooldownHours
		if hours <= 0 {
			hours = 6
		}
		cd = ingest.NewCooldown(time.Duration(hours) * time.Hour)
		s.cooldowns[source.Name] = cd
	}
	var fps []string
	byFP := map[string]NormalizedSignal{}
	for _, sig := range signals {
		fps = append(fps, sig.Fingerprint)
		byFP[sig.Fingerprint] = sig
	}
	fresh := cd.Fresh(fps)
	if len(fresh) == 0 {
		return 0, 0, nil
	}

	groups := map[string][]NormalizedSignal{}
	for _, fp := range fresh {
		sig := byFP[fp]
		key := ingest.Signature(sig.Labels, source.Spec.Grouping.SignatureLabels)
		groups[key] = append(groups[key], sig)
	}

	touched := 0
	for key, group := range groups {
		if err := s.routeSignalGroup(ctx, source, key, group, combine); err != nil {
			return 0, 0, err
		}
		touched++
	}

	patch := client.MergeFrom(source.DeepCopy())
	now := metav1.Now()
	source.Status.LastReceived = &now
	source.Status.ReceivedTotal += int64(len(fresh))
	_ = s.Client.Status().Patch(ctx, source, patch)
	return len(fresh), touched, nil
}

// routeSignalGroup lands one signature group as an input on the matching
// conversation (window reuse; created on demand).
func (s *Server) routeSignalGroup(ctx context.Context, source *agentopsv1alpha1.SignalSource, signature string, group []NormalizedSignal, combine combineFunc) error {
	windowDays := source.Spec.Grouping.WindowDays
	if windowDays <= 0 {
		windowDays = 7
	}
	var list agentopsv1alpha1.ConversationList
	if err := s.Reader.List(ctx, &list, client.InNamespace(s.Namespace),
		client.MatchingLabels{controller.LabelSignatureHash: ingest.SignatureHash(signature)}); err != nil {
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

	// input lane: base kind for new work, recurrence once a session exists
	inputType := agentopsv1alpha1.InputAlert
	jobName := ""
	if group[0].Kind == "job" {
		inputType = agentopsv1alpha1.InputJob
		jobName = source.Name
	}
	if conv == nil {
		title := ""
		for _, sig := range group {
			if sig.Title != "" {
				title = sig.Title
				break
			}
		}
		if title == "" {
			title = "🔍 " + source.Name
		}
		conv = &agentopsv1alpha1.Conversation{}
		conv.Namespace = s.Namespace
		conv.GenerateName = "alert-"
		if inputType == agentopsv1alpha1.InputJob {
			conv.GenerateName = "job-"
		}
		conv.Labels = map[string]string{controller.LabelSignatureHash: ingest.SignatureHash(signature)}
		conv.Spec = agentopsv1alpha1.ConversationSpec{
			ProfileRef: source.Spec.ProfileRef,
			Title:      title,
			Signature:  signature,
		}
		if source.Spec.ChannelRef != nil {
			conv.Spec.ChannelRef = source.Spec.ChannelRef
		}
		if err := s.Client.Create(ctx, conv); err != nil {
			return err
		}
	} else if conv.Status.SessionID != "" {
		inputType = agentopsv1alpha1.InputRecurrence // same problem/job, resume with context
	}

	ci := &agentopsv1alpha1.ConversationInput{}
	ci.Namespace = s.Namespace
	ci.GenerateName = conv.Name + "-in-"
	ci.Spec = agentopsv1alpha1.ConversationInputSpec{
		ConversationRef: agentopsv1alpha1.ObjectRef{Name: conv.Name},
		Type:            inputType,
		Payload:         combine(group),
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
			JobName:    jobName,
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

// ---- signal adapter contract (auth + endpoints) -----------------------------

// signalAuth guards /signal/* (constant-time): the master token has full
// scope; a per-SignalAdapter token — derived with the signal-specific context
// and validated by re-derivation against the SignalAdapter list (stateless,
// zero Secret reads) — is scoped to that adapter's spec.type. ChannelAdapter
// tokens validate against no SignalAdapter and get 401 here.
func (s *Server) signalAuth(next http.HandlerFunc) http.HandlerFunc {
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
		var adapters agentopsv1alpha1.SignalAdapterList
		if err := s.Client.List(r.Context(), &adapters, client.InNamespace(s.Namespace)); err == nil {
			for i := range adapters.Items {
				want := chat.DeriveSignalAdapterToken(s.AdapterToken, adapters.Items[i].Name)
				if subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1 {
					ctx := context.WithValue(r.Context(), adapterScopeKey{}, adapters.Items[i].Spec.Type)
					next(w, r.WithContext(ctx))
					return
				}
			}
		}
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
	}
}

type signalInboundReq struct {
	Source  string             `json:"source"`
	Signals []NormalizedSignal `json:"signals"`
}

func (s *Server) handleSignalInbound(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var in signalInboundReq
	if err := json.Unmarshal(body, &in); err != nil || in.Source == "" || len(in.Signals) == 0 {
		writeJSON(w, 400, map[string]string{"error": `need {"source","signals":[...]}`})
		return
	}
	for _, sig := range in.Signals {
		if sig.Fingerprint == "" {
			writeJSON(w, 400, map[string]string{"error": "every signal needs a fingerprint"})
			return
		}
	}
	source := s.signalSource(r, w, in.Source)
	if source == nil {
		return
	}
	queued, touched, err := s.routeSignals(r.Context(), source, in.Signals, combineJoined)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"queued": queued, "conversations": touched})
}

func (s *Server) handleSignalSources(w http.ResponseWriter, r *http.Request) {
	sourceType := r.URL.Query().Get("type")
	if sourceType == "" {
		writeJSON(w, 400, map[string]string{"error": "missing type"})
		return
	}
	if !scopeAllows(r, sourceType) {
		forbidScope(w)
		return
	}
	var list agentopsv1alpha1.SignalSourceList
	if err := s.Client.List(r.Context(), &list, client.InNamespace(s.Namespace)); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	type srcOut struct {
		Name   string          `json:"name"`
		Config json.RawMessage `json:"config,omitempty"`
		// CredentialEnvPrefix locates this source's projected credentials in
		// the adapter's own environment: Secret key K is env <prefix>K.
		CredentialEnvPrefix string `json:"credentialEnvPrefix,omitempty"`
	}
	out := []srcOut{}
	for i := range list.Items {
		src := &list.Items[i]
		if src.Spec.Type != sourceType {
			continue
		}
		o := srcOut{Name: src.Name}
		if src.Spec.Config != nil {
			o.Config = src.Spec.Config.Raw
		}
		if src.Spec.CredentialsSecretRef != nil {
			o.CredentialEnvPrefix = controller.CredentialEnvPrefix(src.Name)
		}
		out = append(out, o)
	}
	writeJSON(w, 200, out)
}

// signalSource resolves a SignalSource by name, enforcing the token scope.
func (s *Server) signalSource(r *http.Request, w http.ResponseWriter, name string) *agentopsv1alpha1.SignalSource {
	var src agentopsv1alpha1.SignalSource
	if err := s.Reader.Get(r.Context(), types.NamespacedName{Namespace: s.Namespace, Name: name}, &src); err != nil {
		writeJSON(w, 404, map[string]string{"error": fmt.Sprintf("unknown signal source %q", name)})
		return nil
	}
	if !scopeAllows(r, src.Spec.Type) {
		forbidScope(w)
		return nil
	}
	return &src
}

func (s *Server) handleSignalStateGet(w http.ResponseWriter, r *http.Request) {
	src := s.signalSource(r, w, r.PathValue("source"))
	if src == nil {
		return
	}
	writeJSON(w, 200, map[string]string{"value": src.Annotations[StateAnnotationPrefix+r.PathValue("key")]})
}

func (s *Server) handleSignalStatePut(w http.ResponseWriter, r *http.Request) {
	src := s.signalSource(r, w, r.PathValue("source"))
	if src == nil {
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
	patch := client.MergeFrom(src.DeepCopy())
	if src.Annotations == nil {
		src.Annotations = map[string]string{}
	}
	src.Annotations[StateAnnotationPrefix+r.PathValue("key")] = in.Value
	if err := s.Client.Patch(r.Context(), src, patch); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleSignalStatus(w http.ResponseWriter, r *http.Request) {
	src := s.signalSource(r, w, r.PathValue("name"))
	if src == nil {
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
	patch := client.MergeFrom(src.DeepCopy())
	apimeta.SetStatusCondition(&src.Status.Conditions, cond)
	if err := s.Client.Status().Patch(r.Context(), src, patch); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
