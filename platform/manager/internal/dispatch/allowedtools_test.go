package dispatch

import "testing"

// The allowlist is now exactly what the wiring bound — no profile base, no
// mode. These pin the composition rules that survive: order, dedup, trimming.

func TestAllowedToolsConcatenatesInRefOrder(t *testing.T) {
	got := EffectiveAllowedTools([][]string{{"Read", "Grep"}, {"mcp__victorialogs__*"}})
	if got != "Read,Grep,mcp__victorialogs__*" {
		t.Fatalf("refs must concatenate in order: %q", got)
	}
}

func TestAllowedToolsDedupsKeepingFirstPosition(t *testing.T) {
	got := EffectiveAllowedTools([][]string{{"Read", "Bash"}, {"Bash", "Grep"}, {"Read"}})
	if got != "Read,Bash,Grep" {
		t.Fatalf("first occurrence must keep its position: %q", got)
	}
}

// A conversation whose wiring binds nothing gets nothing. That is the intended
// result under capability-as-wiring, not a degradation to be papered over: an
// agent nobody wired can do nothing, like an unclaimed source routing nothing.
func TestAllowedToolsWithNoBindingsIsEmpty(t *testing.T) {
	if got := EffectiveAllowedTools(nil); got != "" {
		t.Fatalf("no bindings must grant nothing, got %q", got)
	}
	if got := EffectiveAllowedTools([][]string{}); got != "" {
		t.Fatalf("empty bindings must grant nothing, got %q", got)
	}
}

func TestAllowedToolsTrimsAndSkipsBlanks(t *testing.T) {
	got := EffectiveAllowedTools([][]string{{" Read ", "", "  "}, {" Bash"}})
	if got != "Read,Bash" {
		t.Fatalf("entries must be trimmed and blanks dropped: %q", got)
	}
}

// An empty toolset contributes nothing rather than a stray separator, which the
// runtime would read as an empty tool name.
func TestAllowedToolsEmptyToolsetContributesNothing(t *testing.T) {
	got := EffectiveAllowedTools([][]string{{}, {"Read"}, {}})
	if got != "Read" {
		t.Fatalf("empty toolsets must not introduce separators: %q", got)
	}
}
