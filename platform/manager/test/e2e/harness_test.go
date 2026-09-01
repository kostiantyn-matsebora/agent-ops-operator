//go:build e2e

// Package e2e is the end-to-end pack: a real single-node cluster (k3s under
// k3d), the chart installed from the working tree with images built from the
// same commit, and assertions against the live cluster. Its subject is the
// SUBSTRATE — what envtest structurally cannot decide because it runs no
// kubelet, no scheduler, no CSI and no authorizer against real subjects.
//
// Behind the `e2e` build tag so `go test ./...` never provisions a cluster:
//
//	cd platform/manager && go test -tags e2e -timeout 40m ./test/e2e/
//
// Knobs (environment):
//
//	E2E_CLUSTER        k3d cluster name (default agentops-e2e)
//	E2E_REUSE=1        keep an existing cluster and leave it running afterwards
//	E2E_SKIP_BUILD=1   images are already built and imported (with E2E_REUSE)
//	E2E_ARTIFACT_DIR   where failure diagnostics are written (default $TMPDIR/agentops-e2e)
//	E2E_TIER           smoke (default) or full — full adds the real-runtime lane
//	E2E_BUDGET         wall-clock budget of the gating tier (default 20m)
//	CLAUDE_CODE_OAUTH_TOKEN  the real-runtime lane's credential; absent, the lane is SKIPPED
package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Namespace is the release namespace.
const Namespace = "agent-ops"

// Release is the helm release name.
const Release = "agentops"

var (
	env  *Env
	tier string
	// started stamps the gating tier's wall clock.
	started time.Time
)

// Env is everything a test needs: the cluster's client and the port-forwards.
type Env struct {
	Cluster *Cluster
	K       *Kube
	// Ports opened by port-forward, by service name.
	Manager  *Forward
	Console  *Forward
	BotAPI   *Forward
	Adapter  map[string]*Forward
	Values   *InstallValues
	artifact string
}

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	tier = os.Getenv("E2E_TIER")
	if tier == "" {
		tier = "smoke"
	}
	artifact := os.Getenv("E2E_ARTIFACT_DIR")
	if artifact == "" {
		artifact = filepath.Join(os.TempDir(), "agentops-e2e")
	}
	_ = os.MkdirAll(artifact, 0o755)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	started = time.Now()
	cluster, err := EnsureCluster(ctx, clusterName(), os.Getenv("E2E_REUSE") == "1")
	if err != nil {
		fmt.Fprintln(os.Stderr, "cluster:", err)
		return 1
	}
	code := 1
	defer func() {
		if code != 0 {
			cluster.Dump(artifact)
		}
		if os.Getenv("E2E_REUSE") != "1" {
			cluster.Delete()
		}
	}()
	if os.Getenv("E2E_SKIP_BUILD") != "1" {
		if err := BuildAndImport(ctx, cluster, neededImages(tier)); err != nil {
			fmt.Fprintln(os.Stderr, "images:", err)
			return 1
		}
	}
	values := DefaultValues(tier)
	if err := Install(ctx, cluster, values); err != nil {
		fmt.Fprintln(os.Stderr, "install:", err)
		return 1
	}
	k, err := NewKube(cluster.Kubeconfig, Namespace)
	if err != nil {
		fmt.Fprintln(os.Stderr, "client:", err)
		return 1
	}
	if err := WaitReady(ctx, k, values); err != nil {
		fmt.Fprintln(os.Stderr, "readiness:", err)
		return 1
	}
	if os.Getenv("E2E_REUSE") == "1" {
		if err := resetReusedInstall(ctx, cluster, k); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	if err := SetupWiring(ctx, k); err != nil {
		fmt.Fprintln(os.Stderr, "wiring:", err)
		return 1
	}
	var envErr error
	if env, envErr = openHarnessEnv(cluster, k, values, artifact); envErr != nil {
		fmt.Fprintln(os.Stderr, envErr)
		return 1
	}
	code = m.Run()
	return enforceSmokeBudget(code)
}

