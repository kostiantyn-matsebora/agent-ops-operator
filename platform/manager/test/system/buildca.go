//go:build system

package system

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// SYSTEM_BUILD_CA names a CA bundle the image builds must trust. A sandbox whose egress is intercepted
// (a Claude Code cloud session is one) re-signs HTTPS with a CA the host trusts and a build container
// does not, so `npm ci` and `go mod download` inside a build fail, npm by hanging until it crashes.
//
// The bundle is baked into LOCAL COPIES of the Dockerfiles' base images, under the same tags. Docker uses
// a local tag in preference to a remote one, so no Dockerfile changes and none knows about this. Unset, as
// everywhere except such a sandbox, nothing here runs.
const buildCAEnv = "SYSTEM_BUILD_CA"

// buildCALabel marks a base image already carrying the bundle, so a second run does not layer it twice.
const buildCALabel = "agentops.dev/build-ca"

var fromLine = regexp.MustCompile(`(?i)^\s*FROM\s+((?:--\S+\s+)*)(\S+)(?:\s+AS\s+(\S+))?\s*$`)

// baseImages is every EXTERNAL image a Dockerfile builds on: not a stage defined earlier in it, not scratch,
// and not one named through a build argument (which has no value to pull before the build runs).
func baseImages(dockerfile string) []string {
	stages := map[string]bool{}
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.Split(dockerfile, "\n") {
		m := fromLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ref, stage := m[2], m[3]
		if stage != "" {
			stages[strings.ToLower(stage)] = true
		}
		if strings.EqualFold(ref, "scratch") || strings.Contains(ref, "$") || stages[strings.ToLower(ref)] && ref != stage {
			continue
		}
		if !seen[ref] {
			seen[ref] = true
			out = append(out, ref)
		}
	}
	return out
}

// derivedDockerfile adds the bundle to ref, and points every common client's trust setting at it. COPY and
// ENV only, so it works on an image with no shell (distroless).
func derivedDockerfile(ref string) string {
	const bundle = "/etc/ssl/certs/agentops-build-ca.pem"
	return fmt.Sprintf(`FROM %s
COPY build-ca.pem %s
ENV SSL_CERT_FILE=%[2]s NODE_EXTRA_CA_CERTS=%[2]s REQUESTS_CA_BUNDLE=%[2]s CURL_CA_BUNDLE=%[2]s GIT_SSL_CAINFO=%[2]s
LABEL %s=1
`, ref, bundle, buildCALabel)
}

func prepareBuildCA(ctx context.Context, chosen []Image) error {
	src := os.Getenv(buildCAEnv)
	if src == "" {
		return nil
	}
	pem, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("%s=%s: %w", buildCAEnv, src, err)
	}
	refs := map[string]bool{}
	var order []string
	for _, img := range chosen {
		text, err := os.ReadFile(filepath.Join(repoRoot(), img.Dockerfile))
		if err != nil {
			return err
		}
		for _, ref := range baseImages(string(text)) {
			if !refs[ref] {
				refs[ref] = true
				order = append(order, ref)
			}
		}
	}
	dir, err := os.MkdirTemp("", "agentops-buildca-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := os.WriteFile(filepath.Join(dir, "build-ca.pem"), pem, 0o644); err != nil {
		return err
	}
	for _, ref := range order {
		have, _ := exec.CommandContext(ctx, lookPath("docker"), "image", "inspect", "-f",
			`{{index .Config.Labels "`+buildCALabel+`"}}`, ref).Output()
		if strings.TrimSpace(string(have)) == "1" {
			continue
		}
		fmt.Fprintf(os.Stderr, "== trusting the build CA in %s\n", ref)
		cmd := exec.CommandContext(ctx, lookPath("docker"), "build", "-t", ref, "-f", "-", dir)
		cmd.Stdin = bytes.NewBufferString(derivedDockerfile(ref))
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("adding the build CA to %s: %w", ref, err)
		}
	}
	return nil
}

// nodeBundle is where the bundle lands inside a k3s node.
const nodeBundle = "/etc/ssl/certs/agentops-build-ca.pem"

// clusterCAArgs makes a k3s node trust the same bundle. The node's containerd pulls its own system images
// (pause, coredns, the local-path provisioner) from the internet, and behind an intercepting proxy it
// refuses their certificates, so no pod ever leaves ContainerCreating. containerd is a Go program: it reads
// SSL_CERT_FILE, and the bundle is the host's FULL one, so replacing the node's roots with it loses none.
// Empty when SYSTEM_BUILD_CA is unset.
func clusterCAArgs(src string) []string {
	if src == "" {
		return nil
	}
	return []string{
		"--volume", src + ":" + nodeBundle + "@server:*",
		"--env", "SSL_CERT_FILE=" + nodeBundle + "@server:*",
	}
}
