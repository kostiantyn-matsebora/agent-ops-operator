## Context

Three bundles exist today (`chart/charts/kubernetes`, `prometheus`,
`home-assistant`), each pairing a signal/MCP lane with wiring. See
proposal.md for why Pulse needs a fourth.

Two facts about Pulse (rcourtman/pulse), confirmed against its docs and
Dockerfile, drive every decision below:

- It sends outgoing webhooks only. There is no receiver-registration
  concept to call into — "registering" means this adapter calling
  *Pulse's* API to tell it where to post.
- Its own MCP adapter (`cmd/pulse-mcp`) is a stdio binary, launched
  locally by a client. It is not built into or shipped inside Pulse's own
  server Docker image (confirmed from the Dockerfile's build stages).

The installer runs Community edition. Pulse's Pro-gated
plan/approve/execute action-governance flow is out of reach, but scoped
API tokens and the `operator-state` write endpoints (maintenance windows,
criticality tags, `scope_only` policy) are not gated.

## Goals / Non-Goals

**Goals:**
- Let an agent see a firing Pulse alert, investigate it against live
  Pulse data, and record a safe operator annotation — nothing more.
- Keep the real privilege wall external to agentops's own CRDs, following
  the project's existing posture (`MCPConfig`/`MCPToolset` express reach
  and intent, never permission).
- Reuse every existing bundle mechanism that fits (signal adapter contract,
  risk-split toolsets, Pipeline/Coordinator dual wiring) rather than
  inventing new ones.

**Non-Goals:**
- Exposing Pulse's Pro-gated governed-action flow. Nothing in this bundle
  plans, approves or executes a Pulse action.
- Vendoring Pulse's own `pulse-mcp` stdio binary. See Decision 2.
- Generalizing registration to arbitrary, hand-authored Pulse
  SignalSources. See Decision 1.

## Decisions

### Decision 1: Registration is pod-start, env-driven — not SignalSource-driven

`signals/alertmanager` reads its sender-registration config from
`SignalSource.spec.config.register`, polled every 15s via
`GET /signal/sources?adapter=`, so any number of hand-authored sources can
each carry their own registration.

This bundle ships exactly one default source. Carrying that same
indirection would add a CR-mediation layer with nothing to mediate.

Registration config instead comes straight from the pod's own
environment, rendered there by the bundle chart: Pulse base URL, API
token, source name, optional HMAC signing secret.

**Alternative considered**: keep the SignalSource-driven path, just made
eager at startup instead of waiting for the first poll tick. Rejected —
it would still require threading registration config through
`SignalSource.spec.config` for a bundle that only ever renders one source,
buying generality nothing in this bundle uses.

**Trade-off accepted**: a second, hand-authored Pulse SignalSource outside
the bundle's own would never get registered, since nothing reads its
`spec.config`. Acceptable because the bundle contract is one default
source, same as `prometheus-bundle`.

The adapter still keeps a background retry loop. Not for CR polling, but
to retry registration itself on a fixed interval until it succeeds, and to
keep re-checking afterward so a rotated token or a transient Pulse outage
heals without a pod restart.

This is deliberately NOT a one-shot registration at startup only — see
Risks below for why a true one-shot was rejected.

### Decision 2: `mcp-pulse` is a first-party wrapper, not a vendored stdio binary

Pulse's own `pulse-mcp` is a stdio binary, meant to be launched locally by
whatever client is using it. Agentops's `MCPConfig` does support a `stdio`
transport (`command`/`args`, child process of the agent container), but no
bundle uses it today.

`pulse-mcp` is also not shipped inside Pulse's own server image.
Vendoring it means an initContainer that fetches and checksum-verifies a
separate GitHub Releases binary.

Instead, `platform/mcp-pulse/` is a new first-party component, shaped
exactly like `platform/mcp-aops`: stateless, streamable HTTP, forwards
whatever bearer token the caller's `MCPConfig` injected, hardcodes a small
set of tool-to-REST-endpoint mappings.

