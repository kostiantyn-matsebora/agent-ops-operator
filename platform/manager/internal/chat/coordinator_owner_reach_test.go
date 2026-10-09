// Tests for the Coordinator-owner reach class (coordinator-owner-reach,
// design D-A/D-B): the coordinatorRef resolution walk, list_open_roots, and
// close's widened bound. See coordinate_test.go for the ordinary coordinator
// class's own tests (invoke, the narrow close, escalate).
package chat

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// nsName is a shorthand for the namespaced key every Get in this file needs.
func nsName(name string) types.NamespacedName {
	return types.NamespacedName{Namespace: testNS, Name: name}
}

// --- 4.1: ResolveActingCoordinator -----------------------------------------

// The three cases task 4.1 names, verbatim: a plain member resolves via its
// ancestor root, a Coordinator's own root resolves via its own field, and a
// Pipeline-addressed conversation resolves to nothing.
func TestResolveActingCoordinatorThreeCases(t *testing.T) {
	root := coordinatorRoot("root-1", "co-a")
	member := &agentopsv1alpha1.Conversation{}
	member.Name, member.Namespace = "member-1", testNS
	member.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "reaper"}
	pipelineAddressed := &agentopsv1alpha1.Conversation{}
	pipelineAddressed.Name, pipelineAddressed.Namespace = "plain-1", testNS
	pipelineAddressed.Spec.PipelineRef = &agentopsv1alpha1.ObjectRef{Name: "pipeline-a"}

	r, _ := coordFixture(t, testCoordinator("co-a"), root, member, pipelineAddressed)

	t.Run("a plain member resolves via its ancestor root", func(t *testing.T) {
		got, err := r.ResolveActingCoordinator(context.Background(), member)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "co-a" || got.RootName != "root-1" {
			t.Fatalf("got %+v, want Name=co-a RootName=root-1", got)
		}
	})
	t.Run("a Coordinator's own root resolves via its own field", func(t *testing.T) {
		got, err := r.ResolveActingCoordinator(context.Background(), root)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "co-a" || got.RootName != "root-1" {
			t.Fatalf("got %+v, want Name=co-a RootName=root-1", got)
		}
	})
	t.Run("a Pipeline-addressed conversation resolves to nothing", func(t *testing.T) {
		got, err := r.ResolveActingCoordinator(context.Background(), pipelineAddressed)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "" || got.RootName != "" {
			t.Fatalf("got %+v, want the zero value", got)
		}
	})
}

func TestResolveActingCoordinatorPropagatesABrokenAncestryChain(t *testing.T) {
	member := &agentopsv1alpha1.Conversation{}
	member.Name, member.Namespace = "member-1", testNS
	member.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "missing-parent", Entry: "reaper"}
	r, _ := coordFixture(t, member)

	if _, err := r.ResolveActingCoordinator(context.Background(), member); err == nil {
		t.Fatal("want an error when the ancestry chain is broken")
	}
}

// A nested Coordinator's own root — carries BOTH causedBy (from its invoking
// parent) and its own coordinatorRef — resolves via its OWN field, never the
// walk, exactly as D-A's order states: "reads the caller's own coordinatorRef
// first."
func TestResolveActingCoordinatorPrefersTheCallersOwnFieldOverTheWalk(t *testing.T) {
	outerRoot := coordinatorRoot("root-1", "co-a")
	nestedRoot := &agentopsv1alpha1.Conversation{}
	nestedRoot.Name, nestedRoot.Namespace = "nested-1", testNS
	nestedRoot.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "to-b"}
	nestedRoot.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-b"}

	r, _ := coordFixture(t, testCoordinator("co-a"), testCoordinator("co-b"), outerRoot, nestedRoot)

	got, err := r.ResolveActingCoordinator(context.Background(), nestedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "co-b" {
		t.Fatalf("a nested root must resolve via its OWN coordinatorRef, got %+v", got)
	}
	if got.RootName != "root-1" {
		t.Fatalf("RootName is always the causedBy walk's own answer, got %q", got.RootName)
	}
}

// --- 4.2: ListOpenRoots ------------------------------------------------------

