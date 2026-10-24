//go:build conformance

package conformance

import (
	"os"
	"testing"
)

// TestMain removes buildRoot's MkdirTemp directory once every test in this
// package has run. buildDir is process-private and shared across every test
// via buildOnce, so nothing SAFELY reclaims it earlier -- an individual
// test's t.Cleanup would delete binaries tests running after it still need.
func TestMain(m *testing.M) {
	code := m.Run()
	if buildDir != "" {
		_ = os.RemoveAll(buildDir)
	}
	os.Exit(code)
}
