package system

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOOMFixIsOffByDefault(t *testing.T) {
	t.Setenv(oomFixEnv, "")
	args, cleanup, err := oomFixArgs()
	defer cleanup()
	if err != nil || len(args) != 0 {
		t.Fatalf("want no args and no error, got %v, %v", args, err)
	}
}

func TestOOMFixMountsBothDropInsAndCleansUp(t *testing.T) {
	t.Setenv(oomFixEnv, "1")
	args, cleanup, err := oomFixArgs()
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 4 {
		t.Fatalf("want two --volume pairs, got %v", args)
	}
	var files []string
	for i := 0; i < len(args); i += 2 {
		if args[i] != "--volume" || !strings.Contains(args[i+1], nodeContainerdDir) || !strings.HasSuffix(args[i+1], "@server:*") {
			t.Fatalf("unexpected mount %v", args[i:i+2])
		}
		f := strings.SplitN(args[i+1], ":", 2)[0]
		if strings.HasSuffix(f, ".toml") {
			t.Fatalf("a file mount breaks `k3d kubeconfig get`, want a directory: %s", f)
		}
		b, rerr := os.ReadFile(filepath.Join(f, "10-restrict-oom.toml"))
		if rerr != nil || !strings.Contains(string(b), "restrict_oom_score_adj = true") {
			t.Fatalf("drop-in %s unreadable or wrong: %v %q", f, rerr, b)
		}
		files = append(files, f)
	}
	cleanup()
	for _, f := range files {
		if _, serr := os.Stat(f); !os.IsNotExist(serr) {
			t.Fatalf("%s not removed", f)
		}
	}
}
