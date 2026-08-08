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
	crdDir := filepath.Join("..", "..", "chart", "files", "crds")
	entries, err := os.ReadDir(crdDir)
	if err != nil {
		t.Fatal(err)
	}
	rbac, err := os.ReadFile(filepath.Join("..", "..", "chart", "templates", "rbac.yaml"))
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