func TestListOpenRootsListsOnlyUncausedRootsOfTheCallersOwnCoordinator(t *testing.T) {
	caller := coordinatorRoot("root-1", "co-a")
	sibling := coordinatorRoot("root-2", "co-a")
	other := coordinatorRoot("root-3", "co-b")

	r, _ := coordFixture(t, testCoordinator("co-a"), testCoordinator("co-b"), caller, sibling, other)

	got, err := r.ListOpenRoots(context.Background(), caller)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "root-2" {
		t.Fatalf("want only root-2 (co-a, open, not the caller), got %+v", got)
	}
}

// A member root is excluded even when open — whatever Coordinator it
// carries (coordinator-owner-reach: "A member root is excluded even when
// open").
func TestListOpenRootsExcludesAMemberRootEvenWhenItIsItselfANestedCoordinatorsRoot(t *testing.T) {
	caller := coordinatorRoot("root-1", "co-a")
	nestedButCaused := &agentopsv1alpha1.Conversation{}
	nestedButCaused.Name, nestedButCaused.Namespace = "nested-1", testNS
	nestedButCaused.Spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: "co-a"}
	nestedButCaused.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "to-nested"}

	r, _ := coordFixture(t, testCoordinator("co-a"), caller, nestedButCaused)

	got, err := r.ListOpenRoots(context.Background(), caller)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("a member — even one carrying coordinatorRef — must never be listed, got %+v", got)
	}
}

func TestListOpenRootsExcludesAClosedRoot(t *testing.T) {
	caller := coordinatorRoot("root-1", "co-a")
	closedSibling := coordinatorRoot("root-2", "co-a")
	closedSibling.Status.Phase = agentopsv1alpha1.ConversationClosed

	r, _ := coordFixture(t, testCoordinator("co-a"), caller, closedSibling)

	got, err := r.ListOpenRoots(context.Background(), caller)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("a closed root must never be listed, got %+v", got)
	}
}

func TestListOpenRootsExcludesADifferentCoordinatorsRoot(t *testing.T) {
	caller := coordinatorRoot("root-1", "co-a")
	otherCoordinatorRoot := coordinatorRoot("root-2", "co-b")

	r, _ := coordFixture(t, testCoordinator("co-a"), testCoordinator("co-b"), caller, otherCoordinatorRoot)

	got, err := r.ListOpenRoots(context.Background(), caller)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("a different Coordinator's root must never be listed, got %+v", got)
	}
}

// The caller's own root is excluded both when the caller IS the root...
func TestListOpenRootsExcludesTheCallerWhenTheCallerIsItselfTheRoot(t *testing.T) {
	caller := coordinatorRoot("root-1", "co-a")
	r, _ := coordFixture(t, testCoordinator("co-a"), caller)

	got, err := r.ListOpenRoots(context.Background(), caller)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("a root caller must not list itself, got %+v", got)
	}
}

// ...and when the caller is a MEMBER whose ancestor is the root (the reaper's
// own shape) — closing it would cascade to close the caller's own
// conversation mid-run.
func TestListOpenRootsExcludesTheCallersOwnAncestorRootWhenCallerIsAMember(t *testing.T) {
	root := coordinatorRoot("root-1", "co-a")
	reaper := &agentopsv1alpha1.Conversation{}
	reaper.Name, reaper.Namespace = "reaper-1", testNS
	reaper.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "reaper"}

	r, _ := coordFixture(t, testCoordinator("co-a"), root, reaper)

	got, err := r.ListOpenRoots(context.Background(), reaper)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("the reaper's own ancestor root must be excluded, got %+v", got)
	}
}

