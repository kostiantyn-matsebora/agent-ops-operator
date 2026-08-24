// Chart-render assertions for execution identity: the Pipeline fields, the
// several runtime accounts, and — the half a review cannot be trusted to catch
// twice — that NO role this chart renders for an agent can reach a Secret.
package integration

import (
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// clusterRoleRules is one rendered Role or ClusterRole, parsed far enough to
// walk its rules. Only the fields the guards below read.
type clusterRoleRules struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Rules []struct {
		APIGroups []string `json:"apiGroups"`
		Resources []string `json:"resources"`
		Verbs     []string `json:"verbs"`
	} `json:"rules"`
}

// declaredActing renders the one thing that grants an agent anything now: an
// account the install DECLARES, with a posture, for a route to name.
//
// THERE IS NO MODE. `global.agentops.runtime.rbacMode` rendered a named account
// from a release-wide value, and every test below used to sweep its four
// settings. The vocabulary survives PER ACCOUNT, which is what these sweeps
// exercise instead — a preset posture nobody declared is exactly what was
// removed.
var declaredActing = []string{
	"--set", "rbac.runtime.serviceAccounts[0].name=agentops-runtime-acting",
	"--set", "rbac.runtime.serviceAccounts[0].rbacMode=full",
}

// rolesIn parses every Role and ClusterRole out of a rendered manifest.
func rolesIn(t *testing.T, out string) []clusterRoleRules {
	t.Helper()
	var roles []clusterRoleRules
	for _, doc := range strings.Split(out, "\n---") {
		if !strings.Contains(doc, "kind: ClusterRole\n") && !strings.Contains(doc, "kind: Role\n") {
			continue
		}
		var r clusterRoleRules
		if err := yaml.Unmarshal([]byte(doc), &r); err != nil {
			continue // CRD schemas and other documents are not roles
		}
		if r.Kind != "ClusterRole" && r.Kind != "Role" {
			continue
		}
		roles = append(roles, r)
	}
	return roles
}

// 4.5 — unset changes nothing. The no-op half of the migration: an install that
// adopts neither field must render the Pipeline it always rendered.
func TestPipelineExecutionFieldsAreAbsentWhenUnset(t *testing.T) {
	out := helmTemplate(t,
		"--set", "pipelines[0].name=unset-route",
		"--set", "pipelines[0].profile=some-profile")

	doc := pipelineDoc(t, out, "unset-route")
	for _, field := range []string{"runtimeRef:", "serviceAccountName:"} {
		if strings.Contains(doc, field) {
			t.Fatalf("a Pipeline naming neither field must render neither key, found %q in:\n%s", field, doc)
		}
	}
}

// ...and set reaches the rendered Pipeline, which is the other half of the same
// assertion: a values key nothing reads is worse than an absent one, because it
// looks configured.
func TestPipelineExecutionFieldsReachTheRenderedPipeline(t *testing.T) {
	out := helmTemplate(t,
		"--set", "pipelines[0].name=wired-route",
		"--set", "pipelines[0].profile=some-profile",
		"--set", "pipelines[0].runtimeRef=rt-vendor",
		"--set", "pipelines[0].serviceAccountName=agentops-runtime-actor")

	doc := pipelineDoc(t, out, "wired-route")
	if !strings.Contains(doc, "name: rt-vendor") {
		t.Fatalf("runtimeRef did not reach the Pipeline:\n%s", doc)
	}
	if !strings.Contains(doc, "serviceAccountName: agentops-runtime-actor") {
		t.Fatalf("serviceAccountName did not reach the Pipeline:\n%s", doc)
	}
}

// pipelineDoc returns the rendered Pipeline document with the given name.
func pipelineDoc(t *testing.T, out, name string) string {
	t.Helper()
	for _, doc := range strings.Split(out, "\n---") {
		if strings.Contains(doc, "kind: Pipeline\n") && strings.Contains(doc, "name: "+name+"\n") {
			return doc
		}
	}
	t.Fatalf("no rendered Pipeline named %q", name)
	return ""
}

