package namespace

import "testing"

// TestResolveDefaultsConversationsToControl pins the one-code-path rule a
// single-namespace install depends on: an empty conversations value resolves
// to two equal strings, never a zero-valued half.
func TestResolveDefaultsConversationsToControl(t *testing.T) {
	got := Resolve("agent-ops", "")
	want := Namespaces{Control: "agent-ops", Conversations: "agent-ops"}
	if got != want {
		t.Fatalf("Resolve(%q, \"\") = %+v, want %+v", "agent-ops", got, want)
	}
	if got.Distinct() {
		t.Fatalf("a defaulted pair must not report Distinct()")
	}
}

// TestResolveKeepsAnExplicitConversationsValue pins the split case: an
// explicit, different value is never overridden by the default.
func TestResolveKeepsAnExplicitConversationsValue(t *testing.T) {
	got := Resolve("agent-ops", "agent-ops-conversations")
	want := Namespaces{Control: "agent-ops", Conversations: "agent-ops-conversations"}
	if got != want {
		t.Fatalf("Resolve(...) = %+v, want %+v", got, want)
	}
	if !got.Distinct() {
		t.Fatalf("a split pair must report Distinct()")
	}
}
