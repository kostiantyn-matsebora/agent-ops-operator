//go:build system

package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

var lookPathCache sync.Map // name -> resolved absolute path

// lookPath resolves name to a fixed absolute path via PATH, once per name,
// rather than handing exec.Command a bare name to search PATH itself on
// every call — SonarCloud's go:S4036 flags the bare-name pattern as a
// PATH-hijack vector. The system pack already requires docker, k3d and their
// peers to be on PATH (build-test.md), so a failed lookup here is an
// environment prerequisite missing, not a recoverable condition.
// lookPathOrRancher searches PATH, then Rancher Desktop's ~/.rd/bin (containers.md).
func lookPathOrRancher(name string) (string, error) {
	p, err := exec.LookPath(name)
	if err == nil {
		return p, nil
	}
	if home, herr := os.UserHomeDir(); herr == nil {
		if rp, rerr := exec.LookPath(filepath.Join(home, ".rd", "bin", name)); rerr == nil {
			return rp, nil
		}
	}
	return "", err
}

func lookPath(name string) string {
	if v, ok := lookPathCache.Load(name); ok {
		return v.(string)
	}
	p, err := lookPathOrRancher(name)
	if err != nil && name == "docker" {
		// nerdctl (Rancher Desktop's containerd mode) takes the same verbs the pack uses.
		p, err = lookPathOrRancher("nerdctl")
	}
	if err != nil {
		panic(fmt.Sprintf("%s not found on PATH: %v", name, err))
	}
	lookPathCache.Store(name, p)
	return p
}
