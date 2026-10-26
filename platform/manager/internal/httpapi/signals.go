// Normalized-signal routing: the single core every signal source feeds —
// the built-in Alertmanager webhook and external signal adapters alike.
// Adapters normalize and mint the correlation id; the manager applies
// nothing but equality — initiationId cooldown, then id-equality routing
// against an open conversation's own conversationId. "Are these signals one
// problem?" was decided sender-side, wherever its own aggregation lives.
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
	"time"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/addressing"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/ingest"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/signalroute"
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

// oneShot reports whether a kind is a one-shot lane — a later signal is a
// SEPARATE request rather than more news about a standing subject. Chat and
// task are one-shot; alert and job are recurring-subject lanes that group and
// resume. Used only to pick a title's icon now — routing itself no longer
// branches on it: a one-shot sender mints a fresh initiationId per request,
// and a recurring-subject sender reuses its own, so equality routing alone
// already produces this split.
func oneShot(kind string) bool { return kind == KindChat || kind == KindTask }

// Reserved labels a chat signal carries. LabelChatChannel is what lets the
// manager answer on the surface the message came from — without it a chat
// signal is unanswerable, so /signal/inbound refuses it rather than accept one
// whose reply would go nowhere.
const (
	LabelChatChannel = "agentops.dev/channel"
	LabelChatSender  = "agentops.dev/sender"
	// LabelChatMessage is the transport's OWN handle for the message that
	// arrived, set by the adapter that normalized it. OPAQUE here and
	// everywhere else in the manager: it is stored and handed back unaltered,
	// never parsed, compared or constructed — the same treatment a thread id
	// gets.
	//
	// It is what lets a reply say WHICH message it answers, and therefore what
	// lets an adapter offer an action on somebody's own words without holding
	// state to remember them. Optional: an adapter that does not set it loses
	// the linkage and nothing else.
	LabelChatMessage = "agentops.dev/message"
)

// titleFromText renders a conversation title from the request itself:
// whitespace collapsed. Empty for empty input, so the caller keeps its own
// fallback. The icon says which lane asked — 💬 somebody typed it, 🛠 a
// machine posted it.
//
// Bounded by agentopsv1alpha1.MaxConversationTitle — NOT the transport-shaped
// limit the "no manager-side cut" rule warns against (see that constant's own
// comment). Telegram's own 128-rune topic-name cap is a separate, later bound
// at the adapter; this one exists because a one-shot signal's payload has no
// cap at all before it reaches an etcd-permanent field.
func titleFromText(icon, text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	return agentopsv1alpha1.BoundConversationTitle(icon + " " + strings.Join(fields, " "))
}

// orDefault fills an empty string with a fallback (telemetry labelling only).
func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// isChat reports whether a batch is chat input.
func isChat(signals []NormalizedSignal) bool {
	for _, sig := range signals {
		if sig.Kind == KindChat {
			return true
		}
	}
	return false
}

// combineFunc renders one input payload for a group of fresh signals sharing
// one initiationId (callers control multi-signal payload shape).
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

// routeSignals applies pure id-equality routing to a batch of normalized
// signals — the MACHINE lane (chatLane false): a claimant whose create the
// pending backlog refuses stays Routing rather than being dropped (design
// D-8, conversation-capacity's modified "the pending backlog is bounded").
func (s *Server) routeSignals(ctx context.Context, source *agentopsv1alpha1.SignalSource, signals []NormalizedSignal, combine combineFunc) (int, int, string, error) {
	return s.routeSignalsCore(ctx, source, signals, combine, false)
}

