package signalroute

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/dispatch"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/namewords"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// The signal-kind vocabulary, duplicated from internal/httpapi's own
// KindChat/KindTask/KindJob/KindAlert — this package is imported BY
// internal/httpapi (D-3's shared routing path), so it cannot import back.
// SignalSpec.Kind's own comment names this the same closed vocabulary,
// unenforced by either's CRD schema.
const (
	kindChat  = "chat"
	kindTask  = "task"
	kindJob   = "job"
	kindAlert = "alert"
)

// oneShot mirrors internal/httpapi's own oneShot: a one-shot signal's
// payload IS the request, so it makes the title; a recurring-subject one
// (alert, job) keeps the source's name instead.
func oneShot(kind string) bool { return kind == kindChat || kind == kindTask }

// routeOneClaimant attempts one claimant's own routing workflow for sig:
// reuse an open conversation it already originated whose conversationId
// equals the signal's initiationId, or create one, then append the message.
// Returns waiting=true when the create was refused because the pending
// backlog is full — the caller leaves that claimant Routing (design D-13).
//
// Idempotent end to end: the lookup reads the API server, so a conversation a
// crashed pass created is found, and the input is written under a name
// derived from the Signal and claimant, so a repeated pass appends nothing
// twice (design D-5).
func routeOneClaimant(ctx context.Context, deps Deps, claimant chat.Claimant, sig *state.SignalRecord) (conversation string, waiting bool, err error) {
	conv, err := findReusableConversation(ctx, deps.Conversations, claimant, sig.InitiationID)
	if err != nil {
		return "", false, err
	}

	inputType := inputTypeForKind(sig.Kind)
	if conv == nil {
		full, err := BacklogFull(ctx, deps.Conversations, deps.MaxQueued)
		if err != nil {
			return "", false, err
		}
		if full {
			return "", true, nil
		}
		conv, err = createConversation(ctx, deps, claimant, sig)
		if err != nil {
			return "", false, err
		}
	} else if conv.ContextID() != "" {
		inputType = state.InputRecurrence // same problem/job, resume with context
	}

	if err := appendInput(ctx, deps, claimant, conv, sig, inputType); err != nil {
		return "", false, err
	}
	return conv.Name, false, nil
}

// messageOf is the text routed to the agent: the signal's payload verbatim,
// or — for an addressed chat message — the task after `/<name> `.
func messageOf(sig *state.SignalRecord) (text string, addressed bool) {
	if _, task, ok := AddressedTarget(sig); ok {
		return task, true
	}
	return sig.Payload, false
}

// findReusableConversation returns the OPEN conversation, originated by this
// claimant, whose conversationId equals initiationID — pure equality, served
// by the store's conversationId index. The store reads its source of truth
// directly (never a cache): a pass repeated after a crash between create and
// the status write must find the conversation it just made.
func findReusableConversation(ctx context.Context, conversations state.Collection[state.ConversationRecord], claimant chat.Claimant, initiationID string) (*state.ConversationRecord, error) {
	list, err := conversations.List(ctx, state.ByConversationID, initiationID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		conv := &list[i]
		if conv.Phase == state.PhaseClosed {
			continue
		}
		if reusableBy(conv, claimant) {
			return conv, nil
		}
	}
	return nil, nil
}

// reusableBy reports whether an existing conversation may absorb an input
// from this claimant — the provenance half of equality routing, on top of
// the id match the caller already made.
func reusableBy(c *state.ConversationRecord, claimant chat.Claimant) bool {
	if c.PipelineRef != "" {
		return claimant.ClaimantKind() == chat.ClaimantPipeline && c.PipelineRef == claimant.GetName()
	}
	if c.CoordinatorRef != "" {
		return claimant.ClaimantKind() == chat.ClaimantCoordinator && c.CoordinatorRef == claimant.GetName()
	}
	return false
}

// inputTypeForKind is the input lane for a freshly admitted signal — base
// kind for new work. Recurrence is layered on top by the caller once a
// session already exists.
func inputTypeForKind(kind string) string {
	switch kind {
	case kindJob:
		return state.InputJob
	case kindChat, kindTask:
		return state.InputTask
	default:
		return state.InputAlert
	}
}

// nameKindForSignal is the name word AllocateName's "<kind>-<words>" base
// uses — follows the signal's own Kind, not the input lane, so chat and
// task (which share the task lane) are still told apart at a glance in
// `kubectl get`.
func nameKindForSignal(kind string) string {
	switch kind {
	case kindJob:
		return "job"
	case kindChat:
		return "chat"
	case kindTask:
		return "task"
	default:
		return "alert"
	}
}

