## Why

Pulse (rcourtman/pulse) monitors Proxmox VE/PBS/PMG, Docker/Podman,
Kubernetes and TrueNAS. Its alerts and its own tooling are reachable only by
a human today.

The three existing bundles (Kubernetes, Prometheus, Home Assistant) cover
different monitoring surfaces. None of them lets an agent see a firing
Pulse alert, investigate it against live inventory and metrics, or record a
safe operator annotation.

## What Changes

- New `signals/pulse/` webhook-receiving SignalAdapter.
  - Pulse only sends outgoing webhooks. It has no receiver-registration
    concept of its own.
  - "Registration" is this adapter calling Pulse's own
    `POST /api/notifications/webhooks` (Generic service) at pod start. It
    points Pulse at the adapter's own `/webhook/<source>` endpoint.
  - One module owns both the Go-template body it registers and the parser
    that decodes it on receipt. The two cannot drift apart independently.
  - Registration config — Pulse URL, API token, source name, optional HMAC
    signing secret — comes straight from the bundle's own values, not from
    `SignalSource.spec.config`. This bundle ships exactly one default
    source, so `signals/alertmanager`'s generalized, SignalSource-driven
    registration path adds indirection this bundle has no use for.
- New `platform/mcp-pulse/` component: a first-party, stateless MCP server
  fronting Pulse's REST API.
  - Shaped like `platform/mcp-aops` — it forwards whatever bearer token the
    caller's own `MCPConfig` injected, and decides no reach itself.
  - Pulse ships its own MCP adapter (`cmd/pulse-mcp`), but it is a stdio
    binary meant to be launched by a local client. It is not shipped inside
    Pulse's own server image and is not a deployable network server.
  - This bundle writes its own networked (`http`/`sse`) wrapper instead of
    adding stdio-transport provisioning machinery this repo doesn't have
    yet.
- New `chart/charts/pulse/` bundle chart with a read/write profile split.
  - `pulse-observer` (AgentProfile + Pipeline/AgentCapability route):
    read-only investigation over inventory, metrics, alerts and storage
    history.
  - `pulse-operator` (separate AgentProfile + route): adds Pulse's
    Community-tier-safe `operator-state` writes — maintenance windows,
    criticality tags. Pulse's Pro-gated plan/approve/execute
    action-governance flow stays out of reach on Community edition and
    neither profile exposes it.
  - Each route binds its own `MCPConfig`/`MCPToolset` pair. Both
    `MCPConfig`s point at the same `mcp-pulse` server, but each carries a
    different, separately-scoped Pulse API token
    (`monitoring:read` vs. `monitoring:read`+`monitoring:write`).
  - `MCPConfig`/`MCPToolset` never express permission themselves. The real
    wall is which pre-scoped token string reaches which route's Secret —
    the same two-walls shape the Kubernetes bundle already uses, adapted to
    per-caller token forwarding.
  - No substrate rendered: no `AgentRuntime`, no floor SA, no credential
    infra. The parent chart's `global.agentops.runtimeDefaults` +
    `runtimes:` own that, as with every other bundle.

## Capabilities

### New Capabilities
- `pulse-signal-adapter`: the webhook-receiving SignalAdapter. Self
  registration against Pulse's notification-webhooks API at pod start, the
  owned template/parser pairing, HMAC signature verification,
  fingerprinting and normalization into the signal contract.
- `pulse-mcp-tooling`: the first-party `mcp-pulse` server and its two
  `MCPConfig`/`MCPToolset` pairs. The per-caller token-forwarding design and
  the tool-to-Pulse-REST-endpoint mapping for both the observer and
  operator surfaces.
- `pulse-bundle`: the `chart/charts/pulse/` chart itself. Values shape, the
  two profiles, the Pipeline-mode and Coordinator-mode wiring components,
  and what the bundle does and does not render.

### Modified Capabilities

None. This change adds new instances of the existing SignalAdapter,
MCPConfig, MCPToolset, Pipeline, AgentCapability and AgentProfile contracts.
It changes none of their shapes.

## Impact

**Code**: `signals/pulse/` (new Go module), `platform/mcp-pulse/` (new Go
module), `chart/charts/pulse/` (new subchart). `chart/crds/` is unchanged —
no new CRD fields. `.github/components.sh` auto-discovers both new modules
and images.

**Reference docs**: `docs/integrations/pulse.md` (new page),
`docs/CHANGELOG.md` (new bundle entry), `docs/configuration.md` (new
`<!-- generated: renders bundle=pulse -->` block via `docs-generate.py`).
`docs/concepts.md` and `docs/contracts.md` are untouched. No CRD field or
generic contract changes.

**Adopter site**:

- `docs/_data/nav.yml`: new Integrations entry.
- `README.md`: the integration logo strip and the integrations/runtimes
  table both name Kubernetes, Prometheus, Home Assistant and Telegram
  today. Both need a Pulse entry.
- `docs/getting-started.md` and `docs/installation.md` are untouched. Pulse
  is an opt-in bundle like the other three, not part of the read-only demo
  path.
