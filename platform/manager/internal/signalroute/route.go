// Package signalroute is the consumer's routing logic (design D-4..D-6 of the
// signal-persistence change): given ONE Signal it holds, Admit makes every
// routing decision in the spec's order and snapshots the claimants once, and
// Route sends the message to each claimant idempotently.
//
// It operates purely on a Signal CR's own spec/status and what it reads from
// the API server — no in-memory routing state beyond the shared cooldown
// book, so a new leader repeats an interrupted pass safely. Both steps mutate
// the Signal IN MEMORY only; the caller (internal/controller's
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
)

// Deps bundles what a pass needs from its caller.
type Deps struct {
	// Client is used for every write: conversation create/patch, input
	// create, source status.
	Client client.Client
	// Reader serves the reads a pass needs (claimants, reusable-conversation
	// lookup, backlog count). The API server directly in production, never an
	// informer cache: a pass interrupted after a create must find what it
	// just made.
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
// snapshot is on the CR a repeated pass skips Admit, so the cooldown it spent
// can never suppress the repeat.
func Admit(ctx context.Context, deps Deps, sig *agentopsv1alpha1.Signal, source *agentopsv1alpha1.SignalSource) (Result, error) {
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
func Route(ctx context.Context, deps Deps, sig *agentopsv1alpha1.Signal) (Result, error) {
	var res Result
	hadRouted := hasState(sig, agentopsv1alpha1.ClaimantRouted)
	err := routeClaimants(ctx, deps, sig)
	sig.Status.Phase = agentopsv1alpha1.RollupPhase(sig.Status.Claimants)
	res.Wait = hasState(sig, agentopsv1alpha1.ClaimantRouting)
	res.FirstRouted = !hadRouted && hasState(sig, agentopsv1alpha1.ClaimantRouted)
	return res, err
}

func hasState(sig *agentopsv1alpha1.Signal, state agentopsv1alpha1.ClaimantState) bool {
	for i := range sig.Status.Claimants {
		if sig.Status.Claimants[i].State == state {
			return true
		}
	}
	return false
}

// AddressedTarget parses a chat signal's payload for the `/<name> <task>`
// form. ok is false for any other text — including a command that only
// answers (the listing, /close, /exit, a bare /<name>), which /signal/inbound
// answers in place and never stores.
func AddressedTarget(sig *agentopsv1alpha1.Signal) (name, task string, ok bool) {
	if sig.Spec.Kind != kindChat {
		return "", "", false
	}
	cmd, parsed := addressing.Parse(strings.TrimSpace(sig.Spec.Payload))
	if !parsed || chat.AnswersInPlace(cmd) {
		return "", "", false
	}
	return cmd.Pipeline, cmd.Rest, true
}

// admit makes the pre-fan-out decisions (spec steps 1-4). It returns the
// claimants to snapshot, or dropped=true after recording the drop on sig.
func admit(ctx context.Context, deps Deps, sig *agentopsv1alpha1.Signal, source *agentopsv1alpha1.SignalSource) (claimants []chat.Claimant, rollback func(), dropped bool, err error) {
	isChat := sig.Spec.Kind == kindChat
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
		spent, rollback = spendCooldown(ctx, deps, source, sig)
		if !spent {
			// The high-volume case — a flapping alert re-delivered inside its
			// window. Recorded on the Signal, not an event: it is never loud.
			sig.Status.Phase = agentopsv1alpha1.SignalDropped
			sig.Status.Reason = agentopsv1alpha1.SignalDropCooldown
			return nil, nil, true, nil
		}
	}

	if isChat {
		full, err := BacklogFull(ctx, deps.Reader, deps.Namespace, deps.MaxQueued)
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
func drop(ctx context.Context, deps Deps, sig *agentopsv1alpha1.Signal, reason agentopsv1alpha1.SignalDropReason, code, detail string) {
	sig.Status.Phase = agentopsv1alpha1.SignalDropped
	sig.Status.Reason = reason
	Loud(ctx, deps.Activity, sig, activity.KindSignalDropped, code, detail)
}

// Loud emits the activity event and the log line a Dropped (other than
// cooldown) or Failed Signal owes.
func Loud(ctx context.Context, acts *activity.Log, sig *agentopsv1alpha1.Signal, kind, code, detail string) {
	log.FromContext(ctx).Info("signal ended without a conversation",
		"signal", sig.Name, "source", sig.Spec.SourceRef.Name, "initiationId", sig.Spec.InitiationID,
		"event", kind, "reason", code, "detail", detail)
	acts.Emit(activity.Event{
		Kind:   kind,
		From:   activity.Node(activity.NodeSignalSource, sig.Spec.SourceRef.Name),
		Status: activity.StatusError,
		Code:   code,
		Detail: detail + " (" + sig.Spec.InitiationID + ")",
	})
}

// snapshot records the claimant set a Signal fans out to, exactly once.
func snapshot(deps Deps, sig *agentopsv1alpha1.Signal, claimants []chat.Claimant) {
	out := make([]agentopsv1alpha1.SignalClaimant, 0, len(claimants))
	for _, c := range claimants {
		out = append(out, agentopsv1alpha1.SignalClaimant{Ref: claimantRef(c), State: agentopsv1alpha1.ClaimantRouting})
		nodeKind := activity.NodePipeline
		if c.ClaimantKind() == chat.ClaimantCoordinator {
			nodeKind = activity.NodeCoordinator
		}
		deps.Activity.Emit(activity.Event{
			Kind:     activity.KindSignalClaimed,
			From:     activity.Node(activity.NodeSignalSource, sig.Spec.SourceRef.Name),
			To:       activity.Node(nodeKind, c.GetName()),
			Pipeline: c.GetName(),
			Detail:   "1 signal(s)",
		})
	}
	sig.Status.Claimants = out
}

// claimantRef renders a chat.Claimant's own ClaimantRef — the two ClaimantKind
// vocabularies differ in case (chat's is lowercase, the CRD's is capitalized)
// because api/v1alpha1 is self-contained and cannot import internal/chat.
func claimantRef(c chat.Claimant) agentopsv1alpha1.ClaimantRef {
	kind := agentopsv1alpha1.ClaimantKindPipeline
	if c.ClaimantKind() == chat.ClaimantCoordinator {
		kind = agentopsv1alpha1.ClaimantKindCoordinator
	}
	return agentopsv1alpha1.ClaimantRef{Kind: kind, Name: c.GetName()}
}

func chatKind(k agentopsv1alpha1.ClaimantKind) chat.ClaimantKind {
	if k == agentopsv1alpha1.ClaimantKindCoordinator {
		return chat.ClaimantCoordinator
	}
	return chat.ClaimantPipeline
}

// routeClaimants attempts every claimant still Routing: a claimant that no
// longer exists is Skipped, a refused create leaves it Routing, anything else
// is Routed onto its conversation. A terminal claimant is never touched.
func routeClaimants(ctx context.Context, deps Deps, sig *agentopsv1alpha1.Signal) error {
	for i := range sig.Status.Claimants {
		c := &sig.Status.Claimants[i]
		if c.State != agentopsv1alpha1.ClaimantRouting {
			continue
		}
		claimant, err := chat.ClaimantByKind(ctx, deps.Reader, deps.Namespace, chatKind(c.Ref.Kind), c.Ref.Name)
		if err != nil {
			return fmt.Errorf("read claimant %s/%s: %w", c.Ref.Kind, c.Ref.Name, err)
		}
		if claimant == nil {
			c.State = agentopsv1alpha1.ClaimantSkipped
			c.Reason = "claimant no longer exists"
			continue
		}
		convRef, waiting, err := routeOneClaimant(ctx, deps, claimant, sig)
		if err != nil {
			return fmt.Errorf("route claimant %s/%s: %w", c.Ref.Kind, c.Ref.Name, err)
		}
		if waiting {
			continue // backlog full: stays Routing, requeued with a delay
		}
		c.State = agentopsv1alpha1.ClaimantRouted
		c.ConversationRef = convRef
	}
	return nil
}

// BacklogFull reports whether the pending backlog has reached its bound.
// Only PENDING conversations count: an admitted one holds a pod (or is about
// to) and is already bounded by MAX_ACTIVE_CONVERSATIONS.
func BacklogFull(ctx context.Context, reader client.Reader, namespace string, maxQueued int) (bool, error) {
	var list agentopsv1alpha1.ConversationList
	if err := reader.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return false, err
	}
	n := 0
	for i := range list.Items {
		if list.Items[i].Status.Phase == agentopsv1alpha1.ConversationPending {
			n++
		}
	}
	return n >= maxQueued, nil
}
