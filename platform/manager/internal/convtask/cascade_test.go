package convtask

import (
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// async-command-bus design D-7, task 5.3: CascadeOrder is pure — no client,
// no bus — so the deepest-first/root-last ordering is tested directly,
// exactly as httpapi's own TestDeleteCascadeOrderIsDeepestFirstRootLast
// pinned deleteCascadeOrder before it became a one-line delegate to this
// function.
func TestCascadeOrderIsDeepestFirstRootLast(t *testing.T) {
	descendants := []state.ConversationRecord{
		{Name: "grandchild", CausedBy: &state.Provenance{Parent: "mid"}},
		{Name: "mid", CausedBy: &state.Provenance{Parent: "root"}},
		{Name: "other-grandchild", CausedBy: &state.Provenance{Parent: "mid"}},
	}
	got := CascadeOrder("root", descendants)
	if len(got) != 4 || got[len(got)-1] != "root" {
		t.Fatalf("want 4 names ending with root, got %v", got)
	}
	pos := map[string]int{}
	for i, n := range got {
		pos[n] = i
	}
	if pos["grandchild"] >= pos["mid"] || pos["other-grandchild"] >= pos["mid"] {
		t.Fatalf("a child must precede its own parent, got %v", got)
	}
	if pos["mid"] >= pos["root"] {
		t.Fatalf("root must be last, got %v", got)
	}
}

// CloseMemberRequests builds one CascadeMember task per LIVE descendant,
// deepest-first, excluding rootName itself — the caller supplies its own
// final request for rootName.
func TestCloseMemberRequestsOrdersMembersBeforeTheRootAndSharesItsKey(t *testing.T) {
	descendants := []state.ConversationRecord{
		{Name: "mid", CausedBy: &state.Provenance{Parent: "root"}},
		{Name: "leaf", CausedBy: &state.Provenance{Parent: "mid"}},
	}
	reqs := CloseMemberRequests("root", "because", descendants)
	if len(reqs) != 2 {
		t.Fatalf("want one task per descendant (2), got %d", len(reqs))
	}
	names := make([]string, len(reqs))
	for i, r := range reqs {
		if r.Queue != Queue {
			t.Fatalf("every cascade task must be on %q, got %q", Queue, r.Queue)
		}
		if r.Key != commandbus.Key("root") {
			t.Fatalf("every cascade task must share the root's own name as Key, got %q", r.Key)
		}
		p, err := Decode(r.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if !p.CascadeMember || p.Verb != VerbClose || p.Reason != "because" {
			t.Fatalf("want a CascadeMember close task carrying the cascade's reason verbatim, got %+v", p)
		}
		names[i] = p.Conversation
	}
	if names[0] != "leaf" || names[1] != "mid" {
		t.Fatalf("leaf must precede its own parent mid, got %v", names)
	}
}

// An already-Closed descendant gets no task — CloseConversationReason is
// idempotent on one anyway, but a task that can only ever no-op earns
// nothing on /state/tasks.
func TestCloseMemberRequestsSkipsAnAlreadyClosedDescendant(t *testing.T) {
	descendants := []state.ConversationRecord{
		{Name: "closed-already", CausedBy: &state.Provenance{Parent: "root"}, Phase: state.PhaseClosed},
		{Name: "still-open", CausedBy: &state.Provenance{Parent: "root"}},
	}
	reqs := CloseMemberRequests("root", "", descendants)
	if len(reqs) != 1 {
		t.Fatalf("want a task only for the still-open descendant, got %d", len(reqs))
	}
	p, err := Decode(reqs[0].Payload)
	if err != nil {
		t.Fatal(err)
	}
	if p.Conversation != "still-open" {
		t.Fatalf("want still-open's own task, got %+v", p)
	}
}

// rootName itself never gets a CascadeMember task from this function — the
// caller's own top-level request occupies that slot.
func TestCloseMemberRequestsExcludesTheRootItself(t *testing.T) {
	descendants := []state.ConversationRecord{{Name: "root"}}
	reqs := CloseMemberRequests("root", "", descendants)
	if len(reqs) != 0 {
		t.Fatalf("want no task for the root itself, got %d", len(reqs))
	}
}
