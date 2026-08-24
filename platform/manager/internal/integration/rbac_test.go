package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// A wrong RBAC plural fails silently: the informer is forbidden, the reconciler
// simply never runs, and the only symptom is a log line. A blanket rename once
// produced `AgentRuntimes` this way. This pins every CRD's plural — as
// controller-gen spells it — against the manager Role, so a new kind cannot
// ship without its rule either.
func TestManagerRBACCoversEveryCRDPlural(t *testing.T) {
	crdDir := chartDir("crds")
	entries, err := os.ReadDir(crdDir)
	if err != nil {
		t.Fatal(err)
	}
	rbac, err := os.ReadFile(chartDir("templates", "rbac.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	granted := grantedResources(string(rbac))

	seen := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(crdDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var crd struct {
			Spec struct {
				Names struct {
					Plural string `json:"plural"`
				} `json:"names"`
			} `json:"spec"`
		}
		if err := yaml.Unmarshal(raw, &crd); err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		plural := crd.Spec.Names.Plural
		if plural == "" {
			t.Fatalf("%s: no spec.names.plural", e.Name())
		}
		if plural != strings.ToLower(plural) {
			t.Fatalf("%s: plural %q is not lowercase", e.Name(), plural)
		}
		if !granted[plural] {
			t.Fatalf("%s: manager Role grants nothing on %q — the informer would be forbidden "+
				"and its reconciler would silently do nothing", e.Name(), plural)
		}
		seen++
	}
	if seen == 0 {
		t.Fatal("no CRDs found — the chart CRD directory moved?")
	}
}

// grantedResources collects the resource names of every `resources:` line in
// the Role, ignoring /status subresources.
func grantedResources(rbac string) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(rbac, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "resources:") {
			continue
		}
		for _, tok := range strings.Split(strings.TrimPrefix(trimmed, "resources:"), ",") {
			name := strings.Trim(strings.TrimSpace(tok), `[]"`)
			if name != "" && !strings.Contains(name, "/") {
				out[name] = true
			}
		}
	}
	return out
}

// THE OPERATOR MAY BRING STORAGE INTO BEING AND MUST NEVER TAKE IT AWAY.
//
// A Pipeline binding that names a PersistentVolume makes the manager render the
// claim on it, because a pod can mount only a claim — the one place in this
// system where naming a resource creates it. The claim then OUTLIVES the
// Pipeline that asked for it, holding the accumulated context of every
// conversation that route started, which is the one thing here whose loss
// cannot be repaired by reconciling again.
//
// That is guarded twice: the created claim carries no ownerRef, and the Role
// holds no verb that could remove or shrink it. This pins the second half,
// because the first is invisible from a chart render.
func TestManagerMayCreateButNeverDestroyAClaim(t *testing.T) {
	out := helmTemplate(t)

	rule := managerRuleFor(t, out, "persistentvolumeclaims")
	for _, want := range []string{"get", "list", "watch", "create"} {
		if !containsVerb(rule, want) {
			t.Fatalf("the manager needs %q on persistentvolumeclaims to render a bound claim, got %v", want, rule)
		}
	}
	for _, forbidden := range []string{"delete", "deletecollection", "update", "patch"} {
		if containsVerb(rule, forbidden) {
			t.Fatalf("the manager holds %q on persistentvolumeclaims. It may create storage and must NEVER "+
				"remove or rewrite it: a claim it rendered outlives the Pipeline that asked for it and holds "+
				"the accumulated context of every conversation that route started. Verbs: %v", forbidden, rule)
		}
	}
}

// managerRuleFor finds the manager Role rule granting `resource` in the core
// group and returns its verbs.
func managerRuleFor(t *testing.T, rendered, resource string) []string {
	t.Helper()
	for _, doc := range strings.Split(rendered, "\n---\n") {
		if !strings.Contains(doc, "kind: Role\n") || !strings.Contains(doc, "name: agentops-manager") {
			continue
		}
		var role struct {
			Rules []struct {
				APIGroups []string `json:"apiGroups"`
				Resources []string `json:"resources"`
				Verbs     []string `json:"verbs"`
			} `json:"rules"`
		}
		if err := yaml.Unmarshal([]byte(doc), &role); err != nil {
			continue
		}
		for _, r := range role.Rules {
			for _, res := range r.Resources {
				if res == resource {
					return r.Verbs
				}
			}
		}
	}
	t.Fatalf("no manager Role rule grants %q at all — the Pipeline reconciler cannot render a bound claim", resource)
	return nil
}

func containsVerb(verbs []string, want string) bool {
	for _, v := range verbs {
		if v == want || v == "*" {
			return true
		}
	}
	return false
}
