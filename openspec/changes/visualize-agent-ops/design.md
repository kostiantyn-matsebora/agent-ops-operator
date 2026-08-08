# Design: visualize-agent-ops

## Context

agent-ops now has a complete adapter architecture: `ChannelAdapter`/`SignalAdapter` CRs are pure implementations whose name is the routing key, `Pipeline` is the only wiring, and the manager reads no Secrets and grants adapters no RBAC. Observability, however, is `kubectl`-only: the wiring graph (SignalSource → Pipeline → AgentProfile → Channels), condition health, and live Conversation activity are invisible without assembling them by hand from seven CRD kinds.

Kiali proved the pattern for Istio: a read-only console that renders configuration as a topology graph plus live traffic. The twist here is that agent-ops already has a first-class concept for "a surface humans talk to agents through" — the Channel — so the console can be *in* the system it visualizes: a channel adapter that is also a viewer. The pending `add-web-chat-channel` change proposed an in-process web channel inside the manager; since then the architecture moved decisively out-of-process (every signal type is adapter-served, `ChannelAdapter` owns workloads), which makes an adapter-shaped console the architecture-consistent answer.

Constraints that bind this design:

- The manager reads no Secrets and gains no new permissions; the operator creates no RBAC for adapters.
- Adapter modules are self-contained and dependency-free (see `channel-telegram/`, `signal-vmalertmanager/`).
- `spec.config` is opaque to the manager; adapters validate their own.
- No relay loops: a channel implementation never re-ingests its own outbound posts.
- Strictly serial per conversation; channel ops are at-least-once (adapters dedupe by op id).

## Goals / Non-Goals

**Goals:**

- One console showing: CR inventory + conditions, pipeline topology graph, live per-pipeline Conversation runs.
- The console is a conforming channel adapter — conversations on console-wired pipelines are readable *and* writable from the UI.
- Zero new manager surface: the console gets configuration state from the Kubernetes API (its own SA, chart-granted read-only RBAC) and conversation traffic from the existing `/channel/*` contract.
- Ship as an opt-in chart bundle, off by default.

**Non-Goals:**

