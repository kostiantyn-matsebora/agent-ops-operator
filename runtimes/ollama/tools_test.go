package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func mkRepo(t *testing.T, defs map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	agents := filepath.Join(dir, ".claude", "agents")
	if err := os.MkdirAll(agents, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range defs {
		if err := os.WriteFile(filepath.Join(agents, name+".md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func eq(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

// ---- reading the definition (ported from tools.test.js) ----

func TestFrontmatterForms(t *testing.T) {
	cases := map[string]struct {
		in   string
		want []string
	}{
		"inline comma list":       {"---\nname: a\ntools: Read, Grep\n---\nbody\n", []string{"Read", "Grep"}},
		"flow list":               {"---\ntools: [Read, \"Bash(git *)\"]\n---\n", []string{"Read", "Bash(git *)"}},
		"block list":              {"---\nname: a\ntools:\n  - Read\n  - Grep\ndescription: x\n---\n", []string{"Read", "Grep"}},
		"no tools key":            {"---\nname: a\n---\nbody\n", nil},
		"no frontmatter":          {"# Just prose\n\nno frontmatter here\n", nil},
		"indented tools is other": {"---\nnested:\n  tools: Bash\n---\n", nil},
		"BOM is tolerated":        {"\ufeff---\ntools: Read\n---\n", []string{"Read"}},
	}
	for name, c := range cases {
		got, err := parseFrontmatterTools(c.in)
		if err != nil {
			t.Errorf("%s: unexpected error %v", name, err)
		}
		eq(t, got, c.want)
	}
}

func TestFrontmatterErrors(t *testing.T) {
	for name, in := range map[string]string{
		"unterminated":       "---\ntools: Read\nnever closed\n",
		"unclosed flow list": "---\ntools: [Read, Grep\n---\n",
	} {
		if _, err := parseFrontmatterTools(in); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestAgentDeclaredTools(t *testing.T) {
	eq(t, agentDeclaredTools(mkRepo(t, nil), "nobody", t.Logf), nil)
	eq(t, agentDeclaredTools("", "x", t.Logf), nil)
	eq(t, agentDeclaredTools(mkRepo(t, nil), "", t.Logf), nil)

	var logged []string
	dir := mkRepo(t, map[string]string{"broken": "---\ntools: Read\nthis file never closes its frontmatter\n"})
	eq(t, agentDeclaredTools(dir, "broken", func(f string, a ...any) { logged = append(logged, f) }), nil)
	if len(logged) != 1 || !strings.Contains(logged[0], "%s.md") {
		t.Errorf("the reason must reach the pod log once, got %v", logged)
	}

	dir = mkRepo(t, map[string]string{"k8s-engineer": "---\nname: k8s-engineer\ntools: Read, Grep\n---\nrole\n"})
	eq(t, agentDeclaredTools(dir, "k8s-engineer", t.Logf), []string{"Read", "Grep"})
}

// ---- composition ----

func TestCompose(t *testing.T) {
	eq(t, composeAllowedTools([]string{"Read", "Grep"}, "Bash", ModeMerge), []string{"Read", "Grep", "Bash"})
	eq(t, composeAllowedTools([]string{"Read", "Grep"}, "Grep,Bash,Read", ModeMerge), []string{"Read", "Grep", "Bash"})
	eq(t, composeAllowedTools([]string{"Read", "Grep"}, "Bash", ModeOverwrite), []string{"Bash"})
	eq(t, composeAllowedTools(nil, "Bash,Read", ModeMerge), []string{"Bash", "Read"})
	eq(t, composeAllowedTools([]string{"Read"}, "", ModeMerge), []string{"Read"})
	// an absent or unknown mode is merge, never overwrite
	eq(t, composeAllowedTools([]string{"Read"}, "Bash", ""), []string{"Read", "Bash"})
	eq(t, composeAllowedTools([]string{"Read"}, "Bash", "nonsense"), []string{"Read", "Bash"})
	// empty means empty
	eq(t, composeAllowedTools(nil, "", ModeMerge), nil)
	eq(t, composeAllowedTools([]string{"Read", "Bash"}, "", ModeOverwrite), nil)
	eq(t, composeAllowedTools(nil, " Read , , Bash ", ModeMerge), []string{"Read", "Bash"})
}

// ---- the gate ----

func TestGate(t *testing.T) {
	r := NewRegistry()
	for _, n := range []string{"Read", "Bash", "mcp__k8s__pods_list", "mcp__k8s__pods_delete", "mcp__ha__state"} {
		r.Add(Tool{Name: n})
	}
	granted, unavailable, ignored, uninterpretable := r.Gate([]string{"Read", "mcp__k8s__*", "Bash(kubectl:*)", "Nope"})
	eq(t, names(granted), []string{"Read", "mcp__k8s__pods_list", "mcp__k8s__pods_delete"})
	eq(t, unavailable, []string{"Nope"})
	eq(t, ignored, []string{"Bash(kubectl:*)"})
	eq(t, uninterpretable, nil)

	granted, _, _, _ = r.Gate(nil)
	if len(granted) != 0 {
		t.Error("an empty allowlist advertises nothing")
	}
}

// A `*` this runtime's matcher cannot read as a wildcard -- anywhere but a
// single trailing position -- must be reported as UNINTERPRETABLE, never
// folded into "unavailable": the pattern defeated the matcher, it is not a
// claim that no tool or server provides it.
func TestGateReportsAMidStringWildcardAsUninterpretable(t *testing.T) {
	r := NewRegistry()
	r.Add(Tool{Name: "mcp__k8s__pods_list"})
	granted, unavailable, ignored, uninterpretable := r.Gate([]string{"mcp__*__pods_list", "a**"})
	if len(granted) != 0 {
		t.Errorf("an uninterpretable pattern must grant nothing: %v", names(granted))
	}
	eq(t, unavailable, nil)
	eq(t, ignored, nil)
	eq(t, uninterpretable, []string{"mcp__*__pods_list", "a**"})
}

// ---- safeJoin ----

func TestSafeJoinRefusesTraversal(t *testing.T) {
	root := t.TempDir()
	if _, err := safeJoin(root, "..", "etc", "passwd"); err == nil {
		t.Error("want an error for a `..` segment escaping root")
	}
	if _, err := safeJoin(root, "a", "..", "..", "b"); err == nil {
		t.Error("want an error for a joined path that nets outside root")
	}
	if p, err := safeJoin(root, "sub", "file.txt"); err != nil || !strings.HasPrefix(p, root) {
		t.Errorf("an ordinary relative path must resolve under root: %q %v", p, err)
	}
}

func TestAgentDeclaredToolsRefusesAnAgentNameThatTraversesOut(t *testing.T) {
	dir := mkRepo(t, nil)
	var logged []string
	got := agentDeclaredTools(dir, "../../../../etc/passwd", func(f string, a ...any) { logged = append(logged, f) })
	if got != nil {
		t.Errorf("a traversing agent name must declare no tools, got %v", got)
	}
	if len(logged) != 1 {
		t.Errorf("the traversal must be logged once, got %v", logged)
	}
}

func names(ts []Tool) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.Name)
	}
	return out
}
