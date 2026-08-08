package dispatch

import (
	"testing"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
)

func binding(mode string, refs ...string) *agentopsv1alpha1.ToolingBinding {
	b := &agentopsv1alpha1.ToolingBinding{Mode: mode}
	for _, r := range refs {
		b.Refs = append(b.Refs, agentopsv1alpha1.ObjectRef{Name: r})
	}
	return b
}

// No binding: the profile's string is handed through verbatim, so every
// pre-existing conversation dispatches exactly the work unit it did before.
func TestAllowedToolsWithoutBindingIsVerbatim(t *testing.T) {
	if got := EffectiveAllowedTools("Read, Bash ,mcp__x__*", nil, nil); got != "Read, Bash ,mcp__x__*" {
		t.Fatalf("binding-less allowlist must not be rewritten: %q", got)
	}
}

func TestAllowedToolsMergeUnionsAndDedups(t *testing.T) {
	got := EffectiveAllowedTools("Read,Bash", binding(agentopsv1alpha1.ToolingMerge, "ts"),
		[][]string{{"Bash", "mcp__victorialogs__*"}, {"mcp__victoriametrics__*", "Read"}})
	if got != "Read,Bash,mcp__victorialogs__*,mcp__victoriametrics__*" {
		t.Fatalf("merge must union in order, first occurrence keeping position: %q", got)
	}
}

func TestAllowedToolsOverwriteDropsProfileEntries(t *testing.T) {
	got := EffectiveAllowedTools("Read,Bash", binding(agentopsv1alpha1.ToolingOverwrite, "ts"),
		[][]string{{"mcp__victorialogs__*"}})
	if got != "mcp__victorialogs__*" {
		t.Fatalf("overwrite must ignore the profile's allowlist, built-ins included: %q", got)
	}
}

// The default (empty) mode is merge — an operator who omits `mode` extends.
func TestAllowedToolsEmptyModeMerges(t *testing.T) {
	got := EffectiveAllowedTools("Read", binding("", "ts"), [][]string{{"Grep"}})
	if got != "Read,Grep" {
		t.Fatalf("empty mode must behave as merge: %q", got)
	}
}

func TestAllowedToolsTrimsAndSkipsBlanks(t *testing.T) {
	got := EffectiveAllowedTools(" Read , , Bash ", binding(agentopsv1alpha1.ToolingMerge, "ts"),
		[][]string{{"", " Grep "}})
	if got != "Read,Bash,Grep" {
		t.Fatalf("entries must be trimmed and blanks dropped: %q", got)
	}
}

// An empty profile allowlist under merge yields only the toolsets' entries —
// no leading comma, which the runtime would read as an empty tool name.
func TestAllowedToolsMergeOverEmptyProfile(t *testing.T) {
	got := EffectiveAllowedTools("", binding(agentopsv1alpha1.ToolingMerge, "ts"), [][]string{{"Bash"}})
	if got != "Bash" {
		t.Fatalf("merge over an empty profile allowlist: %q", got)
	}
}
