package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/addressing"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
)

// handleSignalInbound is PUBLICATION (signal-persistence D-2): validate the
// batch, apply the source's attributeFilters, store one Signal CR per admitted
// signal and acknowledge. It looks up no claimant, spends no cooldown, checks
// no capacity and parses no addressing for routing — the consumer decides all
// of that, later and on the leader.
//
// The response is `200 {"queued": n}` where n counts signals stored or
// deduplicated, written only after every admitted signal's CR exists. A store
// that fails answers 5xx, and the adapter's at-least-once re-send repeats the
// batch (a signal carrying a signalId that did land is then deduplicated).
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
	// Receipt is recorded before anything is stored: the hop that happened is
	// "this adapter handed the manager a signal", true whether the signal is
	// later claimed, suppressed by cooldown or dropped. The initiationId in
	// `detail` correlates it with the claim that follows.
	for _, sig := range in.Signals {
		s.Activity.Emit(activity.Event{
			Kind:   activity.KindSignalReceived,
			From:   activity.Node(activity.NodeSignalAdapter, source.Spec.Adapter),
			To:     activity.Node(activity.NodeSignalSource, source.Name),
			Code:   orDefault(sig.Kind, KindAlert),
			Detail: sig.InitiationID,
		})
	}
	admitted, filteredOut := partitionByAttributeFilters(source, in.Signals)
	if len(filteredOut) > 0 && logFiltered(source) {
		s.emitDropped(source, filteredOut, activity.CodeFiltered, ReasonFiltered)
	}

	answered, toStore, err := s.answerCommands(r.Context(), admitted)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	stored, err := s.publish(r.Context(), source, toStore)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	out := map[string]any{"queued": answered + stored}
	if len(filteredOut) > 0 {
		out["reason"] = ReasonFiltered
	}
	writeJSON(w, 200, out)
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
// returns how many were stored or deduplicated; the first failed store aborts
// the batch (the caller answers 5xx).
func (s *Server) publish(ctx context.Context, source *agentopsv1alpha1.SignalSource, signals []NormalizedSignal) (int, error) {
	n := 0
	for i := range signals {
		if err := s.publishOne(ctx, source, &signals[i]); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// generatedSignalName names a Signal that carries no signalId: sig-<creation
// time in base36>-<random>. Still generated, with no deduplication — but time
// ordered, so the consumer's arrival order (creationTimestamp, then NAME) keeps
// the order of messages published within one second, which a fully random
// generateName suffix would lose.
func generatedSignalName() string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	suffix := make([]byte, generatedSuffixLen)
	for i := range suffix {
		suffix[i] = alphabet[rand.IntN(len(alphabet))]
	}
	return fmt.Sprintf("%s%0*s-%s", agentopsv1alpha1.SignalNamePrefix, generatedTimeLen,
		strconv.FormatInt(time.Now().UnixNano(), 36), suffix)
}

// generatedTimeLen pads the base36 timestamp to a fixed width so names sort
// as time does; generatedSuffixLen is the random tail that separates two
// Signals published in the same nanosecond.
const (
	generatedTimeLen   = 13
	generatedSuffixLen = 5
)

// publishOne stores one Signal. A signal carrying a signalId is created under
// its deterministic name, and a create answered AlreadyExists IS the duplicate:
// nothing is stored or changed, it still counts as acknowledged, and an
// activity event records it. Publication stays lookup-free.
func (s *Server) publishOne(ctx context.Context, source *agentopsv1alpha1.SignalSource, in *NormalizedSignal) error {
	sig := &agentopsv1alpha1.Signal{}
	sig.Namespace = s.Namespace
	if in.SignalID != "" {
		sig.Name = agentopsv1alpha1.SignalNameFor(source.Name, in.SignalID)
	} else {
		sig.Name = generatedSignalName()
	}
	sig.Spec = agentopsv1alpha1.SignalSpec{
		SourceRef:    agentopsv1alpha1.ObjectRef{Name: source.Name},
		Adapter:      source.Spec.Adapter,
		InitiationID: in.InitiationID,
		SignalID:     in.SignalID,
		Kind:         in.Kind,
		Title:        in.Title,
		Severity:     in.Severity,
		Labels:       boundedLabels(in.Labels),
		Reader:       in.Reader,
		Payload:      in.Payload,
	}
	if err := controllerutil.SetControllerReference(source, sig, s.Client.Scheme()); err != nil {
		return fmt.Errorf("signal owner reference: %w", err)
	}
	err := s.Client.Create(ctx, sig)
	switch {
	case err == nil:
		return nil
	case in.SignalID != "" && apierrors.IsAlreadyExists(err):
		s.Activity.Emit(activity.Event{
			Kind:   activity.KindSignalDropped,
			From:   activity.Node(activity.NodeSignalSource, source.Name),
			Status: activity.StatusOK,
			Code:   activity.CodeDuplicate,
			Detail: "re-send of an already stored message (" + in.InitiationID + ")",
		})
		return nil
	default:
		// Never acknowledged: the 5xx makes the adapter re-send.
		return fmt.Errorf("store signal for %q: %w", in.InitiationID, err)
	}
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

// emitDropped records one signal.dropped per signal a batch lost, carrying the
// reason. A drop is the event an operator most needs and the one a
// status-derived graph can never show: nothing changed, so there is nothing to
// infer from.
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

// orDefault fills an empty string with a fallback (telemetry labelling only).
func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
