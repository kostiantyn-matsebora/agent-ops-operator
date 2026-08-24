package integration

import (
	"strings"
	"testing"
)

// Chart-render assertions for the RUNTIME model: how runtimes are declared,
// what a route naming no account holds, and the guards that stop a values file
// from being silently ignored.
//
// Helm NEVER reports an unread values key, so the retired-key tests below are
// not belt-and-braces — without them a values file left on an old spelling
// installs successfully and does something else.

// The defaults must be SUFFICIENT: an install supplying only the model
// credential gets one working runtime named `default`, and the numbers a
// conversation pod requests are visible in the values rather than compiled into
// the operator.
func TestDefaultInstallRendersOneRuntimeNamedDefault(t *testing.T) {
	out := helmTemplate(t, "--set", "global.agentops.runtimeDefaults.credentialsSecret.token=x")
	if strings.Count(out, "kind: AgentRuntime\n") != 1 {
		t.Fatalf("a default install must render exactly one AgentRuntime:\n%s", out)
	}
	for _, needle := range []string{"name: default", "memory: 1536Mi", "memory: 256Mi", "contextStorage: volume"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("default runtime must carry %q — the defaults are meant to be complete", needle)
		}
	}
}

// A second entry states only what differs. Everything else is inherited, or the
// defaults block is documenting nothing.
func TestSecondRuntimeInheritsEveryDefault(t *testing.T) {
	out := helmTemplate(t,
		"--set", "global.agentops.runtimeDefaults.credentialsSecret.token=x",
		"--set-json", `runtimes=[{"name":"ollama","image":"example.com/ollama:1"}]`)
	if !strings.Contains(out, "image: \"example.com/ollama:1\"") {
		t.Fatalf("the declared runtime must carry its own image:\n%s", out)
	}
	// inherited, not restated in values
	for _, needle := range []string{"memory: 1536Mi", "contextStorage: volume", "port: 15001"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("a runtime stating only name+image must inherit %q", needle)
		}
	}
}

// Egress mediation is ON by default now: the wall that constrains an
// uncooperative agent must not be something an operator has to discover.
func TestEgressMediationIsOnByDefault(t *testing.T) {
	out := helmTemplate(t, "--set", "global.agentops.runtimeDefaults.credentialsSecret.token=x")
	if !strings.Contains(out, "egressMediation:") {
		t.Fatal("egress mediation must be declared on the default runtime")
	}
	if !strings.Contains(out, "EGRESS_PROXY_IMAGE") {
		t.Fatal("the manager needs the proxy image as bootstrap configuration when any runtime asks for mediation")
	}
}

// ...and a runtime declining it gets a pod with nothing added. `false` is a
// zero value, which is exactly what a naive mergeOverwrite would drop.
func TestARuntimeCanDeclineEgressMediation(t *testing.T) {
	out := helmTemplate(t,
		"--set", "global.agentops.runtimeDefaults.credentialsSecret.token=x",
		"--set", "claude.egressMediation.enabled=false")
	if strings.Contains(out, "egressMediation:") {
		t.Fatalf("a runtime declaring egressMediation.enabled=false must render no stanza:\n%s", out)
	}
	if strings.Contains(out, "EGRESS_PROXY_IMAGE") {
		t.Fatal("no runtime asks for mediation, so the manager must not be told to build one")
	}
}

// The floor is created and bound to nothing; the account the DEFAULT points at
// is a reference this chart never creates. Naming is not creating.
func TestNamingTheDefaultAccountDoesNotCreateIt(t *testing.T) {
	out := helmTemplate(t,
		"--set", "global.agentops.runtimeDefaults.credentialsSecret.token=x",
		"--set", "global.agentops.runtimeDefaults.serviceAccountName=an-account-i-own")
	if strings.Contains(out, "name: an-account-i-own\n  namespace:") {
		t.Fatalf("the chart must reference the named default, never create it:\n%s", out)
	}
	if !strings.Contains(out, "name: agentops-runtime\n  namespace:") {
		t.Fatal("the floor account must render regardless, so one route can be taken back to nothing")
	}
	if !strings.Contains(out, "serviceAccountName: an-account-i-own") {
		t.Fatal("the runtime CR must name the install's default account")
	}
}

