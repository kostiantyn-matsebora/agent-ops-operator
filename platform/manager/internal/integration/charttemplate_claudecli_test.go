package integration

import (
	"strings"
	"testing"
)

// Chart-render assertions for the `claude-cli` bundle (openspec change
// claude-runtime-sdk, tasks.md 9.7): the spawn-per-unit claude-code runtime
// `runtimes/claude` carried before this change, kept running as its own
// off-by-default bundle once `runtimes/claude` became SDK-only.
//
// Its values shape matches `chart/charts/claude/values.yaml` field for
// field — no endpoint/model translation the way the ollama bundle needs —
// so this file mirrors TestOllamaBundleRendersOneRuntimeAndNoSubstrate's
// shape rather than copying the ollama-specific env assertions.

// Off by default: the bundle a fresh install never carries, and the only
// AgentRuntime CRs in a default render are `claude` plus its `default` copy.
func TestClaudeCliBundleIsOffByDefault(t *testing.T) {
	out := helmTemplate(t, "--set", "claude.credentialsSecret.token=x")
	if strings.Contains(out, "\n  name: claude-cli\n") {
		t.Fatalf("claude-cli must not render on a default install:\n%s", out)
	}
	if n := strings.Count(out, "\nkind: AgentRuntime\n"); n != 2 {
		t.Fatalf("want claude and its default copy only, got %d", n)
	}
}

// Enabled, it inherits the defaults and carries only what the bundle itself
// states — its own image and its own credential Secret reference, distinct
// from `claude`'s — and renders no substrate of its own (no floor
// ServiceAccount, no context volume).
func TestClaudeCliBundleRendersOneRuntimeAndNoSubstrate(t *testing.T) {
	out := helmTemplate(t,
		"--set", "claude.credentialsSecret.token=x",
		"--set", "claude-cli.enabled=true")
	if n := strings.Count(out, "\nkind: AgentRuntime\n"); n != 3 {
		t.Fatalf("want claude, claude-cli and the default copy, got %d", n)
	}
	rt := findDocByKindAndMetaName(out, "AgentRuntime", "claude-cli")
	if rt == "" {
		t.Fatal("no AgentRuntime named claude-cli rendered")
	}
	for _, want := range []string{
		`image: "ghcr.io/kostiantyn-matsebora/agentops-runtime-claude-cli:`,
		"serviceAccountName: agentops-runtime\n", // the floor, inherited
		"contextStorage: volume\n",
		"idleTtlMinutes: 1\n", // the release default, not the CRD's 10
		".claude/projects/-data-workspace/**",
		// a DIFFERENT credential Secret from claude's own, by design — see
		// chart/charts/claude-cli/values.yaml
		"name: agentops-claude-cli",
	} {
		if !strings.Contains(rt, want) {
			t.Errorf("claude-cli runtime lacks %q:\n%s", want, rt)
		}
	}
	// no substrate: the ServiceAccount count is unchanged from the default render
	if got, want := strings.Count(out, "kind: ServiceAccount\n"), strings.Count(helmTemplate(t, "--set", "claude.credentialsSecret.token=x"), "kind: ServiceAccount\n"); got != want {
		t.Errorf("the bundle must render no ServiceAccount: %d vs %d", got, want)
	}
}

// Both bundles rendering side by side must carry their OWN credential
// Secret each — chart/charts/claude-cli/values.yaml names a different
// Secret from claude's own specifically so one bundle's token never
// overwrites the other's in the same object.
func TestClaudeAndClaudeCliRenderSeparateCredentialSecrets(t *testing.T) {
	out := helmTemplate(t,
		"--set", "claude.credentialsSecret.token=x",
		"--set", "claude-cli.enabled=true",
		"--set", "claude-cli.credentialsSecret.token=y")
	claudeSecret := findDocByKindAndMetaName(out, "Secret", "agentops-claude")
	cliSecret := findDocByKindAndMetaName(out, "Secret", "agentops-claude-cli")
	if claudeSecret == "" {
		t.Fatal("claude's own credential Secret must render")
	}
	if cliSecret == "" {
		t.Fatal("claude-cli's own credential Secret must render, distinct from claude's")
	}
	if !strings.Contains(claudeSecret, "oauthToken: \"x\"") {
		t.Errorf("claude's Secret must carry its own token:\n%s", claudeSecret)
	}
	if !strings.Contains(cliSecret, "oauthToken: \"y\"") {
		t.Errorf("claude-cli's Secret must carry its own token, not claude's:\n%s", cliSecret)
	}
}

