package main

import (
	"strings"
	"sync/atomic"
)

// Self-exclusion: agent-ops never ingests its own machinery as a signal.
//
// This is the signal-lane twin of the no-relay-loops rule for channels — the
// system must never re-ingest its own output as input. The loop this adapter
// can produce is short and complete: the ops agent calls a Home Assistant
// service, the call fails, Home Assistant logs the failure, the log record
// becomes a signal, the signal wakes the ops agent, which calls the service
// again. Nothing downstream breaks it — every occurrence is a real error, so a
// correct re-check confirms it, and the manager's cooldown only slows the
// cycle down.
//
// agent-ops' own health is STATUS, not SIGNAL.
//
// Three INDEPENDENT mechanisms, because any one of them can be blind:
//
//	1. marker        — the record names agent-ops. Needs no read, so it holds
//	                   before any session has authenticated.
//	2. agent surface — the record comes from an API surface agent-ops reaches
//	                   Home Assistant through. Needs no read either, and it is
//	                   the one that actually breaks the loop above.
//	3. own user      — the record names the Home Assistant user backing this
//	                   adapter's token. Precise, but it needs one read
//	                   (auth/current_user) and is simply skipped until that
//	                   read has succeeded.
//
// NONE of the three is configurable, which is a deliberate difference from the
// cluster-events adapter. There, the third mechanism is "everything in the
// operator's own namespace" — coarse enough that an install co-locating its own
// workloads needs a way out. Here every mechanism is narrow already, and an
// editable loop breaker is not a loop breaker.

// ownMarkers appear in the logger, the source location or the text of a record
// that is about agent-ops itself — a custom integration, a script or a
// automation someone named after it.
var ownMarkers = []string{"agentops", "agent-ops", "agent_ops"}

// agentSurfaceLoggers are the Home Assistant components agent-ops reaches this
// instance THROUGH. An error logged by one of them is, by construction, an
// error about a call an agent made: reporting it wakes the agent that made it.
//
// Deliberately narrow. `conversation` is NOT here — that is Home Assistant's own
// voice assistant, used by people, and silencing it would cost real errors to
// buy nothing. What this set does not catch is an agent whose TOKEN is
// rejected: `http.ban` logs that against an IP address, with no user to
// attribute it to, and silencing intrusion attempts to cover that case is the
// worse trade.
var agentSurfaceLoggers = []string{
	"homeassistant.components.mcp_server",
	"homeassistant.components.api",
	"homeassistant.components.websocket_api",
}

// selfExcluder decides whether a log record is about agent-ops.
//
// The own-user half is an atomic pointer because it is learned on a session
// that reconnects, while records are evaluated on another goroutine.
type selfExcluder struct {
	user atomic.Pointer[currentUser]
}

func newSelfExcluder() *selfExcluder { return &selfExcluder{} }

// learnUser records who this adapter's token is, enabling mechanism 3. Called
// after each successful connect; a failed read leaves the mechanism off, which
// is why it is not the only one.
func (s *selfExcluder) learnUser(u currentUser) {
	if s == nil || (u.ID == "" && u.Name == "") {
		return
	}
	s.user.Store(&u)
}

// Excludes reports whether this record is about agent-ops' own machinery, and
// why.
//
// A NIL receiver still applies mechanisms 1 and 2. That is deliberate: the
// failure this invariant guards against must not be reachable by forgetting to
// wire the excluder up, so an unconfigured excluder degrades to the rules that
// need no configuration rather than to no rule at all.
func (s *selfExcluder) Excludes(rec *logRecord) (bool, string) {
	// 1. marker — no read, holds with a cold session
	haystack := strings.ToLower(rec.Name + " " + rec.Location() + " " + rec.Text())
	for _, m := range ownMarkers {
		if strings.Contains(haystack, m) {
			return true, "record names agent-ops (" + m + ")"
		}
	}

	// 2. agent surface — no read either, and the one that breaks the loop
	logger := strings.ToLower(rec.Name)
	for _, l := range agentSurfaceLoggers {
		if logger == l || strings.HasPrefix(logger, l+".") {
			return true, "record comes from an API surface agent-ops calls Home Assistant through (" + l + ")"
		}
	}
	if s == nil {
		return false, ""
	}

	// 3. own user — precise, needs the auth/current_user read
	if u := s.user.Load(); u != nil {
		for _, needle := range []string{u.ID, u.Name} {
			if needle == "" {
				continue
			}
			if strings.Contains(haystack, strings.ToLower(needle)) {
				return true, "record names this adapter's own Home Assistant user"
			}
		}
	}
	return false, ""
}