// SILENCE MEANS NO POWER: with nothing declared, no ClusterRoleBinding names
// any runtime account at all.
func TestDefaultInstallBindsNothingToAnyRuntimeAccount(t *testing.T) {
	out := helmTemplate(t, "--set", "global.agentops.runtimeDefaults.credentialsSecret.token=x")
	for _, doc := range strings.Split(out, "\n---\n") {
		if !strings.Contains(doc, "kind: ClusterRoleBinding") && !strings.Contains(doc, "kind: RoleBinding") {
			continue
		}
		if strings.Contains(doc, "name: agentops-runtime\n") {
			t.Fatalf("nothing may ever bind to the floor account:\n%s", doc)
		}
	}
}

// ...and the chart refuses outright to be asked to.
func TestBindingTheFloorAccountIsRefused(t *testing.T) {
	out := helmTemplateErr(t, "--set-json",
		`rbac={"runtime":{"serviceAccounts":[{"name":"agentops-runtime","rbacMode":"full"}]}}`)
	if !strings.Contains(out, "FLOOR account") {
		t.Fatalf("binding the floor must be refused naming it:\n%s", out)
	}
}

// A declared account IS rendered and bound — that is the only way to grant
// anything now.
func TestDeclaredAccountIsRenderedAndBound(t *testing.T) {
	out := helmTemplate(t,
		"--set", "global.agentops.runtimeDefaults.credentialsSecret.token=x",
		"--set-json", `rbac={"runtime":{"serviceAccounts":[{"name":"agentops-runtime-acting","rbacMode":"full"}]}}`)
	if !strings.Contains(out, "name: agentops-runtime-acting\n  namespace:") {
		t.Fatalf("a declared account must be created:\n%s", out)
	}
	if !strings.Contains(out, "kind: ClusterRoleBinding") {
		t.Fatal("a declared account with a posture must be bound")
	}
}

// THE DEFAULT-RUNTIME GUARD. It replaces the rule that the parent always
// rendered `default`, which cannot survive the runtime shipping in a bundle.
func TestMissingDefaultRuntimeFailsTheRender(t *testing.T) {
	out := helmTemplateErr(t,
		"--set", "claude.enabled=false",
		"--set-json", `pipelines=[{"name":"house-ops","profile":"p"}]`)
	for _, needle := range []string{`"default"`, "house-ops"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("the guard must name the missing runtime and the routes: %q missing from\n%s", needle, out)
		}
	}
}

// A demo install with no runtime is the case the guard exists for: the route
// comes from a bundle, so nothing in `pipelines:` would have shown it.
func TestMissingDefaultRuntimeCatchesBundleRoutes(t *testing.T) {
	out := helmTemplateErr(t, "--set", "claude.enabled=false", "--set", "global.demo.enabled=true")
	if !strings.Contains(out, "kubernetes.pipelines.observe") {
		t.Fatalf("the guard must name a bundle-shipped route:\n%s", out)
	}
}

// ...and it must not fire where nothing resolves to the missing name.
func TestEveryRouteNamingItsOwnRuntimeNeedsNoDefault(t *testing.T) {
	out := helmTemplate(t,
		"--set", "claude.enabled=false",
		"--set-json", `runtimes=[{"name":"ollama","image":"example.com/ollama:1"}]`,
		"--set-json", `pipelines=[{"name":"house-ops","profile":"p","runtimeRef":"ollama"}]`)
	if !strings.Contains(out, "name: ollama") {
		t.Fatalf("the declared runtime must render:\n%s", out)
	}
}

// A replacement answering to `default` satisfies it too.
func TestAReplacementDefaultSatisfiesTheGuard(t *testing.T) {
	out := helmTemplate(t,
		"--set", "claude.enabled=false",
		"--set-json", `runtimes=[{"name":"default","image":"example.com/ollama:1"}]`,
		"--set-json", `pipelines=[{"name":"house-ops","profile":"p"}]`)
	if !strings.Contains(out, `image: "example.com/ollama:1"`) {
		t.Fatalf("the replacement runtime must render:\n%s", out)
	}
}

