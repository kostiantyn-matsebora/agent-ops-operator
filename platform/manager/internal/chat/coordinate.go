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
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/dispatch"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/ingest"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// labelSignatureHash MUST equal controller.LabelSignatureHash. Duplicated
// rather than imported: internal/controller already imports internal/chat for
// its own reconcilers, so the reverse import would cycle. Both sides are
// pinned by TestConversationSignatureHashLabelMatchesController.
const labelSignatureHash = "agentops.dev/signature-hash"

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
	// ErrNoCoordinatorScope refuses the Coordinator-owner reach class
	// (list_open_roots, the widened close — coordinator-owner-reach) for a
	// caller that resolves to no Coordinator at all by ResolveActingCoordinator:
	// an ordinary Pipeline-addressed conversation has no access to this reach
	// class, whatever AgentCapability it runs.
	ErrNoCoordinatorScope = fmt.Errorf("caller does not act for any Coordinator")
	// ErrSelfInput guards the rule that a conversation must never receive its
	// own output as an input (coordination-loop) — an inbound message whose
	// origin channel is named identically to its own target conversation.
	ErrSelfInput = fmt.Errorf("refused: this input's origin is its own target conversation")
)

// InvokeMember is the manager's half of the MCP `invoke(agent, task)` verb
// (design D-F): resolve the named `agents[]` entry against the CALLER's own
// Coordinator, refuse a cycle or a spent budget, then create or attach a
// member conversation and hand it the task.
//
// The caller MUST itself be a Coordinator's own conversation
// (`spec.coordinatorRef` set) — invoking is a capability of the coordinating
// agent's OWN conversation, never of an arbitrary one.
func (r *Router) InvokeMember(ctx context.Context, caller *agentopsv1alpha1.Conversation, agentName, task string) (*InvokeResult, error) {
	if caller.Spec.CoordinatorRef == nil {
		return nil, ErrNotCoordinatorRoot
	}
	var co agentopsv1alpha1.Coordinator
	if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: caller.Spec.CoordinatorRef.Name}, &co); err != nil {
		return nil, err
	}
	entry := findAgentEntry(&co, agentName)
	if entry == nil {
		return nil, ErrUnknownAgent
	}
	if err := r.refuseCycleOrSpentBudget(ctx, caller, entry); err != nil {
		return nil, err
	}

	signature := "invoke:" + entry.Name
	member, err := r.findReusableMember(ctx, caller.Name, entry.Name, signature)
	if err != nil {
		return nil, err
	}
	if member != nil {
		if err := r.appendInputIdempotent(ctx, member.Name, agentopsv1alpha1.InputItem{
			ID: newInputID(), Type: agentopsv1alpha1.InputTask, Payload: task, ReceivedAt: metav1.Now(),
			Origin: &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginMember, Name: caller.Name, Entry: entry.Name},
		}); err != nil {
			return nil, err
		}
		return &InvokeResult{Member: member.Name, Created: false}, nil
	}

	created, err := r.createMember(ctx, caller, &co, entry, task, signature)
	if err != nil {
		return nil, err
	}
	if err := r.bumpAgentsInvoked(ctx, caller.Name); err != nil {
		return nil, err
	}
	return &InvokeResult{Member: created.Name, Created: true}, nil
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
func (r *Router) refuseCycleOrSpentBudget(ctx context.Context, caller *agentopsv1alpha1.Conversation, entry *agentopsv1alpha1.CoordinatorAgentEntry) error {
	if entry.CoordinatorRef != nil {
		chain, err := r.coordinatorChain(ctx, caller)
		if err != nil {
			return err
		}
		if chain[entry.CoordinatorRef.Name] {
			return ErrCoordinatorCycle
		}
	}
	if budget := caller.Status.Budget; budget != nil && budget.MaxAgents > 0 && budget.AgentsInvoked >= budget.MaxAgents {
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
func (r *Router) CoordinatorAgents(ctx context.Context, caller *agentopsv1alpha1.Conversation) ([]agentopsv1alpha1.CoordinatorAgentEntry, error) {
	if caller.Spec.CoordinatorRef == nil {
		return nil, ErrNotCoordinatorRoot
	}
	var co agentopsv1alpha1.Coordinator
	if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: caller.Spec.CoordinatorRef.Name}, &co); err != nil {
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
func (r *Router) Descendants(ctx context.Context, rootName string) ([]agentopsv1alpha1.Conversation, error) {
	var out []agentopsv1alpha1.Conversation
	seen := map[string]bool{rootName: true}
	frontier := []string{rootName}
	for i := 0; i < 1000 && len(frontier) > 0; i++ {
		var next []string
		for _, parent := range frontier {
			var list agentopsv1alpha1.ConversationList
			if err := r.Reader.List(ctx, &list, client.InNamespace(r.Namespace),
				client.MatchingLabels{agentopsv1alpha1.LabelCausedBy: parent}); err != nil {
				return nil, err
			}
			for j := range list.Items {
				m := list.Items[j]
				if m.Spec.CausedBy == nil || m.Spec.CausedBy.Parent != parent || seen[m.Name] {
					continue // the label is a hint; the field is the fact
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
func (r *Router) causedByAncestry(ctx context.Context, conv *agentopsv1alpha1.Conversation) ([]*agentopsv1alpha1.Conversation, error) {
	chain := []*agentopsv1alpha1.Conversation{conv}
	cur := conv
	for i := 0; i < 1000; i++ {
		if cur.Spec.CausedBy == nil {
			return chain, nil
		}
		var parent agentopsv1alpha1.Conversation
		if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: cur.Spec.CausedBy.Parent}, &parent); err != nil {
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
func (r *Router) coordinatorChain(ctx context.Context, conv *agentopsv1alpha1.Conversation) (map[string]bool, error) {
	ancestry, err := r.causedByAncestry(ctx, conv)
	if err != nil {
		return nil, err
	}
	visited := map[string]bool{}
	for _, c := range ancestry {
		if c.Spec.CoordinatorRef != nil {
			visited[c.Spec.CoordinatorRef.Name] = true
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
func (r *Router) ResolveActingCoordinator(ctx context.Context, conv *agentopsv1alpha1.Conversation) (ActingCoordinator, error) {
	if conv.Spec.CausedBy == nil {
		if conv.Spec.CoordinatorRef == nil {
			return ActingCoordinator{}, nil
		}
		return ActingCoordinator{Name: conv.Spec.CoordinatorRef.Name, RootName: conv.Name}, nil
	}
	ancestry, err := r.causedByAncestry(ctx, conv)
	if err != nil {
		return ActingCoordinator{}, err
	}
	root := ancestry[len(ancestry)-1]
	name := ""
	switch {
	case conv.Spec.CoordinatorRef != nil:
		name = conv.Spec.CoordinatorRef.Name
	case root.Spec.CoordinatorRef != nil:
		name = root.Spec.CoordinatorRef.Name
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
	Phase   agentopsv1alpha1.ConversationPhase
	Members []string
}

// ListOpenRoots is the manager's half of the MCP `list_open_roots()` verb
// (coordinator-owner-reach, design D-A/D-B): every OPEN, UNCAUSED
// conversation whose coordinatorRef names the SAME Coordinator the caller
// acts for — never a member (whatever Coordinator it carries), never a
// different Coordinator's root, and never the caller's own root, since
// closing it would cascade to close the caller's own conversation mid-run.
func (r *Router) ListOpenRoots(ctx context.Context, caller *agentopsv1alpha1.Conversation) ([]OpenRoot, error) {
	scope, err := r.ResolveActingCoordinator(ctx, caller)
	if err != nil {
		return nil, err
	}
	if scope.Name == "" {
		return nil, ErrNoCoordinatorScope
	}
	var list agentopsv1alpha1.ConversationList
	if err := r.Reader.List(ctx, &list, client.InNamespace(r.Namespace)); err != nil {
		return nil, err
	}
	var out []OpenRoot
	for i := range list.Items {
		c := &list.Items[i]
		if c.Spec.CausedBy != nil {
			continue // a member — never an uncaused root, whatever it carries
		}
		if c.Spec.CoordinatorRef == nil || c.Spec.CoordinatorRef.Name != scope.Name {
			continue // not this Coordinator's own root
		}
		if c.Name == scope.RootName {
			continue // the caller's own root
		}
		if c.Status.Phase == agentopsv1alpha1.ConversationClosed {
			continue
		}
		members, err := r.directMemberEntries(ctx, c.Name)
		if err != nil {
			return nil, err
		}
		out = append(out, OpenRoot{
			Name: c.Name, Title: c.Spec.Title, Brief: c.Status.Brief, Phase: c.Status.Phase, Members: members,
		})
	}
	return out, nil
}

// directMemberEntries lists the agents[] entry names of rootName's own DIRECT
// members only — one hop, never a grandchild — deduplicated so an entry
// reattached to more than once (or re-invoked after its first member closed)
// is named once. The reaper re-invokes exactly these entries to re-check them.
func (r *Router) directMemberEntries(ctx context.Context, rootName string) ([]string, error) {
	var list agentopsv1alpha1.ConversationList
	if err := r.Reader.List(ctx, &list, client.InNamespace(r.Namespace),
		client.MatchingLabels{agentopsv1alpha1.LabelCausedBy: rootName}); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for i := range list.Items {
		m := &list.Items[i]
		if m.Spec.CausedBy == nil || m.Spec.CausedBy.Parent != rootName || seen[m.Spec.CausedBy.Entry] {
			continue // the label is a hint; the field is the fact
		}
		seen[m.Spec.CausedBy.Entry] = true
		out = append(out, m.Spec.CausedBy.Entry)
	}
	return out, nil
}

// isOwnCoordinatorSiblingRoot is `close`'s WIDENED bound (design D-B,
// aops-mcp-server): a caller that resolves to a Coordinator may close any
// OPEN, UNCAUSED root of that SAME Coordinator, other than its own ancestor
// root — never a member, however shallow (even one inside the caller's own
// Coordinator's tree — the ordinary one-hop bound already owns that case),
// and never a root of a different Coordinator.
func (r *Router) isOwnCoordinatorSiblingRoot(ctx context.Context, caller, target *agentopsv1alpha1.Conversation) (bool, error) {
	if target.Spec.CausedBy != nil {
		return false, nil // a member is never a target of this reach class
	}
	if target.Spec.CoordinatorRef == nil {
		return false, nil
	}
	scope, err := r.ResolveActingCoordinator(ctx, caller)
	if err != nil {
		return false, err
	}
	if scope.Name == "" || target.Spec.CoordinatorRef.Name != scope.Name {
		return false, nil // no Coordinator to act for, or a different Coordinator's root
	}
	if target.Name == scope.RootName {
		return false, nil // the caller's own ancestor root
	}
	return true, nil
}

// findReusableMember is conversation-provenance's reuse rule: a live
// conversation with the same signature AND the same (parent, entry) —
// depth plays no part.
func (r *Router) findReusableMember(ctx context.Context, parentName, entryName, signature string) (*agentopsv1alpha1.Conversation, error) {
	var list agentopsv1alpha1.ConversationList
	if err := r.Reader.List(ctx, &list, client.InNamespace(r.Namespace),
		client.MatchingLabels{labelSignatureHash: ingest.SignatureHash(signature)}); err != nil {
		return nil, err
	}
	for i := range list.Items {
		c := &list.Items[i]
		if c.Status.Phase == agentopsv1alpha1.ConversationClosed {
			continue
		}
		if c.Spec.CausedBy != nil && c.Spec.CausedBy.Parent == parentName && c.Spec.CausedBy.Entry == entryName {
			return c, nil
		}
	}
	return nil, nil
}

// createMember opens a new member conversation for one `agents[]` entry,
// resolving its capability whether the entry names an AgentCapability or nests
// another Coordinator — mirroring httpapi.createConversationForGroup's
// wiring-snapshot shape, with CausedBy in place of Signal/PipelineRef and no
// channels bound (coordination-loop: "a caused conversation binds no
// channel").
func (r *Router) createMember(ctx context.Context, caller *agentopsv1alpha1.Conversation, co *agentopsv1alpha1.Coordinator,
	entry *agentopsv1alpha1.CoordinatorAgentEntry, task, signature string) (*agentopsv1alpha1.Conversation, error) {

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
	member := &agentopsv1alpha1.Conversation{}
	member.Namespace = r.Namespace
	member.GenerateName = "member-"
	member.Labels = map[string]string{
		labelSignatureHash:             ingest.SignatureHash(signature),
		agentopsv1alpha1.LabelCausedBy: caller.Name,
	}
	member.Spec = agentopsv1alpha1.ConversationSpec{
		ProfileRef:         agentopsv1alpha1.ObjectRef{Name: capability.ProfileName()},
		Toolsets:           capability.Toolsets.DeepCopy(),
		MCPConfigs:         capability.MCPConfigs.DeepCopy(),
		RuntimeRef:         snap.RuntimeRef,
		ServiceAccountName: snap.ServiceAccountName,
		ContextClaimName:   snap.ContextClaimName,
		WorkspaceClaimName: snap.WorkspaceClaimName,
		CausedBy:           &agentopsv1alpha1.Provenance{Parent: caller.Name, Entry: entry.Name},
		Title:              memberTitle(entry.Name, task),
		Signature:          signature,
	}
	if nested != nil {
		member.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: entry.CoordinatorRef.Name}
		claimant := coordinatorClaimant{nested}
		member.Spec.EscalationChannelRefs = claimant.EscalationChannelRefs()
	}
	if err := r.Client.Create(ctx, member); err != nil {
		return nil, err
	}
	if nested != nil {
		if budget := (coordinatorClaimant{nested}).SnapshotBudget(); budget != nil {
			patch := client.MergeFrom(member.DeepCopy())
			member.Status.Budget = budget
			if err := r.Client.Status().Patch(ctx, member, patch); err != nil {
				return nil, err
			}
		}
	}
	if err := r.appendInputIdempotent(ctx, member.Name, agentopsv1alpha1.InputItem{
		ID: newInputID(), Type: agentopsv1alpha1.InputTask, Payload: task, ReceivedAt: metav1.Now(),
		Origin: &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginMember, Name: caller.Name, Entry: entry.Name},
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
	for attempt := 0; attempt < 5; attempt++ {
		var fresh agentopsv1alpha1.Conversation
		if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: convName}, &fresh); err != nil {
			return err
		}
		if fresh.Status.Budget == nil {
			return nil // not itself a Coordinator's root: nothing to bump
		}
		patch := client.MergeFrom(fresh.DeepCopy())
		fresh.Status.Budget.AgentsInvoked++
		if err := r.Client.Status().Patch(ctx, &fresh, patch); err != nil {
			if apierrors.IsConflict(err) {
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("conflict bumping agentsInvoked on %s", convName)
}

// AppendMemberResult routes one member's result onto its immediate PARENT,
// one hop, never the tree's ultimate root (coordination-loop). dedupID is the
// caller's idempotency key (e.g. "member:<conversation>:<runId>") — a repeat
// append with the SAME id while it is still pending is a no-op; the durable
// per-run marker is the caller's job (RunStatus.RoutedToParent), since once
// this input is processed and pruned there is nothing left here to compare
// against.
func (r *Router) AppendMemberResult(ctx context.Context, causedBy *agentopsv1alpha1.Provenance, memberName, dedupID, result string) error {
	if causedBy == nil || causedBy.Parent == memberName {
		return nil // no parent, or a provenance bug — never feed a conversation its own output
	}
	item := agentopsv1alpha1.InputItem{
		ID: dedupID, Type: agentopsv1alpha1.InputTask, Payload: result, ReceivedAt: metav1.Now(),
		Origin: &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginMember, Name: memberName, Entry: causedBy.Entry},
	}
	return r.appendInputIdempotent(ctx, causedBy.Parent, item)
}

// appendInputIdempotent appends one input to a NAMED conversation (not
// necessarily the caller's own object), skipping conversations already
// Closed (coordination-loop: "a closed parent receives nothing") and skipping
// a repeat of an id already pending, with optimistic retry on conflict.
func (r *Router) appendInputIdempotent(ctx context.Context, convName string, item agentopsv1alpha1.InputItem) error {
	for attempt := 0; attempt < 5; attempt++ {
		var fresh agentopsv1alpha1.Conversation
		if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: convName}, &fresh); err != nil {
			return client.IgnoreNotFound(err)
		}
		if fresh.Status.Phase == agentopsv1alpha1.ConversationClosed {
			return nil
		}
		for i := range fresh.Spec.Inputs {
			if fresh.Spec.Inputs[i].ID == item.ID {
				return nil
			}
		}
		patch := client.MergeFrom(fresh.DeepCopy())
		fresh.Spec.Inputs = append(fresh.Spec.Inputs, item)
		if err := r.Client.Patch(ctx, &fresh, patch); err != nil {
			if apierrors.IsConflict(err) {
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("conflict appending input to %s", convName)
}

// CloseCoordinated is the manager's half of the MCP `close(conversation,
// reason)` verb (conversation-close, widened by coordinator-owner-reach): a
// reason is required, and the target must be the caller itself, a
// conversation it directly caused (ONE HOP — a deeper descendant is refused
// naming it out of scope; reaching one means asking the direct member to
// close it, whose own close cascades in turn), OR — only when the caller
// RESOLVES to a Coordinator (design D-A) — any OTHER open, uncaused root of
// that SAME Coordinator (design D-B).
func (r *Router) CloseCoordinated(ctx context.Context, caller *agentopsv1alpha1.Conversation, targetName, reason string) error {
	if reason == "" {
		return fmt.Errorf("a coordinator's close requires a reason")
	}
	if targetName == caller.Name {
		return r.closeWithCascade(ctx, caller, reason)
	}
	var target agentopsv1alpha1.Conversation
	if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: targetName}, &target); err != nil {
		return client.IgnoreNotFound(err)
	}
	if target.Spec.CausedBy != nil && target.Spec.CausedBy.Parent == caller.Name {
		return r.closeWithCascade(ctx, &target, reason)
	}
	ok, err := r.isOwnCoordinatorSiblingRoot(ctx, caller, &target)
	if err != nil {
		return err
	}
	if !ok {
		return ErrOutOfScope
	}
	return r.closeWithCascade(ctx, &target, reason)
}

// CloseBudgetExceeded is design D-E's one closing path for every budget edge
// (maxAgents at invoke, maxTurns after a run, deadline on reconcile): close
// every live member first, then escalate THIS conversation — never a
// separate close on it, since escalate already performs the one of two
// outcomes that applies (D-D).
func (r *Router) CloseBudgetExceeded(ctx context.Context, conv *agentopsv1alpha1.Conversation, digest string) error {
	if err := r.cascadeCloseMembers(ctx, conv.Name, "budget-exceeded"); err != nil {
		return err
	}
	return r.Escalate(ctx, conv, digest)
}

// closeWithCascade closes ONE conversation with a reason, then closes every
// LIVE conversation it directly caused with the SAME reason — recursively,
// since a closed member may itself have members (conversation-close).
func (r *Router) closeWithCascade(ctx context.Context, conv *agentopsv1alpha1.Conversation, reason string) error {
	if err := r.closeConversationReason(ctx, conv, reason); err != nil {
		return err
	}
	return r.cascadeCloseMembers(ctx, conv.Name, reason)
}

// cascadeCloseMembers closes every LIVE direct member of parentName, same
// reason verbatim, each recursively cascading to its own members.
func (r *Router) cascadeCloseMembers(ctx context.Context, parentName, reason string) error {
	var list agentopsv1alpha1.ConversationList
	if err := r.Reader.List(ctx, &list, client.InNamespace(r.Namespace),
		client.MatchingLabels{agentopsv1alpha1.LabelCausedBy: parentName}); err != nil {
		return err
	}
	for i := range list.Items {
		member := &list.Items[i]
		if member.Status.Phase == agentopsv1alpha1.ConversationClosed {
			continue
		}
		if member.Spec.CausedBy == nil || member.Spec.CausedBy.Parent != parentName {
			continue // the label is a hint; the field is the fact
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
func (r *Router) closeConversationReason(ctx context.Context, conv *agentopsv1alpha1.Conversation, reason string) error {
	if conv.Status.Phase == agentopsv1alpha1.ConversationClosed {
		return nil
	}
	farewell := fmt.Sprintf("👋 Conversation closed (%s). This thread is archived — reply here "+
		"and I will start a fresh conversation, or reopen this one from the console to continue "+
		"with its history.", reason)
	r.eachBoundThread(ctx, conv, "", func(ch *agentopsv1alpha1.Channel, tid *string) {
		r.Ops.EnqueueFarewell(ctx, ch, conv, tid, Notice(farewell))
	})
	patch := client.MergeFrom(conv.DeepCopy())
	now := metav1.Now()
	conv.Status.Phase = agentopsv1alpha1.ConversationClosed
	conv.Status.ClosedAt = &now
	conv.Status.CloseReason = boundedString(reason, agentopsv1alpha1.MaxCloseReason)
	return client.IgnoreNotFound(r.Client.Status().Patch(ctx, conv, patch))
}

// Escalate is the MCP `escalate(message)` verb (design D-D,
// coordination-escalation): an UNCAUSED conversation binds its snapshotted
// escalation channels and opens a human thread with message as the first
// post; a CAUSED one opens no thread at all — it closes with message as its
// reason and its result, which reaches its OWN parent as an ordinary
// member-result input, bubbling one hop at a time until a call reaches the
// uncaused root.
func (r *Router) Escalate(ctx context.Context, conv *agentopsv1alpha1.Conversation, message string) error {
	if conv.Spec.CausedBy != nil {
		if err := r.closeConversationReason(ctx, conv, message); err != nil {
			return err
		}
		return r.AppendMemberResult(ctx, conv.Spec.CausedBy, conv.Name,
			"escalate:"+conv.Name+":"+strconv.FormatInt(time.Now().UnixNano(), 36), message)
	}
	if conv.Status.EscalatedAt != nil {
		return nil // already escalated: no second binding, no replayed digest
	}
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Spec.ChannelRefs = append([]agentopsv1alpha1.ObjectRef{}, conv.Spec.EscalationChannelRefs...)
	if err := r.Client.Patch(ctx, conv, patch); err != nil {
		return err
	}
	statusPatch := client.MergeFrom(conv.DeepCopy())
	now := metav1.Now()
	conv.Status.EscalatedAt = &now
	conv.Status.EscalationMessage = boundedString(message, 2000)
	return r.Client.Status().Patch(ctx, conv, statusPatch)
}

// memberTitle names a member conversation from the entry it was invoked as,
// plus the task's own words when there are any — bounded to fit a chat topic
// name, the same shape httpapi.titleForGroup gives a signal-opened one.
func memberTitle(entryName, task string) string {
	fields := strings.Fields(task)
	if len(fields) == 0 {
		return "🤝 " + entryName
	}
	title := "🤝 " + entryName + ": " + strings.Join(fields, " ")
	if runes := []rune(title); len(runes) > 60 {
		title = string(runes[:60])
	}
	return title
}

func boundedString(s string, limit int) string {
	if len(s) > limit {
		return s[:limit]
	}
	return s
}
