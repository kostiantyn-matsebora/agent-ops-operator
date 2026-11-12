// Coordination (design coordinated-agents, D-B through D-E2): a Coordinator's
// own conversation invoking `agents[]` entries as members, a member's result
// routing back to its parent, cascading close, and escalation.
//
// Lives beside the rest of the router rather than in httpapi: it needs the
// same Client/Reader/Ops/Activity/Runtime the router already carries, and
// closing or escalating a conversation is the same "say goodbye, then write
// the status" shape closeConversation already owns.
package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/dispatch"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/namewords"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// InvokeResult is what `invoke` reports (aops-mcp-server: "created or
// attached"), never a result — invocation is asynchronous by design.
type InvokeResult struct {
	Member  string
	Created bool
}

// errs a coordinated verb refuses with, named so the HTTP layer can tell a
// scope refusal from a not-found from an internal error.
var (
	ErrNotCoordinatorRoot = fmt.Errorf("caller is not a Coordinator's own conversation")
	ErrUnknownAgent       = fmt.Errorf("no such agents[] entry")
	ErrCoordinatorCycle   = fmt.Errorf("would repeat a Coordinator already in this coordination")
	ErrMaxAgents          = fmt.Errorf("maxAgents reached")
	ErrOutOfScope         = fmt.Errorf("out of scope: not the caller itself, a direct member, or an open root of the caller's own Coordinator")
	// ErrOutOfSubtree refuses `read` and `get_tree` — narrower than
	// ErrOutOfScope. Those two verbs' bound is the caller's own subtree
	// only (aops-mcp-server: "read | the calling conversation's own
	// subtree, at any depth"), with NO coordinator-sibling widening —
	// unlike `close`, which ErrOutOfScope's wording correctly describes.
	// aops-mcp-server also requires "A refusal SHALL reach the caller as
	// an error naming the bound that refused it" — reusing ErrOutOfScope's
	// wider wording here told a caller a coordinator-sibling root WAS in
	// scope for read when it never has been. Measured live: the self-heal
	// reaper's list_open_roots returned sibling roots, read refused every
	// one of them, and the message's own "...or an open root of the
	// caller's own Coordinator" read as though it should have worked —
	// the agent concluded the roots must belong to a DIFFERENT Coordinator
	// instead, which was simply wrong.
	ErrOutOfSubtree = fmt.Errorf("out of scope: not the caller itself or within its own subtree")
	// ErrCannotCloseHumanRoot refuses the widened Coordinator-owner close
	// bound for a target isHumanInitiated reports true for — a distinct,
	// named reason rather than the generic ErrOutOfScope, per
	// aops-mcp-server's "a refusal SHALL reach the caller as an error
	// naming the bound that refused it."
	ErrCannotCloseHumanRoot = fmt.Errorf("out of scope: a conversation a person started is closed only by that person, never by the Coordinator")
	// ErrNoCoordinatorScope refuses the Coordinator-owner reach class
	// (list_open_roots, the widened close — coordinator-owner-reach) for a
	// caller that resolves to no Coordinator at all by ResolveActingCoordinator:
	// an ordinary Pipeline-addressed conversation has no access to this reach
	// class, whatever AgentCapability it runs.
	ErrNoCoordinatorScope = fmt.Errorf("caller does not act for any Coordinator")
	// ErrCallerClosed refuses `invoke` from a Closed caller. Closed is inert:
	// a close walks its subtree before the ancestor and again after it, and
	// that second sweep is only the last word if nothing more can be created
	// under a conversation that has closed.
	ErrCallerClosed = fmt.Errorf("refused: the calling conversation is closed")
	// ErrSelfInput guards the rule that a conversation must never receive its
	// own output as an input (coordination-loop) — an inbound message whose
	// origin channel is named identically to its own target conversation.
	ErrSelfInput = fmt.Errorf("refused: this input's origin is its own target conversation")
)

