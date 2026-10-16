// mcp-pulse: a first-party MCP server fronting Pulse's (rcourtman/pulse)
// REST API (design decision 2, pulse-integration-bundle). It holds NO Pulse
// credential of its own and makes NO scope decision — every call forwards
// whatever bearer token the caller's bound MCPConfig header supplied,
// unchanged, to the matching Pulse REST endpoint, and relays Pulse's own
// response or refusal verbatim. Two risk-split MCPConfig/MCPToolset pairs
// (pulse-observer-mcp/pulse-observability, pulse-operator-mcp/pulse-admin)
// point at this one server — the wall is which pre-scoped token each pair's
// Secret carries, not anything this server decides.
//
// Streamable HTTP, one endpoint, stateless: every tool call completes within
// its own request, so there is no server-side session to keep between calls.
//
// Tools: get_resources, get_metrics, get_alerts, get_storage_history (read),
// plus set_maintenance_window, set_criticality_tag (the two Community-tier
// operator-state writes) — see tools.go for the REST mapping each uses.
//
// A caller declares itself by the one header its MCPConfig entry sets from
// its OWN env (never a tool argument a model could pick on its own behalf):
//
//	Authorization: Bearer ${PULSE_TOKEN}   (the pre-scoped Pulse API token)
//
// The server reads it and forwards it unchanged to Pulse.
//
// Environment: PULSE_URL, LISTEN_ADDR (default ":8080").
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

// baseContext supplies the parent for main's signal-derived context — a test
// swaps it to inject a context it can cancel directly, so it can call the
// real main() in-process and stop it exactly as a real SIGTERM would.
var baseContext = context.Background

func main() {
	listen := os.Getenv("LISTEN_ADDR")
	if listen == "" {
		listen = ":8080"
	}
	srv := newServer(mustEnv("PULSE_URL"))

	ctx, stop := signal.NotifyContext(baseContext(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("mcp-pulse starting (listen=%s, pulse=%s)", listen, srv.client.baseURL)

	httpSrv := &http.Server{Addr: listen, Handler: srv.handler()}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("mcp-pulse server: %v", err)
	}
}