// The reaper's primary case, end to end: it resolves co-a through its
// ancestor root and sees exactly the OTHER open co-a roots, each carrying its
// own direct members' entry names.
func TestListOpenRootsReturnsSiblingRootsWithTheirDirectMembersProjection(t *testing.T) {
	root := coordinatorRoot("root-1", "co-a")
	reaper := &agentopsv1alpha1.Conversation{}
	reaper.Name, reaper.Namespace = "reaper-1", testNS
	reaper.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "reaper"}

	incident := coordinatorRoot("incident-1", "co-a")
	incident.Spec.Title = "ingress 502s"
	incident.Status.Brief = "checking the ingress"
	member1 := coordMember("member-1", "incident-1", "endpoint-check")
	member2 := coordMember("member-2", "incident-1", "log-scan")
	closedMember := coordMember("member-3", "incident-1", "log-scan") // same entry re-invoked, closed first time
	closedMember.Status.Phase = agentopsv1alpha1.ConversationClosed

	r, _ := coordFixture(t, testCoordinator("co-a"), root, reaper, incident, member1, member2, closedMember)

	got, err := r.ListOpenRoots(context.Background(), reaper)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "incident-1" {
		t.Fatalf("want only incident-1, got %+v", got)
	}
	row := got[0]
	if row.Title != "ingress 502s" || row.Brief != "checking the ingress" || row.Phase != "" {
		t.Fatalf("want the same projection shape list_conversations gives, got %+v", row)
	}
	wantMembers := map[string]bool{"endpoint-check": true, "log-scan": true}
	if len(row.Members) != 2 {
		t.Fatalf("want 2 distinct entry names (log-scan deduplicated), got %v", row.Members)
	}
	for _, m := range row.Members {
		if !wantMembers[m] {
			t.Fatalf("unexpected member entry %q in %v", m, row.Members)
		}
	}
}

// An Idle root that never produced a single member had no agent the reaper
// could ever "re-check through" — its own stated method — so it is excluded
// rather than surfaced identically forever. This is the job-cb5vg bug,
// reproduced directly: a Coordinator-addressed root whose only run failed
// before invoking anyone.
func TestListOpenRootsExcludesAnIdleRootWithNoMembers(t *testing.T) {
	caller := coordinatorRoot("root-1", "co-a")
	deadEnd := coordinatorRoot("root-2", "co-a")
	deadEnd.Status.Phase = agentopsv1alpha1.ConversationIdle

	r, _ := coordFixture(t, testCoordinator("co-a"), caller, deadEnd)

	got, err := r.ListOpenRoots(context.Background(), caller)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("an Idle root with no members has nothing left to re-check, got %+v", got)
	}
}

// A root still actively Working, with no member YET, is not a dead end — it
// may invoke one before this very run ends. Only Idle-and-memberless is
// excluded.
func TestListOpenRootsIncludesAWorkingRootWithNoMembersYet(t *testing.T) {
	caller := coordinatorRoot("root-1", "co-a")
	stillRunning := coordinatorRoot("root-2", "co-a")
	stillRunning.Status.Phase = agentopsv1alpha1.ConversationWorking

	r, _ := coordFixture(t, testCoordinator("co-a"), caller, stillRunning)

	got, err := r.ListOpenRoots(context.Background(), caller)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "root-2" {
		t.Fatalf("a Working root with no member yet must still be listed, got %+v", got)
	}
}

// An Idle root WITH members is the ordinary healed-or-not case and is
// unaffected by the dead-end exclusion above.
func TestListOpenRootsIncludesAnIdleRootThatHasMembers(t *testing.T) {
	caller := coordinatorRoot("root-1", "co-a")
	healed := coordinatorRoot("root-2", "co-a")
	healed.Status.Phase = agentopsv1alpha1.ConversationIdle
	member := coordMember("member-1", "root-2", "k8s-observe")

	r, _ := coordFixture(t, testCoordinator("co-a"), caller, healed, member)

	got, err := r.ListOpenRoots(context.Background(), caller)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "root-2" {
		t.Fatalf("an Idle root with a member is the ordinary case, got %+v", got)
	}
}

func TestListOpenRootsRefusesACallerWithNoCoordinatorScope(t *testing.T) {
	pipelineAddressed := &agentopsv1alpha1.Conversation{}
	pipelineAddressed.Name, pipelineAddressed.Namespace = "plain-1", testNS
	r, _ := coordFixture(t, pipelineAddressed)

	if _, err := r.ListOpenRoots(context.Background(), pipelineAddressed); err != ErrNoCoordinatorScope {
		t.Fatalf("want ErrNoCoordinatorScope, got %v", err)
	}
}

// coordMember builds a member conversation the way createMember does: the
// causedBy field AND the label, since ListOpenRoots' directMemberEntries
// trusts only the field and uses the label merely as an index — mirroring
// httpapi's own coordMember test helper.
func coordMember(name, parent, entry string) *agentopsv1alpha1.Conversation {
	m := &agentopsv1alpha1.Conversation{}
	m.Name, m.Namespace = name, testNS
	m.Labels = map[string]string{agentopsv1alpha1.LabelCausedBy: parent}
	m.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: parent, Entry: entry}
	return m
}

