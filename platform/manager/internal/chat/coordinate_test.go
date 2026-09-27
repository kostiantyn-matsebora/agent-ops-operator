package chat

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

func coordFixture(t *testing.T, objs ...client.Object) (*Router, client.Client) {
	t.Helper()
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithObjects(objs...).Build()
	q := &OpQueue{Client: c, Namespace: testNS, Registry: NewRegistry()}
	return &Router{Client: c, Reader: c, Namespace: testNS, Ops: q}, c
}

func testProfile(name string) *agentopsv1alpha1.AgentProfile {
	p := &agentopsv1alpha1.AgentProfile{}
	p.Name, p.Namespace = name, testNS
	return p
}

func testCapability(name, profile string) *agentopsv1alpha1.AgentCapability {
	c := &agentopsv1alpha1.AgentCapability{}
	c.Name, c.Namespace = name, testNS
	c.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: profile}
	return c
}

func testCoordinator(name string, agents ...agentopsv1alpha1.CoordinatorAgentEntry) *agentopsv1alpha1.Coordinator {
	co := &agentopsv1alpha1.Coordinator{}
	co.Name, co.Namespace = name, testNS
	co.Spec.Agents = agents
	return co
}

// coordinatorRoot is an UNCAUSED root conversation whose entry point is a
// Coordinator — the shape createConversationForGroup would have produced.
func coordinatorRoot(name, coordinator string) *agentopsv1alpha1.Conversation {
	c := &agentopsv1alpha1.Conversation{}
	c.Name, c.Namespace = name, testNS
	c.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: coordinator}
	return c
}

func TestInvokeMemberCreatesANewConversation(t *testing.T) {
	entry := agentopsv1alpha1.CoordinatorAgentEntry{
		Name: "worker", Description: "does the work",
		CapabilityRef: &agentopsv1alpha1.ObjectRef{Name: "cap-worker"},
	}
	co := testCoordinator("co-a", entry)
	caller := coordinatorRoot("root-1", "co-a")
	r, c := coordFixture(t, co, caller, testCapability("cap-worker", "profile-worker"), testProfile("profile-worker"))

	result, err := r.InvokeMember(context.Background(), caller, "worker", "look into it")
	if err != nil {
		t.Fatalf("InvokeMember: %v", err)
	}
	if !result.Created {
		t.Fatalf("want a newly created member, got attached to %s", result.Member)
	}

	var member agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: result.Member}, &member); err != nil {
		t.Fatalf("member not created: %v", err)
	}
	if member.Spec.CausedBy == nil || member.Spec.CausedBy.Parent != caller.Name || member.Spec.CausedBy.Entry != "worker" {
		t.Fatalf("causedBy must name the caller and the entry, got %+v", member.Spec.CausedBy)
	}
	if len(member.Spec.ChannelRefs) != 0 {
		t.Fatalf("a caused conversation binds no channel, got %v", member.Spec.ChannelRefs)
	}
	if member.Spec.ProfileRef.Name != "profile-worker" {
		t.Fatalf("capability not resolved: %+v", member.Spec.ProfileRef)
	}
	if len(member.Spec.Inputs) != 1 || member.Spec.Inputs[0].Payload != "look into it" {
		t.Fatalf("task must be queued as the member's first input, got %+v", member.Spec.Inputs)
	}
	if member.Spec.Inputs[0].Origin == nil || member.Spec.Inputs[0].Origin.Kind != agentopsv1alpha1.OriginMember ||
		member.Spec.Inputs[0].Origin.Name != caller.Name || member.Spec.Inputs[0].Origin.Entry != "worker" {
		t.Fatalf("the task's origin must cross the causedBy edge, got %+v", member.Spec.Inputs[0].Origin)
	}
}

func TestInvokeMemberAttachesToALiveMemberOfTheSameEntry(t *testing.T) {
	entry := agentopsv1alpha1.CoordinatorAgentEntry{
		Name: "worker", Description: "does the work",
		CapabilityRef: &agentopsv1alpha1.ObjectRef{Name: "cap-worker"},
	}
	co := testCoordinator("co-a", entry)
	caller := coordinatorRoot("root-1", "co-a")
	r, c := coordFixture(t, co, caller, testCapability("cap-worker", "profile-worker"), testProfile("profile-worker"))

	first, err := r.InvokeMember(context.Background(), caller, "worker", "first task")
	if err != nil {
		t.Fatalf("first invoke: %v", err)
	}
	second, err := r.InvokeMember(context.Background(), caller, "worker", "second task")
	if err != nil {
		t.Fatalf("second invoke: %v", err)
	}
	if second.Created {
		t.Fatalf("a second invoke of the same entry from the same parent must attach, not create")
	}
	if second.Member != first.Member {
		t.Fatalf("attach must name the SAME member: first=%s second=%s", first.Member, second.Member)
	}
	var member agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: first.Member}, &member); err != nil {
		t.Fatal(err)
	}
	if len(member.Spec.Inputs) != 2 {
		t.Fatalf("attach appends a second input rather than replacing, got %d", len(member.Spec.Inputs))
	}
}

