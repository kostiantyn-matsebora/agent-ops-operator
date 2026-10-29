package signalroute

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/dispatch"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/namewords"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
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
func routeOneClaimant(ctx context.Context, deps Deps, claimant chat.Claimant, sig *agentopsv1alpha1.Signal) (*agentopsv1alpha1.ObjectRef, bool, error) {
	conv, err := findReusableConversation(ctx, deps.Reader, deps.Namespace, claimant, sig.Spec.InitiationID)
	if err != nil {
		return nil, false, err
	}

	inputType := inputTypeForKind(sig.Spec.Kind)
	if conv == nil {
		full, err := BacklogFull(ctx, deps.Reader, deps.Namespace, deps.MaxQueued)
		if err != nil {
			return nil, false, err
		}
		if full {
			return nil, true, nil
		}
		conv, err = createConversation(ctx, deps, claimant, sig)
		if err != nil {
			return nil, false, err
		}
	} else if conv.ContextID() != "" {
		inputType = agentopsv1alpha1.InputRecurrence // same problem/job, resume with context
	}

	if err := appendInput(ctx, deps, claimant, conv, sig, inputType); err != nil {
		return nil, false, err
	}
	return &agentopsv1alpha1.ObjectRef{Name: conv.Name}, false, nil
}

// messageOf is the text routed to the agent: the signal's payload verbatim,
// or — for an addressed chat message — the task after `/<name> `.
func messageOf(sig *agentopsv1alpha1.Signal) (text string, addressed bool) {
	if _, task, ok := AddressedTarget(sig); ok {
		return task, true
	}
	return sig.Spec.Payload, false
}

