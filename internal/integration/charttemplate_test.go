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