func TestInvokeMemberRefusesAnUnknownAgent(t *testing.T) {
	co := testCoordinator("co-a")
	caller := coordinatorRoot("root-1", "co-a")
	r, _ := coordFixture(t, co, caller)

	if _, err := r.InvokeMember(context.Background(), caller, "nope", "task"); err != ErrUnknownAgent {
		t.Fatalf("want ErrUnknownAgent, got %v", err)
	}
}

func TestInvokeMemberRefusesANonCoordinatorCaller(t *testing.T) {
	caller := &agentopsv1alpha1.Conversation{}
	caller.Name, caller.Namespace = "plain-1", testNS
	r, _ := coordFixture(t, caller)

	if _, err := r.InvokeMember(context.Background(), caller, "worker", "task"); err != ErrNotCoordinatorRoot {
		t.Fatalf("want ErrNotCoordinatorRoot, got %v", err)
	}
}

// A repeated Coordinator in the chain is a CYCLE (design D-E2): coordinator A
// invokes B, and B's own agents[] loops back to A.
func TestInvokeMemberRefusesACycle(t *testing.T) {
	coA := testCoordinator("co-a")
	coB := testCoordinator("co-b", agentopsv1alpha1.CoordinatorAgentEntry{
		Name: "loopback", Description: "back to A",
		CoordinatorRef: &agentopsv1alpha1.ObjectRef{Name: "co-a"},
	})
	root := coordinatorRoot("root-1", "co-a")
	member := &agentopsv1alpha1.Conversation{}
	member.Name, member.Namespace = "member-1", testNS
	member.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-b"}
	member.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: root.Name, Entry: "to-b"}

	r, _ := coordFixture(t, coA, coB, root, member)

	if _, err := r.InvokeMember(context.Background(), member, "loopback", "task"); err != ErrCoordinatorCycle {
		t.Fatalf("want ErrCoordinatorCycle, got %v", err)
	}
}

// A DIRECT cycle: co-a's own agents[] entry loops straight back to co-a, and
// the caller is co-a's own root — no ancestor walk needed to see it.
func TestInvokeMemberRefusesADirectCycle(t *testing.T) {
	coA := testCoordinator("co-a", agentopsv1alpha1.CoordinatorAgentEntry{
		Name: "loopback", Description: "back to itself",
		CoordinatorRef: &agentopsv1alpha1.ObjectRef{Name: "co-a"},
	})
	root := coordinatorRoot("root-1", "co-a")
	r, _ := coordFixture(t, coA, root)

	if _, err := r.InvokeMember(context.Background(), root, "loopback", "task"); err != ErrCoordinatorCycle {
		t.Fatalf("want ErrCoordinatorCycle, got %v", err)
	}
}

// Nesting never pools a budget across levels (design D-E): a NESTED
// Coordinator's own conversation enforces only ITS OWN snapshotted limits,
// independent of its ancestor's — spending the nested one's budget must not
// touch the ancestor's.
func TestNestedCoordinatorBudgetIsIndependentOfItsAncestor(t *testing.T) {
	coA := testCoordinator("co-a") // the ancestor invokes no further agents in this test
	entry := agentopsv1alpha1.CoordinatorAgentEntry{
		Name: "helper", Description: "a nested helper",
		CapabilityRef: &agentopsv1alpha1.ObjectRef{Name: "cap-helper"},
	}
	coB := testCoordinator("co-b", entry)
	root := coordinatorRoot("root-1", "co-a")
	root.Status.Budget = &agentopsv1alpha1.ConversationBudget{MaxAgents: 5, AgentsInvoked: 0}
	nested := &agentopsv1alpha1.Conversation{}
	nested.Name, nested.Namespace = "nested-1", testNS
	nested.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: root.Name, Entry: "to-b"}
	nested.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-b"}
	nested.Status.Budget = &agentopsv1alpha1.ConversationBudget{MaxAgents: 1, AgentsInvoked: 0}

	r, c := coordFixture(t, coA, coB, root, nested, testCapability("cap-helper", "profile-helper"), testProfile("profile-helper"))

	if _, err := r.InvokeMember(context.Background(), nested, "helper", "task"); err != nil {
		t.Fatalf("InvokeMember on the nested coordinator: %v", err)
	}
	var gotNested, gotRoot agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: nested.Name}, &gotNested)
	c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: root.Name}, &gotRoot)
	if gotNested.Status.Budget.AgentsInvoked != 1 {
		t.Fatalf("the NESTED conversation's own budget must count the invoke, got %+v", gotNested.Status.Budget)
	}
	if gotRoot.Status.Budget.AgentsInvoked != 0 {
		t.Fatalf("the ANCESTOR's budget must be untouched by a nested level's invoke, got %+v", gotRoot.Status.Budget)
	}
}