// routeSignalsCore is the one place a batch becomes Signal CRs and is routed
// through signalroute.RouteClaimants (design D-2/D-3: "persist, then route
// inline, then respond" — one routing path serves ingest and the
// reconciler). Shared by the machine lane (routeSignals) and the chat
// lane's own unambiguous, single-claimant tail (routeChatSignals).
//
// It persists (or absorbs) a Signal CR for every initiationId group BEFORE
// any routing decision, then makes the pre-fan-out decision — unclaimed,
// cooldown, and (chatLane only) capacity — exactly ONCE per FRESH signal.
// An absorbed signal (one folded into an already non-terminal Signal) never
// repeats that decision: it already passed its own first attempt (design
// D-5a: "Dropped stays strictly pre-fan-out").
//
// A source is SHAREABLE, so routing FANS OUT: every Ready pipeline listing
// the source gets its own conversation per id, with its own profile and
// capabilities. Per-source cooldown is deliberately evaluated ONCE, above
// the fan-out: an id is admitted once and then delivered to each server.
func (s *Server) routeSignalsCore(ctx context.Context, source *agentopsv1alpha1.SignalSource, signals []NormalizedSignal, combine combineFunc, chatLane bool) (int, int, string, error) {
	servers := chat.PipelinesForSource(ctx, s.Client, s.Namespace, source.Name)
	unclaimed := len(servers) == 0
	if !unclaimed {
		s.emitClaims(source, servers, len(signals))
	}

	groups, ids := groupByInitiationID(signals)

	// Persist or absorb EVERY group first — the CR exists before any
	// decision is made about it (design D-2).
	type persisted struct {
		sig      *agentopsv1alpha1.Signal
		absorbed bool
	}
	byID := make(map[string]persisted, len(ids))
	var freshIDs []string
	for _, id := range ids {
		sig, absorbed, err := s.persistOrAbsorbSignal(ctx, source, id, groups[id], combine)
		if err != nil {
			return 0, 0, "", err
		}
		byID[id] = persisted{sig, absorbed}
		if !absorbed {
			freshIDs = append(freshIDs, id)
		}
	}

	// Cooldown is consulted ONLY for FRESH signals — an absorbed one already
	// passed this decision at its own creation.
	cooldownPassed := map[string]bool{}
	if !unclaimed && len(freshIDs) > 0 {
		cooldownPassed = s.freshAfterCooldown(ctx, source, signals, freshIDs)
	}

	landed, touched, reason := 0, 0, ""
	for _, id := range ids {
		p := byID[id]
		group := groups[id]

		if !p.absorbed {
			dropped, responseReason, err := s.decidePreFanOut(ctx, source, p.sig, group, unclaimed, cooldownPassed[id], chatLane)
			if err != nil {
				return 0, 0, "", err
			}
			if responseReason != "" {
				reason = responseReason
			}
			if dropped {
				continue
			}
		}

		before := claimantStates(p.sig)
		if err := signalroute.RouteClaimants(ctx, s.signalRouteDeps(), p.sig); err != nil {
			return 0, 0, "", err
		}
		if newly := countNewlyRouted(before, p.sig); newly > 0 {
			touched += newly
			landed += len(group)
		}
	}

	s.bumpReceived(ctx, source, landed)
	return landed, touched, reason, nil
}

// emitClaims records the claim — a decision about the BATCH, before any
// Signal's own routing runs — one event PER server, a Coordinator exactly as
// a Pipeline (design D-B).
func (s *Server) emitClaims(source *agentopsv1alpha1.SignalSource, servers []chat.Claimant, n int) {
	for i := range servers {
		nodeKind := activity.NodePipeline
		if servers[i].ClaimantKind() == chat.ClaimantCoordinator {
			nodeKind = activity.NodeCoordinator
		}
		s.Activity.Emit(activity.Event{
			Kind:     activity.KindSignalClaimed,
			From:     activity.Node(activity.NodeSignalSource, source.Name),
			To:       activity.Node(nodeKind, servers[i].GetName()),
			Pipeline: servers[i].GetName(),
			Detail:   fmt.Sprintf("%d signal(s)", n),
		})
	}
}

