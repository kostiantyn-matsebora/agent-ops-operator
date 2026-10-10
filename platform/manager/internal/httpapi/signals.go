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
	"sort"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/addressing"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/dispatch"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/ingest"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/namewords"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
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
	// prompt), "job" (task-lane prompt for a recurring job), "task" (task lane,
	// a one-off ask from a machine), or "chat" (task lane, from a human on a
	// chat surface).
	Kind string `json:"kind,omitempty"`
	// Reader is the OPAQUE key of the person who sent this — a salted hash the
	// originating surface computed, never an identity. Its only use is stamping
	// their own read watermark when their thread is created, so a conversation
	// somebody just started is not shown back to them as unread.
	//
	// NOT a label, deliberately: labels feed signature grouping and are
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

// KindAlert is the default lane: read-only investigation prompt, grouped by the
// source's signature labels.
const KindAlert = "alert"

// oneShot reports whether a kind is a one-shot lane — a later signal is a
// SEPARATE request rather than more news about a standing subject. Chat and
// task are one-shot; alert and job are recurring-subject lanes that group and
// resume. This is the distinction the signature fallback keys on.
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
// updated). Wiring is pipeline-only: a source no Ready pipeline lists drops the
// batch with a reason BEFORE cooldown (so re-sent fingerprints route once one
// does). Returns fresh signals, conversations touched, and the drop reason ("" when routed).
//
// A source is SHAREABLE, so this FANS OUT: every Ready pipeline listing the
// source gets its own conversation for each group, with its own profile and
// capabilities. Per-source policy — cooldown and signature grouping — is
// deliberately evaluated ONCE, above the fan-out: a fingerprint is admitted
// once and then delivered to each server. Moving either inside the loop would
// let the first pipeline spend the cooldown and starve the rest.
func (s *Server) routeSignals(ctx context.Context, source *agentopsv1alpha1.SignalSource, signals []NormalizedSignal, combine combineFunc) (int, int, string, error) {
	servers := chat.PipelinesForSource(ctx, s.Client, s.Namespace, source.Name)
	if len(servers) == 0 {
		reason := "source not served by any Ready pipeline or Coordinator (Wired=False) — signals dropped"
		s.emitDropped(source, signals, activity.CodeUnclaimed, reason)
		return 0, 0, reason, nil
	}
	// The claim is a decision about the BATCH and it happens here, before any
	// conversation exists — so these events carry no conversation. That is not a
	// gap: the chain a consumer walks is adapter -> source -> claimant ->
	// conversation, and the next hop (conversation.created) is where a
	// conversation starts existing to be named. One event PER server, because a
	// shared source really is claimed by each of them — a Coordinator exactly
	// as a Pipeline (design D-B).
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
			Detail:   fmt.Sprintf("%d signal(s)", len(signals)),
		})
	}
	cd := s.cooldown(source, signals)
	var fps []string
	byFP := map[string]NormalizedSignal{}
	for _, sig := range signals {
		fps = append(fps, sig.Fingerprint)
		byFP[sig.Fingerprint] = sig
	}
	fresh := cd.Fresh(fps)
	if len(fresh) == 0 {
		// The high-volume case — a flapping alert re-delivered inside its
		// window — writes NOTHING. Only admitting a fingerprint moves the record.
		return 0, 0, "", nil
	}
	s.recordCooldown(ctx, source, cd)

	groups := map[string][]NormalizedSignal{}
	for _, fp := range fresh {
		sig := byFP[fp]
		key := ingest.Signature(sig.Labels, source.Spec.Grouping.SignatureLabels)
		if oneShot(sig.Kind) && len(source.Spec.Grouping.SignatureLabels) == 0 {
			// The fallback splits on what the LANE is about, not on whether
			// labels happen to be present:
			//
			//   alert / job  — recurring-subject lanes. The second signal is
			//                  more news about the same thing, so the default
			//                  alert labels (alertgroup/alertname/namespace)
			//                  fold it into the open conversation and resume the
			//                  session. the prometheus bundle ships `grouping: {}` and
			//                  depends on this; signal-cron fires a DISTINCT
			//                  fingerprint per tick and depends on the empty
			//                  signature collapsing them into one conversation.
			//   chat / task  — one-shot lanes. The second signal is a second
			//                  request, and the default labels are alert
			//                  vocabulary neither one carries, so every request
			//                  would hash to the same empty signature and pile
			//                  into a single conversation.
			//
			// Do NOT "simplify" this to a blanket per-fingerprint rule: that
			// regresses alert grouping and gives every cron tick its own
			// conversation. A source that DOES declare signatureLabels is
			// unaffected in every lane — asking for grouping gets it.
			key = sig.Fingerprint
		}
		groups[key] = append(groups[key], sig)
	}

	touched, landed, reason := 0, 0, ""
	for key, group := range groups {
		// One conversation per serving pipeline. `landed` counts SIGNALS, so it
		// moves once however many pipelines took the group; `touched` counts
		// CONVERSATIONS, so it moves per pipeline. Conflating them would report
		// a doubled intake on a shared source.
		landedHere := false
		for i := range servers {
			_, routed, err := s.routeSignalGroup(ctx, source, servers[i], key, group, combine, len(servers))
			if err != nil {
				return 0, 0, "", err
			}
			if !routed {
				// Capacity is decided per conversation, so one claimant can be
				// refused while another takes the same group. Naming the refused
				// claimant is the difference between "we are full" and "this lane
				// is full".
				reason = ReasonAtCapacity
				s.emitDropped(source, group, activity.CodeAtCapacity,
					ReasonAtCapacity+" (pipeline "+servers[i].GetName()+")")
				continue
			}
			touched++
			landedHere = true
		}
		if landedHere {
			landed += len(group)
		}
	}
	if reason != "" {
		log.FromContext(ctx).Info("signal batch declined: pending backlog is full",
			"source", source.Name, "maxQueuedConversations", s.maxQueued())
	}

	s.bumpReceived(ctx, source, landed)
	return landed, touched, reason, nil
}

