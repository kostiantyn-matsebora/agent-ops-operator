## Context

See `proposal.md` for motivation and `docs/adr/0001-bound-component-reach.md` for
the decision and the options weighed.

Constraints that shape the approach:

- Runtime pods already run non-root at a fixed uid, pod-level, and already carry
  an opt-in native sidecar (`context-sync`) that learns work boundaries by
  PROXYING the work contract. Both are load-bearing here.
- The manager reads no Secrets and adapters hold no RBAC. A new component that
  needed either would be the wrong shape for this codebase.
- MCP endpoints the chart ships are plain HTTP inside the cluster.
- The agent controls its own repository checkout, which matters for what may be
  trusted as an input to enforcement.

## Goals / Non-Goals

**Goals**

- Enforce the wiring's MCP access against an agent that does not cooperate.
- Leave every non-MCP flow byte-identical, with no new trust anchor.
- Keep the enforcing component credential-free with respect to Kubernetes.
- Ship network restriction for all components as one install decision.

**Non-Goals**

- TLS interception of any kind.
- Governing tool ARGUMENTS. This decides who may call what.
- stdio MCP servers. They are child processes of the agent container.
- Egress restriction at the network level. Interception covers the agent, and
  the components' egress is a separate question.
- Authentication for the surfaces that lack it. Named in the proposal as owed.

## Decisions

### D1 — Interception by redirect at pod start, distinguished by process identity

An init container installs a redirect of the agent container's outbound TCP to
the proxy's port, excluding traffic whose source identity is the proxy's own so
forwarded traffic is not re-intercepted. The proxy recovers the caller's intended
destination from the connection itself.

This is the shape service meshes use, for the reason they use it: it holds for
destinations the agent chooses, not only destinations we configured.

*Alternatives*: proxy environment variables or a rewritten `mcp.json` — both are
configuration the agent can ignore, which fails the spec's bypass requirement.
CNI-level redirection — assumes infrastructure the chart cannot require.

### D2 — Fail-closed is the default state, not an added check

The redirect exists before the agent container starts. Until the proxy is
serving, redirected connections are refused by the kernel. Nothing has to detect
the not-ready case, and there is no window in which traffic escapes unmediated.

### D3 — The proxy learns the access decision from the work unit in flight

All agent egress is intercepted, including the work contract. The proxy reads the
tool access off the work unit as it passes, exactly as `context-sync` learns work
boundaries from the same stream.

This buys three properties at once. The proxy needs no Kubernetes access. What is
configured and what is enforced come from one source and cannot drift. And
toolset edits reach a running conversation on its next work unit, preserving the
behaviour `mcp-toolset-model` already specifies.

Before the first work unit, MCP is denied.

*Alternatives*: inject statically at pod creation — diverges the moment a toolset
is edited mid-conversation, which the existing spec explicitly promises against.
Query the manager — requires a credential and an authenticated endpoint that does
not exist.

### D4 — Enforcement uses the WIRING's contribution, not the composed allowlist

The final CLI allowlist is the wiring's tools composed with the agent
definition's `tools:` frontmatter, and that composition happens in the runtime
because only it holds the checkout.

**The proxy enforces the wiring's contribution alone.** The definition lives in a
repository the agent can write to, so treating it as an input to enforcement
would let the enforced set be edited by the party being enforced against. Asking
the runtime to report its composed allowlist has the same defect one step
removed.

Consequence to state in docs: an MCP tool granted ONLY by an agent definition and
not by any bound toolset is refused under mediation. That is consistent with
capabilities being wiring, exclusively. Built-in tools a definition adds are
unaffected, since they are not MCP traffic.

### D5 — Tool identity is normalised to the wiring's vocabulary

An MCP server names its tools locally (`pods_exec`). Toolset patterns name them
as the runtime sees them (`mcp__kubernetes__pods_exec`). The proxy knows which
server key a connection belongs to, so it composes the qualified name before
matching, using the same glob semantics the allowlist uses elsewhere.

Discovery is filtered with the same predicate that gates invocation, so the two
cannot disagree.

### D6 — Non-MCP traffic is forwarded as opaque bytes

A connection whose destination is not a bound MCP endpoint is copied through
without parsing. TLS is untouched end to end. Streaming is preserved rather than
buffered, because MCP itself uses long-lived event streams.

### D7 — An unenforceable MCP endpoint is reported, never silently passed

If a bound MCP endpoint cannot be enforced — an `https` URL, or a transport the
proxy does not parse — the conversation surfaces a condition saying so. The
failure mode this avoids is an install believing it is mediated while one server
is not.

### D8 — Network restriction is one decision, rendered per component

A single values decision turns on default-deny ingress plus an allow per wired
flow, keyed on labels the workloads already carry. Bundle-owned MCP servers are
rendered by their own subcharts, reading the decision from global scope, because
a subchart reads no other parent scope.

Installations can name additional permitted callers in values. Egress is not
restricted.

### D9 — The post-install warning names the gap and the check

The output states that these objects apply successfully on a cluster that does
not enforce them, names what stays reachable in that case, and gives the
operator a way to establish which they have. It does not call the components
protected.

### D10 — The proxy is the first component that READS a pattern, and it says so

There is no matcher to be in parity with: the manager concatenates and dedups
patterns and passes the string through, which is why the CRD needs no resolution
status. The proxy is the first place in the repo that interprets one.

The semantics are not invented here. MCP tools are named
`mcp__<server>__<tool>`, so `mcp__kubernetes__*` means every tool the server
registered under that key. A literal matches exactly, a trailing `*` matches any
suffix.

What the proxy adds is VISIBILITY, and it gets it for free: the agent calls
`tools/list` at session start, and the proxy is already filtering that response.
At that moment the server has just named every tool it registers, so the proxy
logs what the wiring resolved to — `14 of 20 granted` — and warns when a binding
grants nothing at all. A mistyped pattern presents as an agent that cannot do
anything, which is a long way from the toolset that caused it.

Opacity moves rather than disappearing, and the spec says so instead of leaving
the old sentence to be read as still true everywhere.

## Risks / Trade-offs

- **The pod holds a credential stronger than the agent's rights** → accepted in
  the ADR. Mitigated by keeping the proxy's reach to exactly one conversation's
  endpoints, so the credential is worth no more than the mediation it performs.
- **Privileged init container conflicts with `restricted` Pod Security
  admission** → the feature is opt-in, and the namespace requirement is stated
  where an operator decides to enable it, not discovered at first dispatch.
- **iptables vs nftables backends differ across distributions** → pin the
  interception image and its backend, and verify on a cluster of each kind before
  the feature is called done. This is a known source of silent no-op.
- **Dual-stack clusters** → interception must cover IPv6 or the agent reaches
  MCP over IPv6 unmediated. Treat single-family interception as a defect.
- **A future MCP transport the proxy cannot parse** → D7 turns it into a visible
  condition rather than silent pass-through.
- **Network restriction that is too tight breaks a flow at the worst moment** →
  every allowed flow is pinned by an integration expectation, and the feature is
  off by default.

## Migration Plan

Nothing migrates. Both halves are opt-in and absent by default, so an existing
install upgrades to identical behaviour.

Enabling mediation recreates runtime pods on their next conversation. Running
conversations keep the pod they have.

Rollback is disabling the value. There is no persisted state on either side.

## Open Questions

- Whether the interception init container ships as part of the proxy image or as
  a separate minimal image. Affects pull cost per conversation, not behaviour.