// freshAfterCooldown consults the per-source cooldown for the fresh ids and
// returns the set that passed it, recording the cooldown when any did.
func (s *Server) freshAfterCooldown(ctx context.Context, source *agentopsv1alpha1.SignalSource, signals []NormalizedSignal, freshIDs []string) map[string]bool {
	cd := s.cooldown(source, signals)
	passed := cd.Fresh(freshIDs)
	if len(passed) > 0 {
		s.recordCooldown(ctx, source, cd)
	}
	out := make(map[string]bool, len(passed))
	for _, id := range passed {
		out[id] = true
	}
	return out
}

// decidePreFanOut makes the pre-fan-out decision for one FRESH signal —
// unclaimed, cooldown, and (chatLane only) capacity — recording a drop
// straight to Dropped. The caller never calls this for an ABSORBED signal:
// one already passed its own first attempt and the decision is never
// re-made (design D-5a: "Dropped stays strictly pre-fan-out"). Returns
// whether the signal was dropped, and the batch-response reason to surface
// (empty for a silent cooldown suppression, which has never surfaced one).
func (s *Server) decidePreFanOut(ctx context.Context, source *agentopsv1alpha1.SignalSource, sig *agentopsv1alpha1.Signal, group []NormalizedSignal, unclaimed, cooldownOK, chatLane bool) (dropped bool, responseReason string, err error) {
	if unclaimed {
		responseReason = "source not served by any Ready pipeline or Coordinator (Wired=False) — signals dropped"
		s.markSignalDropped(ctx, sig, agentopsv1alpha1.SignalDropUnclaimed)
		s.emitDropped(source, group, activity.CodeUnclaimed, responseReason)
		return true, responseReason, nil
	}
	if !cooldownOK {
		// The high-volume case — a flapping alert re-delivered inside its
		// window — is now RECORDED rather than silent (design: "Cooldown
		// suppression is recorded, never silent"). No reason surfaces in the
		// response — this never has.
		s.markSignalDropped(ctx, sig, agentopsv1alpha1.SignalDropCooldown)
		return true, "", nil
	}
	if !chatLane {
		return false, "", nil
	}
	full, err := signalroute.BacklogFull(ctx, s.Reader, s.Namespace, s.maxQueued())
	if err != nil {
		return false, "", err
	}
	if !full {
		return false, "", nil
	}
	// THE ONE PLACE the backlog-full decision is made BEFORE any claimant
	// exists — chat keeps its immediate decline (design D-8); a machine
	// signal (chatLane false) instead reaches signalroute.RouteClaimants,
	// which leaves a blocked claimant Routing rather than dropping it.
	log.FromContext(ctx).Info("signal batch declined: pending backlog is full",
		"source", source.Name, "maxQueuedConversations", s.maxQueued())
	s.markSignalDropped(ctx, sig, agentopsv1alpha1.SignalDropCapacity)
	s.emitDropped(source, group, activity.CodeAtCapacity, ReasonAtCapacity)
	return true, ReasonAtCapacity, nil
}