// titleForSignal picks the conversation title: the signal's own Title if it
// set one, else — for a one-shot kind, whose payload IS the request — a
// title derived from it, else the source's own name.
func titleForSignal(sourceName string, sig *state.SignalRecord) string {
	if sig.Title != "" {
		return sig.Title
	}
	if oneShot(sig.Kind) {
		icon := "💬"
		if sig.Kind == kindTask {
			icon = "🛠"
		}
		text, _ := messageOf(sig)
		if title := titleFromText(icon, text); title != "" {
			return title
		}
	}
	return "🔍 " + sourceName
}

// titleFromText renders a conversation title from the request itself,
// mirroring internal/httpapi's own helper of the same name/shape.
func titleFromText(icon, text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	return agentopsv1alpha1.BoundConversationTitle(icon + " " + strings.Join(fields, " "))
}

// wordsForSignal is the conversation NAME's word source, falling back to
// the source's own name — a required Kubernetes field, so always present —
// when the title itself carries no word namewords.Words can extract.
func wordsForSignal(sourceName, title string) string {
	if words := namewords.Words(title); words != "" {
		return words
	}
	return namewords.Words(sourceName)
}

// createConversation opens a new conversation adopting sig's initiationId
// as its own conversationId, resolving the claimant's capability and
// execution wiring ONCE. Returns nil, nil only when the backlog is full is
// checked by the caller — this function is never asked to create while
// full.
func createConversation(ctx context.Context, deps Deps, claimant chat.Claimant, sig *state.SignalRecord) (*state.ConversationRecord, error) {
	sourceName := sig.SourceRef

	title := titleForSignal(sourceName, sig)
	capability, err := dispatch.ResolveCapability(ctx, deps.Reader, claimant)
	if err != nil {
		return nil, fmt.Errorf("%s: resolve capability: %w", claimant.GetName(), err)
	}
	snap := runtimepod.SnapshotFor(ctx, deps.Reader, deps.Namespace, claimant.GetName(), capability, deps.Runtime)
	conv := &state.ConversationRecord{
		ProfileRef:         capability.ProfileName(),
		ChannelRefs:        append([]state.Ref{}, claimant.BoundChannelRefs()...),
		Toolsets:           dispatch.ToolsetBindingOf(capability.Toolsets),
		MCPConfigs:         dispatch.ToolingBindingOf(capability.MCPConfigs),
		RuntimeRef:         snap.RuntimeRef,
		ServiceAccountName: snap.ServiceAccountName,
		ContextClaimName:   snap.ContextClaimName,
		WorkspaceClaimName: snap.WorkspaceClaimName,
		Signal: &state.SignalProvenance{
			SourceRef: sourceName,
			Labels:    sig.Labels,
			Severity:  sig.Severity,
		},
		Title:          title,
		ConversationID: sig.InitiationID,
	}
	// An addressed conversation binds the addressing channel too, so the
	// answer comes back to where the person is looking.
	if _, addressed := messageOf(sig); addressed {
		conv.ChannelRefs = withChannel(conv.ChannelRefs, sig.Labels[agentopsv1alpha1.LabelChatChannel])
	}
	nodeKind := activity.NodePipeline
	if claimant.ClaimantKind() == chat.ClaimantCoordinator {
		nodeKind = activity.NodeCoordinator
		conv.CoordinatorRef = claimant.GetName()
		conv.EscalationChannelRefs = claimant.EscalationChannelRefs()
	} else {
		conv.PipelineRef = claimant.GetName()
	}
	// Only for a person on a chat surface: an alert has no reader, and a
	// machine posting a task is not owed a read mark.
	if sig.Kind == kindChat && sig.Reader != "" {
		if ch := sig.Labels[agentopsv1alpha1.LabelChatChannel]; ch != "" {
			conv.OriginReader = &state.OriginReader{Channel: ch, Key: sig.Reader}
		}
	}
	// A Coordinator-rooted conversation's own resource ceiling is STATUS, and
	// a record carries spec and status alike, so it rides the one Create.
	if budget := claimant.SnapshotBudget(); budget != nil {
		conv.Budget = budget
	}
	if _, err := namewords.AllocateName(ctx, deps.Conversations,
		nameKindForSignal(sig.Kind), wordsForSignal(sourceName, title), conv); err != nil {
		return nil, err
	}
	deps.Activity.Emit(activity.Event{
		Kind:     activity.KindConversationCreated,
		From:     activity.Node(nodeKind, claimant.GetName()),
		To:       activity.Node(activity.NodeConversation, conv.Name),
		Pipeline: claimant.GetName(), Conversation: conv.Name,
		Detail: conv.Title,
	})
	return conv, nil
}