func TestInvokeMemberRefusesAtMaxAgentsAndClosesTheCaller(t *testing.T) {
	entry := agentopsv1alpha1.CoordinatorAgentEntry{
		Name: "worker", Description: "does the work",
		CapabilityRef: &agentopsv1alpha1.ObjectRef{Name: "cap-worker"},
	}
	co := testCoordinator("co-a", entry)
	caller := coordinatorRoot("root-1", "co-a")
	caller.Status.Budget = &agentopsv1alpha1.ConversationBudget{MaxAgents: 1, AgentsInvoked: 1}
	r, c := coordFixture(t, co, caller, testCapability("cap-worker", "profile-worker"), testProfile("profile-worker"))

	if _, err := r.InvokeMember(context.Background(), caller, "worker", "task"); err != ErrMaxAgents {
		t.Fatalf("want ErrMaxAgents, got %v", err)
	}
	var got agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: caller.Name}, &got); err != nil {
		t.Fatal(err)
	}
	// The caller here is the UNCAUSED root, so budget-exceeded ESCALATES it
	// (design D-D/D-E) rather than closing it — the digest opens a human
	// thread and the root stays open.
	if got.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatal("budget-exceeded on the uncaused root must escalate, never close it")
	}
	if got.Status.EscalatedAt == nil {
		t.Fatal("a spent maxAgents budget must escalate the caller")
	}
	if !strings.Contains(got.Status.EscalationMessage, "budget-exceeded") {
		t.Fatalf("the digest must say why, got %q", got.Status.EscalationMessage)
	}
}

// The SAME edge on a NESTED caller (one carrying causedBy) bubbles instead:
// budget-exceeded closes it, and the digest lands on ITS OWN parent.
func TestInvokeMemberRefusesAtMaxAgentsAndBubblesWhenCallerIsNested(t *testing.T) {
	entry := agentopsv1alpha1.CoordinatorAgentEntry{
		Name: "worker", Description: "does the work",
		CapabilityRef: &agentopsv1alpha1.ObjectRef{Name: "cap-worker"},
	}
	co := testCoordinator("co-b", entry)
	root := coordinatorRoot("root-1", "co-a")
	nestedCaller := &agentopsv1alpha1.Conversation{}
	nestedCaller.Name, nestedCaller.Namespace = "member-1", testNS
	nestedCaller.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: root.Name, Entry: "to-b"}
	nestedCaller.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-b"}
	nestedCaller.Status.Budget = &agentopsv1alpha1.ConversationBudget{MaxAgents: 1, AgentsInvoked: 1}

	r, c := coordFixture(t, testCoordinator("co-a"), co, root, nestedCaller,
		testCapability("cap-worker", "profile-worker"), testProfile("profile-worker"))

	if _, err := r.InvokeMember(context.Background(), nestedCaller, "worker", "task"); err != ErrMaxAgents {
		t.Fatalf("want ErrMaxAgents, got %v", err)
	}
	var gotCaller, gotRoot agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: nestedCaller.Name}, &gotCaller)
	c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: root.Name}, &gotRoot)

	if gotCaller.Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatalf("a nested caller's spent budget closes it via the bubble, phase=%s", gotCaller.Status.Phase)
	}
	if !strings.Contains(gotCaller.Status.CloseReason, "budget-exceeded") {
		t.Fatalf("closeReason must say why, got %q", gotCaller.Status.CloseReason)
	}
	if len(gotRoot.Spec.Inputs) != 1 {
		t.Fatalf("the bubble must land as an input on the parent, got %+v", gotRoot.Spec.Inputs)
	}
}