// 4.6 — THE CASE THE CHANGE EXISTS FOR. A second trust level renders a second
// ServiceAccount with its own RBAC and NO second AgentRuntime. Before this, the
// account had nowhere to live but the runtime, so the only way to express two
// postures was to clone a CR that differed in one field.
func TestASecondAccountNeedsNoSecondRuntime(t *testing.T) {
	out := helmTemplate(t,
		"--set", "rbac.runtime.serviceAccounts[0].name=agentops-runtime-actor",
		"--set", "rbac.runtime.serviceAccounts[0].rbacMode=full",
		"--set", "pipelines[0].name=acting-route",
		"--set", "pipelines[0].profile=some-profile",
		"--set", "pipelines[0].serviceAccountName=agentops-runtime-actor")

	if !strings.Contains(out, "name: agentops-runtime-actor") {
		t.Fatal("the second ServiceAccount did not render")
	}
	// The acting ClusterRole for a named account carries that account's own name.
	actingRole := false
	for _, role := range rolesIn(t, out) {
		// Cluster-scoped names carry the release namespace, so two installs in
		// one cluster do not collide over them.
		if strings.HasPrefix(role.Metadata.Name, "agentops-runtime-actor") && len(role.Rules) > 0 {
			actingRole = true
		}
	}
	if !actingRole {
		t.Fatal("the second account rendered without its own acting role — an identity with no " +
			"bindings is not a trust level, it is a name")
	}
	// The FLOOR account is still there. Adding an account must not replace the
	// one every unnamed Pipeline runs as.
	if !strings.Contains(out, "name: agentops-runtime\n") {
		t.Fatal("the FLOOR runtime account must still render — it is what a Pipeline naming none gets")
	}
	// Counted per DOCUMENT, not per line: `kind: AgentRuntime` also appears
	// inside the CRD that DEFINES the kind.
	runtimes := 0
	for _, doc := range strings.Split(out, "\n---") {
		if strings.Contains(doc, "\nkind: AgentRuntime\n") {
			runtimes++
		}
	}
	if runtimes != 1 {
		t.Fatalf("%d AgentRuntime objects rendered, want exactly 1 — expressing a second trust level "+
			"must not need a cloned runtime, which is the whole point of the field", runtimes)
	}
}

// AN ENTRY FOR THE FLOOR ACCOUNT IS REFUSED. Binding to it would make silence
// mean power again, which is the one thing this whole model exists to prevent —
// and it would also take away the floor's other job, being NAMEABLE on a route
// that must hold nothing.
func TestAnEntryForTheFloorAccountIsRefused(t *testing.T) {
	msg := helmTemplateErr(t, "--set", "rbac.runtime.serviceAccounts[0].name=agentops-runtime")
	if !strings.Contains(msg, "FLOOR account") {
		t.Fatalf("the failure must say it is the floor being bound: %s", msg)
	}
}

// 4.7 — still true, and worth re-pinning here because this change is the first
// that makes a second runtime identity expressible at all. A bundle naming an
// account in its Pipeline values REFERENCES one; it must not invent one.
func TestNoBundleRendersARuntimeServiceAccount(t *testing.T) {
	out := helmTemplate(t,
		"--set", "kubernetes.enabled=true",
		"--set", "kubernetes.pipelines.enabled=true",
		"--set", "kubernetes.pipelines.admin.serviceAccountName=agentops-runtime-actor",
		"--set", "prometheus.enabled=true")

	for _, doc := range strings.Split(out, "\n---") {
		if !strings.Contains(doc, "kind: ServiceAccount\n") {
			continue
		}
		if !strings.Contains(doc, "name: agentops-runtime-actor\n") {
			continue
		}
		t.Fatalf("a bundle's Pipeline named an account and something rendered it — naming is a "+
			"REFERENCE, and the parent owns every runtime identity:\n%s", doc)
	}
}

// `full` is no longer a cluster-admin binding. This is the assertion the whole
// RBAC half of the change reduces to.
func TestAFullAccountBindsNoClusterAdmin(t *testing.T) {
	for _, extra := range [][]string{
		nil,
		{"--set", "kubernetes.enabled=true", "--set", "kubernetes.allowMutations=true"},
	} {
		args := append(append([]string{}, declaredActing...), extra...)
		out := helmTemplate(t, args...)
		for _, doc := range strings.Split(out, "\n---") {
			if !strings.Contains(doc, "\nkind: ClusterRoleBinding\n") {
				continue
			}
			if strings.Contains(doc, "\n  name: cluster-admin\n") {
				t.Fatalf("a full account bound cluster-admin — an agent has a shell, so that is every "+
					"Secret in the cluster readable by a model:\n%s", doc)
			}
		}
	}
}

