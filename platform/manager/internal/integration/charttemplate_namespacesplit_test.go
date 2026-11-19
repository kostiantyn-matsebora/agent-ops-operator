// Chart render assertions for split-conversation-namespace (task 9.9): the
// placement rule's table, read back from `helm template` output rather than
// from the templates themselves. No Kubernetes API call anywhere in this
// file — every assertion is a string check on rendered YAML, exactly like
// every other charttemplate_*_test.go file.
package integration

import (
	"slices"
	"strings"
	"testing"
)

// TestSplitRendersTwoManagerRolesAndNoNewClusterRole pins decision 4: a
// manager Role (not ClusterRole) per namespace, and the only manager
// ClusterRole anywhere is the pre-existing, opt-in drain-awareness one.
func TestSplitRendersTwoManagerRolesAndNoNewClusterRole(t *testing.T) {
	out := helmTemplate(t)

	controlRole := findDocByKindAndName(t, out, "Role", "agentops-manager")
	if !strings.Contains(controlRole, "namespace: default") {
		t.Errorf("the control Role must render in the release namespace:\n%s", controlRole)
	}
	convRole := findDocByKindAndName(t, out, "Role", "agentops-manager-conversations")
	if !strings.Contains(convRole, "namespace: agent-ops-conversations") {
		t.Errorf("the conversations Role must render in the conversations namespace:\n%s", convRole)
	}
	if strings.Contains(out, "kind: ClusterRole") {
		t.Errorf("no ClusterRole may render with drain awareness off:\n%s", out)
	}

	// drain awareness on: still exactly the pre-existing nodes ClusterRole,
	// never a second one for either manager Role.
	withDrain := helmTemplate(t, "--set", "rbac.drainAware=true")
	if n := strings.Count(withDrain, "\nkind: ClusterRole\n"); n != 1 {
		t.Errorf("drain awareness must add exactly one ClusterRole (nodes), got %d", n)
	}
}

// TestSplitRuntimeAccountsRenderInConversationsNamespace pins decision 1's
// "rbac.runtime.serviceAccounts[]" row: an additional runtime identity is
// what a runtime pod runs as, so it follows the pod.
func TestSplitRuntimeAccountsRenderInConversationsNamespace(t *testing.T) {
	out := helmTemplate(t, "--set-json",
		`rbac.runtime.serviceAccounts=[{"name":"agentops-runtime-acting"}]`)

	sa := findDocByKindAndName(t, out, "ServiceAccount", "agentops-runtime-acting")
	if !strings.Contains(sa, "namespace: agent-ops-conversations") {
		t.Errorf("a declared runtime account must render in the conversations namespace:\n%s", sa)
	}
}

// TestSplitKubernetesBundleRouteAccountRendersInConversations pins the
// k8s-bundle spec: a route ServiceAccount (and every binding naming it)
// follows the pods it identifies, into the conversations namespace, while
// the rest of the bundle — the Pipeline, the profile, the toolsets — stays
// in the release namespace.
func TestSplitKubernetesBundleRouteAccountRendersInConversations(t *testing.T) {
	out := helmTemplate(t,
		"--set", "kubernetes.enabled=true", "--set", "kubernetes.wiringEnabled=true",
		"--set-json", `kubernetes.pipelines.observe.rbac.clusterRoles=[{"name":"test-grant","rules":[{"apiGroups":[""],"resources":["pods"],"verbs":["get"]}]}]`,
	)

	sa := findDocByKindAndName(t, out, "ServiceAccount", "agentops-k8s-observe")
	if !strings.Contains(sa, "namespace: agent-ops-conversations") {
		t.Errorf("the route account must render in the conversations namespace:\n%s", sa)
	}
	binding := findDocByKindAndName(t, out, "ClusterRoleBinding", "agentops-k8s-observe-test-grant-default")
	if !strings.Contains(binding, "namespace: agent-ops-conversations") {
		t.Errorf("the binding's subject must name the account in the conversations namespace:\n%s", binding)
	}

	pipe := findDocByKindAndName(t, out, "Pipeline", "k8s-observe")
	if !strings.Contains(pipe, "namespace: default") {
		t.Errorf("the Pipeline itself is control-plane wiring and stays in the release namespace:\n%s", pipe)
	}
}

