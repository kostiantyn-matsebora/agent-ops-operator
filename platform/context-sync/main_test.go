package main

import (
	"os"
	"testing"
	"time"
)

func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func baseEnv() map[string]string {
	return map[string]string{
		envUpstream: "http://manager:8080",
		envStore:    "/data/context/conv-1",
		envPaths:    ".claude/projects/-data-workspace/**",
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	setEnv(t, baseEnv())
	c, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.interval != 2*time.Minute {
		t.Fatalf("interval = %v, want the 2m default", c.interval)
	}
	if c.retain != 3 {
		t.Fatalf("retain = %d, want 3", c.retain)
	}
	if c.live != "/data/context" {
		t.Fatalf("live = %q, want /data/context", c.live)
	}
}

// An empty include list would persist NOTHING while looking configured — the
// exact silent failure this whole change exists to end.
func TestLoadConfigRefusesAnEmptyIncludeList(t *testing.T) {
	env := baseEnv()
	env[envPaths] = "   \n  \n"
	setEnv(t, env)
	if _, err := loadConfig(); err == nil {
		t.Fatal("an empty include list must be refused, not silently accepted")
	}
}

func TestLoadConfigRequiresItsEndpoints(t *testing.T) {
	for _, missing := range []string{envUpstream, envStore} {
		env := baseEnv()
		delete(env, missing)
		// t.Setenv cannot unset, so clear explicitly.
		t.Run(missing, func(t *testing.T) {
			for k, v := range env {
				t.Setenv(k, v)
			}
			t.Setenv(missing, "")
			if _, err := loadConfig(); err == nil {
				t.Fatalf("%s missing must be an error", missing)
			}
		})
	}
}

// Zero is a legitimate setting, not a disabled config: it means work-boundary
// checkpoints only, which is right for a low-churn backend.
func TestZeroIntervalMeansWorkBoundariesOnly(t *testing.T) {
	env := baseEnv()
	env[envInterval] = "0s"
	setEnv(t, env)
	c, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.interval != 0 {
		t.Fatalf("interval = %v, want 0", c.interval)
	}
}

func TestLoadConfigRejectsNonsense(t *testing.T) {
	env := baseEnv()
	env[envInterval] = "soon"
	setEnv(t, env)
	if _, err := loadConfig(); err == nil {
		t.Fatal("an unparseable interval must fail loudly rather than fall back silently")
	}

	env2 := baseEnv()
	env2[envRetain] = "0"
	setEnv(t, env2)
	t.Setenv(envInterval, "")
	if _, err := loadConfig(); err == nil {
		t.Fatal("retain must be at least 1: keeping zero generations keeps no context")
	}
}

func TestLinesIgnoresBlanks(t *testing.T) {
	got := lines("a\n\n  b  \n\n")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("lines = %q", got)
	}
}

// A skip must not update the "last wrote" clock, or the debounce would suppress
// real checkpoints after a run of skips.
func TestSkipDoesNotAdvanceTheWriteClock(t *testing.T) {
	live, store := t.TempDir(), t.TempDir()
	write(t, live, "a.jsonl", "one")
	s := &syncer{
		cfg:      config{live: live, paths: []string{"**"}},
		store:    &Store{Root: store, Retain: 2},
		reporter: &reporter{},
	}
	s.checkpoint("first")
	first := s.lastAt
	if first.IsZero() {
		t.Fatal("a real checkpoint must stamp the clock")
	}
	s.checkpoint("second") // nothing changed: a skip
	if !s.lastAt.Equal(first) {
		t.Fatal("a skip must not advance the write clock")
	}
	if _, err := os.Stat(store); err != nil {
		t.Fatal(err)
	}
}
