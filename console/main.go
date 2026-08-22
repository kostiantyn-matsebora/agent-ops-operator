// console: the agent-ops console — a channel adapter that is also the viewer,
// and (when granted a signal identity) an originator.
//
// It fans in five sources the browser must never touch directly:
//
//	config:   Kubernetes list/watch, agentops.dev kinds (own SA, read-only)
//	install:  Kubernetes list/watch, deployments + pods (images, readiness)
//	traffic:  one SSE connection to the manager's GET /activity/stream
//	outbound: long-poll GET /channel/ops?adapter=console  ->  live transcripts
//	inbound:  a message typed in the UI                   ->  POST /channel/inbound
//	start:    "new conversation" -> POST /signal/inbound   (chat signal)
//
// The console is IN the system it shows: conversations on pipelines listing its
// Channel bind a console thread, so watching a run and replying to it are the
// same screen, and a conversation the console STARTS arrives already joined —
// the claiming pipeline's channel set includes the console Channel.
//
// There is NO write path to the Kubernetes API in this module at all. The only
// writes anywhere are POST /channel/inbound and POST /signal/inbound, both
// through the manager, both gated by WRITE_ENABLED and an identity.
//
// Environment: see config.go.
package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("configuration: %v", err)
	}

	kube, err := NewInClusterKube(cfg.Namespace)
	if err != nil {
		// Without API access the console has nothing to show — failing at
		// startup names the missing grant instead of serving empty pages.
		log.Fatalf("kubernetes access: %v (ChannelAdapter needs kubernetesAccess: true and a chart-granted read-only Role)", err)
	}

	cache := NewCache(kube, append(append([]string{}, Kinds...), InstallKinds...))
	transcripts := NewTranscripts()
	mgr := NewManager(cfg.ManagerURL, cfg.AdapterToken)
	adapter := NewAdapter(mgr, cache, transcripts, cfg.AdapterName)
	activity := NewActivityWindow(mgr, 5000)
	originator := NewOriginator(cfg.ManagerURL, cfg.SignalAdapterToken, cfg.SignalSourceName)
	api := NewAPI(APIDeps{
		Cache: cache, Transcripts: transcripts, Adapter: adapter, Activity: activity,
		Manager: mgr, Originator: originator, Metrics: NewMetricsClient(cfg.MetricsURL), Config: cfg,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// The auth mode is logged because it is the one setting whose wrong value is
	// invisible from inside the UI: a console that authenticates nobody looks
	// exactly like one you are already signed in to. What is logged HERE is only
	// the process DEFAULT — a chart reaches this pod through the served
	// Channel's config, which arrives after startup and wins. The adapter logs
	// the effective mode when it resolves one, so the two lines together say
	// where the answer came from.
	log.Printf("agent-ops console starting (adapter=%s, namespace=%s, listen=%s, authDefault=%s, writes=%v, originates=%v)",
		cfg.AdapterName, cfg.Namespace, cfg.ListenAddr, authMode(cfg.AuthEnabled, cfg.ExternalAuthenticator),
		cfg.WriteEnabled, originator != nil)

	go cache.Run(ctx)
	go adapter.Run(ctx)
	// Renders what people typed. The manager does not post those to threads —
	// on a transport the sender already sees their own message — but a VIEWER
	// has to be told, or a conversation's transcript starts at the answer with
	// the question missing.
	go activity.Run(ctx)

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: api.Handler(UIHandler()), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("console server: %v", err)
	}
}