// groupByInitiationID groups a batch by the raw id — several signals in one
// batch sharing one initiationId are one sender-side notification and
// combine into one Signal CR, exactly as several occurrences of one id
// across separate calls land on the same open one (design D-5). ids is
// sorted for a deterministic persist/route order.
func groupByInitiationID(signals []NormalizedSignal) (map[string][]NormalizedSignal, []string) {
	groups := map[string][]NormalizedSignal{}
	for _, sig := range signals {
		groups[sig.InitiationID] = append(groups[sig.InitiationID], sig)
	}
	ids := make([]string, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return groups, ids
}

// persistOrAbsorbSignal creates the Signal CR for one initiationId group of
// a batch, owned by source — or, when a still-open (non-terminal) Signal of
// the same source already carries that initiationId, absorbs the group
// into it instead (design D-5): payload appended, occurrences bumped, no
// second CR. A match that is itself TERMINAL is never reopened (design
// D-5a) — this creates a fresh CR beside it, the terminal one left an
// untouched audit record. Returns the Signal and whether it was absorbed
// (vs. freshly created).
func (s *Server) persistOrAbsorbSignal(ctx context.Context, source *agentopsv1alpha1.SignalSource, id string, group []NormalizedSignal, combine combineFunc) (*agentopsv1alpha1.Signal, bool, error) {
	key := agentopsv1alpha1.SignalInitiationIndexKey(source.Name, id)
	var list agentopsv1alpha1.SignalList
	if err := s.Client.List(ctx, &list, client.InNamespace(s.Namespace),
		client.MatchingFields{agentopsv1alpha1.IndexSignalInitiationID: key}); err != nil {
		return nil, false, fmt.Errorf("list signals for absorb: %w", err)
	}
	for i := range list.Items {
		sig := &list.Items[i]
		if isTerminalSignalPhase(sig.Status.Phase) {
			continue
		}
		patch := client.MergeFrom(sig.DeepCopy())
		sig.Spec.Payload = appendPayload(sig.Spec.Payload, group, combine)
		sig.Spec.Occurrences++
		if err := s.Client.Patch(ctx, sig, patch); err != nil {
			return nil, false, fmt.Errorf("absorb signal %s: %w", sig.Name, err)
		}
		return sig, true, nil
	}

	sig := &agentopsv1alpha1.Signal{}
	sig.Namespace = s.Namespace
	sig.GenerateName = "sig-"
	sig.Spec = agentopsv1alpha1.SignalSpec{
		SourceRef:    agentopsv1alpha1.ObjectRef{Name: source.Name},
		Adapter:      source.Spec.Adapter,
		InitiationID: id,
		Kind:         group[0].Kind,
		Title:        group[0].Title,
		Severity:     group[0].Severity,
		Labels:       boundedLabels(group[0].Labels),
		Reader:       group[0].Reader,
		Payload:      combine(group),
		Occurrences:  1,
	}
	if err := controllerutil.SetControllerReference(source, sig, s.Client.Scheme()); err != nil {
		return nil, false, fmt.Errorf("signal owner ref: %w", err)
	}
	// Persist-then-ack (design D-2): a failed Create must never be reported
	// as accepted — the caller propagates this error straight to a 500, so
	// the adapter's at-least-once re-send repeats the same initiationId
	// safely rather than believing a signal landed that never did.
	if err := s.Client.Create(ctx, sig); err != nil {
		return nil, false, fmt.Errorf("create signal: %w", err)
	}
	return sig, false, nil
}

// appendPayload joins an absorbed Signal's existing payload with the newly
// arrived group's own combined payload — the same join combine already
// applies within one batch, now applied across occurrences over the
// Signal's open lifetime (SignalSpec.Payload's own doc comment).
func appendPayload(existing string, group []NormalizedSignal, combine combineFunc) string {
	added := combine(group)
	switch {
	case existing == "":
		return added
	case added == "":
		return existing
	default:
		return existing + "\n---\n" + added
	}
}

// isTerminalSignalPhase reports whether a Signal's phase is one absorb must
// never reopen (design D-5a).
func isTerminalSignalPhase(p agentopsv1alpha1.SignalPhase) bool {
	switch p {
	case agentopsv1alpha1.SignalRouted, agentopsv1alpha1.SignalDropped, agentopsv1alpha1.SignalFailed:
		return true
	default:
		return false
	}
}

// markSignalDropped records a Signal's pre-fan-out drop (design D-5a: a
// Dropped Signal never gets a claimant snapshot at all). Best-effort: a
// failed status write leaves the Signal Pending rather than Dropped, which
// a later call (this same batch retried, or — once it exists — the
// reconciler) will simply re-decide.
func (s *Server) markSignalDropped(ctx context.Context, sig *agentopsv1alpha1.Signal, reason agentopsv1alpha1.SignalDropReason) {
	patch := client.MergeFrom(sig.DeepCopy())
	sig.Status.Phase = agentopsv1alpha1.SignalDropped
	sig.Status.Reason = reason
	if err := s.Client.Status().Patch(ctx, sig, patch); err != nil {
		log.FromContext(ctx).Info("marking signal dropped failed",
			"signal", sig.Name, "reason", reason, "error", err.Error())
	}
}

// claimantStates snapshots each claimant's CURRENT state, so
// countNewlyRouted can tell a claimant THIS call routed from one an earlier
// call on the same Signal already finished.
func claimantStates(sig *agentopsv1alpha1.Signal) []agentopsv1alpha1.ClaimantState {
	out := make([]agentopsv1alpha1.ClaimantState, len(sig.Status.Claimants))
	for i := range sig.Status.Claimants {
		out[i] = sig.Status.Claimants[i].State
	}
	return out
}

// countNewlyRouted counts the claimants THIS signalroute.RouteClaimants call
// moved into Routed — the response's own "conversations touched" count must
// never recount a claimant an earlier call on this same Signal already
// routed.
func countNewlyRouted(before []agentopsv1alpha1.ClaimantState, sig *agentopsv1alpha1.Signal) int {
	n := 0
	for i := range sig.Status.Claimants {
		wasRouting := i >= len(before) || before[i] == agentopsv1alpha1.ClaimantRouting
		if wasRouting && sig.Status.Claimants[i].State == agentopsv1alpha1.ClaimantRouted {
			n++
		}
	}
	return n
}

// signalRouteDeps bundles the Server's own dependencies into the shape
// signalroute.RouteClaimants needs — see that package's own Deps doc.
func (s *Server) signalRouteDeps() signalroute.Deps {
	return signalroute.Deps{
		Client: s.Client, Reader: s.Reader, Namespace: s.Namespace,
		Activity: s.Activity, Runtime: s.Runtime, MaxQueued: s.maxQueued(),
	}
}

// cooldown returns this source's suppression window, RECOVERED from the source
// on first use in this process. The in-memory map is the hot path; the CR is
// the record, so a restart mid-incident does not re-open conversations for
// signals still inside their window.
func (s *Server) cooldown(source *agentopsv1alpha1.SignalSource, signals []NormalizedSignal) *ingest.Cooldown {
	if cd := s.cooldowns[source.Name]; cd != nil {
		return cd
	}
	hours := source.Spec.CooldownHours
	if hours <= 0 {
		hours = 6
		if isChat(signals) {
			// Chat defaults cooldown OFF. Id dedup exists so a flapping
			// alert opens one investigation; a person asking the same thing twice
			// means it twice, and swallowing the second ask would be a bug wearing
			// dedup's clothes.
			hours = 0
		}
	}
	recorded := make([]ingest.Entry, 0, len(source.Status.Cooldown))
	for _, e := range source.Status.Cooldown {
		recorded = append(recorded, ingest.Entry{InitiationID: e.InitiationID, At: e.At.Time})
	}
	cd := ingest.NewCooldownFrom(time.Duration(hours)*time.Hour, recorded)
	s.cooldowns[source.Name] = cd
	return cd
}

// recordCooldown writes the suppression window back to the source. Called only
// when an id was newly admitted — which already costs a conversation
// create or patch, so this adds no write to any path that was previously free.
// Entries past the window are pruned and the list is bounded by the write.
func (s *Server) recordCooldown(ctx context.Context, source *agentopsv1alpha1.SignalSource, cd *ingest.Cooldown) {
	entries := cd.Entries()
	if entries == nil {
		return // window disabled — nothing to recover
	}
	out := make([]agentopsv1alpha1.CooldownEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, agentopsv1alpha1.CooldownEntry{
			InitiationID: e.InitiationID, At: metav1.NewTime(e.At),
		})
	}
	patch := client.MergeFrom(source.DeepCopy())
	source.Status.Cooldown = out
	if err := s.Client.Status().Patch(ctx, source, patch); err != nil {
		// Suppression still holds in memory; only the recovery copy is stale.
		// Failing the batch over it would trade a duplicate investigation for a
		// dropped signal, which is the worse of the two.
		log.FromContext(ctx).Info("recording cooldown on the signal source failed; "+
			"suppression holds in memory but will not survive a restart",
			"source", source.Name, "error", err.Error())
	}
}

