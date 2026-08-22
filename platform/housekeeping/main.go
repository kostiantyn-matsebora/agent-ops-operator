// housekeeping reclaims what a deleted conversation leaves on disk: its
// workspace directory and its session transcripts.
//
// It exists as its own workload for one reason — THE MANAGER MOUNTS NO
// PERSISTENT VOLUME, by invariant, and mounting the claim ROOT is exactly the
// reach subPath isolation denies runtime pods. So the etcd half of the
// lifecycle (autoclose, autodelete) stays in the manager, and only the disk
// half lives here.
//
// It runs once and exits: a CronJob, not a daemon. It performs NO Kubernetes
// writes — it reads conversations to decide what is an orphan, and nothing else.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil && v >= 0 {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(strings.TrimSpace(os.Getenv(key))); err == nil && d > 0 {
		return d
	}
	return def
}

func main() {
	namespace := env("POD_NAMESPACE", "")
	if namespace == "" {
		log.Fatal("POD_NAMESPACE is required")
	}
	opts := Options{
		DryRun:       envBool("DRY_RUN", true), // safe by default; the chart decides
		MaxDeletions: envInt("MAX_DELETIONS", 50),
		SessionGrace: envDuration("SESSION_GRACE", 7*24*time.Hour),
	}
	workspaceRoot := env("WORKSPACE_ROOT", "")
	sessionsRoot := env("SESSIONS_ROOT", "")

	kube, err := NewInClusterKube(namespace)
	if err != nil {
		log.Fatalf("in-cluster client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	if opts.DryRun {
		log.Print("DRY RUN — nothing will be removed")
	}
	var failed bool
	for _, job := range []struct {
		name string
		root string
		run  func(context.Context, string, Lister, Options) (Report, error)
	}{
		{"workspaces", workspaceRoot, ReclaimWorkspaces},
		{"sessions", sessionsRoot, ReclaimSessions},
	} {
		if job.root == "" {
			log.Printf("%s: no root configured, skipping", job.name)
			continue
		}
		rep, err := job.run(ctx, job.root, kube, opts)
		if err != nil {
			// Report and carry on: one half failing must not stop the other,
			// and a partial reclaim is fine — an orphan missed now is reclaimed
			// on the next run.
			log.Printf("%s: %v", job.name, err)
			failed = true
		}
		log.Print(rep.String())
	}
	if failed {
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "housekeeping complete")
}
