//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// repoRoot walks up from this file to the repository root.
func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}

// Image is one image the pack builds and imports.
type Image struct {
	Component  string // as components.sh names it, or a test name
	Context    string
	Dockerfile string
	Tag        string // <ref>:e2e
}

// testImages are the two doubles under test/, which components.sh deliberately
// does not list.
var testImages = []Image{
	{Component: "test-stub-runtime", Context: "test/stubruntime", Dockerfile: "test/stubruntime/Dockerfile", Tag: "agentops-test-stub-runtime:e2e"},
	{Component: "test-fake-bot-api", Context: "test/fakebotapi", Dockerfile: "test/fakebotapi/Dockerfile", Tag: "agentops-test-fake-bot-api:e2e"},
}

// neededImages names the components a tier installs. Everything else the
// repository ships is reported as NOT built, so the omission is visible
// rather than read as "covered".
func neededImages(tier string) map[string]bool {
	need := map[string]bool{
		"manager": true, "console": true, "context-sync": true, "egress-proxy": true,
		"channel-telegram": true, "gateway-telegram": true, "signal-telegram": true,
		"signal-alertmanager": true, "signal-k8s-events": true, "signal-cron": true,
	}
	if tier == "full" {
		need["runtime-claude"] = true
	}
	return need
}

// discover reads the component list from .github/components.sh — the same
// answer CI's matrices get, never a list kept here.
func discover() ([]Image, error) {
	out, err := exec.Command(filepath.Join(repoRoot(), ".github", "components.sh"), "images").Output()
	if err != nil {
		return nil, fmt.Errorf("components.sh images: %w", err)
	}
	var items []struct {
		Component  string `json:"component"`
		Context    string `json:"context"`
		Dockerfile string `json:"dockerfile"`
	}
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, err
	}
	var images []Image
	for _, it := range items {
		images = append(images, Image{Component: it.Component, Context: it.Context, Dockerfile: it.Dockerfile,
			Tag: "agentops-" + it.Component + ":e2e"})
	}
	return images, nil
}

// BuildAndImport builds every needed image for the host architecture and
// imports it into the node. IMPORTED IMAGES ARE NEVER PULLED — which is why
// one dedicated test (the local registry) exercises the pull path.
func BuildAndImport(ctx context.Context, c *Cluster, need map[string]bool) error {
	all, err := discover()
	if err != nil {
		return err
	}
	var build, skipped []string
	var tags []string
	var chosen []Image
	for _, img := range all {
		if need[img.Component] {
			chosen = append(chosen, img)
			build = append(build, img.Component)
		} else {
			skipped = append(skipped, img.Component)
		}
	}
	chosen = append(chosen, testImages...)
	sort.Strings(skipped)
	fmt.Fprintf(os.Stderr, "building %d images: %s\nNOT built for this tier (not installed by it): %s\n",
		len(chosen), strings.Join(build, " "), strings.Join(skipped, " "))
	for _, img := range chosen {
		if err := dockerBuild(ctx, img); err != nil {
			return err
		}
		tags = append(tags, img.Tag)
	}
	return c.Import(ctx, tags...)
}

func dockerBuild(ctx context.Context, img Image) error {
	root := repoRoot()
	args := []string{"buildx", "build", "--load",
		"-f", filepath.Join(root, img.Dockerfile),
		"-t", img.Tag,
		filepath.Join(root, img.Context)}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	fmt.Fprintf(os.Stderr, "== docker build %s (%s)\n", img.Tag, img.Context)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("building %s: %w", img.Tag, err)
	}
	return nil
}
