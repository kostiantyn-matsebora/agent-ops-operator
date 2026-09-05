//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// InstallValues is what the pack decides about the install; everything else
// is the chart's default, because a pack that tunes the install tests an
// install nobody runs.
type InstallValues struct {
	Tier         string
	AdapterToken string
	UIToken      string
	BotToken     string
	ChatID       string
	// Runtime is the AgentRuntime the stub answers to.
	Runtime string
}

// DefaultValues is the install the pack runs against.
func DefaultValues(tier string) *InstallValues {
	return &InstallValues{
		Tier:         tier,
		AdapterToken: "e2e-adapter-token-not-a-secret",
		UIToken:      "e2e-ui-token-not-a-secret",
		BotToken:     "1234567890:e2e-fake-bot-token",
		ChatID:       "-1001234567890",
		Runtime:      "stub",
	}
}

// valuesYAML renders the values file. Images are the ones the pack built and
// imported, tagged :e2e; the reference runtime bundle is OFF and the stub is
// the release's default runtime, so a route naming nothing runs on it.
func (v *InstallValues) valuesYAML() string {
	claude := "false"
	// The real-runtime lane (TestRealRuntime) is the only consumer of the
	// kubernetes MCP server — it asks the agent to read a live pod name
	// through it. Off elsewhere: it is a real workload pull
	// (containers/kubernetes-mcp-server) that no other lane exercises.
	k8sMCP := "false"
	if v.Tier == "full" {
		claude = "true"
		k8sMCP = "true"
	}
	return fmt.Sprintf(`# rendered by the e2e pack
image:
  repository: agentops-manager
  tag: e2e
adapterAuth:
  token: %[1]q
global:
  agentops:
    runtimeDefaults:
      idleTtlMinutes: 1
      contextSync:
        image: agentops-context-sync:e2e
      egressMediation:
        image: agentops-egress-proxy:e2e
claude:
  enabled: %[2]s
  image: agentops-runtime-claude:e2e
runtimes:
  - name: %[3]s
    default: true
    image: agentops-test-stub-runtime:e2e
    contextStorage: volume
# ONE NODE: k3s's local-path provisioner serves ReadWriteOnce only, and on a
# single node that is exactly what a shared context volume needs. The chart's
# ReadWriteMany default is for a real cluster with a real shared filesystem.
persistence:
  context:
    accessModes: [ReadWriteOnce]
console:
  enabled: true
  image:
    repository: agentops-console
    tag: e2e
  auth:
    uiToken: %[4]q
telegram:
  enabled: true
  apiBase: http://agentops-test-fake-bot-api:8081
  channelAdapter:
    image: {repository: agentops-channel-telegram, tag: e2e}
  signalAdapter:
    image: {repository: agentops-signal-telegram, tag: e2e}
  router:
    image: {repository: agentops-gateway-telegram, tag: e2e}
  surface:
    enabled: true
    name: tg-ops
    chatId: %[5]q
    credentials:
      botToken: %[6]q
kubernetes:
  enabled: true
  eventsAdapter:
    enabled: true
    image: {repository: agentops-signal-k8s-events, tag: e2e}
  mcpServers:
    enabled: %[7]s
  mcp:
    enabled: %[7]s
  pipelines:
    enabled: false
prometheus:
  enabled: true
  alertmanager:
    enabled: true
    image: {repository: agentops-signal-alertmanager, tag: e2e}
  pipelines:
    enabled: false
`, v.AdapterToken, claude, v.Runtime, v.UIToken, v.ChatID, v.BotToken, k8sMCP)
}

// Install applies the CRDs, deploys the fake Bot API, and installs the chart
// from the working tree.
func Install(ctx context.Context, c *Cluster, v *InstallValues) error {
	root := repoRoot()
	// CRDs FIRST, always: helm installs a CRD only when absent and never
	// upgrades one, and a cluster that carried an older CRD would prune every
	// new field silently.
	if out, err := c.Kubectl(ctx, "apply", "-f", filepath.Join(root, "chart", "crds")); err != nil {
		return fmt.Errorf("applying CRDs: %v\n%s", err, out)
	}
	if out, err := c.Kubectl(ctx, "create", "namespace", Namespace); err != nil && !strings.Contains(out, "AlreadyExists") {
		return fmt.Errorf("namespace: %v\n%s", err, out)
	}
	if out, err := c.Kubectl(ctx, "-n", Namespace, "apply", "-f", filepath.Join(root, "test", "fakebotapi", "deploy", "fake-bot-api.yaml")); err != nil {
		return fmt.Errorf("fake bot api: %v\n%s", err, out)
	}
	// CreateTemp, not a fixed name under os.TempDir(): the old name was the
	// SAME for every cluster, so two e2e runs on one machine would race on it
	// too, not merely predictable to another local user.
	valuesFile, err := os.CreateTemp("", "agentops-e2e-values-*.yaml")
	if err != nil {
		return err
	}
	values := valuesFile.Name()
	// helm reads it once, at the upgrade/install call below, and nothing
	// after this function needs it.
	defer func() { _ = os.Remove(values) }()
	_, writeErr := valuesFile.Write([]byte(v.valuesYAML()))
	closeErr := valuesFile.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if out, err := c.Helm(ctx, "dependency", "build", filepath.Join(root, "chart")); err != nil {
		return fmt.Errorf("helm dependency build: %v\n%s", err, out)
	}
	// No --wait: the context claim binds on FIRST CONSUMER under k3s's
	// local-path class, so it stays Pending until a runtime pod mounts it and
	// helm's waiter would call that a failed install. Readiness is asserted
	// per Deployment instead, by WaitReady.
	out, err := c.Helm(ctx, "upgrade", "--install", Release, filepath.Join(root, "chart"),
		"-n", Namespace, "-f", values, "--timeout", "10m")
	if err != nil {
		return fmt.Errorf("helm install: %v\n%s", err, out)
	}
	return nil
}

// WaitReady waits for the manager and every enabled adapter Deployment.
func WaitReady(ctx context.Context, k *Kube, v *InstallValues) error {
	deployments := []string{
		"agentops-manager", "agentops-adapter-console", "agentops-test-fake-bot-api",
		"agentops-adapter-telegram", "agentops-signal-telegram", "agentops-gateway-telegram",
		"agentops-signal-k8s-events", "agentops-signal-alertmanager",
	}
	// The kubernetes MCP server workload renders only under the full tier —
	// see valuesYAML's k8sMCP — so it is the one Deployment this list adds
	// conditionally rather than unconditionally.
	if v.Tier == "full" {
		deployments = append(deployments, "agentops-mcp-k8s")
	}
	for _, d := range deployments {
		if err := k.WaitDeploymentAvailable(ctx, d, 5*time.Minute); err != nil {
			return err
		}
	}
	return nil
}