// EVERY RETIRED KEY FAILS THE RENDER, NAMING ITS REPLACEMENT. One case per key,
// because a guard that covers four of five is the one that lets an install
// through silently.
func TestRetiredValuesKeysFailTheRender(t *testing.T) {
	for _, tc := range []struct{ name, arg, jsonArg string }{
		{name: "runtime block", arg: "runtime.image=x"},
		{name: "rbacMode", arg: "global.agentops.runtime.rbacMode=full"},
		{name: "runtime serviceAccountName", arg: "global.agentops.runtime.serviceAccountName=y"},
		{name: "runtime allowPodExecution", arg: "global.agentops.runtime.allowPodExecution=true"},
		{name: "rbac.runtime.clusterRoles", jsonArg: "rbac.runtime.clusterRoles=[]"},
		{name: "rbac.runtime.bindClusterRoles", jsonArg: "rbac.runtime.bindClusterRoles=[]"},
		{name: "rbac.runtime.namespaced", jsonArg: "rbac.runtime.namespaced=[]"},
		{name: "k8s-bundle key", arg: "k8s-bundle.enabled=true"},
		{name: "ha-bundle key", arg: "ha-bundle.enabled=true"},
		{name: "prometheus-bundle key", arg: "prometheus-bundle.enabled=true"},
		{name: "telegram-bundle key", arg: "telegram-bundle.enabled=true"},
		{name: "vm-bundle key", arg: "vm-bundle.enabled=true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flag, val := "--set", tc.arg
			if tc.jsonArg != "" {
				flag, val = "--set-json", tc.jsonArg
			}
			out := helmTemplateErr(t, flag, val)
			if !strings.Contains(out, "RETIRED") {
				t.Fatalf("%s must fail the render naming its replacement:\n%s", tc.name, out)
			}
		})
	}
}

// The Kubernetes bundle's four consequences are a STATED setting of its own now,
// never a release-wide permission value — and none of them derives from another.
func TestKubernetesMutationsAreStatedNotDerived(t *testing.T) {
	base := []string{"--set", "global.agentops.runtimeDefaults.credentialsSecret.token=x", "--set", "global.demo.enabled=true"}

	off := helmTemplate(t, base...)
	if !strings.Contains(off, "--read-only") {
		t.Fatal("the MCP server must be read-only by default")
	}
	if strings.Contains(off, "name: k8s-admin") {
		t.Fatal("the mutating toolset must not render by default")
	}

	on := helmTemplate(t, append(append([]string{}, base...), "--set", "kubernetes.allowMutations=true")...)
	if strings.Contains(on, "--read-only") {
		t.Fatal("allowMutations must drop --read-only")
	}
	if !strings.Contains(on, "name: k8s-admin") {
		t.Fatal("allowMutations must render the mutating toolset")
	}

	// Setting the toolset must NOT move the server's flag...
	toolsetOnly := helmTemplate(t, append(append([]string{}, base...), "--set", "kubernetes.mcp.toolsets.admin.enabled=true")...)
	if !strings.Contains(toolsetOnly, "--read-only") {
		t.Fatal("enabling the mutating toolset must leave the server's read-only flag alone")
	}
	// ...and setting the server's flag must NOT render the toolset.
	serverOnly := helmTemplate(t, append(append([]string{}, base...), "--set", "kubernetes.mcpServers.readOnly=false")...)
	if strings.Contains(serverOnly, "name: k8s-admin") {
		t.Fatal("a writable server must not render the mutating toolset on its own")
	}
}

// No bundle renders the substrate: with every bundle on there is still exactly
// one floor account and one runtime, both from the parent's own model.
func TestNoBundleRendersTheFloorOrTheDefaults(t *testing.T) {
	out := helmTemplate(t,
		"--set", "global.agentops.runtimeDefaults.credentialsSecret.token=x",
		"--set", "kubernetes.enabled=true",
		"--set", "telegram.enabled=true",
		"--set", "prometheus.enabled=true",
		"--set", "prometheus.mcpServers.backend=http://prom.example.com:9090",
		"--set", "home-assistant.enabled=true",
		"--set", "home-assistant.homeAssistant.endpoint=http://ha.example.com:8123",
		"--set", "home-assistant.homeAssistant.credentials.controlToken=c",
		"--set", "home-assistant.homeAssistant.credentials.operatorToken=o")
	if got := strings.Count(out, "name: agentops-runtime\n  namespace:"); got != 1 {
		t.Fatalf("exactly one floor ServiceAccount must render, got %d:\n%s", got, out)
	}
	if got := strings.Count(out, "kind: AgentRuntime\n"); got != 1 {
		t.Fatalf("exactly one AgentRuntime must render with every bundle on, got %d", got)
	}
}