func TestCloseCoordinatedSelf(t *testing.T) {
	caller := coordinatorRoot("root-1", "co-a")
	r, c := coordFixture(t, testCoordinator("co-a"), caller)

	if err := r.CloseCoordinated(context.Background(), caller, "root-1", "resolved: done"); err != nil {
		t.Fatal(err)
	}
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: "root-1"}, &got)
	if got.Status.Phase != agentopsv1alpha1.ConversationClosed || got.Status.CloseReason != "resolved: done" {
		t.Fatalf("self-close must succeed and record the reason: %+v", got.Status)
	}
}

func TestCloseCoordinatedRequiresAReason(t *testing.T) {
	caller := coordinatorRoot("root-1", "co-a")
	r, _ := coordFixture(t, testCoordinator("co-a"), caller)

	if err := r.CloseCoordinated(context.Background(), caller, "root-1", ""); err == nil {
		t.Fatal("a coordinator's close with no reason must be refused")
	}
}

func TestCloseCoordinatedDirectMemberCascadesToGrandchildren(t *testing.T) {
	root := coordinatorRoot("root-1", "co-a")
	member := &agentopsv1alpha1.Conversation{}
	member.Name, member.Namespace = "member-1", testNS
	member.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: root.Name, Entry: "worker"}
	member.Labels = map[string]string{agentopsv1alpha1.LabelCausedBy: root.Name}
	grandchild := &agentopsv1alpha1.Conversation{}
	grandchild.Name, grandchild.Namespace = "grandchild-1", testNS
	grandchild.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: member.Name, Entry: "helper"}
	grandchild.Labels = map[string]string{agentopsv1alpha1.LabelCausedBy: member.Name}

	r, c := coordFixture(t, testCoordinator("co-a"), root, member, grandchild)

	if err := r.CloseCoordinated(context.Background(), root, member.Name, "budget-exceeded"); err != nil {
		t.Fatal(err)
	}
	var gotMember, gotGrandchild agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: member.Name}, &gotMember)
	c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: grandchild.Name}, &gotGrandchild)
	if gotMember.Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatalf("the direct member must close, got %s", gotMember.Status.Phase)
	}
	if gotGrandchild.Status.Phase != agentopsv1alpha1.ConversationClosed || gotGrandchild.Status.CloseReason != "budget-exceeded" {
		t.Fatalf("closing a member must cascade to ITS member with the same reason, got %+v", gotGrandchild.Status)
	}
}

func TestCloseCoordinatedRefusesAGrandchildDirectly(t *testing.T) {
	root := coordinatorRoot("root-1", "co-a")
	member := &agentopsv1alpha1.Conversation{}
	member.Name, member.Namespace = "member-1", testNS
	member.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: root.Name, Entry: "worker"}
	grandchild := &agentopsv1alpha1.Conversation{}
	grandchild.Name, grandchild.Namespace = "grandchild-1", testNS
	grandchild.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: member.Name, Entry: "helper"}

	r, c := coordFixture(t, testCoordinator("co-a"), root, member, grandchild)

	err := r.CloseCoordinated(context.Background(), root, grandchild.Name, "trying to reach too far")
	if err != ErrOutOfScope {
		t.Fatalf("want ErrOutOfScope, got %v", err)
	}
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: grandchild.Name}, &got)
	if got.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatal("an out-of-scope close must change nothing")
	}
}

func TestEscalateUncausedRootBindsChannelsAndStampsTheDigest(t *testing.T) {
	root := coordinatorRoot("root-1", "co-a")
	root.Spec.EscalationChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "ops-desk"}}
	r, c := coordFixture(t, testCoordinator("co-a"), root, nsChannel("ops-desk", "telegram"))

	if err := r.Escalate(context.Background(), root, "3 members failed, see the tree"); err != nil {
		t.Fatal(err)
	}
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: root.Name}, &got)
	if len(got.Spec.ChannelRefs) != 1 || got.Spec.ChannelRefs[0].Name != "ops-desk" {
		t.Fatalf("escalate must bind the snapshotted escalation channels, got %v", got.Spec.ChannelRefs)
	}
	if got.Status.EscalatedAt == nil {
		t.Fatal("escalatedAt must be stamped")
	}
	if got.Status.EscalationMessage != "3 members failed, see the tree" {
		t.Fatalf("the digest must be recorded, got %q", got.Status.EscalationMessage)
	}
	if got.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatal("escalating the uncaused root must NOT close it")
	}
}