// InvokeMember is the manager's half of the MCP `invoke(agent, task)` verb
// (design D-F): resolve the named `agents[]` entry against the Coordinator
// the CALLER acts for, refuse a cycle or a spent budget, then create or
// attach a member conversation and hand it the task.
//
// The caller resolves which Coordinator it acts for the SAME way the
// Coordinator-owner reach class does (coordinator-owner-reach, design D-A,
// ResolveActingCoordinator): its own `coordinatorRef` when it is itself a
// Coordinator's root, or — failing that — the uncaused root its `causedBy`
// chain leads to. This is what lets an ORDINARY `agents[]` member invoke a
// sibling entry of its own Coordinator, which `coordinator-self-heal`
// requires of the reaper: it is invoked as a plain member of the
// cron-triggered root, and must itself call `invoke` to re-check a domain
// capability, never merely the coordinating root. A caller that resolves to
// no Coordinator at all (an ordinary Pipeline-addressed conversation) is
// refused exactly as before.
func (r *Router) InvokeMember(ctx context.Context, caller *state.ConversationRecord, agentName, task string) (*InvokeResult, error) {
	if caller.Phase == state.PhaseClosed {
		return nil, ErrCallerClosed
	}
	scope, err := r.ResolveActingCoordinator(ctx, caller)
	if err != nil {
		return nil, err
	}
	if scope.Name == "" {
		return nil, ErrNotCoordinatorRoot
	}
	var co agentopsv1alpha1.Coordinator
	if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: scope.Name}, &co); err != nil {
		return nil, err
	}
	entry := findAgentEntry(&co, agentName)
	if entry == nil {
		return nil, ErrUnknownAgent
	}
	if err := r.refuseCycleOrSpentBudget(ctx, caller, entry); err != nil {
		return nil, err
	}

	// The member's correlation id, minted by THIS invoke — the sender of the
	// work, per the one id-ownership rule every lane obeys
	// (conversation-provenance: "a member's correlation id is minted by the
	// invoking invoke verb... never computed by the manager"). Scoped to
	// (parent, entry) by construction, so a different entry or a different
	// caller never collides on it.
	initiationID := "invoke:" + entry.Name
	member, err := r.findReusableMember(ctx, caller.Name, entry.Name, initiationID)
	if err != nil {
		return nil, err
	}
	if member != nil {
		if err := r.appendInputIdempotent(ctx, member.Name, state.InputItem{
			ID: newInputID(), Type: state.InputTask, Payload: task, ReceivedAt: time.Now(),
			Origin: &state.InputOrigin{Kind: state.OriginMember, Name: caller.Name, Entry: entry.Name},
		}); err != nil {
			return nil, err
		}
		return &InvokeResult{Member: member.Name, Created: false}, nil
	}

	created, err := r.createMember(ctx, caller, &co, entry, task, initiationID)
	if err != nil {
		return nil, err
	}
	if err := r.bumpAgentsInvoked(ctx, caller.Name); err != nil {
		return nil, err
	}
	if err := r.closeMemberIfCallerClosed(ctx, caller.Name, created); err != nil {
		return nil, err
	}
	return &InvokeResult{Member: created.Name, Created: true}, nil
}

// closeMemberIfCallerClosed is the other half of ErrCallerClosed. The check at
// the top of InvokeMember reads the caller BEFORE the member exists, so a close
// that lands between that read and the member's creation would leave one open
// member under a closed ancestor.
//
// Re-reading the caller AFTER the member exists settles it either way. If the
// close landed first, this sees Closed and closes the member. If it lands
// later, its second sweep runs after the close write, which is after this
// read, which is after the member was created, so the sweep finds it.
func (r *Router) closeMemberIfCallerClosed(ctx context.Context, callerName string, member *state.ConversationRecord) error {
	fresh, _, err := r.Conversations.Get(ctx, state.Key(callerName))
	if err != nil {
		return state.IgnoreNotFound(err)
	}
	if fresh.Phase != state.PhaseClosed {
		return nil
	}
	reason := fresh.CloseReason
	if reason == "" {
		reason = "the calling conversation closed"
	}
	if err := r.closeConversationReason(ctx, member, reason); err != nil {
		return err
	}
	return ErrCallerClosed
}

// findAgentEntry returns the named `agents[]` entry, or nil.
func findAgentEntry(co *agentopsv1alpha1.Coordinator, name string) *agentopsv1alpha1.CoordinatorAgentEntry {
	for i := range co.Spec.Agents {
		if co.Spec.Agents[i].Name == name {
			return &co.Spec.Agents[i]
		}
	}
	return nil
}

// refuseCycleOrSpentBudget is InvokeMember's two refusals: an entry naming a
// Coordinator already in the caller's chain, and a spent maxAgents budget
// (which also closes the caller budget-exceeded).
func (r *Router) refuseCycleOrSpentBudget(ctx context.Context, caller *state.ConversationRecord, entry *agentopsv1alpha1.CoordinatorAgentEntry) error {
	if entry.CoordinatorRef != nil {
		chain, err := r.coordinatorChain(ctx, caller)
		if err != nil {
			return err
		}
		if chain[entry.CoordinatorRef.Name] {
			return ErrCoordinatorCycle
		}
	}
	if budget := caller.Budget; budget != nil && budget.MaxAgents > 0 && budget.AgentsInvoked >= budget.MaxAgents {
		digest := fmt.Sprintf("budget-exceeded: maxAgents (%d) reached", budget.MaxAgents)
		if err := r.CloseBudgetExceeded(ctx, caller, digest); err != nil {
			return fmt.Errorf("%w, and closing on it failed: %v", ErrMaxAgents, err)
		}
		return ErrMaxAgents
	}
	return nil
}

