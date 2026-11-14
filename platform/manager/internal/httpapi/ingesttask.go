// ingesttask.go is the "in.signal" queue's own wire vocabulary and its
// consumer (async-signal-ingest design D-5): handleSignalInbound
// (publish.go) builds one ingestSignalPayload per admitted signal and hands
// it to commandbus.Bus.Submit as opaque bytes; HandleIngestSignalTask
// decodes the same shape back and does what the request used to do inline —
// answer an in-place chat command, through chat.Router, or create the
// Signal CR.
//
// No separate package, unlike internal/convtask: nothing outside
// internal/httpapi builds one of these — design D-5's own "the task
// payload IS the normalized signal plus the source name", decided entirely
// from POST /signal/inbound's own body, with no second producer the way
// internal/controller is for "conv" tasks.
package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/addressing"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/consumer"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// SignalIngestQueue is the one commandbus.Queue handleSignalInbound submits
// to and HandleIngestSignalTask claims from — "in.signal" literally, named
// once here rather than spelled twice, matching accept.go's own ConvQueue
// for "conv".
const SignalIngestQueue = commandbus.Queue("in.signal")

// ingestSignalPayload is the "in.signal" queue's one wire shape: the
// normalized signal exactly as the request received it, plus the name of
// the SignalSource it arrived on. One task carries exactly one signal
// (design D-2: "one task per signal, not per batch") — the consumer
// re-reads the SignalSource fresh by this name and trusts nothing else
// about it beyond what the signal itself carries.
type ingestSignalPayload struct {
	Source string           `json:"source"`
	Signal NormalizedSignal `json:"signal"`
}

// encode marshals p for commandbus.SubmitRequest.Payload. Never fails — a
// struct of strings/maps the accept handler already validated — so callers
// compose it inline rather than carry an error only panic could ever reach,
// matching convtask.Payload.Encode's own reasoning.
func (p ingestSignalPayload) encode() []byte {
	b, _ := json.Marshal(p)
	return b
}

// decodeIngestSignalPayload is encode's inverse, HandleIngestSignalTask's
// own entry point.
func decodeIngestSignalPayload(raw []byte) (ingestSignalPayload, error) {
	var p ingestSignalPayload
	err := json.Unmarshal(raw, &p)
	return p, err
}

// HandleIngestSignalTask is the "in.signal" queue's consumer.HandlerFunc
// (async-signal-ingest task 2.2): the PROCESS half of what
// handleSignalInbound used to do inline, now running on whichever replica
// next claims the task — never gated on leader election, exactly like
// HandleConvTask. An unreadable payload is commandbus.CodeInvalidCommand (a
// producer bug, never worth retrying, matching HandleConvTask's own
// reasoning); a SignalSource gone by the time this task runs is
// commandbus.CodeNotFound, also terminal — retrying would fail identically.
func (s *Server) HandleIngestSignalTask(ctx context.Context, task commandbus.Task) error {
	p, err := decodeIngestSignalPayload(task.Payload)
	if err != nil {
		return consumer.WithCode(commandbus.CodeInvalidCommand,
			fmt.Errorf("ingest-signal task %s: decode payload: %w", task.UID, err))
	}
	var source agentopsv1alpha1.SignalSource
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: p.Source}, &source); err != nil {
		if apierrors.IsNotFound(err) {
			return consumer.WithCode(commandbus.CodeNotFound,
				fmt.Errorf("ingest-signal task %s: signal source %q: %w", task.UID, p.Source, err))
		}
		return err // unparsed: Fail's own ParseErrorCode fallback -> internal, retried
	}
	answered, rest, err := s.answerCommands(ctx, []NormalizedSignal{p.Signal})
	if err != nil {
		return err
	}
	if answered > 0 {
		return nil // answered in place (design D-5): nothing to store
	}
	if _, err := s.publish(ctx, &source, rest, task.UID); err != nil {
		return err
	}
	return nil
}