// ReasonAtCapacity is the drop reason for a CHAT batch declined because the
// pending backlog is full — spoken on the surface the message came from
// (design D-8). A machine-kind batch never reports this: it waits as a
// Pending Signal instead of being dropped (conversation-capacity's MODIFIED
// "the pending backlog is bounded").
const ReasonAtCapacity = "pending conversation backlog is full — signals dropped"

// ReasonFiltered is the drop reason reported on the batch response whenever
// any signal failed the source's attributeFilters — the same "overwrite is
// acceptable, single-string summary" convention ReasonAtCapacity already
// uses.
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

// bumpReceived records ingest on the source (the single place lastReceived /
// receivedTotal move).
func (s *Server) bumpReceived(ctx context.Context, source *agentopsv1alpha1.SignalSource, n int) {
	if n == 0 {
		return
	}
	patch := client.MergeFrom(source.DeepCopy())
	now := metav1.Now()
	source.Status.LastReceived = &now
	source.Status.ReceivedTotal += int64(n)
	_ = s.Client.Status().Patch(ctx, source, patch)
}

// ---- chat lane --------------------------------------------------------------

// routeChatSignals handles input from a person on a chat surface.
//
// Two things make it different from an alert or a job. First, some chat input
// is a COMMAND whose whole result is a reply — a listing, an unknown agent, a
// usage error — and answering it by opening a Conversation would leave a
// stub conversation behind for every typo. Those emit a send op and nothing
// else. Second, when nothing is wired the user is owed an answer: an alert
// dropping silently is a condition for the operator to find, but a person who
// just typed into a chat is waiting.
//
// Everything else goes down the ordinary signal path, so chat gets the same
// claim check, window reuse and observability as every other source.
func (s *Server) routeChatSignals(ctx context.Context, source *agentopsv1alpha1.SignalSource, signals []NormalizedSignal) (int, int, string, error) {
	servers := chat.PipelinesForSource(ctx, s.Client, s.Namespace, source.Name)
	if len(servers) == 0 {
		reason := "source not served by any Ready pipeline or Coordinator (Wired=False) — signals dropped"
		s.emitDropped(source, signals, activity.CodeUnclaimed, reason)
		s.tellOriginatingSurfaces(ctx, signals, chat.Warn(fmt.Sprintf(
			"⚠️ Nothing here is wired to answer. No Ready Pipeline or Coordinator serves the chat source **%s**, "+
				"so this message was dropped. Add it to one's sources to give it one.", source.Name)))
		return 0, 0, reason, nil
	}

	answered := 0
	var rest []NormalizedSignal
	for _, sig := range signals {
		ch := s.chatChannel(ctx, sig)
		if ch == nil {
			continue // unknown channel: nowhere to answer, nothing to create
		}
		cmd, ok := addressing.Parse(strings.TrimSpace(sig.Payload))
		if !ok {
			rest = append(rest, sig)
			continue
		}
		// Addressed input: /agents and friends answer in place;
		// /<pipeline> <task> still opens a conversation, on the pipeline it
		// names rather than the one claiming the source. sig.Reader rides
		// along exactly as it does for the bare chat lane below — an
		// addressed command is equally a person originating a conversation.
		if err := s.Router.HandleCommand(ctx, ch, cmd,
			sig.Labels[LabelChatSender], sig.Labels[LabelChatMessage], sig.Reader); err != nil {
			return 0, 0, "", err
		}
		answered++
	}
	s.bumpReceived(ctx, source, answered)

	if len(rest) == 0 {
		return answered, 0, "", nil
	}

	// THE ONE PLACE CHAT DOES NOT FAN OUT. Every other lane opens a conversation
	// on each serving pipeline, which is right when the consumer is a system:
	// two investigations of one alert are two useful artifacts. Here the
	// consumer is a person who asked one question on one surface and is owed ONE
	// answer — and, unlike an alert, they can say which agent they meant. So an
	// unaddressed message routes only while the answer is unambiguous, and
	// otherwise is refused with the choices.
	//
	// The lane is told apart by the ARRIVING SIGNAL's kind, which ingest already
	// holds. Nothing on the SignalSource or the SignalAdapter declares "this is
	// a chat source", and nothing should: a reconciler would then need a handle
	// that every adapter author and installer could get wrong, to serve one
	// branch that lives here.
	if len(servers) > 1 {
		reason := "several Ready pipelines or coordinators serve this chat source — the message names none of them"
		s.emitDropped(source, rest, activity.CodeAmbiguous, reason)
		s.tellOriginatingSurfaces(ctx, rest, ambiguousChatMessage(servers))
		return answered, 0, reason, nil
	}

	queued, touched, reason, err := s.routeSignalsCore(ctx, source, rest, combineJoined, true)
	if reason == ReasonAtCapacity {
		// A person is waiting on the surface they typed on; an alert can be
		// found later in a condition, a question cannot.
		s.tellOriginatingSurfaces(ctx, rest, chat.Warn("⚠️ At capacity — too many conversations are already "+
			"waiting for a runtime slot, so this message was dropped. Try again once the backlog clears."))
	}
	return queued + answered, touched, reason, err
}