// --- 4.3: close's widened bound ---------------------------------------------

// A Coordinator-owner may close a sibling root (aops-mcp-server: "A
// Coordinator-owner may close a sibling root").
func TestCloseCoordinatedPermitsASiblingRootOfTheCallersOwnCoordinator(t *testing.T) {
	root := coordinatorRoot("root-1", "co-a")
	reaper := &agentopsv1alpha1.Conversation{}
	reaper.Name, reaper.Namespace = "reaper-1", testNS
	reaper.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "reaper"}
	sibling := coordinatorRoot("incident-1", "co-a")
	sibling.Spec.Signal = automatedSignal() // an alert — not a person's own request

	r, c := coordFixture(t, testCoordinator("co-a"), root, reaper, sibling)

	if err := r.CloseCoordinated(context.Background(), reaper, "incident-1", "resolved by the reaper"); err != nil {
		t.Fatalf("a Coordinator-owner must be able to close a sibling root: %v", err)
	}
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), nsName("incident-1"), &got)
	if got.Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatal("the sibling root must be closed")
	}
}

// automatedSignal is what a genuine alert/job origination's spec.signal
// looks like — no LabelChatChannel, so isHumanInitiated reports false.
func automatedSignal() *agentopsv1alpha1.SignalProvenance {
	return &agentopsv1alpha1.SignalProvenance{
		SourceRef: &agentopsv1alpha1.ObjectRef{Name: "alerts"},
		Labels:    map[string]string{"alertname": "KubeJobFailed"},
	}
}

// chatSignal is what a bare chat message's spec.signal looks like — carries
// LabelChatChannel, so isHumanInitiated reports true even though
// spec.signal is set.
func chatSignal() *agentopsv1alpha1.SignalProvenance {
	return &agentopsv1alpha1.SignalProvenance{
		SourceRef: &agentopsv1alpha1.ObjectRef{Name: "console"},
		Labels:    map[string]string{agentopsv1alpha1.LabelChatChannel: "console"},
	}
}

// The job-cb5vg bug, as a scope test: the widened Coordinator-owner reach
// must never close a sibling root a PERSON started — an addressed task
// command (no spec.signal at all), a bare chat message (spec.signal
// carrying LabelChatChannel), the reaper's own real sweep target. An
// automated sibling (an alert, a job) stays closable, per the test above.
func TestCloseCoordinatedRefusesASiblingRootAPersonStarted(t *testing.T) {
	root := coordinatorRoot("root-1", "co-a")
	reaper := &agentopsv1alpha1.Conversation{}
	reaper.Name, reaper.Namespace = "reaper-1", testNS
	reaper.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "reaper"}

	addressedTask := coordinatorRoot("task-1", "co-a") // no spec.signal — an addressed command
	chatOrigin := coordinatorRoot("chat-1", "co-a")
	chatOrigin.Spec.Signal = chatSignal()

	r, c := coordFixture(t, testCoordinator("co-a"), root, reaper, addressedTask, chatOrigin)

	for _, name := range []string{"task-1", "chat-1"} {
		t.Run(name, func(t *testing.T) {
			err := r.CloseCoordinated(context.Background(), reaper, name, "sweeping")
			if err != ErrCannotCloseHumanRoot {
				t.Fatalf("want ErrCannotCloseHumanRoot, got %v", err)
			}
			var got agentopsv1alpha1.Conversation
			c.Get(context.Background(), nsName(name), &got)
			if got.Status.Phase == agentopsv1alpha1.ConversationClosed {
				t.Fatal("a person's own request must never be closed by the widened reach")
			}
		})
	}
}