// cooldown returns this source's suppression window, RECOVERED from the source
// on first use in this process. The in-memory map is the hot path; the CR is
// the record, so a restart mid-incident does not re-open conversations for
// signals still inside their window.
func (s *Server) cooldown(source *agentopsv1alpha1.SignalSource, signals []NormalizedSignal) *ingest.Cooldown {
	if cd := s.cooldowns[source.Name]; cd != nil {
		return cd
	}
	hours := source.Spec.Grouping.CooldownHours
	if hours <= 0 {
		hours = 6
		if isChat(signals) {
			// Chat defaults cooldown OFF. Fingerprint dedup exists so a flapping
			// alert opens one investigation; a person asking the same thing twice
			// means it twice, and swallowing the second ask would be a bug wearing
			// dedup's clothes.
			hours = 0
		}
	}
	recorded := make([]ingest.Entry, 0, len(source.Status.Cooldown))
	for _, e := range source.Status.Cooldown {
		recorded = append(recorded, ingest.Entry{Fingerprint: e.Fingerprint, At: e.At.Time})
	}
	cd := ingest.NewCooldownFrom(time.Duration(hours)*time.Hour, recorded)
	s.cooldowns[source.Name] = cd
	return cd
}

// recordCooldown writes the suppression window back to the source. Called only
// when a fingerprint was newly admitted — which already costs a conversation
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
			Fingerprint: e.Fingerprint, At: metav1.NewTime(e.At),
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

// ReasonAtCapacity is the drop reason for a batch refused because the pending
// backlog is full — the same channel every other drop reason travels: reported
// in the response for machine origins, spoken on the surface for chat.
const ReasonAtCapacity = "pending conversation backlog is full — signals dropped"

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

	queued, touched, reason, err := s.routeSignals(ctx, source, rest, combineJoined)
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
			s.Ops.EnqueueMessage(ctx, ch, nil, out)
		}
	}
}

// backlogFull reports whether the pending backlog has reached its bound. Only
// PENDING conversations count: an admitted one holds a pod (or is about to) and
// is already bounded by MAX_ACTIVE_CONVERSATIONS.
func (s *Server) backlogFull(ctx context.Context) (bool, error) {
	var list agentopsv1alpha1.ConversationList
	if err := s.Reader.List(ctx, &list, client.InNamespace(s.Namespace)); err != nil {
		return false, err
	}
	n := 0
	for i := range list.Items {
		if list.Items[i].Status.Phase == agentopsv1alpha1.ConversationPending {
			n++
		}
	}
	return n >= s.maxQueued(), nil
}

