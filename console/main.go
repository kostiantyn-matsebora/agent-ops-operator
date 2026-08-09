// console: the agent-ops console — a channel adapter that is also the viewer.
//
// It serves Channels with spec.adapter=console against the operator's adapter
// contract, and renders the whole agentops.dev configuration as a topology
// graph with live conversation activity:
//
//	config:   Kubernetes list/watch (own SA, read-only, chart-granted RBAC)
//	outbound: long-poll GET /channel/ops?adapter=console  ->  live transcripts
//	inbound:  a message typed in the UI                   ->  POST /channel/inbound
//	browser:  embedded SPA + JSON snapshots + one SSE stream
//
// The console is IN the system it shows: conversations on pipelines listing its
// Channel bind a console thread, so watching a run and replying to it are the
// same screen. Joining a pipeline is an ordinary `channels[]` edit made by a
// human — the console never mutates a CR (there is no write path to the
// Kubernetes API in this module at all).
//
// Environment: MANAGER_URL, ADAPTER_TOKEN, ADAPTER_NAME (default "console"),
// LISTEN_ADDR (default ":8080" — in-cluster the reconciler injects it from
// ChannelAdapter.spec.port), POD_NAMESPACE (injected by kubernetesAccess),
// UI_TOKEN (fallback browser token for channels declaring no credentials),
// projected AGENTOPS_CRED_* vars (key uiToken).
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required env %s", key)
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	adapterName := envOr("ADAPTER_NAME", "console")
	listen := envOr("LISTEN_ADDR", ":8080")
	namespace := envOr("POD_NAMESPACE", "")

	kube, err := NewInClusterKube(namespace)
	if err != nil {
		// Without API access the console has nothing to show — failing at
		// startup names the missing grant instead of serving empty pages.
		log.Fatalf("kubernetes access: %v (ChannelAdapter needs kubernetesAccess: true and a chart-granted read-only Role)", err)
	}
	if namespace == "" {
		log.Fatalf("missing required env POD_NAMESPACE (injected by ChannelAdapter spec.kubernetesAccess)")
	}

	cache := NewCache(kube, Kinds)
	transcripts := NewTranscripts()
	mgr := NewManager(mustEnv("MANAGER_URL"), mustEnv("ADAPTER_TOKEN"))
	adapter := NewAdapter(mgr, cache, transcripts, adapterName)
	api := NewAPI(cache, transcripts, adapter, os.Getenv("UI_TOKEN"))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("agent-ops console starting (adapter=%s, namespace=%s, listen=%s)", adapterName, namespace, listen)

	go cache.Run(ctx)
	go adapter.Run(ctx)

	srv := &http.Server{Addr: listen, Handler: api.Handler(UIHandler()), ReadHeaderTimeout: 10 * time.Second}
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