// 5.6 — THE TEST THAT HAS TO SURVIVE SOMEONE ADDING A MODE. It walks every role
// this chart renders for an agent, in EVERY mode, and fails on a `secrets`
// resource or a wildcard that would reach one without naming it.
//
// Wildcards are the subtle half: `resources: ["*"]` passes a review that greps
// for "secrets" and grants every one of them.
func TestNoRuntimeRoleCanReachASecret(t *testing.T) {
	// Every mode, and the bundles that render agent-reachable roles of their
	// own — the k8s MCP server's account is a wall on the same path, since an
	// agent reaches the cluster THROUGH it.
	for _, mode := range []string{"none", "readonly", "full"} {
		args := []string{
			"--set", "kubernetes.enabled=true",
			"--set", "kubernetes.pipelines.enabled=true",
			"--set", "kubernetes.allowMutations=true",
			"--set", "prometheus.enabled=true",
			"--set", "rbac.runtime.serviceAccounts[0].name=agentops-runtime-actor",
			"--set", "rbac.runtime.serviceAccounts[0].rbacMode=" + mode,
		}
		out := helmTemplate(t, args...)

		for _, role := range rolesIn(t, out) {
			// The MANAGER's own roles are held to their own rule elsewhere; this
			// guard is about what an AGENT can reach.
			if !isAgentRole(role.Metadata.Name) {
				continue
			}
			for _, rule := range role.Rules {
				for _, res := range rule.Resources {
					if res == "secrets" || strings.HasPrefix(res, "secrets/") {
						t.Fatalf("mode=%q role %q grants %v on %q — no runtime role may carry ANY verb "+
							"on secrets, in any mode", mode, role.Metadata.Name, rule.Verbs, res)
					}
					if res == "*" {
						t.Fatalf("mode=%q role %q has resources: [*] — a wildcard reaches Secrets without "+
							"naming them, which is how a role passes review and fails its purpose",
							mode, role.Metadata.Name)
					}
				}
				for _, g := range rule.APIGroups {
					if g == "*" {
						t.Fatalf("mode=%q role %q has apiGroups: [*]", mode, role.Metadata.Name)
					}
				}
				// 5.4 — a role that can escalate or bind rewrites the rule above.
				for _, g := range rule.APIGroups {
					if g != "rbac.authorization.k8s.io" {
						continue
					}
					for _, v := range rule.Verbs {
						if v == "escalate" || v == "bind" || v == "*" {
							t.Fatalf("mode=%q role %q grants %q on rbac — the role can then widen itself "+
								"and every guard above becomes advisory", mode, role.Metadata.Name, v)
						}
					}
				}
			}
		}
	}
}

// isAgentRole names the roles an AGENT's credentials reach — every account this
// release renders EXCEPT the ones that are not an agent's.
//
// AN ALLOW-LIST OF PREFIXES WENT STALE IMMEDIATELY. It named `agentops-runtime`
// and `agentops-mcp-`, and then bundles started rendering per-route accounts
// (`agentops-k8s-observe`, `agentops-ha-ops`) which matched neither — so a
// bundle could have granted its route anything and this guard would have said
// nothing. It is a DENY-list now: everything is an agent role unless it belongs
// to a component that is not an agent.
func isAgentRole(name string) bool {
	if !strings.HasPrefix(name, "agentops-") {
		return false
	}
	for _, notAnAgent := range []string{
		"agentops-manager",      // the operator itself, held to its own rule
		"agentops-housekeeping", // the reclaiming CronJob, read-only on the API
		"agentops-adapter-",     // channel adapters, granted by the chart per adapter
		"agentops-signal-",      // signal adapters, same
	} {
		if strings.HasPrefix(name, notAnAgent) {
			return false
		}
	}
	return true
}

// The acting role is only worth anything if it actually GRANTS something. A
// guard that only forbids passes trivially on an empty role, and an agent that
// can do nothing under `full` is a regression nobody would attribute to this.
// actingRoles returns every role the acting grant renders, across the
// ClusterRole and the namespaced Roles it is split into.
func actingRoles(t *testing.T, extra ...string) []clusterRoleRules {
	t.Helper()
	args := append(append([]string{"-n", "agent-ops"}, declaredActing...), extra...)
	out := helmTemplate(t, args...)

	var acting []clusterRoleRules
	for _, role := range rolesIn(t, out) {
		if strings.HasPrefix(role.Metadata.Name, "agentops-runtime-acting") {
			acting = append(acting, role)
		}
	}
	if len(acting) == 0 {
		t.Fatal("a declared account with rbacMode=full rendered no acting role at all")
	}
	return acting
}