// routeSignalGroup lands one signature group as an input on the matching
// conversation (window reuse; created on demand with the serving pipeline's
// profile and channel set — the only wiring there is). Reports routed=false
// when the pending backlog is full and the group would have needed a NEW
// conversation — the bound gates creation only, so window reuse keeps appending
// to a pending conversation however full the backlog is. Returns the
// conversation the group landed on, so the caller can attribute the claim.
//
// serverCount is how many Ready pipelines serve the source; it exists only for
// the legacy-conversation rule in reusableBy.
func (s *Server) routeSignalGroup(ctx context.Context, source *agentopsv1alpha1.SignalSource, claimant chat.Claimant, signature string, group []NormalizedSignal, combine combineFunc, serverCount int) (string, bool, error) {
	conv, err := s.findReusableConversation(ctx, source, claimant, signature, serverCount)
	if err != nil {
		return "", false, err
	}

	// input lane: base kind for new work, recurrence once a session exists
	kind := group[0].Kind
	inputType := inputTypeForKind(kind)

	if conv == nil {
		conv, err = s.createConversationForGroup(ctx, source, claimant, signature, group, kind)
		if err != nil {
			return "", false, err
		}
		if conv == nil {
			return "", false, nil // backlog full
		}
	} else if conv.ContextID() != "" {
		inputType = agentopsv1alpha1.InputRecurrence // same problem/job, resume with context
	}

	return s.appendInputToConversation(ctx, source, claimant, conv, group, combine, inputType)
}

// findReusableConversation returns the open conversation this claimant may
// append a matching-signature group to under window reuse, or nil when a new
// one must be opened.
func (s *Server) findReusableConversation(ctx context.Context, source *agentopsv1alpha1.SignalSource, claimant chat.Claimant, signature string, serverCount int) (*agentopsv1alpha1.Conversation, error) {
	windowDays := source.Spec.Grouping.WindowDays
	if windowDays <= 0 {
		windowDays = 7
	}
	var list agentopsv1alpha1.ConversationList
	if err := s.Reader.List(ctx, &list, client.InNamespace(s.Namespace),
		client.MatchingLabels{controller.LabelSignatureHash: ingest.SignatureHash(signature)}); err != nil {
		return nil, err
	}
	cutoff := time.Now().Add(-time.Duration(windowDays) * 24 * time.Hour)
	for i := range list.Items {
		c := &list.Items[i]
		// A CLOSED conversation is not reusable, and this is the rule that
		// makes closing mean anything: without it a matching signature would
		// wake the conversation somebody just tidied away, re-materialising a
		// thread they archived. A match opens a NEW conversation instead.
		if c.Status.Phase == agentopsv1alpha1.ConversationClosed {
			continue
		}
		// The signature hash alone is NOT enough once a source is shared: two
		// claimants fanning out produce two conversations with the SAME
		// signature, and absorbing the other one's would run this group under
		// the wrong profile with the wrong tools.
		if !reusableBy(c, claimant, serverCount) {
			continue
		}
		last := c.CreationTimestamp.Time
		if c.Status.LastActivity != nil {
			last = c.Status.LastActivity.Time
		}
		if last.After(cutoff) {
			return c, nil
		}
	}
	return nil, nil
}

// inputTypeForKind is the input lane for a freshly admitted group — base kind
// for new work. Recurrence is layered on top by the caller once a session
// already exists, and is deliberately not decided here.
func inputTypeForKind(kind string) agentopsv1alpha1.InputType {
	switch kind {
	case KindJob:
		return agentopsv1alpha1.InputJob
	case KindChat, KindTask:
		// Task lane, and deliberately NOT the job lane: job carries
		// recurrence-on-session, which would make a second question — or a
		// second posted task — resume the first one's session as news about a
		// standing job.
		return agentopsv1alpha1.InputTask
	default:
		return agentopsv1alpha1.InputAlert
	}
}

