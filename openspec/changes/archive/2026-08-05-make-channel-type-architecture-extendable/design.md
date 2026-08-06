# Design: make-channel-type-architecture-extendable

## Context

Channel support is compiled into the manager and shaped entirely by Telegram:

- `ChannelSpec` selects the type by nil-checking `spec.telegram` (`api/v1alpha1/channel_types.go`); there is no type discriminator and no place for other types' config.
- `ChatFactory` and `TokenReader` in `cmd/manager/main.go:88-145` are hardcoded to Telegram; the poller (`internal/chat/poller.go`) is a Telegram-specific `Runnable` that owns inbound routing.
- Outbound: the reconciler calls `Provider.EnsureTopic` synchronously and expects an `int64` topic id back in the same call; `ConversationStatus.ThreadID` is `*int64`; agents deliver replies by curling the Telegram Bot API from the runtime pod (`internal/dispatch/templates/*.md`).
- The repo already has the exact extension pattern needed, on the runtime side: `AgentRuntime` + the pull-based `/work` long-poll contract (`internal/httpapi/server.go:99`), with `runtime-claude/` as the in-repo reference implementation.

The pending `add-web-chat-channel` change (planned, not implemented) adds a transport-neutral inbound `Router` extracted from the poller and a `spec.web` sub-struct; this design supersedes the sub-struct approach and reuses the Router as the adapter-facing inbound entry point.

Binding constraints: strictly serial per conversation; HTTP API non-leader-gated; single getUpdates consumer per bot token; API group `agentops.dev/v1alpha1` is provisional pre-1.0 (in-place breaking change acceptable, no conversion webhook).

## Goals / Non-Goals

**Goals:**
- Adopters add a channel type (Slack, Teams, …) by deploying an adapter — zero operator changes, zero operator releases.
- One generic Channel CRD: shared metadata schema + opaque per-type config.
- Manager reads no secrets at all; each adapter owns its credentials.
- Telegram keeps working, bit-for-bit at the routing level, as the extracted reference adapter.