func anyGrants(roles []clusterRoleRules, resource, verb string) bool {
	for i := range roles {
		if grants(&roles[i], resource, verb) {
			return true
		}
	}
	return false
}

// What `full` grants WITHOUT pod execution. This is the default, and it has to
// remain a real operator or the flag is not a choice, it is an off switch.
func TestTheActingRoleGrantsTheVerbsAnAgentFixesWith(t *testing.T) {
	acting := actingRoles(t)

	want := map[string][]string{
		"pods":              {"delete"},
		"pods/eviction":     {"create"},
		"pods/log":          {"get"},
		"nodes/proxy":       {"get"}, // nodes_log, nodes_stats_summary
		"deployments":       {"delete"},
		"deployments/scale": {"update"}, // resources_scale
		"configmaps":        {"create", "update", "delete"},
		"services":          {"create", "update", "delete"},
		"ingresses":         {"create", "update", "delete"},
		"nodes":             {"patch"}, // cordon, cluster-scoped by necessity
	}
	for res, verbs := range want {
		for _, verb := range verbs {
			if !anyGrants(acting, res, verb) {
				t.Errorf("no acting role can %s %s — `full` must still be a real operator with "+
					"pod execution off, or the flag is an off switch rather than a choice", verb, res)
			}
		}
	}
}

// THE FLAG IS THE SECRETS BOUNDARY, so what it gates has to actually be gated.
//
// The kubelet resolves a Secret when it builds a pod, so anything that produces
// or enters a pod reads Secrets without ever asking for one — proven on a live
// cluster against this role. Gating `pods: create` alone would close nothing:
// creating a Job or Deployment writes a pod spec, and patching one edits it.
func TestPodExecutionIsGatedAndOffByDefault(t *testing.T) {
	gated := map[string][]string{
		"pods":            {"create", "patch"},
		"pods/exec":       {"create"},
		"deployments":     {"create", "update", "patch"},
		"statefulsets":    {"create", "update", "patch"},
		"daemonsets":      {"create", "update", "patch"},
		"jobs":            {"create", "update", "patch"},
		"cronjobs":        {"create", "update", "patch"},
		"serviceaccounts": {"create"},
	}

	off := actingRoles(t)
	for res, verbs := range gated {
		for _, verb := range verbs {
			if anyGrants(off, res, verb) {
				t.Errorf("allowPodExecution defaults FALSE but the role can %s %s. Anything that "+
					"produces or enters a pod reads every Secret in the namespace, so the "+
					"no-Secrets rule is only true while these are absent", verb, res)
			}
		}
	}

	on := actingRoles(t, "--set", "global.agentops.runtimeDefaults.allowPodExecution=true")
	for res, verbs := range gated {
		for _, verb := range verbs {
			if !anyGrants(on, res, verb) {
				t.Errorf("allowPodExecution=true must grant %s %s — otherwise the k8s-admin "+
					"toolset advertises tools that 403", verb, res)
			}
		}
	}
}

func grants(role *clusterRoleRules, resource, verb string) bool {
	for _, rule := range role.Rules {
		hasRes, hasVerb := false, false
		for _, r := range rule.Resources {
			if r == resource {
				hasRes = true
			}
		}
		for _, v := range rule.Verbs {
			if v == verb {
				hasVerb = true
			}
		}
		if hasRes && hasVerb {
			return true
		}
	}
	return false
}

