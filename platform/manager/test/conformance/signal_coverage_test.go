//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
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

// shippedSignalModules lists every Go module directly under signals/.
func shippedSignalModules(t *testing.T) []string {
	t.Helper()
	root := filepath.Join(repoRoot(), "signals")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("reading signals/: %v", err)
	}
	var mods []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, e.Name(), "go.mod")); err == nil {
			mods = append(mods, "signals/"+e.Name())
		}
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