- Not a general Kubernetes dashboard: only `agentops.dev` CRs (plus the Conversations' pod names as strings — no pod inspection).
- No mutation of configuration from the UI (no CR editing, no pipeline authoring) in this change — read-only except sending chat messages through the channel contract.
- No historical/metrics storage: the console renders current CR state and bounded `status.runs[]` history; it is not a time-series system.
- No multi-cluster, no auth federation (single bearer token, chart-provisioned).
- Does not implement the `add-web-chat-channel` proposal; see Decision 7 for the relationship.

## Decisions

### 1. The console is an out-of-process channel adapter, not a manager feature

The console ships as its own module (`console/`) declared by a `ChannelAdapter` CR named `console`, exactly like `channel-telegram/`. Rationale: (a) the manager stays minimal and permission-free — a UI needs CR read access the manager should not proxy; (b) "the CR name is the routing key" gives `spec.adapter: console` Channels for free, so wiring the console into pipelines uses only existing machinery; (c) the reconciler already owns adapter Deployments, so deployment is CRs-only.

*Alternative considered*: embed the UI in the manager (the `add-web-chat-channel` shape). Rejected: it would add a browser-facing surface, an auth token, and CR-snapshot APIs to the manager, growing exactly the component the architecture has been shrinking; and it predates the adapter-serves-everything model.

### 2. Configuration state via direct read-only Kubernetes list/watch

The console reads `agentops.dev/v1alpha1` CRs (all kinds: AgentProfile, AgentRuntime, Channel, ChannelAdapter, Conversation, Pipeline, SignalAdapter, SignalSource) with its own ServiceAccount using raw HTTP list/watch against the API server — the same dependency-free technique `signal-vmalertmanager` uses for self-registration (`kubernetesAccess` mounts the token; the k8s watch API is line-delimited JSON over HTTP, no client-go needed). It maintains an in-memory cache per kind, resumable via `resourceVersion`, and derives the topology graph and health from it.

*Alternative considered*: a manager-side `/console/state` API. Rejected: the manager would become a CR-snapshot proxy (new surface, new auth scope) duplicating what the API server already serves with proper RBAC; and watches through the contract would need a new streaming mechanism. The RBAC invariant is preserved because the *chart* (not any reconciler) grants a namespaced read-only Role/RoleBinding to SA `agentops-channel-console` — identical posture to how vm-bundle users grant the vmalertmanager adapter its registration rights.

### 3. `ChannelAdapter` gains `kubernetesAccess` and `port` — SignalAdapter parity, not console-specific hooks

Both fields already exist on `SignalAdapter` with exact semantics we need: `kubernetesAccess` mounts the SA token + injects `POD_NAMESPACE` (identity only — capability granted externally); `port` makes the reconciler own Service `agentops-<kind>-<name>` and inject `LISTEN_ADDR` (charts ship no adapter connectivity). Implementing them on the channel side goes through the shared `adapterworkload.go` machinery, so this is closing an asymmetry, not adding console plumbing. The Service name follows the channel workload convention `agentops-channel-<name>`.

*Alternative considered*: chart-shipped Service + hand-rolled token mount for the console only. Rejected: violates "charts ship no adapter connectivity" (already binding on the signal side) and would make the console deployable only by this chart rather than by CRs.

### 4. Topology graph derived manager-independently from CR spec + status

Nodes: SignalSources, Pipelines, AgentProfiles, Channels, plus their serving adapters. Edges: `Pipeline.spec.sources[]`, `Pipeline.spec.channels[]`, `Pipeline.spec.profileRef`, `Channel/SignalSource.spec.adapter` → adapter CR. Node health maps conditions: `Served` (source/channel has a serving adapter), `Ready`, `Wired` — the same conditions the reconcilers already maintain; the console computes nothing the cluster doesn't already assert, so the graph can never disagree with `kubectl`. Unclaimed sources and unwired channels render as disconnected nodes with their condition reason — making the "unclaimed sources DROP signals" failure mode visible, which today is only discoverable in status.

Live activity overlays the same graph: Conversations reference their Pipeline (label/ownerRef) and carry `status.phase`, `status.inflight`, `status.runs[]`, `status.lastActivity` — enough to badge each pipeline edge with active/idle counts and animate inflight runs, Kiali-style, purely from watch events.

### 5. Console-as-channel: transcript = channel traffic; strict no-relay-loop discipline

The console long-polls `/channel/ops?adapter=console`. `ensure-topic` → it mints a thread id (`console-<conversation-UID>`), completes the op, and the manager binds the thread. `send` ops (results, acks, attributed relays from sibling channels) append to an in-memory per-thread transcript ring buffer pushed to browsers over SSE. A message typed in the UI posts to `/channel/inbound` with the thread id — entering the same router as every other channel, preserving busy-ack and serial semantics.

Two rules are load-bearing:

- **No relay loop**: UI-typed messages go to `/channel/inbound` only; they are rendered locally as "pending" and confirmed when the manager's ack/relay comes back as a `send` op. The console never feeds a received `send` op back inbound.
- **At-least-once tolerance**: transcript appends dedupe by op id (redelivered ops render once).

The transcript is intentionally ephemeral (in-memory, bounded): durable history is `status.runs[]` on the Conversation, which the console also has via its watch — the runs view is the durable record, the transcript is the live wire. Restarting the console loses only unscrolled live messages, and thread ids derived from conversation UIDs (not a counter) survive restarts without state.

For conversations on pipelines *not* wired to the console channel, the console still shows everything CR status carries (phase, runs, results) — it just has no live thread and no send box. The UI distinguishes "observed" (watch-only) from "joined" (console channel bound) conversations.

### 6. Frontend: embedded static SPA, no build toolchain

Hand-written HTML/JS/CSS via `go:embed` (distroless-friendly, no asset pipeline, no npm), matching the module's dependency-free rule. The topology graph is rendered with hand-rolled SVG (nodes + edges + condition coloring); data arrives as one JSON snapshot endpoint plus one SSE stream for deltas (watch events, transcript appends, run updates). Browser auth: single bearer token from the chart-provisioned Secret, projected via the existing `Channel.credentialsSecretRef` mechanism (`AGENTOPS_CRED_<CHANNEL>_uiToken`) — the console's own credential surface, arriving through the standard per-channel projection, readable by the adapter from its env. Cookie set after a token login form; SSE authorized by the cookie.

*Alternative considered*: a React/D3 toolchain build. Rejected for this repo's self-contained rule and image simplicity; hand-rolled SVG is sufficient for tens of nodes.

### 7. Relationship to `add-web-chat-channel`

The console supersedes the web-chat proposal's user need (browser chat with agents) with an adapter-shaped implementation, and adds the observability surface the web-chat change never had. The `add-web-chat-channel` change stays untouched (nothing of it is implemented); once this change lands, that change should be re-scoped or withdrawn — recorded here so the decision isn't lost. One thing it carried worth keeping: result-size limits on `/work/done` matter for browser rendering; the console renders whatever `status.runs[].result` carries, so any truncation tuning remains a separate manager concern.

## Risks / Trade-offs

- [Console watches grant it read access to all agentops CRs, including Conversation payloads that may carry sensitive alert content] → RBAC is chart-granted, namespaced, read-only, and opt-in (`console.enabled: false` by default); the browser surface requires the token; the README documents the trust boundary: anyone with the UI token sees what the SA can read.
- [Raw list/watch without client-go: reconnect/resync bugs (missed events, stale resourceVersion 410s)] → implement the standard relist-on-410 loop; cache correctness is testable without a cluster by replaying watch JSON fixtures; envtest integration covers one full list→watch→event cycle.
- [SSE + singleton adapter: one replica serves all browsers and the channel loop] → acceptable at this system's scale (human-facing, tens of conversations); `singleton` is required anyway for deterministic thread transcripts. Load concern documented, not engineered around.
- [In-memory transcripts lost on restart] → deliberate (Decision 5): durable record is `status.runs[]`; the UI reloads history from runs on reconnect and marks the gap.
- [Two sources of truth on screen (watch cache vs channel ops) could disagree transiently] → the UI treats CR status as authoritative and the live stream as ephemeral overlay; run results shown from `runs[]`, live sends deduped against them by conversation+run where present.
- [ChannelAdapter API additions (`port`, `kubernetesAccess`) widen what a ChannelAdapter CR can request] → both are identity/connectivity only, mirroring fields already shipped and security-reviewed on SignalAdapter; no RBAC is ever created by the operator, so escalation still requires an external grant.

## Migration Plan

1. Land the API/controller parity fields (`kubernetesAccess`, `port` on ChannelAdapter) — independently useful, no behavior change for existing adapters (both optional, default off).
2. Land the `console/` module + image.
3. Land chart bundle (CRs, RBAC, Secret, values), default-off. Enabling is: `console.enabled=true` + add the console channel to chosen Pipelines' `channels[]`.
4. Rollback: disable the flag / delete the ChannelAdapter CR — the reconciler removes the workload and Service; Channels referencing `adapter: console` go `Served=False` (visible, non-destructive); conversations keep their other threads.

## Open Questions

- Whether the console channel should be auto-appended to every Pipeline by a chart toggle (`console.joinAllPipelines`) or always manually wired. Leaning manual-first (Pipeline is THE wiring; a chart mutating user Pipelines is a new and questionable pattern) — the UI can instead show a copy-paste patch for unjoined pipelines.
- Thread-id scheme `console-<conversation-UID>` assumes the manager treats thread ids as fully opaque strings (spec says it does); confirm no length constraints in status printing.
- Whether `GET /channel/state` (manager-side cursor persistence) is needed at all for the console — current design needs no cursor (thread ids are derivable, transcripts ephemeral); expected answer: not used.
