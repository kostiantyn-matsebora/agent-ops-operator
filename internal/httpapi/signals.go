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
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/addressing"
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
	// prompt), "job" (task-lane prompt for a recurring job), "task" (task lane,
	// a one-off ask from a machine), or "chat" (task lane, from a human on a
	// chat surface).
	Kind string `json:"kind,omitempty"`
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
)

// titleFromText renders a conversation title from the request itself:
// whitespace collapsed, bounded to fit a chat topic name and a table column.
// Empty for empty input, so the caller keeps its own fallback. The icon says
// which lane asked — 💬 somebody typed it, 🛠 a machine posted it.
func titleFromText(icon, text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	title := icon + " " + strings.Join(fields, " ")
	// Rune-safe: a byte slice would cut a multi-byte character in half, and chat
	// input is exactly where non-ASCII shows up.
	if runes := []rune(title); len(runes) > 60 {
		title = strings.TrimSpace(string(runes[:59])) + "…"
	}
	return title
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
// updated). Wiring is pipeline-only: an unwired source drops the batch with a
// reason BEFORE cooldown (so re-sent fingerprints route once a pipeline
// claims the source). Returns fresh signals, conversations touched, and the
// drop reason ("" when routed).
func (s *Server) routeSignals(ctx context.Context, source *agentopsv1alpha1.SignalSource, signals []NormalizedSignal, combine combineFunc) (int, int, string, error) {
	pipeline := chat.PipelineForSource(ctx, s.Client, s.Namespace, source.Name)
	if pipeline == nil {
		reason := "source not claimed by a Ready pipeline (Wired=False) — signals dropped"
		s.emitDropped(source, signals, activity.CodeUnclaimed, reason)
		return 0, 0, reason, nil
	}
	// The claim is a decision about the BATCH and it happens here, before any
	// conversation exists — so this event carries no conversation. That is not a
	// gap: the chain a consumer walks is adapter -> source -> pipeline ->
	// conversation, and the next hop (conversation.created) is where a
	// conversation starts existing to be named.
	s.Activity.Emit(activity.Event{
		Kind:     activity.KindSignalClaimed,
		From:     activity.Node(activity.NodeSignalSource, source.Name),
		To:       activity.Node(activity.NodePipeline, pipeline.Name),
		Pipeline: pipeline.Name,
		Detail:   fmt.Sprintf("%d signal(s)", len(signals)),
	})
	cd := s.cooldowns[source.Name]
	if cd == nil {
		hours := source.Spec.Grouping.CooldownHours
		if hours <= 0 {
			hours = 6
			if isChat(signals) {
				// Chat defaults cooldown OFF. Fingerprint dedup exists so a
				// flapping alert opens one investigation; a person asking the
				// same thing twice means it twice, and swallowing the second
				// ask would be a bug wearing dedup's clothes.
				hours = 0
			}
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
		return 0, 0, "", nil
	}

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
			//                  session. vm-bundle ships `grouping: {}` and
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
		_, routed, err := s.routeSignalGroup(ctx, source, pipeline, key, group, combine)
		if err != nil {
			return 0, 0, "", err
		}
		if !routed {
			reason = ReasonAtCapacity
			s.emitDropped(source, group, activity.CodeAtCapacity, ReasonAtCapacity)
			continue
		}
		landed += len(group)
		touched++
	}
	if reason != "" {
		log.FromContext(ctx).Info("signal batch declined: pending backlog is full",
			"source", source.Name, "maxQueuedConversations", s.maxQueued())
	}

	s.bumpReceived(ctx, source, landed)
	return landed, touched, reason, nil
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
	if pipeline := chat.PipelineForSource(ctx, s.Client, s.Namespace, source.Name); pipeline == nil {
		reason := "source not claimed by a Ready pipeline (Wired=False) — signals dropped"
		s.emitDropped(source, signals, activity.CodeUnclaimed, reason)
		s.tellOriginatingSurfaces(ctx, signals, fmt.Sprintf(
			"⚠️ Nothing here is wired to answer. No Ready Pipeline claims the chat source <b>%s</b>, "+
				"so this message was dropped. Add it to a Pipeline's sources to give it an agent.", source.Name))
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
		// names rather than the one claiming the source.
		if err := s.Router.HandleCommand(ctx, ch, cmd); err != nil {
			return 0, 0, "", err
		}
		answered++
	}
	s.bumpReceived(ctx, source, answered)

	if len(rest) == 0 {
		return answered, 0, "", nil
	}
	queued, touched, reason, err := s.routeSignals(ctx, source, rest, combineJoined)
	if reason == ReasonAtCapacity {
		// A person is waiting on the surface they typed on; an alert can be
		// found later in a condition, a question cannot.
		s.tellOriginatingSurfaces(ctx, rest, "⚠️ At capacity — too many conversations are already waiting for "+
			"an agent slot, so this message was dropped. Try again once the backlog clears.")
	}
	return queued + answered, touched, reason, err
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
func (s *Server) tellOriginatingSurfaces(ctx context.Context, signals []NormalizedSignal, text string) {
	told := map[string]bool{}
	for _, sig := range signals {
		name := sig.Labels[LabelChatChannel]
		if name == "" || told[name] {
			continue
		}
		told[name] = true
		if ch := s.chatChannel(ctx, sig); ch != nil {
			s.Ops.EnqueueSend(ctx, ch, nil, text)
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
// conversation (window reuse; created on demand with the claiming pipeline's
// profile and channel set — the only wiring there is). Reports routed=false
// when the pending backlog is full and the group would have needed a NEW
// conversation — the bound gates creation only, so window reuse keeps appending
// to a pending conversation however full the backlog is. Returns the
// conversation the group landed on, so the caller can attribute the claim.
func (s *Server) routeSignalGroup(ctx context.Context, source *agentopsv1alpha1.SignalSource, pipeline *agentopsv1alpha1.Pipeline, signature string, group []NormalizedSignal, combine combineFunc) (string, bool, error) {
	windowDays := source.Spec.Grouping.WindowDays
	if windowDays <= 0 {
		windowDays = 7
	}
	var list agentopsv1alpha1.ConversationList
	if err := s.Reader.List(ctx, &list, client.InNamespace(s.Namespace),
		client.MatchingLabels{controller.LabelSignatureHash: ingest.SignatureHash(signature)}); err != nil {
		return "", false, err
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
	kind := group[0].Kind
	inputType := agentopsv1alpha1.InputAlert
	jobName := ""
	switch kind {
	case KindJob:
		inputType = agentopsv1alpha1.InputJob
		jobName = source.Name
	case KindChat, KindTask:
		// Task lane, and deliberately NOT the job lane: job carries
		// recurrence-on-session and a jobName, which would make a second
		// question — or a second posted task — resume the first one's session
		// as news about a standing job.
		inputType = agentopsv1alpha1.InputTask
	}
	if conv == nil {
		full, err := s.backlogFull(ctx)
		if err != nil {
			return "", false, err
		}
		if full {
			return "", false, nil
		}
		title := ""
		for _, sig := range group {
			if sig.Title != "" {
				title = sig.Title
				break
			}
		}
		if title == "" {
			// A one-shot signal IS the request, so the request makes the title.
			// Falling back to the source name gave every conversation from one
			// surface the SAME name ("🔍 console", "🔍 home-ops"), which makes a
			// list of them unreadable and a search useless. An alert keeps the
			// source name: its payload is a machine document, and the source is
			// the useful label there.
			if oneShot(kind) {
				icon := "💬"
				if kind == KindTask {
					icon = "🛠"
				}
				title = titleFromText(icon, group[0].Payload)
			}
			if title == "" {
				title = "🔍 " + source.Name
			}
		}
		conv = &agentopsv1alpha1.Conversation{}
		conv.Namespace = s.Namespace
		// The name prefix follows the KIND, not the input lane: chat and task
		// share the task lane but are told apart at a glance in `kubectl get`.
		conv.GenerateName = "alert-"
		switch kind {
		case KindJob:
			conv.GenerateName = "job-"
		case KindChat:
			conv.GenerateName = "chat-"
		case KindTask:
			conv.GenerateName = "task-"
		}
		conv.Labels = map[string]string{controller.LabelSignatureHash: ingest.SignatureHash(signature)}
		conv.Spec = agentopsv1alpha1.ConversationSpec{
			ProfileRef:  pipeline.Spec.ProfileRef,
			ChannelRefs: append([]agentopsv1alpha1.ObjectRef{}, pipeline.Spec.ChannelRefs...),
			Toolsets:    pipeline.Spec.Toolsets.DeepCopy(),
			MCPConfigs:  pipeline.Spec.MCPConfigs.DeepCopy(),
			Title:       title,
			Signature:   signature,
		}
		if err := s.Client.Create(ctx, conv); err != nil {
			return "", false, err
		}
		s.Activity.Emit(activity.Event{
			Kind:     activity.KindConversationCreated,
			From:     activity.Node(activity.NodePipeline, pipeline.Name),
			To:       activity.Node(activity.NodeConversation, conv.Name),
			Pipeline: pipeline.Name, Conversation: conv.Name,
			Detail: conv.Spec.Title,
		})
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
			JobName:    jobName,
			PayloadRef: &agentopsv1alpha1.ObjectRef{Name: ci.Name},
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
			Pipeline: pipeline.Name, Conversation: conv.Name, InputID: inputID,
			Code: string(inputType), Detail: source.Name,
		})
		return conv.Name, true, nil
	}
	return "", false, fmt.Errorf("conflict appending input to %s", conv.Name)
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
