//go:build conformance

package conformance

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// repoRoot walks up from this file to the repository root.
func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}

// fixture reads a canonical payload from test/fixtures.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(), "test", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var (
	buildMu    sync.Mutex
	buildCache = map[string]string{}
)

// build compiles a module under the repository into a binary, once per test
// run. Adapters are LISTED here by directory, never imported: the artifact
// under test is the one that ships.
func build(t *testing.T, dir string, tags ...string) string {
	t.Helper()
	key := dir + "|" + strings.Join(tags, ",")
	buildMu.Lock()
	defer buildMu.Unlock()
	if bin, ok := buildCache[key]; ok {
		return bin
	}
	out := filepath.Join(os.TempDir(), "agentops-conformance", strings.ReplaceAll(dir, "/", "-"))
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	args := []string{"build", "-o", out}
	if len(tags) > 0 {
		args = append(args, "-tags", strings.Join(tags, ","))
	}
	args = append(args, ".")
	cmd := exec.Command("go", args...)
	cmd.Dir = filepath.Join(repoRoot(), dir)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building %s: %v\n%s", dir, err, b)
	}
	buildCache[key] = out
	return out
}

// freePort asks the kernel for an unused TCP port.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// Process is a running adapter binary.
type Process struct {
	Name string
	Port int // LISTEN_ADDR port, 0 when the adapter hosts nothing
	cmd  *exec.Cmd
	out  *lockedBuffer
	done chan struct{}
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// start runs a built binary with the given environment (KEY=VALUE), stopping
// it when the test ends and dumping its output on failure.
func start(t *testing.T, name, bin string, env []string) *Process {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}, env...)
	buf := &lockedBuffer{}
	cmd.Stdout, cmd.Stderr = buf, buf
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting %s: %v", name, err)
	}
	p := &Process{Name: name, cmd: cmd, out: buf, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(p.done) }()
	t.Cleanup(func() {
		p.Stop()
		if t.Failed() {
			t.Logf("---- %s output ----\n%s", name, buf.String())
		}
	})
	return p
}

// Stop terminates the process.
func (p *Process) Stop() {
	select {
	case <-p.done:
		return
	default:
	}
	_ = p.cmd.Process.Signal(os.Interrupt)
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		_ = p.cmd.Process.Kill()
		<-p.done
	}
}

// Exited reports whether the process has ended.
func (p *Process) Exited() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

// Output is what the process wrote so far.
func (p *Process) Output() string { return p.out.String() }

// URL is the adapter's own listener.
func (p *Process) URL() string { return fmt.Sprintf("http://127.0.0.1:%d", p.Port) }

// waitFor polls cond until it holds or the deadline passes.
func waitFor(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", timeout, what)
}

// waitHealthy waits for an adapter's own /healthz.
func waitHealthy(t *testing.T, p *Process) {
	t.Helper()
	waitFor(t, p.Name+" to listen", 20*time.Second, func() bool {
		if p.Exited() {
			t.Fatalf("%s exited before it was healthy:\n%s", p.Name, p.Output())
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, "GET", p.URL()+"/healthz", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == 200
	})
}

// contractEnv is the environment every adapter receives from the reconciler.
func contractEnv(mgr *FakeManager, adapterName string, port int) []string {
	env := []string{
		"MANAGER_URL=" + mgr.URL(),
		"ADAPTER_TOKEN=" + mgr.Token,
		"ADAPTER_NAME=" + adapterName,
		"POD_NAMESPACE=agent-ops",
	}
	if port > 0 {
		env = append(env, fmt.Sprintf("LISTEN_ADDR=127.0.0.1:%d", port))
	}
	return env
}

// postJSON posts a body and returns the status and response text.
func postJSON(t *testing.T, url string, body []byte, headers map[string]string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	var b bytes.Buffer
	_, _ = b.ReadFrom(resp.Body)
	return resp.StatusCode, b.String()
}
