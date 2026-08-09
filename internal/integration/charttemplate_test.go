package integration

import (
	"os/exec"
	"strings"
	"testing"
)

// Chart-render assertions for the console bundle.
//
// The properties worth pinning are the ones a template edit can quietly break:
// that disabling it renders NOTHING, that the chart ships no workload objects
// (the reconciler owns them), and that the RBAC it does ship is read-only and
// scoped to agentops.dev. Skipped when helm is not installed — CI has it.

func helmTemplate(t *testing.T, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not installed")
	}
	cmd := exec.Command("helm", append([]string{"template", "test", "../../chart"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helm template failed: %v\n%s", err, out)
	}
	return string(out)
}

// helmTemplateErr renders expecting FAILURE and returns the message. Used for
// the guards whose whole value is that they fire.
func helmTemplateErr(t *testing.T, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not installed")
	}
	cmd := exec.Command("helm", append([]string{"template", "test", "../../chart"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("helm template unexpectedly succeeded:\n%s", out)
	}
	return string(out)
}

func TestConsoleRendersNothingWhenDisabled(t *testing.T) {
	out := helmTemplate(t)
	// console-specific names only: "kind: ChannelAdapter" appears in the CRD
	// definition itself, which ships regardless
	for _, needle := range []string{"agentops-console", "agentops-adapter-console", "app.kubernetes.io/name: agentops-console"} {
		if strings.Contains(out, needle) {
			t.Fatalf("console.enabled=false must render nothing, found %q", needle)
		}
	}
}

func TestConsoleBundleIsCRsAndRBACOnly(t *testing.T) {
	out := helmTemplate(t, "--set", "console.enabled=true")

	for _, needle := range []string{
		"kind: ChannelAdapter",
		"kubernetesAccess: true",
		"singleton: true",
		"port: 8080",
		"kind: Channel",
		"adapter: console",
		"credentialsSecretRef",
		"uiToken",
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("console bundle missing %q", needle)
		}
	}

	// the reconciler owns the workload and the Service — a chart-shipped one
	// would make the console deployable only by this chart
	for _, doc := range splitDocs(out) {
		if !strings.Contains(doc, "agentops-adapter-console") {
			continue
		}
		for _, forbidden := range []string{"kind: Deployment", "kind: Service\n"} {
			if strings.Contains(doc, forbidden) {
				t.Fatalf("chart must not ship %s for the console:\n%s", forbidden, doc)
			}
		}
	}
}

func TestConsoleRoleIsReadOnlyAgentopsOnly(t *testing.T) {
	out := helmTemplate(t, "--set", "console.enabled=true")
	var role string
	for _, doc := range splitDocs(out) {
		// "\nkind: Role\n" and not the RoleBinding's indented roleRef.kind
		if strings.Contains(doc, "\nkind: Role\n") && strings.Contains(doc, "name: agentops-adapter-console") {
			role = doc
		}
	}
	if role == "" {
		t.Fatal("console Role not rendered")
	}
	if !strings.Contains(role, `verbs: ["get", "list", "watch"]`) {
		t.Fatalf("console Role verbs changed:\n%s", role)
	}
	// assert on the RULES, not the prose above them: the template's own comment
	// says the words this check forbids
	rules := stripComments(role)
	for _, forbidden := range []string{"create", "update", "patch", "delete", "secrets", "pods"} {
		if strings.Contains(rules, forbidden) {
			t.Fatalf("console Role must not grant %q:\n%s", forbidden, rules)
		}
	}
	// only the agentops group
	if strings.Count(rules, "apiGroups:") != 1 || !strings.Contains(rules, `apiGroups: ["agentops.dev"]`) {
		t.Fatalf("console Role must cover agentops.dev only:\n%s", rules)
	}
}

func splitDocs(rendered string) []string {
	return strings.Split(rendered, "\n---\n")
}

func stripComments(doc string) string {
	var kept []string
	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// ---- k8s-bundle events lane -------------------------------------------------

// The events adapter now reads pods and replicasets to resolve an event's
// workload and to re-check liveness. The operator grants adapters nothing, so
// the chart is the only place that grant can come from — and if it silently
// went missing the adapter would report Ready=False forever.
func TestEventsAdapterRBACCoversPodsAndReplicaSets(t *testing.T) {
	for _, mode := range []struct{ name, flag, kind string }{
		{"cluster-wide", "true", "ClusterRole"},
		{"namespaced", "false", "Role"},
	} {
		t.Run(mode.name, func(t *testing.T) {
			out := helmTemplate(t, "--set", "k8s-bundle.enabled=true",
				"--set", "k8s-bundle.eventsAdapter.rbac.clusterWide="+mode.flag)

			// Anchor on the document's OWN kind line, which is followed by
			// metadata:. A binding names the same kind and the same name in
			// its roleRef, so both "kind: Role" and the name are ambiguous.
			var found string
			for _, doc := range splitDocs(out) {
				if strings.Contains(doc, "kind: "+mode.kind+"\nmetadata:") &&
					strings.Contains(doc, "name: agentops-signal-k8s-events-events") {
					found = stripComments(doc)
				}
			}
			if found == "" {
				t.Fatalf("no %s for the events adapter rendered", mode.kind)
			}
			for _, needle := range []string{`resources: ["events"]`, `resources: ["pods"]`, `resources: ["replicasets"]`} {
				if !strings.Contains(found, needle) {
					t.Errorf("%s missing %s\n%s", mode.kind, needle, found)
				}
			}
		})
	}
}

// Grouping by pod name is what produced hundreds of conversations: pod names
// are unique per replica and regenerated on every rollout, so the signature
// never repeated and window reuse could never fire.
func TestEventsSourceGroupsByWorkload(t *testing.T) {
	out := helmTemplate(t, "--set", "k8s-bundle.enabled=true")
	src := eventsSourceDoc(t, out)
	if !strings.Contains(src, "- workload") {
		t.Fatalf("the events source must group by workload:\n%s", src)
	}
	if strings.Contains(src, "- name\n") {
		t.Fatalf("per-pod grouping must be gone:\n%s", src)
	}
}

// The default rule set's SHAPE is what keeps it safe, and the shape is what a
// well-meaning edit breaks. Pin the invariants, not the tuning: the numbers
// should stay editable without anyone having to re-derive these properties.
func TestDefaultRulesShape(t *testing.T) {
	out := helmTemplate(t, "--set", "k8s-bundle.enabled=true")
	src := eventsSourceDoc(t, out)

	// Reasons describing something that ALREADY happened must never dwell: a
	// re-check would find the healthy replacement and erase the incident.
	pastTense := []string{"OOMKilling", "SystemOOM", "Evicted", "BackoffLimitExceeded", "DeadlineExceeded"}
	for _, reason := range pastTense {
		line := ruleLineContaining(src, reason)
		if line == "" {
			t.Errorf("past-tense reason %q is not covered by any rule", reason)
			continue
		}
		if !strings.Contains(line, `for: "0"`) {
			t.Errorf("past-tense reason %q must carry for: \"0\", got rule:\n%s", reason, line)
		}
	}

	// The last rule must be a catch-all WITH a dwell, or an unanticipated
	// reason is silently discarded instead of verified.
	rules := ruleBlocks(src)
	if len(rules) == 0 {
		t.Fatal("no default rules rendered")
	}
	last := rules[len(rules)-1]
	if !strings.Contains(last, "matchers: []") {
		t.Fatalf("the last rule must be a catch-all:\n%s", last)
	}
	if strings.Contains(last, "action: drop") {
		t.Fatalf("the catch-all must never be a drop:\n%s", last)
	}
	if !strings.Contains(last, "for:") {
		t.Fatalf("the catch-all must carry a dwell:\n%s", last)
	}
}

// ---- runtime ownership ------------------------------------------------------

// The substrate is the PARENT's: one AgentRuntime, one runtime ServiceAccount,
// whatever bundles are on. The bundle used to ship its own — which is how two
// runtime identities came to exist, one of them granted everything.
func TestParentOwnsExactlyOneRuntime(t *testing.T) {
	for _, combo := range [][]string{
		nil,
		{"--set", "global.demo.enabled=true"},
		{"--set", "k8s-bundle.enabled=true"},
		{"--set", "telegram-bundle.enabled=true"},
		{"--set", "k8s-bundle.enabled=true", "--set", "telegram-bundle.enabled=true"},
	} {
		name := "defaults"
		if len(combo) > 0 {
			name = strings.Join(combo[1:], ",")
		}
		t.Run(name, func(t *testing.T) {
			out := helmTemplate(t, combo...)
			if n := strings.Count(out, "\nkind: AgentRuntime\n"); n != 1 {
				t.Errorf("want exactly 1 AgentRuntime, got %d", n)
			}
			var sas int
			for _, doc := range splitDocs(out) {
				if strings.Contains(doc, "kind: ServiceAccount\nmetadata:\n  name: agentops-runtime\n") {
					sas++
				}
			}
			if sas != 1 {
				t.Errorf("want exactly 1 runtime ServiceAccount, got %d", sas)
			}
			// the bundle-named identity must be gone everywhere, bindings included
			if strings.Contains(out, "agentops-runtime-k8s") {
				t.Error("the bundle-named runtime ServiceAccount must not render")
			}
		})
	}
}

// "Bring your own runtime": the component renders nothing, but the SA stays —
// the manager defaults every runtime pod onto it whoever wrote the CR.
func TestRuntimeDisabledRendersNoRuntimeObjects(t *testing.T) {
	out := helmTemplate(t, "--set", "runtime.enabled=false",
		"--set", "runtime.credentialsSecret.token=x")
	// anchored: the CRD document names the kind too, and it ships regardless
	if strings.Contains(out, "\nkind: AgentRuntime\n") {
		t.Error("runtime.enabled=false must render no AgentRuntime")
	}
	if strings.Contains(out, "name: agentops-claude") {
		t.Error("runtime.enabled=false must render no credential Secret")
	}
	if !strings.Contains(out, "kind: ServiceAccount\nmetadata:\n  name: agentops-runtime\n") {
		t.Error("the runtime ServiceAccount is not part of the component and must still render")
	}
}

// The release has ONE idle-TTL number. The field must be WRITTEN, not omitted:
// AgentRuntime.spec.idleTtlMinutes carries a CRD default of 10, so an omitted
// field is stored as 10, and the manager prefers any non-zero spec value over
// RUNTIME_IDLE_TTL_M — omitting it looks right in the manifest and silently
// ignores runtimeIdleTtlMinutes in the cluster.
func TestRuntimeIdleTTLFollowsTheReleaseDefault(t *testing.T) {
	out := helmTemplate(t, "--set", "runtimeIdleTtlMinutes=7")
	if !strings.Contains(out, "idleTtlMinutes: 7") {
		t.Error("an empty runtime.idleTtlMinutes must follow runtimeIdleTtlMinutes")
	}
	out = helmTemplate(t, "--set", "runtimeIdleTtlMinutes=7", "--set", "runtime.idleTtlMinutes=30")
	if !strings.Contains(out, "idleTtlMinutes: 30") {
		t.Error("an explicit runtime.idleTtlMinutes must win")
	}
}

// Empty rbacMode grants NOTHING outside demo mode — defaulting it to readonly
// would silently bind cluster `view` on every upgrade. `full` is never inferred.
func TestRuntimeRbacModeResolution(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		want    []string
		notWant []string
	}{
		{"unset grants nothing", nil, nil,
			[]string{"agentops-runtime-view", "agentops-runtime-cluster-admin"}},
		{"demo is read-only", []string{"--set", "global.demo.enabled=true"},
			[]string{"agentops-runtime-view", "agentops-runtime-cluster-ro"},
			[]string{"agentops-runtime-cluster-admin"}},
		{"none", []string{"--set", "global.agentops.runtime.rbacMode=none", "--set", "global.demo.enabled=true"}, nil,
			[]string{"agentops-runtime-view", "agentops-runtime-cluster-admin"}},
		{"full", []string{"--set", "global.agentops.runtime.rbacMode=full"},
			[]string{"agentops-runtime-cluster-admin"}, nil},
		{"targeted grants compose with the mode",
			[]string{"--set", "global.agentops.runtime.rbacMode=readonly", "--set", "rbac.runtime.bindClusterRoles={edit}"},
			[]string{"agentops-runtime-view", "agentops-runtime-edit"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := helmTemplate(t, tc.args...)
			for _, needle := range tc.want {
				if !strings.Contains(out, needle) {
					t.Errorf("missing %q", needle)
				}
			}
			for _, needle := range tc.notWant {
				if strings.Contains(out, needle) {
					t.Errorf("must not render %q", needle)
				}
			}
		})
	}
}

// The old key would otherwise be read by nothing, running agents under an
// identity the operator did not choose.
func TestMovedRuntimeSAKeyFails(t *testing.T) {
	msg := helmTemplateErr(t, "--set", "serviceAccounts.runtime=agentops-runtime-k8s")
	if !strings.Contains(msg, "global.agentops.runtime.serviceAccountName") {
		t.Fatalf("the failure must name the new key:\n%s", msg)
	}
}

// ---- k8s-bundle MCP ---------------------------------------------------------

// mcp and mcpServers flip together, so the config's URL always has a Service to
// default onto. The guard exists for the combination that is genuinely broken.
func TestMCPEndpointGuardStillBites(t *testing.T) {
	msg := helmTemplateErr(t, "--set", "k8s-bundle.enabled=true",
		"--set", "k8s-bundle.mcpServers.enabled=false")
	if !strings.Contains(msg, "mcp.url is required") {
		t.Fatalf("the endpoint guard must name the missing URL:\n%s", msg)
	}
}

// One knob configures both identities coherently: with derivation, rbacMode
// full must render the mutating toolset and a write-capable server with no
// other value set — and an explicit readOnly must still recover the separation.
func TestMCPServerDerivesFromRuntimeRbacMode(t *testing.T) {
	readOnly := helmTemplate(t, "--set", "k8s-bundle.enabled=true")
	if !strings.Contains(readOnly, "- --read-only") {
		t.Error("default posture must be a read-only server")
	}
	if strings.Contains(readOnly, "name: k8s-admin") {
		t.Error("no mutating toolset without a server that registers those tools")
	}

	full := helmTemplate(t, "--set", "k8s-bundle.enabled=true",
		"--set", "global.agentops.runtime.rbacMode=full")
	if strings.Contains(full, "- --read-only") {
		t.Error("rbacMode=full must yield a write-capable server")
	}
	if !strings.Contains(full, "name: k8s-admin") {
		t.Error("rbacMode=full must render the mutating toolset with no other value set")
	}
	if !strings.Contains(full, "name: agentops-mcp-k8s-cluster-admin") {
		t.Error("rbacMode=full must yield a full server ServiceAccount")
	}

	recovered := helmTemplate(t, "--set", "k8s-bundle.enabled=true",
		"--set", "global.agentops.runtime.rbacMode=full",
		"--set", "k8s-bundle.mcpServers.readOnly=true")
	if !strings.Contains(recovered, "- --read-only") {
		t.Error("an explicit readOnly must win over the derivation")
	}
	if strings.Contains(recovered, "name: k8s-admin") {
		t.Error("a read-only server must not render the mutating toolset")
	}
}

// Collapsing the two identities removes the only thing this component adds
// over kubectl. The guard now compares against the release-wide SA.
func TestMCPServerRefusesTheRuntimeIdentity(t *testing.T) {
	msg := helmTemplateErr(t, "--set", "k8s-bundle.enabled=true",
		"--set", "k8s-bundle.mcpServers.serviceAccountName=agentops-runtime")
	if !strings.Contains(msg, "global.agentops.runtime.serviceAccountName") {
		t.Fatalf("the guard must name the global key:\n%s", msg)
	}
}

func eventsSourceDoc(t *testing.T, rendered string) string {
	t.Helper()
	for _, doc := range splitDocs(rendered) {
		if strings.Contains(doc, "kind: SignalSource") && strings.Contains(doc, "name: cluster-events") {
			return stripComments(doc)
		}
	}
	t.Fatal("no cluster-events SignalSource rendered")
	return ""
}

// ruleBlocks splits the rendered `rules:` list into its entries.
func ruleBlocks(src string) []string {
	_, after, ok := strings.Cut(src, "\n    rules:\n")
	if !ok {
		return nil
	}
	body, _, _ := strings.Cut(after, "\n    severities:")
	var out []string
	for _, chunk := range strings.Split(body, "\n    - ") {
		if strings.TrimSpace(chunk) != "" {
			out = append(out, chunk)
		}
	}
	return out
}

func ruleLineContaining(src, reason string) string {
	for _, block := range ruleBlocks(src) {
		if strings.Contains(block, reason) {
			return block
		}
	}
	return ""
}