// THE GUARD THE WHOLE MODEL RESTS ON. Nothing may bind to the FLOOR account —
// the one a Pipeline naming no `serviceAccountName` runs as — in ANY mode.
//
// This change first shipped with the mode bound to that account, so a route
// inherited the release's maximum by not typing a field: three of four routes
// in the reference install held pod-delete and node-patch, two of them routes
// that reach no Kubernetes API at all. Shrinking `full` from `cluster-admin` to
// an enumerated role fixed the blast radius and left the model inverted.
//
// SILENCE MUST MEAN NO POWER. If this test fails, that inversion is back.
func TestNothingEverBindsToTheFloorAccount(t *testing.T) {
	const floor = "agentops-runtime"

	for _, mode := range []string{"none", "readonly", "full"} {
		args := []string{
			"--set", "kubernetes.enabled=true",
			"--set", "kubernetes.pipelines.enabled=true",
			"--set", "global.demo.enabled=true",
			"--set", "rbac.runtime.serviceAccounts[0].name=agentops-runtime-acting",
			"--set", "rbac.runtime.serviceAccounts[0].rbacMode=" + mode,
		}
		out := helmTemplate(t, args...)

		for _, doc := range strings.Split(out, "\n---") {
			if !strings.Contains(doc, "\nkind: ClusterRoleBinding\n") &&
				!strings.Contains(doc, "\nkind: RoleBinding\n") {
				continue
			}
			subjects := doc
			if i := strings.Index(doc, "subjects:"); i >= 0 {
				subjects = doc[i:]
			}
			for _, line := range strings.Split(subjects, "\n") {
				if strings.TrimSpace(line) == "name: "+floor {
					t.Fatalf("mode=%q: a binding names the FLOOR account %q as a subject. "+
						"A Pipeline that declares no serviceAccountName must hold NOTHING — "+
						"grants belong on a named account a route opts into:\n%s", mode, floor, doc)
				}
			}
		}
	}
}

// ...and a DECLARED account still has to MEAN something, or the guard above
// passes by granting nobody anything.
func TestADeclaredAccountRendersItsOwnNamedRole(t *testing.T) {
	full := helmTemplate(t, declaredActing...)
	if !strings.Contains(full, "name: agentops-runtime-acting") {
		t.Fatal("a declared account must render for a route to opt into")
	}
	ro := helmTemplate(t,
		"--set", "rbac.runtime.serviceAccounts[0].name=agentops-runtime-readonly",
		"--set", "rbac.runtime.serviceAccounts[0].rbacMode=readonly")
	if !strings.Contains(ro, "name: agentops-runtime-readonly") {
		t.Fatal("a declared readonly account must render too")
	}
}

// A bundle that ships a Pipeline ships that route's identity and names it.
// The bundle is the only scope that knows what its own routes do.
func TestABundleRendersItsOwnRoutesIdentity(t *testing.T) {
	out := helmTemplate(t,
		"--set", "kubernetes.enabled=true",
		"--set", "kubernetes.pipelines.enabled=true")

	if !strings.Contains(out, "name: agentops-k8s-observe") {
		t.Fatal("kubernetes shipped a route without rendering its ServiceAccount")
	}
	doc := pipelineDoc(t, out, "k8s-observe")
	if !strings.Contains(doc, "serviceAccountName: agentops-k8s-observe") {
		t.Fatalf("the bundle rendered an account and did not name it on its own route:\n%s", doc)
	}

	// Route off, account gone: an identity with no route is a subject nobody
	// can explain.
	off := helmTemplate(t, "--set", "kubernetes.enabled=true",
		"--set", "kubernetes.pipelines.enabled=false")
	if strings.Contains(off, "name: agentops-k8s-observe") {
		t.Fatal("a bundle rendered a route account with no route to use it")
	}
}

// telegram ships no Pipeline, so it ships no identity. This is the row
// that keeps "a bundle renders accounts" from becoming "every bundle renders
// accounts".
func TestABundleWithNoRouteRendersNoIdentity(t *testing.T) {
	out := helmTemplate(t, "--set", "telegram.enabled=true")
	for _, doc := range strings.Split(out, "\n---") {
		if strings.Contains(doc, "\nkind: ServiceAccount\n") &&
			strings.Contains(doc, "agentops-telegram") {
			t.Fatalf("telegram ships no Pipeline and must ship no account:\n%s", doc)
		}
	}
}

