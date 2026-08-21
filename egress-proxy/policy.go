package main

import (
	"strings"
	"sync"
)

// policy is the access decision this proxy enforces, and where it came from.
//
// THE DECISION ARRIVES ON THE WORK UNIT. The proxy holds no Kubernetes
// credential and is told nothing at startup beyond which endpoints to enforce
// on. It learns what is granted by reading the work contract it is already
// forwarding — see design decision D3.
//
// Three properties follow, and each was a design goal rather than a
// convenience:
//
//   - No credential in the pod can be stolen to learn or widen the policy.
//   - What the runtime is CONFIGURED with and what the proxy ENFORCES come from
//     one message, so they cannot drift.
//   - A toolset edited mid-conversation takes effect on the next work unit,
//     which is the behaviour mcp-toolset-model already promises.
//
// Before the first work unit there is no decision, and the closed state is the
// answer: an agent that has been given no work has been granted nothing.
type policy struct {
	mu      sync.RWMutex
	known   bool
	allowed []string
}

func newPolicy() *policy { return &policy{} }

// set records the access decision carried by a work unit.
func (p *policy) set(tools []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.known = true
	p.allowed = append([]string(nil), tools...)
}

// permits reports whether a qualified tool name is granted.
//
// An unknown policy denies. That is the fail-closed rule stated once: a proxy
// that has not yet seen a work unit must not pass a tool call, because "we do
// not know yet" and "it is allowed" are the same answer to an attacker and
// opposite answers to an operator.
func (p *policy) permits(qualified string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.known {
		return false
	}
	for _, pat := range p.allowed {
		if matchTool(pat, qualified) {
			return true
		}
	}
	return false
}

// ready reports whether any decision has been seen, so a refusal can say WHICH
// refusal it is. "Not granted" and "nothing granted yet" are the same denial
// and very different bug reports.
func (p *policy) ready() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.known
}

// qualify composes the name the wiring uses from the name the server uses.
//
// An MCP server names its tools locally ("pods_exec"). A toolset names them as
// the runtime sees them ("mcp__kubernetes__pods_exec"). Enforcement happens
// against the WIRING's vocabulary, so the local name is composed up rather than
// the patterns being taken apart — patterns are opaque by contract, and
// decomposing them here would make this component the second place that knows
// what a toolset pattern means.
func qualify(serverKey, tool string) string {
	if serverKey == "" {
		return tool
	}
	return "mcp__" + serverKey + "__" + tool
}

// matchTool applies one allowlist pattern to one qualified tool name.
//
// MCP tools are named `mcp__<server>__<tool>`, so `mcp__kubernetes__*` means
// every tool the server registered under the key `kubernetes` — which is the
// established convention, not a rule invented here. A literal matches exactly,
// a trailing `*` matches any suffix.
//
// This is the FIRST place in the repo that reads a pattern rather than passing
// it through. The manager still does not: it concatenates and dedups, and the
// CRD needs no resolution status. Opacity moved, it did not disappear.
func matchTool(pattern, name string) bool {
	if pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(name, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == name
}
