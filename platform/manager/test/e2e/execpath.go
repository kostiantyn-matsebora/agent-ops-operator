//go:build e2e

package e2e

import (
	"fmt"
	"os/exec"
	"sync"
)

var lookPathCache sync.Map // name -> resolved absolute path

// lookPath resolves name to a fixed absolute path via PATH, once per name,
// rather than handing exec.Command a bare name to search PATH itself on
// every call — SonarCloud's go:S4036 flags the bare-name pattern as a
// PATH-hijack vector. The e2e pack already requires docker, k3d and their
// peers to be on PATH (build-test.md), so a failed lookup here is an
// environment prerequisite missing, not a recoverable condition.
func lookPath(name string) string {
	if v, ok := lookPathCache.Load(name); ok {
		return v.(string)
	}
	p, err := exec.LookPath(name)
	if err != nil {
		panic(fmt.Sprintf("%s not found on PATH: %v", name, err))
	}
	lookPathCache.Store(name, p)
	return p
}
