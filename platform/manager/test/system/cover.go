//go:build system

package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// The system tier's COVERAGE run, behind SYSTEM_COVER=1.
//
// Coverage of the lines a change added is judged per tier (.github/scripts/
// coverage-diff.py), and a cluster is the only place some of the operator runs.
// So the manager is built with `go build -cover` (images.go passes GOCOVER=1),
// the Helm post-renderer in covrender/ points its GOCOVERDIR at a hostPath on the
// node, and after the pack this file stops the manager gracefully, copies the
// counters off the node and turns them into a profile.
//
// A RUN THAT WROTE NOTHING IS A FAILURE, NOT A CLEAN DIFF. CollectCover returns an
// error when no counter file came back, and the caller leaves no profile behind, so
// the station reads the tier as `collection failed` instead of "nothing changed".

// coverDir is the node directory the manager's counters land in.
const coverDir = "/covdata"

// CoverProfile is the profile's file name inside SYSTEM_ARTIFACT_DIR.
const CoverProfile = "system-coverage.out"

func coverEnabled() bool { return os.Getenv("SYSTEM_COVER") == "1" }

func coverNode(c *Cluster) string { return "k3d-" + c.Name + "-server-0" }

// prepareCover makes the node directory the manager, running as a non-root
// user, can write into. It is emptied first so a reused cluster's earlier run
// cannot be mistaken for this one's.
func prepareCover(ctx context.Context, c *Cluster) error {
	out, err := exec.CommandContext(ctx, lookPath("docker"), "exec", coverNode(c), "sh", "-c",
		"rm -rf "+coverDir+" && mkdir -m 777 "+coverDir).CombinedOutput()
	if err != nil {
		return fmt.Errorf("prepare %s on the node: %v\n%s", coverDir, err, out)
	}
	return nil
}

// buildRenderer compiles the Helm post-renderer and returns its path.
func buildRenderer(ctx context.Context) (string, error) {
	dir, err := os.MkdirTemp("", "agentops-covrender-*")
	if err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "covrender")
	cmd := exec.CommandContext(ctx, lookPath("go"), "build", "-o", bin, "./test/system/covrender")
	cmd.Dir = filepath.Join(repoRoot(), "platform", "manager")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build covrender: %v\n%s", err, out)
	}
	return bin, nil
}

// CollectCover stops the manager so it flushes its counters, copies them off the
// node and writes artifactDir/CoverProfile.
func CollectCover(ctx context.Context, c *Cluster, artifactDir string) error {
	if out, err := c.Kubectl(ctx, "-n", Namespace, "scale", "deployment/agentops-manager", "--replicas=0"); err != nil {
		return fmt.Errorf("stop the manager: %v\n%s", err, out)
	}
	if out, err := c.Kubectl(ctx, "-n", Namespace, "wait", "--for=delete", "pod",
		"-l", "app.kubernetes.io/name=agentops-manager", "--timeout=3m"); err != nil {
		return fmt.Errorf("wait for the manager to exit: %v\n%s", err, out)
	}
	raw, err := os.MkdirTemp("", "agentops-covdata-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(raw) }()
	if out, err := exec.CommandContext(ctx, lookPath("docker"), "cp", coverNode(c)+":"+coverDir+"/.", raw).CombinedOutput(); err != nil {
		return fmt.Errorf("copy the counters off the node: %v\n%s", err, out)
	}
	entries, err := os.ReadDir(raw)
	if err != nil || len(entries) == 0 {
		return fmt.Errorf("the manager wrote no coverage counters to %s: it was not built with -cover, or it did not exit cleanly", coverDir)
	}
	profile := filepath.Join(artifactDir, CoverProfile)
	if out, err := exec.CommandContext(ctx, lookPath("go"), "tool", "covdata", "textfmt", "-i="+raw, "-o="+profile).CombinedOutput(); err != nil {
		return fmt.Errorf("go tool covdata: %v\n%s", err, out)
	}
	return nil
}