// CoordinatorAgents is the manager's half of the MCP `list_agents` tool
// (design D-F, aops-mcp-server): the calling conversation's own Coordinator's
// `agents[]` entries. The caller must itself be a Coordinator's own
// conversation, exactly as InvokeMember requires — listing agents is a
// capability of the coordinating agent's own conversation.
func (r *Router) CoordinatorAgents(ctx context.Context, caller *state.ConversationRecord) ([]agentopsv1alpha1.CoordinatorAgentEntry, error) {
	if caller.CoordinatorRef == "" {
		return nil, ErrNotCoordinatorRoot
	}
	var co agentopsv1alpha1.Coordinator
	if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: caller.CoordinatorRef}, &co); err != nil {
		return nil, err
	}
	return co.Spec.Agents, nil
}

// Descendants lists every conversation in rootName's own subtree, at any
// depth, DOWNWARD only — never rootName's ancestors or their other branches
// (aops-mcp-server's `get_tree` bound). BFS by LabelCausedBy, each hit
// verified against the CausedBy field itself — the label is a hint, the same
// pattern cascadeCloseMembers already uses to close a subtree. Bounded
// defensively against a malformed causedBy graph, exactly as descendsFrom is.
func (r *Router) Descendants(ctx context.Context, rootName string) ([]state.ConversationRecord, error) {
	var out []state.ConversationRecord
	seen := map[string]bool{rootName: true}
	frontier := []string{rootName}
	for i := 0; i < 1000 && len(frontier) > 0; i++ {
		var next []string
		for _, parent := range frontier {
			members, err := r.Conversations.List(ctx, state.ByCausedBy, parent)
			if err != nil {
				return nil, err
			}
			for j := range members {
				m := members[j]
				if m.CausedBy == nil || m.CausedBy.Parent != parent || seen[m.Name] {
					continue // the index is a hint; the field is the fact
				}
				seen[m.Name] = true
				out = append(out, m)
				next = append(next, m.Name)
			}
		}
		frontier = next
	}
	return out, nil
}

// causedByAncestry walks `causedBy` from conv up to and including the
// UNCAUSED root, returning every conversation visited in order — conv itself
// first, the uncaused root last. Shared by the cycle guard (coordinatorChain)
// and the Coordinator-owner resolution walk (design D-A,
// ResolveActingCoordinator) — one traversal, two different questions asked of
// its result. Bounded defensively: the ordinary budgets already keep a live
// chain finite, so a walk this long means something else is wrong.
func (r *Router) causedByAncestry(ctx context.Context, conv *state.ConversationRecord) ([]*state.ConversationRecord, error) {
	chain := []*state.ConversationRecord{conv}
	cur := conv
	for i := 0; i < 1000; i++ {
		if cur.CausedBy == nil {
			return chain, nil
		}
		parent, _, err := r.Conversations.Get(ctx, state.Key(cur.CausedBy.Parent))
		if err != nil {
			return nil, err
		}
		chain = append(chain, &parent)
		cur = &parent
	}
	return nil, fmt.Errorf("causedBy chain from %s did not terminate", conv.Name)
}

// coordinatorChain collects the calling conversation's OWN coordinatorRef,
// then every ancestor's, walking `causedBy` to the uncaused root (design
// D-E2).
func (r *Router) coordinatorChain(ctx context.Context, conv *state.ConversationRecord) (map[string]bool, error) {
	ancestry, err := r.causedByAncestry(ctx, conv)
	if err != nil {
		return nil, err
	}
	visited := map[string]bool{}
	for _, c := range ancestry {
		if c.CoordinatorRef != "" {
			visited[c.CoordinatorRef] = true
		}
	}
	return visited, nil
}

// ActingCoordinator is what the Coordinator-owner reach class resolves a
// caller into (design D-A, coordinator-owner-reach): which Coordinator it
// acts for, and the name of its own ROOT — itself when it carries no
// causedBy, or the uncaused root its causedBy chain leads to otherwise. Both
// fields are empty together when the caller resolves to no Coordinator at
// all — an ordinary Pipeline-addressed conversation.
type ActingCoordinator struct {
	// Name is the Coordinator this caller acts for, "" when it acts for none.
	Name string
	// RootName is the caller's own uncaused root — itself when it IS one.
	// Always "" alongside Name == "".
	RootName string
}