// Cross-Coordinator close is refused (aops-mcp-server scenario).
func TestCloseCoordinatedRefusesADifferentCoordinatorsRoot(t *testing.T) {
	root := coordinatorRoot("root-1", "co-a")
	reaper := &agentopsv1alpha1.Conversation{}
	reaper.Name, reaper.Namespace = "reaper-1", testNS
	reaper.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "reaper"}
	otherCoordinatorRoot := coordinatorRoot("root-b", "co-b")

	r, c := coordFixture(t, testCoordinator("co-a"), testCoordinator("co-b"), root, reaper, otherCoordinatorRoot)

	err := r.CloseCoordinated(context.Background(), reaper, "root-b", "trying to reach another Coordinator")
	if err != ErrOutOfScope {
		t.Fatalf("want ErrOutOfScope, got %v", err)
	}
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), nsName("root-b"), &got)
	if got.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatal("a cross-Coordinator close must change nothing")
	}
}

// A member — even deep, even within the caller's own Coordinator's tree — is
// ALWAYS refused by the widened bound. The ordinary one-hop bound already
// owns a direct member; this asserts the branch never reaches further.
func TestCloseCoordinatedRefusesAMemberEvenWithinTheCallersOwnCoordinator(t *testing.T) {
	root := coordinatorRoot("root-1", "co-a")
	reaper := &agentopsv1alpha1.Conversation{}
	reaper.Name, reaper.Namespace = "reaper-1", testNS
	reaper.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "reaper"}
	incident := coordinatorRoot("incident-1", "co-a")
	deepMember := coordMember("deep-member", "incident-1", "endpoint-check")

	r, c := coordFixture(t, testCoordinator("co-a"), root, reaper, incident, deepMember)

	err := r.CloseCoordinated(context.Background(), reaper, "deep-member", "trying to reach a member")
	if err != ErrOutOfScope {
		t.Fatalf("want ErrOutOfScope, got %v", err)
	}
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), nsName("deep-member"), &got)
	if got.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatal("a member must never be reached by the widened bound")
	}
}

// A Pipeline-addressed caller (resolves to no Coordinator) keeps the narrow,
// unwidened bound.
func TestCloseCoordinatedKeepsTheNarrowBoundForAPipelineAddressedCaller(t *testing.T) {
	caller := &agentopsv1alpha1.Conversation{}
	caller.Name, caller.Namespace = "plain-1", testNS
	other := &agentopsv1alpha1.Conversation{}
	other.Name, other.Namespace = "other-1", testNS

	r, c := coordFixture(t, caller, other)

	err := r.CloseCoordinated(context.Background(), caller, "other-1", "not mine to close")
	if err != ErrOutOfScope {
		t.Fatalf("want ErrOutOfScope, got %v", err)
	}
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), nsName("other-1"), &got)
	if got.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatal("a Pipeline-addressed caller must never reach the widened bound")
	}
}

// The caller's own ancestor root is excluded from the widened close bound
// too, even when named directly — the reaper cannot end its own existence by
// closing the root that invoked it.
func TestCloseCoordinatedRefusesTheCallersOwnAncestorRoot(t *testing.T) {
	root := coordinatorRoot("root-1", "co-a")
	reaper := &agentopsv1alpha1.Conversation{}
	reaper.Name, reaper.Namespace = "reaper-1", testNS
	reaper.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "reaper"}

	r, c := coordFixture(t, testCoordinator("co-a"), root, reaper)

	err := r.CloseCoordinated(context.Background(), reaper, "root-1", "trying to close my own root")
	if err != ErrOutOfScope {
		t.Fatalf("want ErrOutOfScope, got %v", err)
	}
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), nsName("root-1"), &got)
	if got.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatal("the reaper must never be able to close its own ancestor root")
	}
}

// --- isHumanInitiated -------------------------------------------------------

func TestIsHumanInitiated(t *testing.T) {
	cases := []struct {
		name string
		conv *agentopsv1alpha1.Conversation
		want bool
	}{
		{"an addressed task command carries no spec.signal at all", &agentopsv1alpha1.Conversation{}, true},
		{"a bare chat message carries LabelChatChannel", &agentopsv1alpha1.Conversation{
			Spec: agentopsv1alpha1.ConversationSpec{Signal: chatSignal()},
		}, true},
		{"an alert or a job carries spec.signal with no chat label", &agentopsv1alpha1.Conversation{
			Spec: agentopsv1alpha1.ConversationSpec{Signal: automatedSignal()},
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isHumanInitiated(tc.conv); got != tc.want {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
		})
	}
}
