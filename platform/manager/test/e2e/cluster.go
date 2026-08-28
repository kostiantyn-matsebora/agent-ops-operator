//go:build e2e

package e2e

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Cluster is one k3d cluster and the kubeconfig that reaches it.
type Cluster struct {
	Name       string
	Kubeconfig string
	reused     bool
}

// EnsureCluster creates the cluster, or adopts a running one when reuse is
// asked for. Traefik and servicelb are disabled: they add startup time and a
// LoadBalancer the chart does not assume, and a Service that behaves
// differently under test than in a typical install is false confidence.
func EnsureCluster(ctx context.Context, name string, reuse bool) (*Cluster, error) {
	c := &Cluster{Name: name}
	exists := exec.Command("k3d", "cluster", "get", name).Run() == nil
	if exists && !reuse {
		if out, err := exec.CommandContext(ctx, "k3d", "cluster", "delete", name).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("deleting the stale cluster: %v\n%s", err, out)
		}
		exists = false
	}
	if !exists {
		// The registries file names the pull test's registry as plain HTTP;
		// containerd reads it at start only, so it is given at creation.
		regs := filepath.Join(os.TempDir(), "agentops-e2e-"+name+"-registries.yaml")
		if err := os.WriteFile(regs, []byte(registriesYAML), 0o644); err != nil {
			return nil, err
		}
		args := []string{"cluster", "create", name,
			"--k3s-arg", "--disable=traefik@server:0",
			"--k3s-arg", "--disable=servicelb@server:0",
			"--registry-config", regs,
			"--wait", "--timeout", "180s",
			"--no-lb",
		}
		if out, err := exec.CommandContext(ctx, "k3d", args...).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("k3d cluster create: %v\n%s", err, out)
		}
	} else {
		c.reused = true
	}
	kc, err := exec.CommandContext(ctx, "k3d", "kubeconfig", "get", name).Output()
	if err != nil {
		return nil, fmt.Errorf("k3d kubeconfig get: %w", err)
	}
	path := filepath.Join(os.TempDir(), "agentops-e2e-"+name+".kubeconfig")
	if err := os.WriteFile(path, kc, 0o600); err != nil {
		return nil, err
	}
	c.Kubeconfig = path
	// Wait for the node to be Ready and the local-path provisioner to exist —
	// the context volume is bound by it.
	if out, err := c.Kubectl(ctx, "wait", "--for=condition=Ready", "node", "--all", "--timeout=180s"); err != nil {
		return nil, fmt.Errorf("node readiness: %v\n%s", err, out)
	}
	return c, nil
}

// Delete tears the cluster down.
func (c *Cluster) Delete() {
	_ = exec.Command("k3d", "cluster", "delete", c.Name).Run()
}

// Kubectl runs kubectl against the cluster.
func (c *Cluster) Kubectl(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "kubectl", append([]string{"--kubeconfig", c.Kubeconfig}, args...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Helm runs helm against the cluster.
func (c *Cluster) Helm(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "helm", append([]string{"--kubeconfig", c.Kubeconfig}, args...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Import loads locally built images into the node so they are never pulled.
func (c *Cluster) Import(ctx context.Context, images ...string) error {
	args := append([]string{"image", "import", "-c", c.Name, "-m", "direct"}, images...)
	cmd := exec.CommandContext(ctx, "k3d", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("k3d image import: %v\n%s", err, out)
	}
	return nil
}

// Forward is a kubectl port-forward to one Service.
type Forward struct {
	Port int
	cmd  *exec.Cmd
}

// URL is the local base URL.
func (f *Forward) URL() string { return fmt.Sprintf("http://127.0.0.1:%d", f.Port) }

// Stop ends the port-forward.
func (f *Forward) Stop() {
	if f.cmd != nil && f.cmd.Process != nil {
		_ = f.cmd.Process.Kill()
	}
}

var forwardingRE = regexp.MustCompile(`Forwarding from 127\.0\.0\.1:(\d+)`)

// Forward opens a port-forward on a free local port and waits for it.
func (c *Cluster) Forward(namespace, target string, remotePort int) (*Forward, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	cmd := exec.Command("kubectl", "--kubeconfig", c.Kubeconfig, "-n", namespace, "port-forward", target,
		fmt.Sprintf("%d:%d", port, remotePort))
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	ready := make(chan bool, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if forwardingRE.MatchString(sc.Text()) {
				ready <- true
				break
			}
		}
	}()
	select {
	case <-ready:
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("port-forward %s never came up: %s", target, stderr.String())
	}
	return &Forward{Port: port, cmd: cmd}, nil
}

// Dump writes the unconditional failure diagnostics: manager and adapter
// logs, the pod list, cluster events and the full YAML of every agentops
// CR. A remote cluster that no longer exists is unreproducible, so a failure
// that did not capture its own context costs a full re-run to learn anything.
func (c *Cluster) Dump(dir string) {
	_ = os.MkdirAll(dir, 0o755)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	write := func(name string, args ...string) {
		out, _ := c.Kubectl(ctx, args...)
		_ = os.WriteFile(filepath.Join(dir, name), []byte(out), 0o644)
	}
	write("pods.txt", "-n", Namespace, "get", "pods", "-o", "wide")
	write("pods.yaml", "-n", Namespace, "get", "pods", "-o", "yaml")
	write("events.txt", "get", "events", "-A", "--sort-by=.lastTimestamp")
	write("agentops-crs.yaml", "-n", Namespace, "get",
		"agentprofiles,agentruntimes,channels,channeladapters,conversations,mcpconfigs,mcptoolsets,pipelines,signaladapters,signalsources",
		"-o", "yaml")
	write("helm-values.yaml", "-n", Namespace, "get", "secret", "-l", "owner=helm", "-o", "name")
	out, _ := c.Kubectl(ctx, "-n", Namespace, "get", "pods", "-o", "jsonpath={range .items[*]}{.metadata.name}{\"\\n\"}{end}")
	for _, pod := range strings.Fields(out) {
		write("log-"+pod+".txt", "-n", Namespace, "logs", pod, "--all-containers", "--tail=2000")
		write("log-"+pod+"-previous.txt", "-n", Namespace, "logs", pod, "--all-containers", "--previous", "--tail=500")
	}
	fmt.Fprintf(os.Stderr, "diagnostics written to %s\n", dir)
}