// ResolveActingCoordinator is the resolution walk design D-A specifies,
// REUSING causedByAncestry — the exact traversal the invoke-time cycle guard
// (coordinatorChain) already performs — rather than a new walk of its own.
//
// It reads the caller's OWN coordinatorRef first. If empty, it walks
// causedBy to the UNCAUSED root and reads THAT root's coordinatorRef
// instead. This is a read of existing fields, never a new one.
//
// runtimepod.ResolveActingCoordinator DUPLICATES the Name half of this (see
// that function's doc comment for why): a MEMBER conversation — one with no
// coordinatorRef of its own, like the self-heal reaper — still needs a
// usable `coordinator:<name>:<conversation>` token in its own pod, derived
// at pod-build time before any caller has presented anything for this
// function to authenticate.
func (r *Router) ResolveActingCoordinator(ctx context.Context, conv *state.ConversationRecord) (ActingCoordinator, error) {
	if conv.CausedBy == nil {
		if conv.CoordinatorRef == "" {
			return ActingCoordinator{}, nil
		}
		return ActingCoordinator{Name: conv.CoordinatorRef, RootName: conv.Name}, nil
	}
	ancestry, err := r.causedByAncestry(ctx, conv)
	if err != nil {
		return ActingCoordinator{}, err
	}
	root := ancestry[len(ancestry)-1]
	name := ""
	switch {
	case conv.CoordinatorRef != "":
		name = conv.CoordinatorRef
	case root.CoordinatorRef != "":
		name = root.CoordinatorRef
	}
	if name == "" {
		return ActingCoordinator{}, nil
	}
	return ActingCoordinator{Name: name, RootName: root.Name}, nil
}

// OpenRoot is one row of `list_open_roots`' projection (coordinator-owner-
// reach): the same bounded shape `list_conversations` already returns, plus
// Members — the agents[] entry names of this root's own direct members,
// which is what the reaper re-invokes to re-check. Never a transcript or a
// run.
type OpenRoot struct {
	Name    string
	Title   string
	Brief   string
	Phase   string
	Members []string
}

// ListOpenRoots is the manager's half of the MCP `list_open_roots()` verb
// (coordinator-owner-reach, design D-A/D-B): every OPEN, UNCAUSED
// conversation whose coordinatorRef names the SAME Coordinator the caller
// acts for — never a member (whatever Coordinator it carries), never a
// different Coordinator's root, and never the caller's own root, since
// closing it would cascade to close the caller's own conversation mid-run.
//
// ALSO EXCLUDED: an Idle root with NO members. The reaper's whole method is
// "re-check each one through the agent that originally handled it" — a root
// that finished its last run (Idle, not Working or still admitting) and never
// produced a single member never had an agent to re-check through, so there
// is nothing here that re-checking could ever find changed. Left in, this is
// not a stale edge case: a root whose first run failed before it could invoke
// anyone (an auth error, a missing tool) surfaces forever, identically, every
// hourly cycle — measured live on job-cb5vg, 96 consecutive cycles restating
// one 2026-10-03 run's error with zero new information, because the member
// that would have let the reaper confirm a fix never existed to re-check.
func (r *Router) ListOpenRoots(ctx context.Context, caller *state.ConversationRecord) ([]OpenRoot, error) {
	scope, err := r.ResolveActingCoordinator(ctx, caller)
	if err != nil {
		return nil, err
	}
	if scope.Name == "" {
		return nil, ErrNoCoordinatorScope
	}
	list, err := r.Conversations.List(ctx, state.ByAll, "")
	if err != nil {
		return nil, err
	}
	var out []OpenRoot
	for i := range list {
		c := &list[i]
		if !isOpenSiblingRoot(c, scope) {
			continue
		}
		members, err := r.directMemberEntries(ctx, c.Name)
		if err != nil {
			return nil, err
		}
		if c.Phase == state.PhaseIdle && len(members) == 0 {
			continue // nothing this root ever handed off to — nothing to re-check
		}
		out = append(out, OpenRoot{
			Name: c.Name, Title: c.Title, Brief: c.Brief, Phase: c.Phase, Members: members,
		})
	}
	return out, nil
}

// isOpenSiblingRoot reports whether c is an open, uncaused root of the
// acting Coordinator other than the caller's own.
func isOpenSiblingRoot(c *state.ConversationRecord, scope ActingCoordinator) bool {
	if c.CausedBy != nil {
		return false // a member — never an uncaused root, whatever it carries
	}
	if c.CoordinatorRef == "" || c.CoordinatorRef != scope.Name {
		return false // not this Coordinator's own root
	}
	if c.Name == scope.RootName {
		return false // the caller's own root
	}
	return c.Phase != state.PhaseClosed
}

// directMemberEntries lists the agents[] entry names of rootName's own DIRECT
// members only — one hop, never a grandchild — deduplicated so an entry
// reattached to more than once (or re-invoked after its first member closed)
// is named once. The reaper re-invokes exactly these entries to re-check them.
func (r *Router) directMemberEntries(ctx context.Context, rootName string) ([]string, error) {
	list, err := r.Conversations.List(ctx, state.ByCausedBy, rootName)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for i := range list {
		m := &list[i]
		if m.CausedBy == nil || m.CausedBy.Parent != rootName || seen[m.CausedBy.Entry] {
			continue // the index is a hint; the field is the fact
		}
		seen[m.CausedBy.Entry] = true
		out = append(out, m.CausedBy.Entry)
	}
	return out, nil
}

