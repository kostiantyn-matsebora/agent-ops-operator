// Package signalroute is the consumer's routing logic (design D-4..D-6 of the
// signal-persistence change): given ONE Signal it holds, Admit makes every
// routing decision in the spec's order and snapshots the claimants once, and
// Route sends the message to each claimant idempotently.
//
// It operates purely on the state.SignalRecord it is handed and what it reads
// from the API server — no in-memory routing state beyond the shared cooldown
// book, so a new leader repeats an interrupted pass safely. Both steps mutate
// the record IN MEMORY only; the caller (internal/controller's
// SignalReconciler) owns the status writes, the work queue, the
// retry/requeue and housekeeping.
package signalroute

import (
	"context"
	"fmt"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/addressing"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/ingest"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// Deps bundles what a pass needs from its caller.
type Deps struct {
	// Client is used for the source status writes (the receipt counters).
	Client client.Client
	// Conversations holds every conversation: the reusable-conversation
	// lookup, the backlog count and conversation create. Read straight from
	// its source of truth, never a cache: a pass interrupted after a create
	// must find what it just made.
	Conversations state.Collection[state.ConversationRecord]
	// Runs holds the conversations' append-only run history, read to tell
	// whether a signal's message already reached its conversation.
	Runs state.Appended[state.ConversationRecord, state.Run]
	// Blobs holds a queued input's out-of-line payload.
	Blobs state.Blobs
	// CooldownWindows holds one cooldown window per signal source, read and
	// replaced by this package's own helpers (window.go) over Mutate.
	CooldownWindows state.Collection[state.CooldownWindow]
	// Receipts holds one receipt counter per signal source, counted by this
	// package's own helper (window.go) over Mutate.
	Receipts state.Collection[state.Received]
	// Reader serves the reads a pass needs (claimants, capability and
	// runtime). The API server directly in production.
	Reader client.Reader
	// Namespace every object above lives in — the manager is single-namespace
	// per install.
	Namespace string
	// Activity records the claim, drop and conversation hops. A nil log is
	// inert (internal/activity's own contract).
	Activity *activity.Log
	// Ops is the in-memory channel-operation queue the chat notices go
	// through, fire-and-forget. Nil skips notices.
	Ops *chat.OpQueue
	// Cooldowns is the leader's per-source cooldown map, shared by every
	// consumer of the pool and checked-and-spent atomically per source.
	Cooldowns *ingest.CooldownBook
	// Runtime is the bootstrap execution config SnapshotFor falls back to.
	Runtime runtimepod.Config
	// MaxQueued bounds the pending-conversation backlog a NEW conversation
	// may be created against.
	MaxQueued int
}

// Result reports what a step did, for the caller to act on.
type Result struct {
	// Dropped is true when Admit ended the Signal before fan-out; the phase and
	// reason are already on sig.
	Dropped bool
	// Wait is true while a claimant stays Routing (a machine signal at a full
	// backlog): the caller requeues the Signal with a delay.
	Wait bool
	// FirstRouted is true when Route moved the Signal from "no claimant
	// routed" to "at least one routed" — the moment the source's receivedTotal
	// moves, once per signal.
	FirstRouted bool
	// Rollback undoes the cooldown Admit spent. The caller invokes it when the
	// status write recording the snapshot fails, so the repeated pass is not
	// suppressed by its own earlier attempt. Nil when nothing was spent.
	Rollback func()
}

// Admit makes the routing decisions of a Signal with no claimant snapshot yet,
// in the spec's order — the addressed target, the source's Ready claimants,
// unclaimed, ambiguous, cooldown (checked and spent atomically per source),
// capacity — and ends in either a Dropped phase with its reason (Result.Dropped)
// or the one-time claimant snapshot. It mutates sig in memory only.
//
// The caller persists that outcome BEFORE Route creates anything: once the
// snapshot is on the record a repeated pass skips Admit, so the cooldown it spent
// can never suppress the repeat.
func Admit(ctx context.Context, deps Deps, sig *state.SignalRecord, source *agentopsv1alpha1.SignalSource) (Result, error) {
	var res Result
	claimants, rollback, dropped, err := admit(ctx, deps, sig, source)
	res.Rollback = rollback
	if err != nil {
		return res, err
	}
	if dropped {
		res.Dropped = true
		return res, nil
	}
	snapshot(deps, sig, claimants)
	return res, nil
}

// Route attempts every claimant still Routing (and ONLY those) by equality
// reuse: a claimant that no longer exists is Skipped, a create the pending
// backlog refuses leaves that claimant Routing, anything else is Routed onto
// its conversation. status.phase is recomputed from the claimants. A non-nil
// error leaves sig's status as far as the pass got, so the caller persists the
// partial progress before retrying.
func Route(ctx context.Context, deps Deps, sig *state.SignalRecord) (Result, error) {
	var res Result
	hadRouted := hasState(sig, state.ClaimantRouted)
	err := routeClaimants(ctx, deps, sig)
	sig.Phase = state.SignalRollupPhase(sig.Claimants)
	res.Wait = hasState(sig, state.ClaimantRouting)
	res.FirstRouted = !hadRouted && hasState(sig, state.ClaimantRouted)
	return res, err
}

func hasState(sig *state.SignalRecord, want string) bool {
	for i := range sig.Claimants {
		if sig.Claimants[i].State == want {
			return true
		}
	}
	return false
}

// AddressedTarget parses a chat signal's payload for the `/<name> <task>`
// form. ok is false for any other text — including a command that only
// answers (the listing, /close, /exit, a bare /<name>), which /signal/inbound
// answers in place and never stores.
func AddressedTarget(sig *state.SignalRecord) (name, task string, ok bool) {
	if sig.Kind != kindChat {
		return "", "", false
	}
	cmd, parsed := addressing.Parse(strings.TrimSpace(sig.Payload))
	if !parsed || chat.AnswersInPlace(cmd) {
		return "", "", false
	}
	return cmd.Pipeline, cmd.Rest, true
}

// admit makes the pre-fan-out decisions (spec steps 1-4). It returns the
// claimants to snapshot, or dropped=true after recording the drop on sig.
func admit(ctx context.Context, deps Deps, sig *state.SignalRecord, source *agentopsv1alpha1.SignalSource) (claimants []chat.Claimant, rollback func(), dropped bool, err error) {
	isChat := sig.Kind == kindChat
	var ch *agentopsv1alpha1.Channel
	if isChat {
		if ch = channelOf(ctx, deps, sig); ch == nil {
			drop(ctx, deps, sig, agentopsv1alpha1.SignalDropUnaddressable, activity.CodeUnaddressable,
				"chat signal names no existing Channel — nowhere to answer")
			return nil, nil, true, nil
		}
	}

	if name, _, addressed := AddressedTarget(sig); addressed {
		target, err := chat.ResolveClaimant(ctx, deps.Reader, deps.Namespace, name)
		if err != nil {
			return nil, nil, false, fmt.Errorf("resolve addressed %q: %w", name, err)
		}
		if target == nil {
			tell(ctx, deps, ch, sig, unknownTargetNotice(name))
			drop(ctx, deps, sig, agentopsv1alpha1.SignalDropUnclaimed, activity.CodeUnclaimed,
				fmt.Sprintf("addressed pipeline or coordinator %q does not exist", name))
			return nil, nil, true, nil
		}
		claimants = []chat.Claimant{target}
	} else {
		claimants = chat.PipelinesForSource(ctx, deps.Reader, deps.Namespace, source.Name)
		switch {
		case len(claimants) == 0:
			tell(ctx, deps, ch, sig, unwiredNotice(source.Name))
			drop(ctx, deps, sig, agentopsv1alpha1.SignalDropUnclaimed, activity.CodeUnclaimed,
				"source not served by any Ready pipeline or Coordinator (Wired=False) — signal dropped")
			return nil, nil, true, nil
		case isChat && len(claimants) > 1:
			tell(ctx, deps, ch, sig, ambiguousNotice(claimants))
			drop(ctx, deps, sig, agentopsv1alpha1.SignalDropAmbiguous, activity.CodeAmbiguous,
				"several Ready pipelines or coordinators serve this chat source — the message names none of them")
			return nil, nil, true, nil
		}
		var spent bool
		var spendErr error
		spent, rollback, spendErr = spendCooldown(ctx, deps, source, sig)
		if spendErr != nil {
			return nil, nil, false, spendErr
		}
		if !spent {
			// The high-volume case — a flapping alert re-delivered inside its
			// window. Recorded on the Signal, not an event: it is never loud.
			sig.Phase = state.SignalDropped
			sig.Reason = string(agentopsv1alpha1.SignalDropCooldown)
			return nil, nil, true, nil
		}
	}

	if isChat {
		full, err := BacklogFull(ctx, deps.Conversations, deps.MaxQueued)
		if err != nil {
			rollbackIf(rollback)
			return nil, nil, false, err
		}
		if full {
			// A person is waiting on the surface they typed on: decline at
			// once, addressed chat included.
			rollbackIf(rollback)
			tell(ctx, deps, ch, sig, capacityNotice())
			drop(ctx, deps, sig, agentopsv1alpha1.SignalDropCapacity, activity.CodeAtCapacity,
				"pending conversation backlog is full — signal dropped")
			return nil, nil, true, nil
		}
	}
	return claimants, rollback, false, nil
}

func rollbackIf(f func()) {
	if f != nil {
		f()
	}
}

// drop records a pre-fan-out drop on sig and surfaces it as loudly as the
// unwired drop always was: an activity event and a log line.
func drop(ctx context.Context, deps Deps, sig *state.SignalRecord, reason agentopsv1alpha1.SignalDropReason, code, detail string) {
	sig.Phase = state.SignalDropped
	sig.Reason = string(reason)
	Loud(ctx, deps.Activity, sig, activity.KindSignalDropped, code, detail)
}

// Loud emits the activity event and the log line a Dropped (other than
// cooldown) or Failed Signal owes.
func Loud(ctx context.Context, acts *activity.Log, sig *state.SignalRecord, kind, code, detail string) {
	log.FromContext(ctx).Info("signal ended without a conversation",
		"signal", sig.Name, "source", sig.SourceRef, "initiationId", sig.InitiationID,
		"event", kind, "reason", code, "detail", detail)
	acts.Emit(activity.Event{
		Kind:   kind,
		From:   activity.Node(activity.NodeSignalSource, sig.SourceRef),
		Status: activity.StatusError,
		Code:   code,
		Detail: detail + " (" + sig.InitiationID + ")",
	})
}

// snapshot records the claimant set a Signal fans out to, exactly once.
func snapshot(deps Deps, sig *state.SignalRecord, claimants []chat.Claimant) {
	out := make([]state.SignalClaimant, 0, len(claimants))
	for _, c := range claimants {
		out = append(out, claimantOf(c))
		nodeKind := activity.NodePipeline
		if c.ClaimantKind() == chat.ClaimantCoordinator {
			nodeKind = activity.NodeCoordinator
		}
		deps.Activity.Emit(activity.Event{
			Kind:     activity.KindSignalClaimed,
			From:     activity.Node(activity.NodeSignalSource, sig.SourceRef),
			To:       activity.Node(nodeKind, c.GetName()),
			Pipeline: c.GetName(),
			Detail:   "1 signal(s)",
		})
	}
	sig.Claimants = out
}

// claimantOf renders a chat.Claimant as the snapshot entry a record keeps,
// still Routing — the two ClaimantKind vocabularies differ in case (chat's is
// lowercase, the record's is capitalized) because internal/state is
// self-contained and cannot import internal/chat.
func claimantOf(c chat.Claimant) state.SignalClaimant {
	kind := state.ClaimantKindPipeline
	if c.ClaimantKind() == chat.ClaimantCoordinator {
		kind = state.ClaimantKindCoordinator
	}
	return state.SignalClaimant{Kind: kind, Name: c.GetName(), State: state.ClaimantRouting}
}

func chatKind(k string) chat.ClaimantKind {
	if k == state.ClaimantKindCoordinator {
		return chat.ClaimantCoordinator
	}
	return chat.ClaimantPipeline
}

// routeClaimants attempts every claimant still Routing: a claimant that no
// longer exists is Skipped, a refused create leaves it Routing, anything else
// is Routed onto its conversation. A terminal claimant is never touched.
func routeClaimants(ctx context.Context, deps Deps, sig *state.SignalRecord) error {
	for i := range sig.Claimants {
		c := &sig.Claimants[i]
		if c.State != state.ClaimantRouting {
			continue
		}
		claimant, err := chat.ClaimantByKind(ctx, deps.Reader, deps.Namespace, chatKind(c.Kind), c.Name)
		if err != nil {
			return fmt.Errorf("read claimant %s/%s: %w", c.Kind, c.Name, err)
		}
		if claimant == nil {
			c.State = state.ClaimantSkipped
			c.Reason = "claimant no longer exists"
			continue
		}
		convRef, waiting, err := routeOneClaimant(ctx, deps, claimant, sig)
		if err != nil {
			return fmt.Errorf("route claimant %s/%s: %w", c.Kind, c.Name, err)
		}
		if waiting {
			continue // backlog full: stays Routing, requeued with a delay
		}
		c.State = state.ClaimantRouted
		c.ConversationRef = convRef
	}
	return nil
}

// BacklogFull reports whether the pending backlog has reached its bound.
// Only PENDING conversations count: an admitted one holds a pod (or is about
// to) and is already bounded by MAX_ACTIVE_CONVERSATIONS.
func BacklogFull(ctx context.Context, conversations state.Collection[state.ConversationRecord], maxQueued int) (bool, error) {
	pending, err := conversations.List(ctx, state.ByPhase, state.PhasePending)
	if err != nil {
		return false, err
	}
	return len(pending) >= maxQueued, nil
}