// withChannel adds a channel ref unless it is already bound.
func withChannel(refs []state.Ref, name string) []state.Ref {
	if name == "" {
		return refs
	}
	for _, r := range refs {
		if r == name {
			return refs
		}
	}
	return append(refs, name)
}

// InputName is the name of the ConversationInput carrying sig's message to one
// claimant — derived deterministically from the Signal and the claimant, so a
// repeated pass finds the delivery it already made rather than making
// another. Pipeline and Coordinator share a namespace and may share a name,
// hence the kind letter.
func InputName(sigName string, ref state.SignalClaimant) string {
	kind := "p"
	if ref.Kind == state.ClaimantKindCoordinator {
		kind = "c"
	}
	return sigName + "-" + kind + "-" + ref.Name
}

// inputDelivered reports whether the conversation already carries the named
// input: queued in its inputs, or — once dispatch consumed it and pruned the
// queue — cited by a run's recorded inputs. The blob itself is deleted with
// its queue entry, so its absence proves nothing.
func inputDelivered(ctx context.Context, runs state.Appended[state.ConversationRecord, state.Run], conv *state.ConversationRecord, blobKey string) (bool, error) {
	if queuedInput(conv, blobKey) {
		return true, nil
	}
	recorded, err := runs.List(ctx, state.Key(conv.Name))
	if err != nil {
		return false, err
	}
	for r := range recorded {
		for i := range recorded[r].Inputs {
			if recorded[r].Inputs[i].PayloadRef == state.Key(blobKey) {
				return true, nil
			}
		}
	}
	return false, nil
}

// queuedInput reports whether the conversation's queue holds the named blob.
func queuedInput(conv *state.ConversationRecord, blobKey string) bool {
	for i := range conv.Inputs {
		if conv.Inputs[i].PayloadRef == state.Key(blobKey) {
			return true
		}
	}
	return false
}

// appendInput records sig's message as a blob under its derived name and
// queues it on the conversation, re-reading the record on a lost race
// (state.Mutate). A delivery that already happened (an earlier pass of the
// same Signal and claimant) is left alone.
func appendInput(ctx context.Context, deps Deps, claimant chat.Claimant, conv *state.ConversationRecord, sig *state.SignalRecord, inputType string) error {
	sourceName := sig.SourceRef
	ciName := InputName(sig.Name, claimantOf(claimant))
	text, _ := messageOf(sig)

	fresh, _, err := deps.Conversations.Get(ctx, state.Key(conv.Name))
	if err != nil {
		return err
	}
	delivered, err := inputDelivered(ctx, deps.Runs, &fresh, ciName)
	if err != nil || delivered {
		return err
	}
	// Written only now that the queue is known not to carry it, and an
	// existing blob under the name is the same delivery's earlier half.
	if err := deps.Blobs.Put(ctx, state.Key(ciName), state.Blob{
		ConversationName: conv.Name,
		Type:             inputType,
		Payload:          text,
		Labels:           sig.Labels,
		Severity:         sig.Severity,
	}); err != nil {
		return err
	}
	inputID := "in-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	queued := false
	if _, err := state.Mutate(ctx, deps.Conversations, state.Key(conv.Name), func(rec *state.ConversationRecord) error {
		queued = false
		if queuedInput(rec, ciName) {
			return nil
		}
		rec.Inputs = append(rec.Inputs, state.InputItem{
			ID:         inputID,
			Type:       inputType,
			PayloadRef: state.Key(ciName),
			Origin: &state.InputOrigin{
				Kind: state.OriginSignal, Name: sourceName,
				SignalKind: orDefault(sig.Kind, kindAlert),
			},
			ReceivedAt: time.Now(),
		})
		queued = true
		return nil
	}); err != nil {
		return fmt.Errorf("append input to %s: %w", conv.Name, err)
	}
	if !queued {
		return nil
	}
	deps.Activity.Emit(activity.Event{
		Kind:     activity.KindInputQueued,
		From:     activity.Node(activity.NodeSignalSource, sourceName),
		To:       activity.Node(activity.NodeConversation, conv.Name),
		Pipeline: claimant.GetName(), Conversation: conv.Name, InputID: inputID,
		Code: inputType, Detail: sourceName,
	})
	return nil
}

// orDefault fills an empty string with a fallback, mirroring
// internal/httpapi's own helper of the same name.
func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