// EVERY BINDING MUST POINT AT A ROLE THIS CHART WRITES OUT.
//
// `full` was a `cluster-admin` binding, and `readonly` bound the built-in
// `view` — which is cluster-wide, so a "read-only" MCP server could read the
// release's own namespace and the Conversations it was serving. Both are gone.
//
// A built-in role is one nobody in this repo can review: its contents come from
// the Kubernetes distribution, change between versions, and aggregate roles
// other controllers contribute to.
func TestNoBindingUsesABuiltInClusterRole(t *testing.T) {
	builtIn := map[string]bool{
		"cluster-admin": true, "admin": true, "edit": true, "view": true,
	}
	for _, mode := range []string{"none", "readonly", "full"} {
		args := []string{"-n", "agent-ops",
			"--set", "kubernetes.enabled=true",
			"--set", "kubernetes.pipelines.enabled=true",
			"--set", "kubernetes.allowMutations=true",
			"--set", "prometheus.enabled=true",
			"--set", "rbac.runtime.serviceAccounts[0].name=agentops-runtime-acting",
			"--set", "rbac.runtime.serviceAccounts[0].rbacMode=" + mode}
		out := helmTemplate(t, args...)

		for _, doc := range strings.Split(out, "\n---") {
			if !strings.Contains(doc, "\nkind: ClusterRoleBinding\n") &&
				!strings.Contains(doc, "\nkind: RoleBinding\n") {
				continue
			}
			i := strings.Index(doc, "roleRef:")
			if i < 0 {
				continue
			}
			for _, line := range strings.Split(doc[i:], "\n") {
				t2 := strings.TrimSpace(line)
				if !strings.HasPrefix(t2, "name: ") {
					continue
				}
				if builtIn[strings.TrimPrefix(t2, "name: ")] {
					t.Errorf("mode=%q: a binding uses the built-in role %q. Every grant must be a "+
						"role this chart writes out and an operator can read:\n%s",
						mode, strings.TrimPrefix(t2, "name: "), doc)
				}
				break
			}
		}
	}
}

// THE PROPERTY THAT MAKES A CLUSTER-WIDE GRANT DEFENSIBLE.
//
// The grants reach every namespace including the operator's own. What keeps
// that from exposing anything is not scope but OMISSION: `agentops.dev` is
// never named in any rule, and RBAC is deny-by-default, so Conversations,
// Pipelines and profiles are unreadable wherever an agent looks.
//
// Namespaced Roles were tried instead and reverted — RBAC cannot express
// "everywhere except", so bounding an agent cost 224 objects on a 28-namespace
// cluster and made every new namespace invisible until someone edited values.
// This test is what that reversal rests on.
func TestAgentRolesNeverGrantAgentopsCRs(t *testing.T) {
	for _, mode := range []string{"none", "readonly", "full"} {
		args := []string{"-n", "agent-ops",
			"--set", "kubernetes.enabled=true",
			"--set", "kubernetes.pipelines.enabled=true",
			"--set", "kubernetes.allowMutations=true",
			"--set", "prometheus.enabled=true",
			"--set", "rbac.runtime.serviceAccounts[0].name=agentops-runtime-acting",
			"--set", "rbac.runtime.serviceAccounts[0].rbacMode=" + mode}
		out := helmTemplate(t, args...)

		for _, role := range rolesIn(t, out) {
			if !isAgentRole(role.Metadata.Name) {
				continue
			}
			for _, rule := range role.Rules {
				for _, g := range rule.APIGroups {
					if g == "agentops.dev" {
						t.Errorf("mode=%q: agent role %q grants %v on the agentops.dev group. "+
							"The grant is cluster-wide, so this would expose every Conversation "+
							"in the install", mode, role.Metadata.Name, rule.Verbs)
					}
				}
				for _, res := range rule.Resources {
					if res == "clusterroles" || res == "clusterrolebindings" {
						t.Errorf("mode=%q: agent role %q reads %q — that listing names every "+
							"identity in the install and cannot be narrowed",
							mode, role.Metadata.Name, res)
					}
				}
			}
		}
	}
}

// The whole grant is a handful of objects, and that is the point of the
// reversal. If this climbs back into the hundreds, per-namespace bindings are
// back and every new namespace is invisible to the agent again.
func TestTheGrantIsAHandfulOfObjects(t *testing.T) {
	out := helmTemplate(t, append([]string{"-n", "agent-ops",
		"--set", "kubernetes.enabled=true",
		"--set", "kubernetes.allowMutations=true"}, declaredActing...)...)

	agentRoles := 0
	for _, role := range rolesIn(t, out) {
		if isAgentRole(role.Metadata.Name) {
			agentRoles++
		}
	}
	if agentRoles > 6 {
		t.Fatalf("%d agent roles rendered. The namespaced-Role model cost 224 objects on a "+
			"28-namespace cluster — if this is climbing, that model is back", agentRoles)
	}
}