**Non-Goals:**
- No adapter SDK/library beyond the documented HTTP contract and the reference implementation.
- No multi-version CRD conversion (pre-1.0: in-place break + documented migration).
- No changes to the runtime `/work` contract or dispatch semantics beyond the thread-id type and delivery wording.
- No dynamic adapter discovery/registration CRD (an adapter simply serves the Channels whose `type` it claims; collisions are an operational concern).
- The web UI channel itself (that's `add-web-chat-channel`, to be rebased).

## Decisions

### D1: CRD split — `spec.type` + opaque `spec.config`
```yaml
spec:
  type: telegram                 # required, immutable; adapter-defined identifier
  defaultProfileRef: {name: ...} # existing, type-agnostic
  delivery:                      # optional, type-agnostic delivery hints (D6)
    mode: agent | result         # default: result
    agentInstructions: "..."     # optional prompt snippet when mode=agent
  config:                        # optional, opaque: x-kubernetes-preserve-unknown-fields
    chatId: "-100123"
    pollingEnabled: true
```
`config` is `apiextensionsv1`-style unstructured (`x-kubernetes-preserve-unknown-fields: true`, stored as `runtime.RawExtension`); the operator never parses it — only adapters do, and each adapter validates its own config shape at its layer. `type` is immutable via CEL (`self == oldSelf`) — retyping a live channel with existing conversations is undefined behavior otherwise.

*Alternative considered:* keep typed sub-structs and add one per known type — exactly the coupling this change removes. *Also considered:* per-type CRDs (`TelegramChannel` kind etc.) — heavier (CRD install per adapter, cross-kind refs from Conversation), rejected.

### D2: Pull-based adapter contract mirroring `/work` (adapter connects to manager, never the reverse)
New manager endpoints (non-leader-gated, same server):
- `GET  /channel/ops?type=<type>&wait=<s>` — long-poll for outbound operations addressed to any Channel of that type. Operation = `{id, channel, conversation, kind: ensure-topic | send, payload}` where `send` payload carries the message (HTML subset) and thread id.
- `POST /channel/ops/{id}/done` — async completion, e.g. `{threadId: "..."}` for `ensure-topic`, or `{error: "..."}`.
- `POST /channel/inbound` — `{channel, threadId?, text, sender?}` fed straight into the shared transport-neutral `Router` (same code path as built-ins; busy-acks and command handling come back as `send` ops).

Rationale: this is the proven shape of the runtime contract — no manager→adapter service discovery, no endpoint field in the CRD, NetworkPolicy-friendly (adapters dial out only), and adapter credentials never leave the adapter. The manager queues ops in memory keyed by channel type; ops are re-enqueued by the reconciler's normal requeue if unclaimed (queue loss on manager restart is safe because every op is derived from CR state that reconciliation regenerates).

*Alternative considered:* manager calls an adapter URL from channel metadata (push) — requires service discovery, TLS/auth toward adapters, and breaks the "everything dials the manager" symmetry; rejected.

### D3: `EnsureTopic` becomes asynchronous; reconciler tolerates pending topics
Today the reconciler blocks on `EnsureTopic` in-call. With remote adapters that's a queue round-trip, so: reconciler enqueues `ensure-topic` when `ChannelRef != nil && Status.ThreadID == nil` (idempotent — op keyed by conversation), requeues, and proceeds when the adapter's `done` lands the thread id in status. Runtime-pod creation already tolerates ordering; inputs stay queued (serialism unchanged). In-process providers (registry, D5) complete synchronously through the same op pipeline so there is exactly one code path.

### D4: `threadId` becomes a string end-to-end
`ConversationStatus.ThreadID *int64` → `*string`; `dispatch.WorkUnit.ThreadID` likewise; runtime env value unchanged in name, now string-typed. Telegram's adapter formats its int topic ids as decimal strings (lossless round-trip). **BREAKING** for anything parsing it as a number; migration: existing conversations' numeric values remain valid string content.

*Alternative considered:* keep int64 and let adapters hash — lossy and collision-prone for Slack `ts`/Teams ids; rejected.

### D5: In-process provider registry with remote fallback
`internal/chat` keeps a `Registry: map[type]Provider` for built-ins compiled into the manager (the future `web` provider registers here; nothing else does after extraction). Resolution: registered type → in-process provider; unknown type → generic remote provider that enqueues ops for `/channel/ops` consumers. `ChatFactory`/`TokenReader` closures in `main.go` are deleted; the manager reads no secrets (bot-token RBAC dropped; adapter auth is D7's one exception, see there).

### D6: Delivery instructions come from channel metadata, not compiled-in wording
Dispatch selects prompt delivery wording from `spec.delivery`: `mode: result` (default) = "your printed answer is the deliverable, captured via /work/done"; `mode: agent` = inject `delivery.agentInstructions` verbatim (the Telegram channel CR carries today's curl instructions there). This removes the last type-specific knowledge from dispatch; `format.md` is already being made channel-neutral by the sibling change. Fixture updates are deliberate.

### D7: Adapter auth — one shared Secret mounted both sides, no manager secret reads
`/channel/*` requires a bearer token. The chart provisions a Secret and mounts it as env into both the manager (its expected token) and each adapter deployment. Because the token reaches the manager by env at deploy time, the manager still performs zero Secret API reads — the invariant becomes absolute. Multiple adapters share the token in v1 (per-type tokens are future work).

*Alternative considered:* per-channel `authTokenSecretRef` read via `GetAPIReader` (the old bot-token pattern) — keeps a manager secret-read path alive for no benefit once Telegram is out; rejected.

### D8: Telegram extracted to `channel-telegram/` as the reference adapter
Own module directory (precedent: `runtime-claude/`), own Dockerfile/image (`agentops-channel-telegram`), reusing the existing Go Telegram code (API client, update parsing, offset annotation via its own patch RBAC — or offset kept in adapter-local state; decision at implementation: keep the Channel-annotation offset, adapters get RBAC to patch Channels they serve). Approver filtering stays adapter-side (Telegram user ids are transport-specific). Single-consumer invariant: chart runs the adapter at `replicas: 1` with `strategy: Recreate`; leader election inside the adapter is future work. Chart ships the adapter Deployment gated on `telegramAdapter.enabled` (default false — Telegram becomes opt-in; the default out-of-box channel is the web one from the sibling change).

### D9: Sequencing with `add-web-chat-channel`
This change lands first (it moves the CRD ground the web change stands on). The web change is then rebased: `spec.web` → `type: web` + `spec.config`, its Router extraction merges with D2's inbound entry point, its synthesized-int64 thread ids become plain strings. That rebase belongs to the other change's artifacts (`/opsx:update` there), not here.

## Risks / Trade-offs

- [Breaking CRD change with live installs] → pre-1.0 provisional API is the stated policy; ship a one-page migration (kubectl-edit recipe telegram-sub-struct → type/config) and bump chart major.
- [Op queue is in-memory; manager restart drops queued ops] → all ops derive from CR state; reconciler requeue regenerates them. Sends triggered by inbound acks are fire-and-forget UX and may be lost on restart — accepted.
- [At-least-once op delivery → duplicate sends possible on adapter retry] → ops carry stable ids; adapters keep a small dedup window; duplicate chat acks are cosmetic.
- [Opaque config loses API-server validation for Telegram fields] → the adapter validates and reports via a `ChannelReady`-style condition on status (adapters get status-patch RBAC); worse than CEL but the price of schema-less extensibility.
- [Shared adapter token = any adapter can claim any type] → acceptable single-tenant v1; noted as future per-type credentials.
- [Router/poller refactor overlaps the sibling change's tasks 2.x] → whichever implements first extracts the Router; the second rebases (D9 fixes the order: this one first).
- [Adapter long-poll adds latency vs in-process calls] → same 20-30s long-poll pattern as runtimes; acks land in ~one round-trip, fine for chat.

## Migration Plan

1. Land CRD + manager changes + Telegram adapter in one release (the manager loses in-process Telegram the same moment the adapter exists).
2. Upgrade steps for a live install: scale manager down (stops old poller → no getUpdates consumer), migrate Channel CRs (`spec.telegram.*` → `type: telegram` + `spec.config.*`, token secret ref moves to adapter values), `helm upgrade` (new CRD, manager, `telegramAdapter.enabled=true` with the token secret name), scale up. The single-consumer invariant holds because old poller and new adapter are never live simultaneously.
3. Rollback: reverse order — disable adapter, restore previous chart/CRD/CR shapes from backup. `threadId` values written as strings of digits parse back to int64 losslessly.

## Open Questions — RESOLVED during implementation

- **Offset storage** → neither adapter RBAC nor local volume: the contract grew a manager-backed state API (`GET/PUT /channel/state/{channel}/{key}`, persisted as `agentops.dev/adapter-state-*` annotations by the manager). Adapters need zero Kubernetes access — the reference adapter is dependency-free Go. For the same reason the contract also serves channel configs (`GET /channel/channels?type=`) and accepts Ready-condition reports (`POST /channel/channels/{name}/status`).
- **Inbound batching** → not added; the adapter posts per-message (getUpdates batches are unrolled adapter-side). Revisit only if a transport shows real overhead.
- **Numeric `threadId` consumers** → repo grep found only `runtime-claude/runtime.js`, which already stringifies (`String(unit.threadId)`); nothing parses it numerically. Called out in the README migration notes regardless.
