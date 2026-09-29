// mcp-aops: the aops MCP server (design coordinated-agents D-F,
// aops-mcp-server). A THIN CLIENT of the manager's /coordinate/* surface —
// it decides no reach itself, forwarding every caller's bearer token and
// declared identity exactly as presented and letting the manager refuse
// whatever it must.
//
// Streamable HTTP, one endpoint, stateless: every tool call completes within
// its own request, so there is no server-side session to keep between calls.
//
// Tools: list_agents, list_conversations, get_conversation, get_tree,
// invoke, close, escalate, read — the eight names design D-F lists.
//
// A caller declares itself by two headers the runtime pod's own MCPConfig
// entry sets from its OWN env, never from a tool argument a model could pick
// on its own behalf:
//
//	Authorization: Bearer ${AOPS_MCP_TOKEN}     (proves the identity below)
//	X-Aops-Conversation: ${CONVO_ID}            (a coordinated conversation)
//	X-Aops-Channel: ${CHANNEL_NAME}             (a channel-reader, e.g. the voice lane)
//
// The server reads both, forwards them to the manager under the manager's
// own field names, and returns whatever the manager decided — 401/403 folded
// into a tool error, never surfaced as a transport failure a client retries
// blindly.
//
// Environment: MANAGER_URL, LISTEN_ADDR (default ":8080").
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
	srv := newServer(mustEnv("MANAGER_URL"))

	ctx, stop := signal.NotifyContext(baseContext(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("mcp-aops starting (listen=%s, manager=%s)", listen, srv.client.baseURL)

	httpSrv := &http.Server{Addr: listen, Handler: srv.handler()}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("mcp-aops server: %v", err)
	}
}