// TestSplitClaimsRenderInConversationsNamespace pins decision 1's claims
// row: every chart-rendered persistence claim is a workload sharing a
// runtime pod's claims.
func TestSplitClaimsRenderInConversationsNamespace(t *testing.T) {
	out := helmTemplate(t) // the context claim is on by default

	claim := findDocByKindAndName(t, out, "PersistentVolumeClaim", "agentops-context")
	if !strings.Contains(claim, "namespace: agent-ops-conversations") {
		t.Errorf("the context claim must render in the conversations namespace:\n%s", claim)
	}
}

// TestSplitCronJobsRenderInConversationsNamespace pins decision 1: both
// housekeeping and the context-probe mount the same claims a runtime pod
// does, and must render beside them. The context-probe also needs its own
// agentops app label — agentops-context-probe matches no self-exclusion
// name prefix, so without it an unprefixed pod in the conversations
// namespace depends entirely on mechanism 4 for exclusion in a
// single-namespace release where mechanism 4 never applies.
func TestSplitCronJobsRenderInConversationsNamespace(t *testing.T) {
	out := helmTemplate(t,
		"--set", "housekeeping.enabled=true",
		"--set", "contextProbe.enabled=true",
		"--set", "claude.credentialsSecret.token=x")

	hk := findDocByKindAndName(t, out, "CronJob", "agentops-housekeeping")
	if !strings.Contains(hk, "namespace: agent-ops-conversations") {
		t.Errorf("housekeeping's CronJob must render in the conversations namespace:\n%s", hk)
	}

	probe := findDocByKindAndName(t, out, "CronJob", "agentops-context-probe")
	if !strings.Contains(probe, "namespace: agent-ops-conversations") {
		t.Errorf("the context-probe CronJob must render in the conversations namespace:\n%s", probe)
	}
	if !strings.Contains(probe, "app.kubernetes.io/name: agentops-context-probe") {
		t.Errorf("the context-probe must carry its own agentops app label:\n%s", probe)
	}
}

// TestSplitFloorExistsInBothNamespacesBoundToNothing pins the floor
// account's own row: rendered in BOTH namespaces, and named as a
// RoleBinding/ClusterRoleBinding subject in NEITHER — anywhere in the
// whole render, across every bundle this suite can turn on.
func TestSplitFloorExistsInBothNamespacesBoundToNothing(t *testing.T) {
	out := helmTemplate(t,
		"--set", "global.demo.enabled=true",
		"--set", "telegram.enabled=true",
		"--set", "console.enabled=true",
		"--set", "housekeeping.enabled=true",
		"--set", "rbac.drainAware=true")

	controlFloor := findDocByKindAndName(t, out, "ServiceAccount", "agentops-runtime")
	if !strings.Contains(controlFloor, "namespace: default") {
		t.Errorf("the floor must render in the control namespace:\n%s", controlFloor)
	}
	if n := countDocsContaining(out, "kind: ServiceAccount\nmetadata:\n  name: agentops-runtime\n"); n != 2 {
		t.Fatalf("the floor must render in BOTH namespaces exactly once each, got %d docs", n)
	}
	convFloorFound := false
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "kind: ServiceAccount\nmetadata:\n  name: agentops-runtime\n") &&
			strings.Contains(doc, "namespace: agent-ops-conversations") {
			convFloorFound = true
		}
	}
	if !convFloorFound {
		t.Error("the floor must render in the conversations namespace too")
	}

	// no RoleBinding or ClusterRoleBinding anywhere names the floor as a
	// subject — the search is over every binding doc in the WHOLE render,
	// not just rbac.yaml's own, since a bundle could in principle bind it.
	for _, doc := range splitDocs(out) {
		if !strings.Contains(doc, "kind: RoleBinding") && !strings.Contains(doc, "kind: ClusterRoleBinding") {
			continue
		}
		if strings.Contains(doc, "name: agentops-runtime\n") {
			t.Errorf("a binding names the floor account as a subject, which must never happen:\n%s", doc)
		}
	}
}

// TestSplitConversationNamespaceCarriesResourcePolicyKeep pins decision 9:
// the chart-created namespace survives `helm uninstall`.
func TestSplitConversationNamespaceCarriesResourcePolicyKeep(t *testing.T) {
	out := helmTemplate(t)
	ns := findDocByKindAndName(t, out, "Namespace", "agent-ops-conversations")
	if !strings.Contains(ns, "helm.sh/resource-policy: keep") {
		t.Errorf("the chart-created conversations namespace must carry resource-policy: keep:\n%s", ns)
	}
	if !strings.Contains(ns, "app.kubernetes.io/name: agentops-conversations") {
		t.Errorf("the chart-created namespace must carry a release label:\n%s", ns)
	}
}