// titleForGroup picks the conversation title: the first signal that named
// one, else — for a one-shot kind, whose payload IS the request — a title
// derived from it, else the source's own name.
func titleForGroup(source *agentopsv1alpha1.SignalSource, group []NormalizedSignal, kind string) string {
	for _, sig := range group {
		if sig.Title != "" {
			return sig.Title
		}
	}
	// A one-shot signal IS the request, so the request makes the title.
	// Falling back to the source name gave every conversation from one
	// surface the SAME name ("🔍 console", "🔍 home-ops"), which makes a list
	// of them unreadable and a search useless. An alert keeps the source
	// name: its payload is a machine document, and the source is the useful
	// label there.
	if oneShot(kind) {
		icon := "💬"
		if kind == KindTask {
			icon = "🛠"
		}
		if title := titleFromText(icon, group[0].Payload); title != "" {
			return title
		}
	}
	return "🔍 " + source.Name
}

// wordsForGroup is the conversation NAME's word source — independent of
// titleForGroup's own return value, which stays whatever is most readable
// as spec.Title even when it carries no word namewords.Words can extract
// (a non-ASCII question, a caption of only punctuation or emoji). Falling
// back to the SOURCE's own name — a required Kubernetes field, so always
// present — is what guarantees every creation path a non-empty word
// source, widened here to every kind rather than only where titleForGroup
// itself already falls back to it.
func wordsForGroup(source *agentopsv1alpha1.SignalSource, title string) string {
	if words := namewords.Words(title); words != "" {
		return words
	}
	return namewords.Words(source.Name)
}

// createConversationForGroup opens a new conversation for this signature,
// resolving the claimant's capability and execution wiring ONCE — whether
// this claimant inlines it or names it through capabilityRef makes no
// difference from here on. Returns nil, nil when the backlog is full: no
// conversation, and nothing wrong.
func (s *Server) createConversationForGroup(ctx context.Context, source *agentopsv1alpha1.SignalSource, claimant chat.Claimant, signature string, group []NormalizedSignal, kind string) (*agentopsv1alpha1.Conversation, error) {
	full, err := s.backlogFull(ctx)
	if err != nil {
		return nil, err
	}
	if full {
		return nil, nil
	}

	conv := &agentopsv1alpha1.Conversation{}
	conv.Namespace = s.Namespace
	// The name KIND word follows the KIND, not the input lane: chat and
	// task share the task lane but are told apart at a glance in
	// `kubectl get`. The word-chain itself, and the actual Create, are
	// AllocateName's — readable-conversation-names: an object name is a
	// deterministic word-chain, never metadata.generateName's random
	// suffix.
	nameKind := "alert"
	switch kind {
	case KindJob:
		nameKind = "job"
	case KindChat:
		nameKind = "chat"
	case KindTask:
		nameKind = "task"
	}
	title := titleForGroup(source, group, kind)
	conv.Labels = map[string]string{controller.LabelSignatureHash: ingest.SignatureHash(signature)}
	capability, err := dispatch.ResolveCapability(ctx, s.Reader, claimant)
	if err != nil {
		return nil, fmt.Errorf("%s: resolve capability: %w", claimant.GetName(), err)
	}
	// The execution wiring is materialized beside the tooling, through the
	// ONE helper both origination paths call. Freezing it is what stops a
	// later Pipeline or Coordinator edit changing the identity an inflight
	// conversation's next pod runs as.
	snap := runtimepod.SnapshotFor(ctx, s.Reader, s.Namespace, claimant.GetName(), capability, s.Runtime)
	conv.Spec = agentopsv1alpha1.ConversationSpec{
		ProfileRef:         agentopsv1alpha1.ObjectRef{Name: capability.ProfileName()},
		ChannelRefs:        append([]agentopsv1alpha1.ObjectRef{}, claimant.BoundChannelRefs()...),
		Toolsets:           capability.Toolsets.DeepCopy(),
		MCPConfigs:         capability.MCPConfigs.DeepCopy(),
		RuntimeRef:         snap.RuntimeRef,
		ServiceAccountName: snap.ServiceAccountName,
		// WHERE this conversation's two volumes are, resolved ONCE. A later
		// edit to the claimant's persistence moves only conversations
		// created after it.
		ContextClaimName:   snap.ContextClaimName,
		WorkspaceClaimName: snap.WorkspaceClaimName,
		// PROVENANCE, written once beside the claimant. Without it a
		// finished conversation cannot say what STARTED it: the source's
		// other copy is `spec.inputs[].origin`, which pruning empties, and
		// the labels' other copy is the ConversationInput, which is deleted
		// with the queue entry.
		Signal: &agentopsv1alpha1.SignalProvenance{
			SourceRef: &agentopsv1alpha1.ObjectRef{Name: source.Name},
			// The first signal's labels represent the group: they share a
			// signature, so they agree on everything grouping keyed off.
			Labels: boundedLabels(group[0].Labels),
		},
		Title:     title,
		Signature: signature,
	}
	// Provenance names EXACTLY one originating wiring object (design D-B):
	// a Pipeline names PipelineRef, a Coordinator names CoordinatorRef and
	// snapshots its OWN declared channels as EscalationChannelRefs too —
	// already bound into ChannelRefs above via BoundChannelRefs
	// (coordinator-unconditional-channels: a coordinator root is reachable
	// from creation, exactly like a Pipeline's).
	nodeKind := activity.NodePipeline
	if claimant.ClaimantKind() == chat.ClaimantCoordinator {
		nodeKind = activity.NodeCoordinator
		conv.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: claimant.GetName()}
		conv.Spec.EscalationChannelRefs = claimant.EscalationChannelRefs()
	} else {
		conv.Spec.PipelineRef = &agentopsv1alpha1.ObjectRef{Name: claimant.GetName()}
	}
	// Only for a person on a chat surface: an alert has no reader, and a
	// machine posting a task is not owed a read mark.
	if kind == KindChat && group[0].Reader != "" {
		if chatChannel := group[0].Labels[LabelChatChannel]; chatChannel != "" {
			conv.Spec.OriginReader = &agentopsv1alpha1.OriginReader{
				Channel: chatChannel, Key: group[0].Reader,
			}
		}
	}
	if _, err := namewords.AllocateName(ctx, s.Client, s.Namespace, nameKind, wordsForGroup(source, title), conv); err != nil {
		return nil, err
	}
	// A Coordinator-rooted conversation's own resource ceiling (design D-E)
	// is STATUS, so it needs its own write after Create — a status
	// subresource is never persisted by Create itself.
	if budget := claimant.SnapshotBudget(); budget != nil {
		patch := client.MergeFrom(conv.DeepCopy())
		conv.Status.Budget = budget
		if err := s.Client.Status().Patch(ctx, conv, patch); err != nil {
			return nil, err
		}
	}
	s.Activity.Emit(activity.Event{
		Kind:     activity.KindConversationCreated,
		From:     activity.Node(nodeKind, claimant.GetName()),
		To:       activity.Node(activity.NodeConversation, conv.Name),
		Pipeline: claimant.GetName(), Conversation: conv.Name,
		Detail: conv.Spec.Title,
	})
	return conv, nil
}

