package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot is four levels up from this package — the same walk chartDir makes.
func repoRoot() string { return filepath.Join(chartDir(), "..") }

// The stub runtime and the fake Bot API carry a Dockerfile and a go.mod each,
// and .github/components.sh takes the UNION of both — so without the explicit
// exclusion the next release tag publishes agentops-stubruntime and every CI
// matrix grows by two. The exclusion is asserted here, not trusted.
func TestComponentsDiscoveryExcludesTheTestTree(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not installed (components.sh needs it) — CI has it")
	}
	root := repoRoot()
	for _, dir := range []string{"test/stubruntime", "test/fakebotapi"} {
		for _, f := range []string{"Dockerfile", "go.mod"} {
			if _, err := os.Stat(filepath.Join(root, dir, f)); err != nil {
				t.Fatalf("%s/%s must exist for this test to mean anything: %v", dir, f, err)
			}
		}
	}
	for _, sub := range []string{"images", "modules"} {
		out, err := exec.Command(filepath.Join(root, ".github/components.sh"), sub).Output()
		if err != nil {
			t.Fatalf("components.sh %s: %v", sub, err)
		}
		if strings.Contains(string(out), "test/") || strings.Contains(string(out), "stubruntime") || strings.Contains(string(out), "fakebotapi") {
			t.Fatalf("components.sh %s must not list anything under test/:\n%s", sub, out)
		}
		if sub == "images" {
			var items []map[string]string
			if err := json.Unmarshal(out, &items); err != nil || len(items) == 0 {
				t.Fatalf("images must still list the real components: %v %s", err, out)
			}
		}
	}
}

// The stub does not ship: no chart default, sample CR or documented install
// path names its image. A grep is the whole test, because the failure it
// guards is one line of YAML somebody thought was harmless.
func TestNothingShippedReferencesTheStubRuntime(t *testing.T) {
	root := repoRoot()
	for _, dir := range []string{"chart", "platform/manager/config/samples", "docs/installation.md", "docs/getting-started.md", "README.md"} {
		path := filepath.Join(root, dir)
		_ = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || strings.Contains(p, "/node_modules/") {
				return nil
			}
			if strings.HasSuffix(p, ".md") && !strings.HasSuffix(dir, ".md") {
				return nil // a chart README may describe the pack; the shipped objects may not
			}
			b, _ := os.ReadFile(p)
			if strings.Contains(string(b), "stub-runtime") || strings.Contains(string(b), "stubruntime") {
				t.Errorf("%s references the stub runtime", p)
			}
			return nil
		})
	}
}
