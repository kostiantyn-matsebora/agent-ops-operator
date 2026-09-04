// signal-cron: the reference signal adapter. Serves SignalSources with
// spec.adapter=cron against the operator's signal contract — fires the
// configured input on a schedule as a job-lane signal. No Kubernetes access;
// the same bring-your-own pattern applies to any signal transport (PagerDuty,
// email, k8s events, …).
//
// Channel of proof for the contract:
//
//	config:   {"schedule": "0 6 * * *", "input": "...", "title": "..."}   (opaque spec.config)
//	cursor:   state key "last-fire" via /signal/state (restart-safe)
//	firing:   POST /signal/inbound, kind=job, fingerprint "<source>@<tick>"
//
// The deterministic fingerprint makes at-least-once delivery idempotent (the
// manager's cooldown absorbs a re-fired tick), and the stable labels give
// every source its own signature under the default signatureLabels — so
// consecutive runs land in ONE conversation, later ticks resuming the agent
// session (a recurring job that remembers). Missed ticks during downtime fire
// at most once (the latest missed tick) on recovery; ticks from before a
// source was first seen never fire.
//
// Run single-instance (the SignalAdapter reconciler's singleton default) —
// two schedulers would double-fire ticks.
//
// Environment: MANAGER_URL, ADAPTER_TOKEN, ADAPTER_NAME (default "cron").
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// sourceConfig is this adapter's interpretation of SignalSource spec.config.
type sourceConfig struct {
	Schedule string `json:"schedule"`
	Input    string `json:"input"`
	// Title for the conversation (optional; the manager derives one otherwise).
	Title string `json:"title,omitempty"`
}

type servedSource struct {
	sched *Schedule
	cfg   sourceConfig
}

type adapter struct {
	mgr        *Manager
	sourceType string

	mu       sync.Mutex
	sources  map[string]servedSource
	reported map[string]string // last status per source (avoid spam)
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required env %s", key)
	}
	return v
}

// baseContext supplies the parent for main's signal-derived context. A test
// swaps it to inject a context it can cancel directly, so it can call the
// real main() in-process and stop it exactly as a real SIGTERM would —
// without sending any OS signal that could also reach other tests sharing
// this process.
var baseContext = context.Background

func main() {
	sourceType := os.Getenv("ADAPTER_NAME")
	if sourceType == "" {
		sourceType = "cron"
	}
	a := &adapter{
		mgr:        NewManager(mustEnv("MANAGER_URL"), mustEnv("ADAPTER_TOKEN")),
		sourceType: sourceType,
		sources:    map[string]servedSource{},
		reported:   map[string]string{},
	}
	ctx, stop := signal.NotifyContext(baseContext(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("signal-cron adapter starting (adapter=%s)", sourceType)

	for ctx.Err() == nil {
		a.refreshSources(ctx)
		a.mu.Lock()
		names := make([]string, 0, len(a.sources))
		for name := range a.sources {
			names = append(names, name)
		}
		a.mu.Unlock()
		for _, name := range names {
			if err := a.evaluate(ctx, name, time.Now()); err != nil && ctx.Err() == nil {
				log.Printf("evaluate %s: %v", name, err)
			}
		}
		sleepCtx(ctx, 15*time.Second)
	}
}

// refreshSources re-reads served sources, validates config, and reports
// validity changes as the source's Ready condition.
func (a *adapter) refreshSources(ctx context.Context) {
	infos, err := a.mgr.Sources(ctx, a.sourceType)
	if err != nil {
		log.Printf("list sources: %v", err)
		return
	}
	next := map[string]servedSource{}
	for _, info := range infos {
		var cfg sourceConfig
		var sched *Schedule
		problem := ""
		if len(info.Config) == 0 {
			problem = "spec.config is missing"
		} else if err := json.Unmarshal(info.Config, &cfg); err != nil {
			problem = "spec.config is not valid JSON for the cron adapter: " + err.Error()
		} else if cfg.Input == "" {
			problem = "spec.config.input is required"
		} else if sched, err = ParseCron(cfg.Schedule); err != nil {
			problem = "spec.config.schedule: " + err.Error()
		}
		a.mu.Lock()
		last := a.reported[info.Name]
		a.mu.Unlock()
		if problem != "" {
			if last != problem {
				_ = a.mgr.ReportStatus(ctx, info.Name, false, "InvalidConfig", problem)
				a.mu.Lock()
				a.reported[info.Name] = problem
				a.mu.Unlock()
			}
			continue // keep serving the other sources
		}
		if last != "ok" {
			_ = a.mgr.ReportStatus(ctx, info.Name, true, "AdapterReady", "served by signal-cron")
			a.mu.Lock()
			a.reported[info.Name] = "ok"
			a.mu.Unlock()
		}
		next[info.Name] = servedSource{sched: sched, cfg: cfg}
	}
	a.mu.Lock()
	a.sources = next
	a.mu.Unlock()
}

// evaluate fires the latest elapsed tick of one source, if any. Fire-then-
// persist: a crash between the two re-fires the same fingerprint, which the
// manager's cooldown absorbs.
func (a *adapter) evaluate(ctx context.Context, name string, now time.Time) error {
	a.mu.Lock()
	src, ok := a.sources[name]
	a.mu.Unlock()
	if !ok {
		return nil
	}
	raw, err := a.mgr.GetState(ctx, name, "last-fire")
	if err != nil {
		return err
	}
	if raw == "" {
		// first sight: never fire ticks from before the source existed
		return a.mgr.PutState(ctx, name, "last-fire", now.UTC().Format(time.RFC3339))
	}
	last, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		log.Printf("resetting unparsable last-fire cursor for %s: %q", name, raw)
		return a.mgr.PutState(ctx, name, "last-fire", now.UTC().Format(time.RFC3339))
	}

	// latest scheduled tick in (last, now] — at most one fire per evaluation
	var latest time.Time
	for t := src.sched.Next(last); !t.IsZero() && !t.After(now); t = src.sched.Next(t) {
		latest = t
	}
	if latest.IsZero() {
		return nil
	}

	tick := latest.UTC().Format(time.RFC3339)
	err = a.mgr.Inbound(ctx, name, []Signal{{
		Fingerprint: name + "@" + tick,
		// distinct default-signature labels per source, so different cron
		// sources never group into one conversation; "source" supports custom
		// grouping.signatureLabels
		Labels:  map[string]string{"alertgroup": "cron", "alertname": name, "source": name},
		Title:   src.cfg.Title,
		Payload: src.cfg.Input,
		Kind:    "job",
	}})
	if err != nil {
		return err
	}
	log.Printf("fired %s tick %s", name, tick)
	return a.mgr.PutState(ctx, name, "last-fire", tick)
}

func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