// isOwnCoordinatorSiblingRoot is `close`'s WIDENED bound (design D-B,
// aops-mcp-server): a caller that resolves to a Coordinator may close any
// OPEN, UNCAUSED root of that SAME Coordinator, other than its own ancestor
// root — never a member, however shallow (even one inside the caller's own
// Coordinator's tree — the ordinary one-hop bound already owns that case),
// and never a root of a different Coordinator.
func (r *Router) isOwnCoordinatorSiblingRoot(ctx context.Context, caller, target *state.ConversationRecord) (bool, error) {
	if target.CausedBy != nil {
		return false, nil // a member is never a target of this reach class
	}
	if target.CoordinatorRef == "" {
		return false, nil
	}
	scope, err := r.ResolveActingCoordinator(ctx, caller)
	if err != nil {
		return false, err
	}
	if scope.Name == "" || target.CoordinatorRef != scope.Name {
		return false, nil // no Coordinator to act for, or a different Coordinator's root
	}
	if target.Name == scope.RootName {
		return false, nil // the caller's own ancestor root
	}
	return true, nil
}

// isHumanInitiated reports whether a conversation exists because a PERSON
// asked for it — an addressed `/<pipeline> <task>` command (no spec.signal
// at all) or a bare chat message (spec.signal present but carrying the chat
// lane's own LabelChatChannel label) — as opposed to an alert, a job or any
// other machine signal, which always carries spec.signal with neither mark.
//
// CloseCoordinated uses this to refuse ever auto-closing a person's own
// request under the widened Coordinator-owner reach: the self-heal reaper
// may close a sibling root it finds healed, but never one somebody is still
// waiting on an answer to, however long it has sat open. This is a
// mechanical guarantee rather than a prompt instruction on purpose — the
// reaper's own task text already said "not a new incident, re-check each
// one", and that alone did not stop it from reading a human-originated root
// as fair game.
func isHumanInitiated(c *state.ConversationRecord) bool {
	if c.Signal == nil {
		return true
	}
	_, isChat := c.Signal.Labels[agentopsv1alpha1.LabelChatChannel]
	return isChat
}

// findReusableMember is conversation-provenance's reuse rule: a live
// conversation with the same conversationId AND the same (parent, entry) —
// depth plays no part. Reads the same conversationId index equality routing
// does (state.ByConversationID), never the removed
// agentops.dev/signature-hash label.
func (r *Router) findReusableMember(ctx context.Context, parentName, entryName, initiationID string) (*state.ConversationRecord, error) {
	list, err := r.Conversations.List(ctx, state.ByConversationID, initiationID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		c := &list[i]
		if c.Phase == state.PhaseClosed {
			continue
		}
		if c.CausedBy != nil && c.CausedBy.Parent == parentName && c.CausedBy.Entry == entryName {
			return c, nil
		}
	}
	return nil, nil
}

