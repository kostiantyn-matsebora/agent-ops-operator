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

func TestACloudSessionTurnsTheOOMFixOnUnlessToldOtherwise(t *testing.T) {
	cases := []struct {
		name, remote, env string
		set               bool
		want              bool
	}{
		{"workstation, unset", "", "", false, false},
		{"cloud, unset", "1", "", false, true},
		{"cloud, true", "true", "", false, true},
		{"cloud, explicitly off", "1", "0", true, false},
		{"workstation, explicitly on", "", "1", true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("CLAUDE_CODE_REMOTE", c.remote)
			if c.set {
				t.Setenv(oomFixEnv, c.env)
			} else {
				os.Unsetenv(oomFixEnv)
			}
			if got := oomFixEnabled(); got != c.want {
				t.Fatalf("oomFixEnabled() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestACloudSessionFindsTheBuildCABundleItself(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "bundle.crt")
	if err := os.WriteFile(bundle, []byte("pem"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", bundle)
	os.Unsetenv(buildCAEnvName)

	t.Setenv("CLAUDE_CODE_REMOTE", "")
	if got := buildCAPath(); got != "" {
		t.Fatalf("a workstation must not pick a bundle up by itself, got %q", got)
	}
	t.Setenv("CLAUDE_CODE_REMOTE", "1")
	if got := buildCAPath(); got != bundle && got != "/root/.ccr/ca-bundle.crt" {
		t.Fatalf("a cloud session must find a bundle, got %q", got)
	}
	t.Setenv(buildCAEnvName, "/explicit.crt")
	if got := buildCAPath(); got != "/explicit.crt" {
		t.Fatalf("an explicit bundle wins, got %q", got)
	}
	t.Setenv(buildCAEnvName, "")
	if got := buildCAPath(); got != "" {
		t.Fatalf("an explicit empty value turns it off, got %q", got)
	}
}

func TestCCRCABundleMovesWhereACloudKeepsTheBundle(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "moved.crt")
	if err := os.WriteFile(bundle, []byte("pem"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Unsetenv(buildCAEnvName)
	t.Setenv("CLAUDE_CODE_REMOTE", "1")
	t.Setenv("SSL_CERT_FILE", "")
	t.Setenv("CCR_CA_BUNDLE", bundle)
	if got := buildCAPath(); got != bundle {
		t.Fatalf("buildCAPath() = %q, want the CCR_CA_BUNDLE file %q", got, bundle)
	}
}