**Alternative considered**: vendor `pulse-mcp` via an initContainer fetch
plus checksum verify, using `type: stdio`. Rejected for this change — it
would be the first use of stdio transport in any bundle, a new
provisioning mechanism.

It would also inherit Pulse's own tool surface and versioning, rather
than a surface this bundle controls and can scope precisely to
Community-tier, two-profile needs. Worth reconsidering if a future
vendor's MCP surface is large enough that reimplementing it is the worse
trade.

**Trade-off accepted**: this repository now owns and maintains a Pulse
REST-to-MCP mapping, rather than inheriting Pulse's own tool manifest. The
mapping is small (inventory/metrics/alerts/storage reads, plus two
operator-state writes), so the maintenance cost is bounded.

### Decision 3: Two MCPConfigs, two tokens, one server

`MCPConfig` and `MCPToolset` never express permission in this codebase —
see `wiring.md`. The Kubernetes bundle's real wall is its MCP server's own
ServiceAccount RBAC. Home Assistant has no scoped tokens, so its two
profiles share one credential and its split is toolset-only (advisory).

Pulse's API tokens ARE scope-able (`monitoring:read` vs.
`monitoring:read`+`monitoring:write`). This bundle uses that: two
`MCPConfig`s, `pulse-observer-mcp` and `pulse-operator-mcp`, both pointing
at the one deployed `mcp-pulse` Service, each with a `headers.valueFrom`
pointing at its own Secret-backed, separately-scoped token.

The toolset split sits on top of that wall, not instead of it —
`pulse-observability` enumerates read patterns, `pulse-admin` adds
enumerated write patterns. Same relationship the Kubernetes bundle
already has between its server-identity wall and its two toolsets.

**Why not one MCPConfig, two toolsets alone** (the Home Assistant shape):
the toolset allowlist is enforced by the runtime deciding which tool
calls it will even emit. That enforcement differs across the three
runtimes — Claude's `--allowedTools`, ollama's once-applied gate,
copilot's per-call vocabulary check.

It governs a cooperating agent's declared intent, not what an
uncooperative one could still reach. Alert text is attacker-reachable
content, so a prompt-injected agent is a real case, not a hypothetical
one.

Pulse tokens make a real wall nearly free — one more Secret per route,
which the chart needs anyway. Relying on the toolset alone would give up
a wall this vendor, unlike Home Assistant, actually supports.

## Risks / Trade-offs

- **[Risk]** A true one-shot startup registration could leave a pod
  permanently unregistered if Pulse happens to be down exactly at pod
  start, with no later signal to retry.
  **[Mitigation]** The adapter keeps a background retry loop past startup,
  not just a startup attempt, reporting a degraded Ready reason throughout
  and healing automatically once Pulse answers.

- **[Risk]** Pulse's Generic webhook needs a caller-supplied payload
  template. The registered template and the inbound parser must agree, and
  the chart doesn't enforce that at render time.
  **[Mitigation]** One Go module owns both halves (never independently
  values-configurable), with a pinned test fixture asserting they agree —
  this is a code-level guarantee, not a render-time check, same posture
  `signals/alertmanager` already takes with its own normalize/register
  pairing.

- **[Risk]** `mcp-pulse` is new, first-party code with its own
  REST-to-MCP mapping to maintain, unlike the third-party images every
  prior bundle's MCP server component references.
  **[Mitigation]** The mapping is intentionally small and scoped to what
  the two profiles need (not a general Pulse API client), bounding the
  ongoing surface.

- **[Risk]** A future Pulse API version could change its scope names or
  its webhook template mechanism, silently breaking either the
  registration template/parser pair or the tool mapping.
  **[Mitigation]** Out of scope for this change to solve generally — noted
  here so a version bump of the Pulse backend is a known place to check,
  same as the Prometheus bundle's own pinned-tag note for its third-party
  MCP server image.

## Migration Plan

New bundle, additive only. No existing chart values, CRDs or specs change
shape. An install adopts it by setting `pulse.enabled: true` and supplying
Pulse's URL plus at least one profile's token — same adoption shape as
`home-assistant` or `prometheus`. No rollback concern beyond disabling the
bundle again.
