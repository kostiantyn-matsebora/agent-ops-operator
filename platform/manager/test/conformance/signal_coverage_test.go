//go:build conformance

package conformance

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// signalImplementations is every signal adapter this suite runs, keyed by
// its module directory. The claim "every shipped signal adapter is under
// test" is checked against the repository below, never assumed.
var signalImplementations = map[string]func(*testing.T){
	"signals/alertmanager": TestSignalAlertmanagerConformance,
	"signals/cron":         TestSignalCronConformance,
	"signals/ha":           TestSignalHAConformance,
	"signals/k8s-events":   TestSignalK8sEventsConformance,
	"signals/pulse":        TestSignalPulseConformance,
	"signals/telegram":     TestSignalTelegramConformance,
}

// shippedSignalModules lists every directory under signals/ that ships an
// image, by .github/components.sh's own rule: it holds a go.mod or a
// Dockerfile, at any depth, node_modules excluded. A non-Go adapter ships
// from its Dockerfile alone, so a go.mod-only listing would miss it.
func shippedSignalModules(t *testing.T) []string {
	t.Helper()
	repo := repoRoot()
	root := filepath.Join(repo, "signals")
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		t.Fatalf("signals/ is not a directory at the repository root: %v", err)
	}
	seen := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "node_modules" {
			return filepath.SkipDir
		}
		if d.IsDir() || (d.Name() != "go.mod" && d.Name() != "Dockerfile") {
			return nil
		}
		rel, err := filepath.Rel(repo, filepath.Dir(path))
		if err != nil {
			return err
		}
		seen[filepath.ToSlash(rel)] = true
		return nil
	})
	if err != nil {
		t.Fatalf("walking signals/: %v", err)
	}
	mods := make([]string, 0, len(seen))
	for m := range seen {
		mods = append(mods, m)
	}
	sort.Strings(mods)
	return mods
}

func TestEverySignalAdapterModuleIsUnderTest(t *testing.T) {
	mods := shippedSignalModules(t)
	if len(mods) == 0 {
		t.Fatalf("no module found under signals/ — the listing, not the claim, is broken")
	}
	for _, m := range mods {
		if _, ok := signalImplementations[m]; !ok {
			t.Errorf("%s is a shipped signal adapter the conformance suite does not run — add its section and list it in signalImplementations", m)
		}
	}
}

func TestSignalImplementationListNamesNoMissingModule(t *testing.T) {
	present := map[string]bool{}
	for _, m := range shippedSignalModules(t) {
		present[m] = true
	}
	for m := range signalImplementations {
		if !present[m] {
			t.Errorf("signalImplementations lists %s, which is not a module under signals/", m)
		}
	}
}

// Mapping a new module to an existing adapter's test would satisfy the
// listing check while running nothing against the new binary.
func TestEachSignalModuleHasItsOwnConformanceTest(t *testing.T) {
	owner := map[uintptr]string{}
	for m, fn := range signalImplementations {
		if fn == nil {
			t.Errorf("signalImplementations lists %s with no test", m)
			continue
		}
		ptr := reflect.ValueOf(fn).Pointer()
		if prev, dup := owner[ptr]; dup {
			t.Errorf("%s and %s share one conformance test — each adapter binary needs its own", prev, m)
		}
		owner[ptr] = m
	}
}
