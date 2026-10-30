// Normalized-signal PUBLICATION: the single door every signal source feeds —
// external signal adapters over POST /signal/inbound. The endpoint validates,
// applies the source's attributeFilters, stores one Signal CR per admitted
// signal and acknowledges (publish.go). It decides nothing else: routing is
// the consumer's alone (internal/controller's SignalReconciler), which is what
// lets consumption stop while publication keeps storing.
package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/ingest"
)

// NormalizedSignal is one signal in the contract's normalized shape.
type NormalizedSignal struct {
	// InitiationID is the sender-minted correlation id, REQUIRED on every
	// signal — the manager rejects one that omits it rather than synthesize
	// or default it. Cooldown keys on it, and equality routing matches it
	// against an open conversation's own spec.conversationId: the SAME id,
	// minted where the sender's own aggregation already happens, or
	// propagated verbatim when a round trip hands back a manager-issued
	// conversationId. Replaces the retired Fingerprint.
	InitiationID string `json:"initiationId"`
	// SignalID is the sender's OPTIONAL id for this one MESSAGE, stable across
	// every re-send of it. It is not the InitiationID, which correlates many
	// messages on purpose. When present it names the stored Signal, so a
	// re-send is deduplicated without a lookup (publish.go).
	SignalID string `json:"signalId,omitempty"`
	// Labels are attribution only now — stored as SignalProvenance and
	// rendered on signal cards. They feed no grouping decision: the manager
	// groups purely by InitiationID.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// Title overrides the conversation title when this signal opens one.
	Title string `json:"title,omitempty"`
	// Payload is the raw signal content handed to the agent (stored out of
	// line as a ConversationInput).
	Payload string `json:"payload,omitempty"`
	// Kind selects the input lane: "alert" (default; read-only investigation
	// prompt), "job" (task-lane prompt for a recurring job), "task" (task lane,
	// a one-off ask from a machine), or "chat" (task lane, from a human on a
	// chat surface).
	Kind string `json:"kind,omitempty"`
	// Severity is the signal's own optional urgency rating — one of exactly
	// the four values validSeverities accepts. Absent means unrated, the only
	// state a signal carried before this field existed. Validated ahead of any
	// routing decision (handleSignalInbound), then threaded onto the
	// conversation's SignalProvenance (write-once, at creation only) and onto
	// each created ConversationInputSpec (per occurrence) — the same two
	// stops, and the same write-once rule, Labels already follows.
	// +optional
	Severity agentopsv1alpha1.Severity `json:"severity,omitempty"`
	// Reader is the OPAQUE key of the person who sent this — a salted hash the
	// originating surface computed, never an identity. Its only use is stamping
	// their own read watermark when their thread is created, so a conversation
	// somebody just started is not shown back to them as unread.
	//
	// NOT a label, deliberately: labels are attribution only now and are
	// rendered on signal cards, and a per-person value has no business in
	// either.
	// +optional
	Reader string `json:"reader,omitempty"`
}

// KindChat marks a signal that is a person talking on a chat surface rather
// than a machine reporting. It takes the task lane like a job, but NOT job's
// recurrence-on-session semantics — a second question is a second
// conversation, not a resumption of the first.
const KindChat = "chat"

// KindTask is a one-off ask posted by a machine — the programmatic origination
// path, and the only one there is now that no endpoint names a Pipeline.
//
// It differs from "job" in both halves of the job lane: it carries NO jobName
// (a one-off ask is not a standing job named for its source) and it does not
// resume a session as a recurrence (a second post is a second request). It
// differs from "chat" in exactly one way: no channel label is required, because
// replies go to the claiming Pipeline's channelRefs rather than back to the
// surface somebody typed on.
const KindTask = "task"

// KindJob is the recurring-job lane: task-style prompt, jobName set from the
// source, and recurrence-on-session so later ticks resume the agent.
const KindJob = "job"

// KindAlert is the default lane: read-only investigation prompt, grouped by
// the sender's own initiationId.
const KindAlert = "alert"

// validSeverities is the closed vocabulary POST /signal/inbound accepts for a
// signal's optional Severity — named once so the acceptance check and the 400
// response naming the accepted values can never drift apart.
var validSeverities = map[agentopsv1alpha1.Severity]bool{
	agentopsv1alpha1.SeverityCritical: true,
	agentopsv1alpha1.SeverityError:    true,
	agentopsv1alpha1.SeverityWarning:  true,
	agentopsv1alpha1.SeverityInfo:     true,
}

// severityVocabulary names the four accepted values, verbatim, in the 400 a
// signal's unmapped severity gets.
const severityVocabulary = "critical, error, warning, info"

// Reserved labels a chat signal carries. LabelChatChannel is what lets the
// manager answer on the surface the message came from — without it a chat
// signal is unanswerable, so /signal/inbound refuses it rather than accept one
// whose reply would go nowhere.
const (
	LabelChatChannel = agentopsv1alpha1.LabelChatChannel
	LabelChatSender  = agentopsv1alpha1.LabelChatSender
	// LabelChatMessage is the transport's OWN handle for the message that
	// arrived — opaque, optional, stored and handed back unaltered.
	LabelChatMessage = agentopsv1alpha1.LabelChatMessage
)

// ReasonFiltered is the drop reason reported on the batch response whenever
// any signal failed the source's attributeFilters — decided at publication,
// before anything is stored, and the only routing-flavoured reason the
// response still carries.
const ReasonFiltered = "signal failed the source's attributeFilters — signal(s) dropped"