// findReusableConversation returns the OPEN conversation, originated by this
// claimant, whose conversationId equals initiationID — pure equality. It reads
// the API server directly (never an informer cache, and so no field index): a
// pass repeated after a crash between create and the status write must find
// the conversation it just made.
func findReusableConversation(ctx context.Context, c client.Reader, namespace string, claimant chat.Claimant, initiationID string) (*agentopsv1alpha1.Conversation, error) {
	var list agentopsv1alpha1.ConversationList
	if err := c.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	for i := range list.Items {
		conv := &list.Items[i]
		if conv.Spec.ConversationID != initiationID || conv.Status.Phase == agentopsv1alpha1.ConversationClosed {
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
func reusableBy(c *agentopsv1alpha1.Conversation, claimant chat.Claimant) bool {
	if c.Spec.PipelineRef != nil {
		return claimant.ClaimantKind() == chat.ClaimantPipeline && c.Spec.PipelineRef.Name == claimant.GetName()
	}
	if c.Spec.CoordinatorRef != nil {
		return claimant.ClaimantKind() == chat.ClaimantCoordinator && c.Spec.CoordinatorRef.Name == claimant.GetName()
	}
	return false
}

// inputTypeForKind is the input lane for a freshly admitted signal — base
// kind for new work. Recurrence is layered on top by the caller once a
// session already exists.
func inputTypeForKind(kind string) agentopsv1alpha1.InputType {
	switch kind {
	case kindJob:
		return agentopsv1alpha1.InputJob
	case kindChat, kindTask:
		return agentopsv1alpha1.InputTask
	default:
		return agentopsv1alpha1.InputAlert
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
func titleForSignal(sourceName string, sig *agentopsv1alpha1.Signal) string {
	if sig.Spec.Title != "" {
		return sig.Spec.Title
	}
	if oneShot(sig.Spec.Kind) {
		icon := "💬"
		if sig.Spec.Kind == kindTask {
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
func createConversation(ctx context.Context, deps Deps, claimant chat.Claimant, sig *agentopsv1alpha1.Signal) (*agentopsv1alpha1.Conversation, error) {
	sourceName := sig.Spec.SourceRef.Name
	conv := &agentopsv1alpha1.Conversation{}
	conv.Namespace = deps.Namespace

	title := titleForSignal(sourceName, sig)
	capability, err := dispatch.ResolveCapability(ctx, deps.Reader, claimant)
	if err != nil {
		return nil, fmt.Errorf("%s: resolve capability: %w", claimant.GetName(), err)
	}
	snap := runtimepod.SnapshotFor(ctx, deps.Reader, deps.Namespace, claimant.GetName(), capability, deps.Runtime)
	conv.Spec = agentopsv1alpha1.ConversationSpec{
		ProfileRef:         agentopsv1alpha1.ObjectRef{Name: capability.ProfileName()},
		ChannelRefs:        append([]agentopsv1alpha1.ObjectRef{}, claimant.BoundChannelRefs()...),
		Toolsets:           capability.Toolsets.DeepCopy(),
		MCPConfigs:         capability.MCPConfigs.DeepCopy(),
		RuntimeRef:         snap.RuntimeRef,
		ServiceAccountName: snap.ServiceAccountName,
		ContextClaimName:   snap.ContextClaimName,
		WorkspaceClaimName: snap.WorkspaceClaimName,
		Signal: &agentopsv1alpha1.SignalProvenance{
			SourceRef: &agentopsv1alpha1.ObjectRef{Name: sourceName},
			Labels:    sig.Spec.Labels,
			Severity:  sig.Spec.Severity,
		},
		Title:          title,
		ConversationID: sig.Spec.InitiationID,
	}
	// An addressed conversation binds the addressing channel too, so the
	// answer comes back to where the person is looking.
	if _, addressed := messageOf(sig); addressed {
		conv.Spec.ChannelRefs = withChannel(conv.Spec.ChannelRefs, sig.Spec.Labels[agentopsv1alpha1.LabelChatChannel])
	}
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
	if sig.Spec.Kind == kindChat && sig.Spec.Reader != "" {
		if ch := sig.Spec.Labels[agentopsv1alpha1.LabelChatChannel]; ch != "" {
			conv.Spec.OriginReader = &agentopsv1alpha1.OriginReader{Channel: ch, Key: sig.Spec.Reader}
		}
	}
	if _, err := namewords.AllocateName(ctx, deps.Client, deps.Namespace,
		nameKindForSignal(sig.Spec.Kind), wordsForSignal(sourceName, title), conv); err != nil {
		return nil, err
	}
	if budget := claimant.SnapshotBudget(); budget != nil {
		patch := client.MergeFrom(conv.DeepCopy())
		conv.Status.Budget = budget
		if err := deps.Client.Status().Patch(ctx, conv, patch); err != nil {
			return nil, err
		}
	}
	deps.Activity.Emit(activity.Event{
		Kind:     activity.KindConversationCreated,
		From:     activity.Node(nodeKind, claimant.GetName()),
		To:       activity.Node(activity.NodeConversation, conv.Name),
		Pipeline: claimant.GetName(), Conversation: conv.Name,
		Detail: conv.Spec.Title,
	})
	return conv, nil
}

// withChannel adds a channel ref unless it is already bound.
func withChannel(refs []agentopsv1alpha1.ObjectRef, name string) []agentopsv1alpha1.ObjectRef {
	if name == "" {
		return refs
	}
	for _, r := range refs {
		if r.Name == name {
			return refs
		}
	}
	return append(refs, agentopsv1alpha1.ObjectRef{Name: name})
}

// InputName is the name of the ConversationInput carrying sig's message to one
// claimant — derived deterministically from the Signal and the claimant, so a
// repeated pass finds the delivery it already made rather than making
// another. Pipeline and Coordinator share a namespace and may share a name,
// hence the kind letter.
func InputName(sigName string, ref agentopsv1alpha1.ClaimantRef) string {
	kind := "p"
	if ref.Kind == agentopsv1alpha1.ClaimantKindCoordinator {
		kind = "c"
	}
	return sigName + "-" + kind + "-" + ref.Name
}

// inputDelivered reports whether the conversation already carries the named
// input: queued in spec.inputs[], or — once dispatch consumed it and pruned
// the queue — cited by a run's recorded inputs. The ConversationInput object
// itself is deleted with its queue entry, so its absence proves nothing.
func inputDelivered(conv *agentopsv1alpha1.Conversation, ciName string) bool {
	for i := range conv.Spec.Inputs {
		if ref := conv.Spec.Inputs[i].PayloadRef; ref != nil && ref.Name == ciName {
			return true
		}
	}
	for r := range conv.Status.Runs {
		for i := range conv.Status.Runs[r].Inputs {
			if ref := conv.Status.Runs[r].Inputs[i].PayloadRef; ref != nil && ref.Name == ciName {
				return true
			}
		}
	}
	return false
}

// appendInput records sig's message as a ConversationInput under its derived
// name and queues it on the conversation, retrying the optimistic patch on
// conflict. A delivery that already happened (an earlier pass of the same
// Signal and claimant) is left alone.
func appendInput(ctx context.Context, deps Deps, claimant chat.Claimant, conv *agentopsv1alpha1.Conversation, sig *agentopsv1alpha1.Signal, inputType agentopsv1alpha1.InputType) error {
	sourceName := sig.Spec.SourceRef.Name
	ciName := InputName(sig.Name, claimantRef(claimant))
	text, _ := messageOf(sig)

	ci := &agentopsv1alpha1.ConversationInput{}
	ci.Namespace = deps.Namespace
	ci.Name = ciName
	ci.Spec = agentopsv1alpha1.ConversationInputSpec{
		ConversationRef: agentopsv1alpha1.ObjectRef{Name: conv.Name},
		Type:            inputType,
		Payload:         text,
		Labels:          sig.Spec.Labels,
		Severity:        sig.Spec.Severity,
	}

	for attempt := 0; attempt < 5; attempt++ {
		var fresh agentopsv1alpha1.Conversation
		if err := deps.Reader.Get(ctx, types.NamespacedName{Namespace: deps.Namespace, Name: conv.Name}, &fresh); err != nil {
			return err
		}
		if inputDelivered(&fresh, ciName) {
			return nil
		}
		// Created only now that the queue is known not to carry it, and an
		// existing object under the name is the same delivery's earlier half.
		if err := deps.Client.Create(ctx, ci); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
		patch := client.MergeFrom(fresh.DeepCopy())
		inputID := "in-" + strconv.FormatInt(time.Now().UnixNano(), 36)
		fresh.Spec.Inputs = append(fresh.Spec.Inputs, agentopsv1alpha1.InputItem{
			ID:         inputID,
			Type:       inputType,
			PayloadRef: &agentopsv1alpha1.ObjectRef{Name: ciName},
			Origin: &agentopsv1alpha1.InputOrigin{
				Kind: agentopsv1alpha1.OriginSignal, Name: sourceName,
				SignalKind: orDefault(sig.Spec.Kind, kindAlert),
			},
			ReceivedAt: metav1.Now(),
		})
		if err := deps.Client.Patch(ctx, &fresh, patch); err != nil {
			if apierrors.IsConflict(err) {
				continue
			}
			return err
		}
		deps.Activity.Emit(activity.Event{
			Kind:     activity.KindInputQueued,
			From:     activity.Node(activity.NodeSignalSource, sourceName),
			To:       activity.Node(activity.NodeConversation, conv.Name),
			Pipeline: claimant.GetName(), Conversation: conv.Name, InputID: inputID,
			Code: string(inputType), Detail: sourceName,
		})
		return nil
	}
	return fmt.Errorf("conflict appending input to %s", conv.Name)
}

// orDefault fills an empty string with a fallback, mirroring
// internal/httpapi's own helper of the same name.
func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