// ambiguousChatMessage renders the refusal a bare message gets on a surface
// several pipelines serve.
//
// It is the TEACHING MOMENT, not an error string: somebody who has never heard
// of the listing command finds out the addressed form exists at the one moment
// they need it, so it names every server with the profile answering for it and
// shows the form ready to copy. Prose in the manager's markdown subset — the
// adapter renders it; nothing here knows a transport dialect.
//
// It also OFFERS each Pipeline as a choice. This is the moment that earns
// controls most: the person has already typed their task, so a surface with
// controls can send THAT text to the Pipeline they pick rather than making them
// type it again. A surface without controls renders the same list, and the
// prose above is why it loses nothing.
func ambiguousChatMessage(servers []chat.Claimant) chat.Message {
	var b strings.Builder
	fmt.Fprintf(&b, "🤔 **%d pipelines/coordinators serve this chat, so I don't know who you meant.**\n\n", len(servers))
	b.WriteString("Address one of them:\n")
	choices := make([]chat.Choice, 0, len(servers))
	for i := range servers {
		fmt.Fprintf(&b, "• `/%s <task>` — %s\n", servers[i].GetName(), servers[i].InlineCapability().ProfileName())
		choices = append(choices, chat.Choice{
			Label:   servers[i].GetName(),
			Command: "/" + servers[i].GetName(),
		})
	}
	fmt.Fprintf(&b, "\nYour message was not sent to anyone. `/%s` shows this list any time.", chat.ListCommand)
	msg := chat.Warn(b.String())
	msg.Choices = choices
	return msg
}