// createMember opens a new member conversation for one `agents[]` entry,
// resolving its capability whether the entry names an AgentCapability or nests
// another Coordinator — mirroring internal/signalroute's own createConversation
// wiring-snapshot shape, with CausedBy in place of Signal/PipelineRef and no
// channels bound (coordination-loop: "a caused conversation binds no
// channel").
func (r *Router) createMember(ctx context.Context, caller *state.ConversationRecord, co *agentopsv1alpha1.Coordinator,
	entry *agentopsv1alpha1.CoordinatorAgentEntry, task, initiationID string) (*state.ConversationRecord, error) {

	var capability agentopsv1alpha1.AgentCapabilitySpec
	var persistenceName string
	var nested *agentopsv1alpha1.Coordinator
	switch {
	case entry.CapabilityRef != nil:
		var ac agentopsv1alpha1.AgentCapability
		if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: entry.CapabilityRef.Name}, &ac); err != nil {
			return nil, fmt.Errorf("agentcapability/%s: %w", entry.CapabilityRef.Name, err)
		}
		capability, persistenceName = ac.Spec, entry.CapabilityRef.Name
	case entry.CoordinatorRef != nil:
		var n agentopsv1alpha1.Coordinator
		if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: entry.CoordinatorRef.Name}, &n); err != nil {
			return nil, fmt.Errorf("coordinator/%s: %w", entry.CoordinatorRef.Name, err)
		}
		nested = &n
		resolved, err := dispatch.ResolveCapability(ctx, r.Reader, coordinatorClaimant{nested})
		if err != nil {
			return nil, fmt.Errorf("coordinator/%s: resolve capability: %w", entry.CoordinatorRef.Name, err)
		}
		capability, persistenceName = resolved, entry.CoordinatorRef.Name
	default:
		return nil, fmt.Errorf("agents[%s] names neither capabilityRef nor coordinatorRef", entry.Name)
	}

	snap := runtimepod.SnapshotFor(ctx, r.Reader, r.Namespace, persistenceName, capability, r.Runtime)
	// The word-chain and the actual Create are AllocateName's below —
	// readable-conversation-names: an object name is a deterministic
	// word-chain, never metadata.generateName's random suffix. The word
	// source is the invoked agents[] entry name, non-empty by
	// construction (a CoordinatorAgentEntry always names one).
	member := &state.ConversationRecord{
		ProfileRef:         capability.ProfileName(),
		Toolsets:           dispatch.ToolsetBindingOf(capability.Toolsets),
		MCPConfigs:         dispatch.ToolingBindingOf(capability.MCPConfigs),
		RuntimeRef:         snap.RuntimeRef,
		ServiceAccountName: snap.ServiceAccountName,
		ContextClaimName:   snap.ContextClaimName,
		WorkspaceClaimName: snap.WorkspaceClaimName,
		CausedBy:           &state.Provenance{Parent: caller.Name, Entry: entry.Name},
		Title:              memberTitle(entry.Name, task),
		ConversationID:     initiationID,
	}
	if nested != nil {
		member.CoordinatorRef = entry.CoordinatorRef.Name
		claimant := coordinatorClaimant{nested}
		member.EscalationChannelRefs = claimant.EscalationChannelRefs()
		// The member's own resource ceiling is status, which a record carries
		// beside the spec, so it rides the one Create.
		member.Budget = claimant.SnapshotBudget()
	}
	if _, err := namewords.AllocateName(ctx, r.Conversations, "member", namewords.Words(entry.Name), member); err != nil {
		return nil, err
	}
	if err := r.appendInputIdempotent(ctx, member.Name, state.InputItem{
		ID: newInputID(), Type: state.InputTask, Payload: task, ReceivedAt: time.Now(),
		Origin: &state.InputOrigin{Kind: state.OriginMember, Name: caller.Name, Entry: entry.Name},
	}); err != nil {
		return nil, err
	}
	r.Activity.Emit(activity.Event{
		Kind:         activity.KindConversationCreated,
		From:         activity.Node(activity.NodeConversation, caller.Name),
		To:           activity.Node(activity.NodeConversation, member.Name),
		Conversation: member.Name,
		Detail:       "invoked as " + entry.Name,
	})
	return member, nil
}

// bumpAgentsInvoked increments a Coordinator-rooted conversation's own
// AgentsInvoked count under optimistic concurrency (design D-E). Counted only
// on an actual new member: reattaching to one already live is not inviting a
// new agent.
func (r *Router) bumpAgentsInvoked(ctx context.Context, convName string) error {
	_, err := state.Mutate(ctx, r.Conversations, state.Key(convName), func(rec *state.ConversationRecord) error {
		if rec.Budget != nil { // nil: not itself a Coordinator's root, nothing to bump
			rec.Budget.AgentsInvoked++
		}
		return nil
	})
	return err
}

// AppendMemberResult routes one member's result onto its immediate PARENT,
// one hop, never the tree's ultimate root (coordination-loop). dedupID is the
// caller's idempotency key (e.g. "member:<conversation>:<runId>") — a repeat
// append with the SAME id while it is still pending is a no-op; the durable
// per-run marker is the caller's job (RunStatus.RoutedToParent), since once
// this input is processed and pruned there is nothing left here to compare
// against.
func (r *Router) AppendMemberResult(ctx context.Context, causedBy *state.Provenance, memberName, dedupID, result string) error {
	if causedBy == nil || causedBy.Parent == memberName {
		return nil // no parent, or a provenance bug — never feed a conversation its own output
	}
	item := state.InputItem{
		ID: dedupID, Type: state.InputTask, Payload: result, ReceivedAt: time.Now(),
		Origin: &state.InputOrigin{Kind: state.OriginMember, Name: memberName, Entry: causedBy.Entry},
	}
	return r.appendInputIdempotent(ctx, causedBy.Parent, item)
}

// appendInputIdempotent appends one input to a NAMED conversation (not
// necessarily the caller's own object), skipping conversations already
// Closed (coordination-loop: "a closed parent receives nothing") and skipping
// a repeat of an id already pending, with optimistic retry on conflict.
func (r *Router) appendInputIdempotent(ctx context.Context, convName string, item state.InputItem) error {
	_, err := state.Mutate(ctx, r.Conversations, state.Key(convName), func(rec *state.ConversationRecord) error {
		if rec.Phase == state.PhaseClosed {
			return nil
		}
		for i := range rec.Inputs {
			if rec.Inputs[i].ID == item.ID {
				return nil
			}
		}
		rec.Inputs = append(rec.Inputs, item)
		return nil
	})
	return state.IgnoreNotFound(err)
}