// appendInputToConversation records the group as a ConversationInput and
// queues it on the conversation's own input list, retrying the optimistic
// patch on conflict.
func (s *Server) appendInputToConversation(ctx context.Context, source *agentopsv1alpha1.SignalSource, claimant chat.Claimant, conv *agentopsv1alpha1.Conversation, group []NormalizedSignal, combine combineFunc, inputType agentopsv1alpha1.InputType) (string, bool, error) {
	kind := group[0].Kind
	ci := &agentopsv1alpha1.ConversationInput{}
	ci.Namespace = s.Namespace
	ci.GenerateName = conv.Name + "-in-"
	ci.Spec = agentopsv1alpha1.ConversationInputSpec{
		ConversationRef: agentopsv1alpha1.ObjectRef{Name: conv.Name},
		Type:            inputType,
		Payload:         combine(group),
		// The first signal's labels represent the group: they all share a
		// signature, so they agree on everything grouping keyed off.
		Labels: group[0].Labels,
	}
	if err := s.Client.Create(ctx, ci); err != nil {
		return "", false, err
	}

	// append the input item with optimistic retry
	for attempt := 0; attempt < 5; attempt++ {
		var fresh agentopsv1alpha1.Conversation
		if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: conv.Name}, &fresh); err != nil {
			return "", false, err
		}
		patch := client.MergeFrom(fresh.DeepCopy())
		inputID := "in-" + strconv.FormatInt(time.Now().UnixNano(), 36)
		fresh.Spec.Inputs = append(fresh.Spec.Inputs, agentopsv1alpha1.InputItem{
			ID:         inputID,
			Type:       inputType,
			PayloadRef: &agentopsv1alpha1.ObjectRef{Name: ci.Name},
			// Provenance for EVERY kind, not just jobs: the source is what a card
			// names, and the signal kind is what keeps a chat message from being
			// posted back at the person who typed it.
			Origin: &agentopsv1alpha1.InputOrigin{
				Kind: agentopsv1alpha1.OriginSignal, Name: source.Name,
				SignalKind: orDefault(kind, KindAlert),
			},
			ReceivedAt: metav1.Now(),
		})
		if err := s.Client.Patch(ctx, &fresh, patch); err != nil {
			if apierrors.IsConflict(err) {
				continue
			}
			return "", false, err
		}
		s.Activity.Emit(activity.Event{
			Kind:     activity.KindInputQueued,
			From:     activity.Node(activity.NodeSignalSource, source.Name),
			To:       activity.Node(activity.NodeConversation, conv.Name),
			Pipeline: claimant.GetName(), Conversation: conv.Name, InputID: inputID,
			Code: string(inputType), Detail: source.Name,
		})
		return conv.Name, true, nil
	}
	return "", false, fmt.Errorf("conflict appending input to %s", conv.Name)
}