// TestSingleNamespaceRenderMatchesThePreSplitShape pins decision 2's
// "single-namespace sets both to one value" claim at the chart level:
// pointing conversationNamespace at the release namespace must render
// NO Namespace object, NO second manager Role, and the floor account
// exactly ONCE — the exact shape a pre-split chart rendered.
func TestSingleNamespaceRenderMatchesThePreSplitShape(t *testing.T) {
	out := helmTemplate(t, "--set", "global.agentops.conversationNamespace=default")

	if strings.Contains(out, "\nkind: Namespace\n") {
		t.Errorf("equal namespaces must render no Namespace object:\n%s", out)
	}
	if strings.Contains(out, "agentops-manager-conversations") {
		t.Errorf("equal namespaces must render no second manager Role:\n%s", out)
	}
	role := findDocByKindAndName(t, out, "Role", "agentops-manager")
	for _, rule := range []string{`"conversations"`, `"pods"`, `"agentprofiles"`} {
		if !strings.Contains(role, rule) {
			t.Errorf("the single, merged Role must still carry every rule (missing %s):\n%s", rule, role)
		}
	}
	if n := countDocsContaining(out, "kind: ServiceAccount\nmetadata:\n  name: agentops-runtime\n"); n != 1 {
		t.Errorf("equal namespaces must render the floor exactly once, got %d", n)
	}
}

// secretNamespaces returns, for the Secret named name, every namespace it
// renders into.
func secretNamespaces(out, name string) []string {
	var ns []string
	for _, doc := range splitDocs(out) {
		if !strings.Contains(doc, "kind: Secret\nmetadata:") || !strings.Contains(doc, "\n  name: "+name+"\n") {
			continue
		}
		for _, line := range strings.Split(doc, "\n") {
			if v, ok := strings.CutPrefix(line, "  namespace: "); ok {
				ns = append(ns, v)
			}
		}
	}
	return ns
}

// runtimeSecretArgs enables every bundle that creates a credential a runtime
// pod resolves, each supplied as a token so the chart is the one making it.
func runtimeSecretArgs() []string {
	return append(pulseEnabledArgs(),
		"--set", "claude.credentialsSecret.token=claude-tok",
		"--set", "home-assistant.enabled=true",
		"--set", "home-assistant.homeAssistant.endpoint=https://ha.example.org",
		"--set", "home-assistant.homeAssistant.credentials.controlToken=control-tok",
		"--set", "home-assistant.homeAssistant.credentials.operatorToken=operator-tok",
	)
}

// runtimeSecrets are the chart-created Secrets a runtime pod resolves — the
// agent's model credential, the home-assistant control and operator tokens,
// and the pulse observer and operator MCP tokens.
var runtimeSecrets = []string{
	"agentops-claude", "agentops-ha-control", "agentops-ha-operator",
	"agentops-pulse-observer", "agentops-pulse-operator",
}

// TestSplitRuntimeSecretsRenderWhereTheirReadersRun: the kubelet reads a Secret
// only from its own pod's namespace and the manager copies none, so every
// credential a runtime pod names must be rendered where the pod runs, with no
// hand-copy. A Secret ONLY runtime pods read renders there alone — a second
// copy in the release namespace would be a credential nothing reads. The one
// exception is the home-assistant operator token, which the admin MCP server
// and the log source also read from the release namespace.
func TestSplitRuntimeSecretsRenderWhereTheirReadersRun(t *testing.T) {
	out := helmTemplate(t, runtimeSecretArgs()...)
	for _, name := range runtimeSecrets {
		want := []string{"agent-ops-conversations"}
		if name == "agentops-ha-operator" {
			want = []string{"default", "agent-ops-conversations"}
		}
		got := secretNamespaces(out, name)
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s must render in exactly %v, rendered in %v", name, want, got)
		}
	}
}

// TestSingleNamespaceRuntimeSecretsRenderOnce: equal namespaces must not
// render a Secret twice — two documents with one name is a duplicate.
func TestSingleNamespaceRuntimeSecretsRenderOnce(t *testing.T) {
	out := helmTemplate(t, append(runtimeSecretArgs(), "--set", "global.agentops.conversationNamespace=default")...)
	for _, name := range runtimeSecrets {
		if ns := secretNamespaces(out, name); len(ns) != 1 || ns[0] != "default" {
			t.Errorf("%s must render exactly once, in the release namespace, got %v", name, ns)
		}
	}
}
