package chat

import (
	"context"
	"testing"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/addressing"
)

// conversation-naming/spec.md: "A task conversation's name hints at the
// addressed pipeline" — the object Name is built from the addressed
// pipeline's own name, never a random metadata.generateName suffix.
func TestCreateTaskConversationNamesTheAddressedPipeline(t *testing.T) {
	r, _, c := closeFixture(t, pipeline("deploy-notes", "prof-x", true))
	cmd, ok := addressing.Parse("/deploy-notes do the thing")
	if !ok {
		t.Fatal("parse failed")
	}
	if err := r.HandleCommand(context.Background(), nsChannel("c1", "slack"), cmd, "", "", ""); err != nil {
		t.Fatal(err)
	}
	var list agentopsv1alpha1.ConversationList
	if err := c.List(context.Background(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("want 1 conversation, got %d", len(list.Items))
	}
	if got := list.Items[0].Name; got != "task-deploy-notes" {
		t.Fatalf("name = %q, want task-deploy-notes", got)
	}
}

// conversation-naming/spec.md: "A member conversation's name hints at the
// invoked entry" — and a second invoke under a DIFFERENT caller for the
// same entry name gets the next collision suffix, never a random one.
func TestInvokeMemberNamesTheInvokedEntry(t *testing.T) {
	entry := agentopsv1alpha1.CoordinatorAgentEntry{
		Name: "researcher", Description: "does the research",
		CapabilityRef: &agentopsv1alpha1.ObjectRef{Name: "cap-researcher"},
	}
	co := testCoordinator("co-naming", entry)
	callerA := coordinatorRoot("root-naming-a", "co-naming")
	callerB := coordinatorRoot("root-naming-b", "co-naming")
	r, _ := coordFixture(t, co, callerA, callerB,
		testCapability("cap-researcher", "profile-researcher"), testProfile("profile-researcher"))

	resultA, err := r.InvokeMember(context.Background(), callerA, "researcher", "look into it")
	if err != nil {
		t.Fatalf("InvokeMember (A): %v", err)
	}
	if resultA.Member != "member-researcher" {
		t.Fatalf("first member name = %q, want member-researcher", resultA.Member)
	}

	// A different parent invoking the SAME entry name is a genuinely new
	// member — findReusableMember scopes reuse to (parent, entry), so this
	// is not an attach.
	resultB, err := r.InvokeMember(context.Background(), callerB, "researcher", "look into it too")
	if err != nil {
		t.Fatalf("InvokeMember (B): %v", err)
	}
	if !resultB.Created {
		t.Fatalf("want a newly created member for a different caller, got attached to %s", resultB.Member)
	}
	if resultB.Member != "member-researcher-2" {
		t.Fatalf("second member name = %q, want member-researcher-2", resultB.Member)
	}
}