// chatChannel resolves the Channel a chat signal came from.
func (s *Server) chatChannel(ctx context.Context, sig NormalizedSignal) *agentopsv1alpha1.Channel {
	name := sig.Labels[LabelChatChannel]
	if name == "" {
		return nil
	}
	var ch agentopsv1alpha1.Channel
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: name}, &ch); err != nil {
		return nil
	}
	return &ch
}

// tellOriginatingSurfaces posts one message to each distinct chat surface a
// batch came from, so a drop is visible where the user is looking.
func (s *Server) tellOriginatingSurfaces(ctx context.Context, signals []NormalizedSignal, msg chat.Message) {
	// A drop is something the reader has to act on, so it goes out as a WARNING
	// notice rather than plain text — the adapter decides how loud that looks.
	//
	// The reply is linked to the message that provoked it when the adapter
	// supplied a handle for one. That linkage is what carries the original
	// forward: a person selecting an offered action is answering their OWN
	// message, and the transport already knows which one it was.
	told := map[string]bool{}
	for _, sig := range signals {
		name := sig.Labels[LabelChatChannel]
		if name == "" || told[name] {
			continue
		}
		told[name] = true
		if ch := s.chatChannel(ctx, sig); ch != nil {
			out := msg
			out.InReplyTo = sig.Labels[LabelChatMessage]
			s.Ops.EnqueueMessage(ctx, ch, nil, nil, out)
		}
	}
}