// partitionByAttributeFilters splits signals into admitted/filteredOut
// against source.Spec.AttributeFilters, evaluated against each signal's own
// Labels. An empty or absent filter list admits everything, unchanged from
// today — ingest.Admits already returns true for a nil/empty requirement
// list, so that guarantee holds without a special case here.
func partitionByAttributeFilters(source *agentopsv1alpha1.SignalSource, signals []NormalizedSignal) (admitted, filteredOut []NormalizedSignal) {
	if len(source.Spec.AttributeFilters) == 0 {
		return signals, nil
	}
	reqs := make([]ingest.FilterRequirement, 0, len(source.Spec.AttributeFilters))
	for _, f := range source.Spec.AttributeFilters {
		reqs = append(reqs, ingest.FilterRequirement{Key: f.Key, Operator: string(f.Operator), Values: f.Values})
	}
	// Compiled ONCE for the whole batch — every signal shares this source's
	// one requirement list, so building the selector per signal would redo
	// the same conversion and validation work for every signal in the batch.
	filter := ingest.Compile(reqs)
	for _, sig := range signals {
		if filter.Admits(sig.Labels) {
			admitted = append(admitted, sig)
		} else {
			filteredOut = append(filteredOut, sig)
		}
	}
	return admitted, filteredOut
}

// logFiltered reports whether a filtered-out signal still gets a
// signal.dropped record — true (the default) unless the source explicitly
// sets LogFiltered to false. The only drop code this field gates; every
// other emitDropped call site logs unconditionally.
func logFiltered(source *agentopsv1alpha1.SignalSource) bool {
	return source.Spec.LogFiltered == nil || *source.Spec.LogFiltered
}

// ---- signal adapter contract (auth + endpoints) -----------------------------

// signalAuth guards /signal/* (constant-time): the master token has full
// scope; a per-SignalAdapter token — derived with the signal-specific context
// and validated by re-derivation against the SignalAdapter list (stateless,
// zero Secret reads) — is scoped to that adapter's name. ChannelAdapter
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
					ctx := context.WithValue(r.Context(), adapterScopeKey{}, adapters.Items[i].Name)
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

// maxSignalIDBytes bounds a signalId. It is opaque to the manager — compared
// for equality, never parsed — so the bound and the character class exist only
// to keep what is hashed into a name sane.
const maxSignalIDBytes = 256

// validSignalID reports whether id is 1..maxSignalIDBytes of printable ASCII
// (0x21-0x7E), so without spaces or control characters.
func validSignalID(id string) bool {
	if len(id) == 0 || len(id) > maxSignalIDBytes {
		return false
	}
	for i := 0; i < len(id); i++ {
		if id[i] < 0x21 || id[i] > 0x7e {
			return false
		}
	}
	return true
}

// validateSignals returns the refusal for the first invalid signal, or "".
func validateSignals(sigs []NormalizedSignal) string {
	for _, sig := range sigs {
		if sig.InitiationID == "" {
			return "every signal needs an initiationId"
		}
		if sig.SignalID != "" && !validSignalID(sig.SignalID) {
			return fmt.Sprintf("signalId must be 1 to %d bytes of printable ASCII without spaces", maxSignalIDBytes)
		}
		// Absent is unrated and always accepted; present and unmapped refuses
		// the WHOLE batch (signal-severity: "one bad signal refuses the whole
		// batch"), before any signal in it is routed, claimed or recorded.
		if sig.Severity != "" && !validSeverities[sig.Severity] {
			return fmt.Sprintf("severity %q is not one of the accepted values: %s", sig.Severity, severityVocabulary)
		}
		// A chat signal names the surface it came from, or it is unanswerable.
		// Refuse it here rather than accept it and silently drop the reply.
		if sig.Kind == KindChat && sig.Labels[LabelChatChannel] == "" {
			return "a chat signal must carry the label " + LabelChatChannel +
				" naming the Channel it arrived on — the reply has nowhere to go without it"
		}
	}
	return ""
}

func (s *Server) handleSignalSources(w http.ResponseWriter, r *http.Request) {
	sourceType, ok := adapterParam(w, r)
	if !ok {
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
		if src.Spec.Adapter != sourceType {
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
	if !scopeAllows(r, src.Spec.Adapter) {
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

// boundedLabels copies a signal's labels for durable storage, capped.
//
// Sorted before truncation, so which labels survive is DETERMINISTIC — a
// conversation that kept a different subset on each write would make its own
// card unstable, and two adapters sending the same map would disagree.
func boundedLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	// The reserved agentops.dev/* labels sort first, so a signal carrying many
	// labels can never lose the one naming its originating surface — the
	// consumer needs it to answer.
	sort.Slice(keys, func(i, j int) bool {
		ri, rj := strings.HasPrefix(keys[i], reservedLabelPrefix), strings.HasPrefix(keys[j], reservedLabelPrefix)
		if ri != rj {
			return ri
		}
		return keys[i] < keys[j]
	})
	if len(keys) > agentopsv1alpha1.MaxSignalLabels {
		keys = keys[:agentopsv1alpha1.MaxSignalLabels]
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = in[k]
	}
	return out
}

// reservedLabelPrefix marks the labels the manager itself reads.
const reservedLabelPrefix = "agentops.dev/"
