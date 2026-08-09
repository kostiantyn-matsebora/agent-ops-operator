# Proposal: visualize-agent-ops

## Why

Today the only way to see what agent-ops is doing is `kubectl get`/`describe` across seven CRD kinds — there is no place to see the wiring (which sources feed which pipelines feed which channels), whether it is healthy, or what is running right now. A Kiali-style console that renders the configured topology and live conversation runs makes the operator observable at a glance — and by making the console itself a channel wired into pipelines, observation and participation become the same surface: you watch a run and reply to the agent from the same screen.

## What Changes

- Add an **agent-ops console**: a web UI showing (1) the current configuration — all agent-ops CRs with their spec, conditions, and health, (2) a **topology graph** of configured pipelines (SignalSources → Pipeline → AgentProfile → Channels, Kiali-style: nodes colored by Ready/Served/Wired conditions, edges from `Pipeline.spec` wiring), and (3) **live runs**: real-time view of Conversations per pipeline — phase, inflight work, recent runs, thread bindings — driven by Kubernetes watches on `Conversation.status`.
- The console is **also a channel adapter** (`ChannelAdapter` CR, e.g. name `console`), not just a viewer: it implements the `/channel/*` contract (ops long-poll + inbound push), so a `Channel` with `spec.adapter: console` can be listed in any Pipeline's `channels[]`. Conversations on pipelines that include the console channel bind a console thread, receive fan-out replies/acks and attributed relays from sibling channels, and accept user messages typed in the UI — the transcript view IS the channel.
- Console state comes from **read-only Kubernetes watches** on agentops.dev CRs (list/watch only, no writes except its own channel duty). RBAC is granted externally by the chart against the adapter's dedicated SA, per the existing adapter security posture — no reconciler creates or binds RBAC.
- **`ChannelAdapter` gains parity with `SignalAdapter` on two implementation properties**: `spec.kubernetesAccess` (mounts the SA token + injects `POD_NAMESPACE` so the console pod can reach the API server — identity only; what the SA may do is granted externally) and `spec.port` (the reconciler owns a Service `agentops-adapter-<name>` and injects `LISTEN_ADDR` — the console's browser-facing endpoint, so charts ship no adapter connectivity, same rule as signal adapters).
- New self-contained `console/` Go module (same pattern as `channel-telegram/`): dependency-free, raw Kubernetes REST list/watch over HTTP (as `signal-vmalertmanager` already does for self-registration), UI assets embedded via `go:embed`, browser-facing endpoints (topology snapshot, CR views, SSE for live updates and transcript streaming).
- Helm chart ships the console as an opt-in bundle: `ChannelAdapter` + `Channel` CRs, read-only RBAC (Role/RoleBinding for agentops.dev CRs) against SA `agentops-adapter-console`, Service, optional Ingress, and browser auth token via env (chart-provisioned Secret, `envFrom` — the manager still reads no Secrets).
- Wiring the console into pipelines stays the user's/chart's choice: the console channel is added to `Pipeline.spec.channels[]` like any other channel (channels are shareable, so one console channel can join every pipeline). No new wiring mechanism.

## Capabilities

### New Capabilities

- `console-adapter`: the console as a conforming channel adapter — `/channel/*` contract compliance, thread creation for console-bound conversations, rendering fan-out replies/acks/relays, submitting UI-typed messages as inbound, and the binding no-relay-loop rule (its own outbound posts are never re-ingested).
- `console-topology`: the configuration and topology surface — CR inventory views (spec + conditions for all agentops.dev kinds), the pipeline wiring graph, node health derived from Ready/Served/Wired conditions, built from read-only list/watch.
- `console-live-runs`: the real-time runs surface — per-pipeline Conversation activity (phase, inflight, runs history, thread bindings, last activity) streamed to the browser as `Conversation.status` changes; transcript view for console-bound conversations with send capability.
- `console-deployment`: Helm packaging — ChannelAdapter/Channel CRs, external read-only RBAC for the console SA, Service/Ingress, browser auth, and values gating (`console.enabled`).

### Modified Capabilities

- `channel-adapter-lifecycle`: `ChannelAdapter.spec` gains `kubernetesAccess` and `port` (parity with `SignalAdapter`) — `kubernetesAccess` mounts the SA token and injects `POD_NAMESPACE`; `port` makes the reconciler own Service `agentops-adapter-<name>` and inject `LISTEN_ADDR`. Default posture remains `automountServiceAccountToken: false` and the operator still creates no RBAC.

## Impact

- `api/v1alpha1/channeladapter_types.go`: add `kubernetesAccess *bool` + `port *int32` (+ deepcopy/CRD regen).
- `internal/controller/adapterworkload.go` (shared adapter workload machinery) + ChannelAdapter reconciler: honor `kubernetesAccess` and `port` (Service ownership) as the SignalAdapter side already does.
- New `console/` module: adapter loop (`/channel/*` client), raw k8s list/watch cache, HTTP server (embedded UI, JSON snapshot APIs, SSE), no dependencies outside the directory.
- `chart/`: `console.*` values, CR templates, RBAC templates, Service/Ingress, token Secret.
- Relation to the pending `add-web-chat-channel` change: that change proposes an in-process web channel inside the manager; this console is the out-of-process adapter answer to the same "chat from a browser" need plus observability. They overlap on intent — if this change lands, the console channel likely supersedes the web-chat proposal (decision recorded in design.md, no code conflict today since neither is implemented).
- No manager HTTP API changes and no new manager permissions: the manager's role is unchanged; all read access belongs to the console's own SA, granted by the chart.