// emitDropped records one signal.dropped per signal a batch lost, carrying the
// reason. A drop is the event an operator most needs and the one a status-derived
// graph can never show: nothing changed, so there is nothing to infer from.
func (s *Server) emitDropped(source *agentopsv1alpha1.SignalSource, signals []NormalizedSignal, code, reason string) {
	for _, sig := range signals {
		s.Activity.Emit(activity.Event{
			Kind:   activity.KindSignalDropped,
			From:   activity.Node(activity.NodeSignalSource, source.Name),
			Status: activity.StatusError,
			Code:   code,
			Detail: reason + " (" + sig.InitiationID + ")",
		})
	}
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

// validateSignals returns the refusal for the first invalid signal, or "".
func validateSignals(sigs []NormalizedSignal) string {
	for _, sig := range sigs {
		if sig.InitiationID == "" {
			return "every signal needs an initiationId"
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

func (s *Server) handleSignalInbound(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	var in signalInboundReq
	if err := json.Unmarshal(body, &in); err != nil || in.Source == "" || len(in.Signals) == 0 {
		writeJSON(w, 400, map[string]string{"error": `need {"source","signals":[...]}`})
		return
	}
	if msg := validateSignals(in.Signals); msg != "" {
		writeJSON(w, 400, map[string]string{"error": msg})
		return
	}
	source := s.signalSource(r, w, in.Source)
	if source == nil {
		return
	}
	// Receipt is recorded before any routing decision: the hop that happened is
	// "this adapter handed the manager a signal", and it is true whether the
	// signal is later claimed, deduped by cooldown, or dropped. It carries no
	// conversation because none has been decided yet — the initiationId in
	// `detail` is what correlates it with the claim that follows.
	for _, sig := range in.Signals {
		s.Activity.Emit(activity.Event{
			Kind:   activity.KindSignalReceived,
			From:   activity.Node(activity.NodeSignalAdapter, source.Spec.Adapter),
			To:     activity.Node(activity.NodeSignalSource, source.Name),
			Code:   orDefault(sig.Kind, "alert"),
			Detail: sig.InitiationID,
		})
	}
	admitted, filteredOut := partitionByAttributeFilters(source, in.Signals)
	if len(filteredOut) > 0 && logFiltered(source) {
		s.emitDropped(source, filteredOut, activity.CodeFiltered, ReasonFiltered)
	}

	route := s.routeSignals
	if isChat(admitted) {
		route = func(ctx context.Context, src *agentopsv1alpha1.SignalSource, sigs []NormalizedSignal, _ combineFunc) (int, int, string, error) {
			return s.routeChatSignals(ctx, src, sigs)
		}
	}
	var queued, touched int
	var reason string
	var err error
	if len(admitted) > 0 {
		queued, touched, reason, err = route(r.Context(), source, admitted, combineJoined)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
	}
	if reason == "" && len(filteredOut) > 0 {
		reason = ReasonFiltered
	}
	out := map[string]any{"queued": queued, "conversations": touched}
	if reason != "" {
		out["reason"] = reason
	}
	writeJSON(w, 200, out)
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
	sort.Strings(keys)
	if len(keys) > agentopsv1alpha1.MaxSignalLabels {
		keys = keys[:agentopsv1alpha1.MaxSignalLabels]
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = in[k]
	}
	return out
}