// CloseCoordinated is the manager's half of the MCP `close(conversation,
// reason)` verb (conversation-close, widened by coordinator-owner-reach): a
// reason is required, and the target must be the caller itself, a
// conversation it directly caused (ONE HOP — a deeper descendant is refused
// naming it out of scope; reaching one means asking the direct member to
// close it, whose own close cascades in turn), OR — only when the caller
// RESOLVES to a Coordinator (design D-A) — any OTHER open, uncaused root of
// that SAME Coordinator (design D-B) — EXCEPT one isHumanInitiated reports
// true for, refused with ErrCannotCloseHumanRoot regardless of how long it
// has sat open: a person's own request is never auto-closed by the widened
// reach, only by that person, a direct cascade from their own conversation,
// or an operator.
func (r *Router) CloseCoordinated(ctx context.Context, caller *state.ConversationRecord, targetName, reason string) error {
	if reason == "" {
		return fmt.Errorf("a coordinator's close requires a reason")
	}
	if targetName == caller.Name {
		// A person's own uncaused root is closed by that person (/close), a
		// cascade from it, or an operator — never by the Coordinator's own
		// verb, however finished it thinks the answer is.
		if caller.CausedBy == nil && isHumanInitiated(caller) {
			return ErrCannotCloseHumanRoot
		}
		return r.closeWithCascade(ctx, caller, reason)
	}
	target, _, err := r.Conversations.Get(ctx, state.Key(targetName))
	if err != nil {
		return state.IgnoreNotFound(err)
	}
	if target.CausedBy != nil && target.CausedBy.Parent == caller.Name {
		return r.closeWithCascade(ctx, &target, reason)
	}
	ok, err := r.isOwnCoordinatorSiblingRoot(ctx, caller, &target)
	if err != nil {
		return err
	}
	if !ok {
		return ErrOutOfScope
	}
	if isHumanInitiated(&target) {
		return ErrCannotCloseHumanRoot
	}
	return r.closeWithCascade(ctx, &target, reason)
}

// CloseBudgetExceeded is design D-E's one closing path for every budget edge
// (maxAgents at invoke, maxTurns after a run, deadline on reconcile): close
// every live member first, then escalate THIS conversation — never a
// separate close on it, since escalate already performs the one of two
// outcomes that applies (D-D).
func (r *Router) CloseBudgetExceeded(ctx context.Context, conv *state.ConversationRecord, digest string) error {
	if err := r.cascadeCloseMembers(ctx, conv.Name, "budget-exceeded"); err != nil {
		return err
	}
	return r.Escalate(ctx, conv, digest)
}

// closeWithCascade closes a conversation and every LIVE conversation it
// caused, same reason verbatim, CHILDREN FIRST (conversation-close, design
// D-H): the `causedBy` subtree is walked recursively — a member's own members
// before that member, since a member may itself be a root — and the
// conversation itself closes LAST.
//
// Children go first because the walk can fail part-way. Returning on the
// first failure then leaves an OPEN, still-reachable ancestor over a partly
// closed subtree, and a repeated close resumes from the members still open.
// The old order — ancestor first — left a closed root over open members that
// nothing could reach any more.
func (r *Router) closeWithCascade(ctx context.Context, conv *state.ConversationRecord, reason string) error {
	if err := r.cascadeCloseMembers(ctx, conv.Name, reason); err != nil {
		return err
	}
	if err := r.closeConversationReason(ctx, conv, reason); err != nil {
		return err
	}
	// A run on the still-open ancestor may have invoked a member after the walk
	// listed. A closed conversation can invoke nothing more, so this second
	// sweep is the last word and finds only stragglers.
	return r.cascadeCloseMembers(ctx, conv.Name, reason)
}

// cascadeCloseMembers closes every LIVE direct member of parentName, same
// reason verbatim, each recursively closing its own members before itself.
// Returns on the first failure, before any later sibling and before the
// parent — see closeWithCascade for why. An already-Closed member keeps its
// own reason and is skipped.
func (r *Router) cascadeCloseMembers(ctx context.Context, parentName, reason string) error {
	list, err := r.Conversations.List(ctx, state.ByCausedBy, parentName)
	if err != nil {
		return err
	}
	for i := range list {
		member := &list[i]
		if member.Phase == state.PhaseClosed {
			continue
		}
		if member.CausedBy == nil || member.CausedBy.Parent != parentName {
			continue // the index is a hint; the field is the fact
		}
		if err := r.closeWithCascade(ctx, member, reason); err != nil {
			return err
		}
	}
	return nil
}