// reusableBy reports whether an existing conversation may absorb a group from
// this claimant — the provenance half of window reuse, on top of the signature
// match the caller already made.
//
// A conversation carrying a pipelineRef is reusable by that Pipeline alone;
// one carrying a coordinatorRef by that Coordinator alone — the two refs are
// mutually exclusive, so a Pipeline claimant never absorbs a Coordinator's
// conversation or the reverse.
//
// One with NEITHER ref predates both fields, and no timestamp can tell which
// claimant made it. Reusing it whenever the signature matched would hand it to
// whichever claimant happened to reconcile first once a source is shared — the
// invisible pick this change exists to delete. Refusing it outright would
// instead re-open every open investigation on upgrade. So it stays reusable
// exactly while ONE claimant serves the source, which IS the state every such
// conversation was created in; the moment a second joins, it is left alone and
// each claimant opens its own. Nothing backfills either ref — inference is
// what it replaces.
func reusableBy(c *agentopsv1alpha1.Conversation, claimant chat.Claimant, serverCount int) bool {
	if c.Spec.PipelineRef != nil {
		return claimant.ClaimantKind() == chat.ClaimantPipeline && c.Spec.PipelineRef.Name == claimant.GetName()
	}
	if c.Spec.CoordinatorRef != nil {
		return claimant.ClaimantKind() == chat.ClaimantCoordinator && c.Spec.CoordinatorRef.Name == claimant.GetName()
	}
	return serverCount == 1
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
			Detail: reason + " (" + sig.Fingerprint + ")",
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
		// A chat signal names the surface it came from, or it is unanswerable.
		// Refuse it here rather than accept it and silently drop the reply.
		if sig.Kind == KindChat && sig.Labels[LabelChatChannel] == "" {
			writeJSON(w, 400, map[string]string{"error": "a chat signal must carry the label " +
				LabelChatChannel + " naming the Channel it arrived on — the reply has nowhere to go without it"})
			return
		}
	}
	source := s.signalSource(r, w, in.Source)
	if source == nil {
		return
	}
	// Receipt is recorded before any routing decision: the hop that happened is
	// "this adapter handed the manager a signal", and it is true whether the
	// signal is later claimed, deduped by cooldown, or dropped. It carries no
	// conversation because none has been decided yet — the fingerprint in
	// `detail` is what correlates it with the claim that follows.
	for _, sig := range in.Signals {
		s.Activity.Emit(activity.Event{
			Kind:   activity.KindSignalReceived,
			From:   activity.Node(activity.NodeSignalAdapter, source.Spec.Adapter),
			To:     activity.Node(activity.NodeSignalSource, source.Name),
			Code:   orDefault(sig.Kind, "alert"),
			Detail: sig.Fingerprint,
		})
	}
	route := s.routeSignals
	if isChat(in.Signals) {
		route = func(ctx context.Context, src *agentopsv1alpha1.SignalSource, sigs []NormalizedSignal, _ combineFunc) (int, int, string, error) {
			return s.routeChatSignals(ctx, src, sigs)
		}
	}
	queued, touched, reason, err := route(r.Context(), source, in.Signals, combineJoined)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
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