// resetReusedInstall: a reused install carries the previous run's
// conversations and the manager's in-memory state — an open storage breaker,
// above all. Start from a clean manager, as a fresh cluster would.
func resetReusedInstall(ctx context.Context, cluster *Cluster, k *Kube) error {
	// Deleted conversations linger under their close-topics finalizer for up
	// to two minutes, and a signal with a lingering one's signature folds
	// into it. Wait until they are really gone.
	_, _ = cluster.Kubectl(ctx, "-n", Namespace, "delete", "conversations", "--all", "--wait=false")
	deadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(deadline) {
		items, err := k.Conversations(ctx)
		if err == nil && len(items) == 0 {
			break
		}
		time.Sleep(3 * time.Second)
	}
	// rollout status waits for the OLD pod to be gone too, so the
	// port-forward below cannot latch onto a terminating one.
	_, _ = cluster.Kubectl(ctx, "-n", Namespace, "rollout", "restart", "deployment/agentops-manager")
	if out, err := cluster.Kubectl(ctx, "-n", Namespace, "rollout", "status", "deployment/agentops-manager", "--timeout=3m"); err != nil {
		return fmt.Errorf("manager restart: %w %s", err, out)
	}
	// The restarted manager re-applies every adapter Deployment and their
	// pods roll shortly after — a port-forward opened before that lands on
	// a pod about to die. Roll them NOW, deliberately, and wait.
	out, _ := cluster.Kubectl(ctx, "-n", Namespace, "get", "deployments", "-l", "app.kubernetes.io/name in (agentops-adapter, agentops-signal-adapter)", "-o", "name")
	for _, d := range strings.Fields(out) {
		_, _ = cluster.Kubectl(ctx, "-n", Namespace, "rollout", "restart", d)
	}
	for _, d := range strings.Fields(out) {
		if out, err := cluster.Kubectl(ctx, "-n", Namespace, "rollout", "status", d, "--timeout=3m"); err != nil {
			return fmt.Errorf("adapter restart: %s %w %s", d, err, out)
		}
	}
	return nil
}

func openHarnessEnv(cluster *Cluster, k *Kube, values *InstallValues, artifact string) (*Env, error) {
	e := &Env{Cluster: cluster, K: k, Values: values, Adapter: map[string]*Forward{}, artifact: artifact}
	var err error
	if e.Manager, err = cluster.Forward(Namespace, "svc/agentops-manager", 8080); err != nil {
		return nil, fmt.Errorf("forward manager: %w", err)
	}
	if e.Console, err = cluster.Forward(Namespace, "svc/agentops-adapter-console", 8080); err != nil {
		return nil, fmt.Errorf("forward console: %w", err)
	}
	if e.BotAPI, err = cluster.Forward(Namespace, "svc/agentops-test-fake-bot-api", 8081); err != nil {
		return nil, fmt.Errorf("forward fake bot api: %w", err)
	}
	return e, nil
}

// enforceSmokeBudget is asserted for the gating tier only: growth must be
// visible rather than gradual, and the nightly pack is allowed to be slow.
func enforceSmokeBudget(code int) int {
	if tier != "smoke" {
		return code
	}
	budget := 20 * time.Minute
	if v := os.Getenv("E2E_BUDGET"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			budget = d
		}
	}
	elapsed := time.Since(started)
	if elapsed > budget {
		fmt.Fprintf(os.Stderr, "WALL-CLOCK BUDGET EXCEEDED: the gating tier took %s, budget %s — move work to the full tier rather than slowing the gate\n", elapsed.Round(time.Second), budget)
		if code == 0 {
			code = 1
		}
		return code
	}
	fmt.Fprintf(os.Stderr, "gating tier took %s of its %s budget\n", elapsed.Round(time.Second), budget)
	return code
}

func clusterName() string {
	if n := os.Getenv("E2E_CLUSTER"); n != "" {
		return n
	}
	return "agentops-e2e"
}

// requireEnv skips a test when the harness did not come up (TestMain already
// reported why), so each test fails for its own reason only.
func requireEnv(t *testing.T) *Env {
	t.Helper()
	if env == nil {
		t.Skip("the harness is not up")
	}
	t.Cleanup(func() {
		if t.Failed() {
			env.Cluster.Dump(filepath.Join(env.artifact, t.Name()))
		}
	})
	return env
}

// fullTier skips a test that belongs to the full pack when the gating tier
// runs, reporting it as skipped rather than failed.
func fullTier(t *testing.T) {
	t.Helper()
	if tier != "full" {
		t.Skip("full tier only (E2E_TIER=full)")
	}
}