// closeConversationReason is closeConversation plus a stamped CloseReason —
// required from a coordinator's own close (conversation-close), absent from
// an ordinary `/close`.
func (r *Router) closeConversationReason(ctx context.Context, conv *state.ConversationRecord, reason string) error {
	if conv.Phase == state.PhaseClosed {
		return nil
	}
	farewell := fmt.Sprintf("👋 Conversation closed (%s). This thread is archived — reply here "+
		"and I will start a fresh conversation, or reopen this one from the console to continue "+
		"with its history.", reason)
	r.eachBoundThread(ctx, conv, "", func(ch *agentopsv1alpha1.Channel, tid *string) {
		r.Ops.EnqueueFarewell(ctx, ch, conv, tid, Notice(farewell))
	})
	now := time.Now()
	return state.IgnoreNotFound(state.Patch(ctx, r.Conversations, conv, func(rec *state.ConversationRecord) {
		rec.Phase = state.PhaseClosed
		rec.ClosedAt = &now
		rec.CloseReason = boundedString(reason, agentopsv1alpha1.MaxCloseReason)
	}))
}

// Escalate is the MCP `escalate(message)` verb (design D-D,
// coordination-escalation, superseded by coordinator-unconditional-channels):
// an UNCAUSED conversation already has its channels bound — at creation, same
// as a Pipeline's — so escalating no longer BINDS anything. It posts message
// as a notice into every thread already open, and stamps `status.escalatedAt`
// so the agent's decision is recorded. A CAUSED one still opens no thread at
// all — it closes with message as its reason and its result, which reaches
// its OWN parent as an ordinary member-result input, bubbling one hop at a
// time until a call reaches the uncaused root.
//
// The caused branch closes through `closeWithCascade`, never the bare
// `closeConversationReason`: a member MAY ITSELF BE A ROOT
// (terminology.md) — a conversation can carry both `causedBy` and its own
// `coordinatorRef` with live `agents[]` invocations. Closing it with no
// cascade would orphan its own sub-members the moment it escalated itself,
// exactly the gap `cascadeCloseMembers` exists to close everywhere else.
func (r *Router) Escalate(ctx context.Context, conv *state.ConversationRecord, message string) error {
	if conv.CausedBy != nil {
		if err := r.closeWithCascade(ctx, conv, message); err != nil {
			return err
		}
		// STABLE, not clock-based (async-command-bus design D-8): a member
		// escalates at most once in its lifetime — closeWithCascade above
		// makes it inert the moment this runs — so the append id needs
		// nothing finer than the member's own name to stay unique, and
		// unlike time.Now().UnixNano() it is IDENTICAL on a redelivery of
		// this same call (a retried /work/done report, a re-claimed
		// commandbus task), which is what makes appendInputIdempotent's
		// dedup actually dedup rather than append the result twice. The bug
		// this replaces: the old id embedded time.Now(), so every redelivery
		// minted a fresh id and the dedup check never matched anything.
		return r.AppendMemberResult(ctx, conv.CausedBy, conv.Name, "escalate:"+conv.Name, message)
	}
	if conv.EscalatedAt != nil {
		return nil // already escalated: no replayed digest
	}
	now := time.Now()
	return state.Patch(ctx, r.Conversations, conv, func(rec *state.ConversationRecord) {
		rec.EscalatedAt = &now
		rec.EscalationMessage = boundedString(message, 2000)
	})
}

// memberTitle names a member conversation from the entry it was invoked as,
// plus the task's own words when there are any.
//
// NOT cut to Telegram's own topic-name shape here — length limits belong to
// the component that knows them (gotchas.md / invariants.md: "a manager-side
// fix would be one transport's limits imposed on all of them"). Telegram
// already enforces its own 128-character topic-name cap
// (`channels/telegram/telegram.go`, `telegramTopicLimit`); a 60-rune cut HERE
// was Telegram's constraint leaking into every other channel and the
// console's own list/chat views, cutting an alert's title mid-word for
// readers who never touch Telegram.
//
// Still bounded, by agentopsv1alpha1.MaxConversationTitle — a different
// bound for a different reason (see that constant's own comment):
// ConversationSpec.Title is an etcd-permanent field set from a task's own
// words, which has nothing to do with any one transport's render shape.
func memberTitle(entryName, task string) string {
	fields := strings.Fields(task)
	if len(fields) == 0 {
		return agentopsv1alpha1.BoundConversationTitle("🤝 " + entryName)
	}
	return agentopsv1alpha1.BoundConversationTitle("🤝 " + entryName + ": " + strings.Join(fields, " "))
}

func boundedString(s string, limit int) string {
	if len(s) > limit {
		return s[:limit]
	}
	return s
}