func TestEscalateMemberBubblesToItsParentInsteadOfOpeningAThread(t *testing.T) {
	root := coordinatorRoot("root-1", "co-a")
	member := &agentopsv1alpha1.Conversation{}
	member.Name, member.Namespace = "member-1", testNS
	member.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: root.Name, Entry: "worker"}
	member.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-b"} // itself nested
	member.Spec.EscalationChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "should-never-be-bound"}}

	r, c := coordFixture(t, testCoordinator("co-a"), testCoordinator("co-b"), root, member)

	if err := r.Escalate(context.Background(), member, "stuck, need a human"); err != nil {
		t.Fatal(err)
	}
	var gotMember, gotRoot agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: member.Name}, &gotMember)
	c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: root.Name}, &gotRoot)

	if gotMember.Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatalf("a nested escalate closes the member, got %s", gotMember.Status.Phase)
	}
	if gotMember.Status.CloseReason != "stuck, need a human" {
		t.Fatalf("closeReason must be the escalate message, got %q", gotMember.Status.CloseReason)
	}
	if len(gotMember.Spec.ChannelRefs) != 0 {
		t.Fatal("a bubbled escalation must never bind a channel")
	}
	if len(gotRoot.Spec.Inputs) != 1 || gotRoot.Spec.Inputs[0].Payload != "stuck, need a human" {
		t.Fatalf("the bubble must land as an input on the parent, got %+v", gotRoot.Spec.Inputs)
	}
	if gotRoot.Status.EscalatedAt != nil {
		t.Fatal("a bubble must never open a thread on the parent")
	}
}

func TestAppendMemberResultSkipsAClosedParent(t *testing.T) {
	parent := coordinatorRoot("root-1", "co-a")
	parent.Status.Phase = agentopsv1alpha1.ConversationClosed
	r, c := coordFixture(t, testCoordinator("co-a"), parent)

	err := r.AppendMemberResult(context.Background(),
		&agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "worker"}, "member-1", "member:member-1:r1", "the result")
	if err != nil {
		t.Fatal(err)
	}
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: "root-1"}, &got)
	if len(got.Spec.Inputs) != 0 {
		t.Fatalf("a closed parent gets nothing appended, got %+v", got.Spec.Inputs)
	}
}

func TestAppendMemberResultIsIdempotentPerDedupID(t *testing.T) {
	parent := coordinatorRoot("root-1", "co-a")
	r, c := coordFixture(t, testCoordinator("co-a"), parent)

	causedBy := &agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "worker"}
	for i := 0; i < 2; i++ {
		if err := r.AppendMemberResult(context.Background(), causedBy, "member-1", "member:member-1:r1", "the result"); err != nil {
			t.Fatal(err)
		}
	}
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: "root-1"}, &got)
	if len(got.Spec.Inputs) != 1 {
		t.Fatalf("the same dedup id must append once while pending, got %d inputs", len(got.Spec.Inputs))
	}
}

func TestHandleMessageRefusesAnOriginNamedForItsOwnConversation(t *testing.T) {
	conv := boundConv("conv-1", "conv-1") // deliberately: channel name == conversation name
	r, _, _ := closeFixture(t, nsChannel("conv-1", "telegram"), conv)

	thread := "thread-conv-1"
	err := r.HandleMessage(context.Background(), nsChannel("conv-1", "telegram"),
		InboundMessage{ThreadID: &thread, Text: "hello"})
	if err != ErrSelfInput {
		t.Fatalf("want ErrSelfInput, got %v", err)
	}
}

func TestCoordinatorChainCollectsTheCallersOwnAndEveryAncestor(t *testing.T) {
	root := coordinatorRoot("root-1", "co-a")
	member := &agentopsv1alpha1.Conversation{}
	member.Name, member.Namespace = "member-1", testNS
	member.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: root.Name, Entry: "to-b"}
	member.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-b"}

	r, _ := coordFixture(t, root, member)
	chain, err := r.coordinatorChain(context.Background(), member)
	if err != nil {
		t.Fatal(err)
	}
	if !chain["co-b"] || !chain["co-a"] {
		t.Fatalf("chain must hold the caller's own coordinator and every ancestor's, got %v", chain)
	}
}