// With `claude` disabled and `claude-cli` the only bundle enabled, it becomes
// the default with no flag needed — the same rule TestDefaultRuntimeIsTheFlaggedOrFirstConfigured
// pins for `ollama`.
func TestClaudeCliAloneBecomesDefaultWithNoFlag(t *testing.T) {
	out := helmTemplate(t,
		"--set", "claude.enabled=false",
		"--set", "claude-cli.enabled=true",
		"--set", "claude-cli.credentialsSecret.token=x",
		"--set", "global.demo.enabled=true") // a route naming no runtimeRef
	doc, src := defaultRuntimeDoc(out)
	if src != "claude-cli" {
		t.Errorf("claude-cli alone must be copied as default, got source %q", src)
	}
	if !strings.Contains(doc, "agentops-claude-cli") {
		t.Errorf("the default copy must carry claude-cli's own credential reference:\n%s", doc)
	}
}

// With both `claude` and `claude-cli` enabled, the two bundles' explicit,
// opposing `default:` flags decide which one the parent copies to `default`
// — the exact scenario openspec/changes/claude-runtime-sdk/tasks.md 9.7
// calls for, hand-verified with `helm template` before this test existed.
func TestBothClaudeBundlesEnabledOpposingDefaultFlagsDecide(t *testing.T) {
	base := []string{
		"--set", "claude.credentialsSecret.token=x",
		"--set", "claude-cli.enabled=true",
		"--set", "claude-cli.credentialsSecret.token=y",
		"--set", "global.demo.enabled=true",
	}

	cliDefault := helmTemplate(t, append(append([]string{}, base...),
		"--set", "claude.default=false", "--set", "claude-cli.default=true")...)
	if _, src := defaultRuntimeDoc(cliDefault); src != "claude-cli" {
		t.Errorf("claude-cli.default=true (claude.default=false) must move the default, got %q", src)
	}

	claudeDefault := helmTemplate(t, append(append([]string{}, base...),
		"--set", "claude.default=true", "--set", "claude-cli.default=false")...)
	if _, src := defaultRuntimeDoc(claudeDefault); src != "claude" {
		t.Errorf("claude.default=true (claude-cli.default=false) must keep claude as default, got %q", src)
	}

	// Both flagged true at once is refused by name, same as any two
	// runtimes disagreeing about who answers to `default`.
	both := helmTemplateErr(t, append(append([]string{}, base...),
		"--set", "claude.default=true", "--set", "claude-cli.default=true")...)
	if !strings.Contains(both, "2 runtimes are flagged") {
		t.Errorf("two flagged defaults must fail naming both, got %s", both)
	}
}

// `claude-cli` is a vendor bundle, so `agentops.defaultRuntimeGuard` must
// still pass with it declared and off — an off bundle contributes nothing
// to the default-runtime check (tasks.md 8.4, pinned here as a render test
// rather than only hand-verified).
func TestClaudeCliOffContributesNothingToTheDefaultRuntimeGuard(t *testing.T) {
	out := helmTemplate(t, "--set", "claude.credentialsSecret.token=x")
	if strings.Contains(out, "\n  name: claude-cli\n") {
		t.Fatalf("an off claude-cli must render nothing for the guard to see:\n%s", out)
	}
	if doc, src := defaultRuntimeDoc(out); src != "claude" || doc == "" {
		t.Errorf("claude alone must still satisfy the guard as today, got source %q", src)
	}
}