// answerCommands answers, in place, the chat COMMANDS whose whole result is a
// reply — the listing, a bare /<pipeline> asking for its task, /close and /exit
// on a general surface. They open and continue nothing, so they are not
// messages to queue and no Signal is stored for them. Everything else is
// returned for publication, an addressed /<pipeline> <task> included. Returns
// how many were answered.
func (s *Server) answerCommands(ctx context.Context, signals []NormalizedSignal) (int, []NormalizedSignal, error) {
	answered := 0
	rest := make([]NormalizedSignal, 0, len(signals))
	for _, sig := range signals {
		if sig.Kind != KindChat {
			rest = append(rest, sig)
			continue
		}
		cmd, ok := addressing.Parse(strings.TrimSpace(sig.Payload))
		if !ok || !chat.AnswersInPlace(cmd) {
			rest = append(rest, sig)
			continue
		}
		ch := s.chatChannel(ctx, sig)
		if ch == nil {
			continue // unknown channel: nowhere to answer, nothing to store
		}
		if err := s.Router.HandleCommand(ctx, ch, cmd,
			sig.Labels[LabelChatSender], sig.Labels[LabelChatMessage], sig.Reader); err != nil {
			return answered, nil, err
		}
		answered++
	}
	return answered, rest, nil
}

// publish creates one Signal CR per signal, in order, owned by its source. It
// returns how many were stored or deduplicated; the first failed store
// aborts the batch (the caller fails the task, which is retried). taskUID
// names the Signal when a signal carries no signalId (design D-6).
func (s *Server) publish(ctx context.Context, source *agentopsv1alpha1.SignalSource, signals []NormalizedSignal, taskUID string) (int, error) {
	n := 0
	for i := range signals {
		if err := s.publishOne(ctx, source, &signals[i], taskUID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// publishOne stores one Signal. A signal carrying a signalId is created under
// its deterministic name, and a create answered AlreadyExists IS the
// duplicate: nothing is stored or changed, it still counts as acknowledged,
// and an activity event records it. A signal with none is named from
// taskUID (design D-6) rather than a clock and a random suffix, so a
// redelivery of the SAME task (a lease lapse, a retried handler) derives the
// same name and is deduplicated too. Publication stays lookup-free either
// way.
func (s *Server) publishOne(ctx context.Context, source *agentopsv1alpha1.SignalSource, in *NormalizedSignal, taskUID string) error {
	name := signalNameFromTask(taskUID)
	if in.SignalID != "" {
		name = agentopsv1alpha1.SignalNameFor(source.Name, in.SignalID)
	}
	// SourceID lets the store link the signal to its source (the kube backend
	// turns it into the controller owner reference); the manager does not
	// depend on that link, it releases a deleted source's signals itself.
	_, err := s.Signals.Create(ctx, state.Key(name), state.SignalRecord{
		SourceID:     string(source.UID),
		SourceRef:    source.Name,
		Adapter:      source.Spec.Adapter,
		InitiationID: in.InitiationID,
		SignalID:     in.SignalID,
		Kind:         in.Kind,
		Title:        in.Title,
		Severity:     string(in.Severity),
		Labels:       boundedLabels(in.Labels),
		Reader:       in.Reader,
		Payload:      in.Payload,
	})
	switch {
	case err == nil:
		log.FromContext(ctx).Info("signal stored", "signal", name, "source", source.Name)
		return nil
	case in.SignalID != "" && errors.Is(err, state.ErrAlreadyExists):
		s.Activity.Emit(activity.Event{
			Kind:   activity.KindSignalDropped,
			From:   activity.Node(activity.NodeSignalSource, source.Name),
			Status: activity.StatusOK,
			Code:   activity.CodeDuplicate,
			Detail: "re-send of an already stored message (" + in.InitiationID + ")",
		})
		return nil
	default:
		// Never settled: the task fails and is retried, exactly as the
		// request's own 5xx used to make the adapter re-send.
		return fmt.Errorf("store signal for %q: %w", in.InitiationID, err)
	}
}

// signalTaskNameHexLen matches v1alpha1.SignalNameFor's own hex length (128
// bits of the digest) — unexported there, so stated again here rather than
// reached for across the package boundary.
const signalTaskNameHexLen = 32

// signalNameFromTask names a Signal with no sender-given signalId: sig-<first
// 32 hex of sha256(task UID)> (design D-6). A task's UID is stable across
// every attempt of its one submission (commandbus.Task's own doc comment:
// "a single Task persists across every attempt"), so a lease-lapse
// redelivery or a retried handler derives the SAME name and Create's
// AlreadyExists becomes the deduplication for this case too — the same
// property SignalNameFor's own scheme already gives a sender-given
// signalId. Time-ordered sort is lost for these Signals; the consumer's
// arrival order (creationTimestamp, then name) tolerates it.
func signalNameFromTask(taskUID string) string {
	sum := sha256.Sum256([]byte(taskUID))
	return agentopsv1alpha1.SignalNamePrefix + hex.EncodeToString(sum[:])[:signalTaskNameHexLen]
}

// chatChannel resolves the Channel a chat signal came from. Nil when it names
// none or the Channel does not exist.
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
