## Why

Restarting any agent-ops component today loses live state, and the losses are silent. The chart's `persistence` block already provisions the runtime home PVC but ships `enabled: false`, so out of the box a runtime pod restart destroys the claude-code session files while `Conversation.status.sessionId` survives — the resume then fails and the agent answers the next message with no history, apologising in the reply. Worse, a manager restart between `POST /work/done` and the adapter claiming the outbound op drops the agent's completed answer forever: the result is durably written to `status.runs[].result`, but the `send` op that would deliver it lives only in the in-memory `OpQueue` and nothing regenerates it. The same restart also empties the ingest cooldown map, re-opening conversations for alerts that were being suppressed.

The system is *nearly* resilient — configuration, conversation state, adapter cursors and admission accounting are all CR-backed and recover by reading. This change closes the remaining gaps so that restarting any component recovers current state, and makes the durable-by-default posture the chart's shipped behaviour rather than an opt-in.

## What Changes

**Durability model, stated once.** Two homes for state, chosen by what the state *is*:

- **Filesystem state → PVC**, because a filesystem is genuinely what it is: the runtime's claude-code session files (`/data/home`) and repo checkout (`/data/workspace`).
- **Everything else → the Kubernetes API**, because it is already durable, already replicated, and already the manager's source of truth. The manager mounts no volume: a PVC would pin it to one node and break leader-election failover, and it would contradict the standing invariant that every op is derivable from CR state.

Concretely:

- **Runtime home persistence is ON by default** (`persistence.enabled: true`). The chart's own guidance already says sessions die with the pod without it; shipping the safe default is the change. **BREAKING** for installs that never set it: a fresh install now requests a PVC, and a cluster with no default StorageClass or no RWX provisioner must set `persistence.enabled: false` explicitly. `helm upgrade` on an existing release is unaffected unless values are reset.
- **New `persistence.workspace` block**: an optional second claim backing `/data/workspace`, mounted per-conversation via `subPath` so concurrent runtime pods never share a checkout. Off by default — a repo re-clone is cheap and correct, whereas a stale shared checkout is neither. Turning it on preserves uncommitted agent work across a pod restart mid-conversation and skips the re-clone. Requires a new `workspace` volume stanza on `AgentRuntime.spec` (today only `home` exists).
- **Outbound `send` ops become derivable.** `Conversation.status.runs[]` gains a delivery marker per bound thread; the reconciler enqueues the reply for any completed run not yet marked delivered, and `/work/done` stops being the only path that can produce it. A manager restart at any point now re-derives the undelivered reply instead of dropping it. `ensure-topic` already worked this way; this brings `send` to the same standard. `close-topic` keeps its documented terminal semantics — the CR carrying it is on its way out.
- **Ingest cooldown moves to the CR.** Fingerprint suppression windows are recorded on the owning `SignalSource` (status, pruned by TTL) instead of an in-process map, so a manager restart no longer re-opens conversations inside an active cooldown. The in-memory map stays as the hot path; the CR is the recovery source read on first use per source.
- **Activity telemetry stays lossy — and says so.** The manager's ring buffer is bounded and best-effort by design and is NOT moved to storage. What changes is honesty across a restart: a client whose cursor predates the current process is told `resync` rather than handed an empty timeline it would read as "nothing happened". The console surfaces the gap in its timeline instead of rendering silence.
- **Console persists nothing.** Its config cache is rebuilt by list→watch and its activity index by cursor replay; both already converge. This change documents that as the intended posture so no volume is added to a component that does not need one.
- **A restart-resilience matrix in the docs**: every component, what state it holds, where that state lives, and what a restart costs. This is the artifact that makes the guarantee checkable rather than asserted.

## Capabilities

### New Capabilities

- `state-durability`: the system-wide model — which state is CR-backed, which is PVC-backed, which is deliberately lossy, and what each component recovers on restart.
- `runtime-workspace-persistence`: the optional workspace volume — `AgentRuntime.spec.workspace`, per-conversation `subPath` isolation, and the chart values that provision it.

### Modified Capabilities

- `conversation-close`: `close-topic` remains terminal and non-regenerated, but the surrounding rule tightens — every OTHER op kind, `send` included, must be derivable from CR state.
- `multi-channel-conversations`: fan-out delivery gains a per-thread delivery marker so a partially-delivered reply completes after a restart instead of re-posting to threads that already have it.
- `signal-source-model`: cooldown suppression becomes durable state on the `SignalSource` rather than manager process memory.
- `console-live-runs`: an activity gap caused by a manager restart is reported as a resync boundary, not rendered as an empty window.
- `agent-runtime-ownership`: `AgentRuntime.spec` gains the `workspace` volume alongside `home`.

## Impact

- **API**: `AgentRuntime.spec.workspace` (new); `Conversation.status.runs[].delivered[]` (new); `SignalSource.status` cooldown entries (new). Deepcopy + CRD regeneration required.
- **Chart**: `persistence.enabled` default flips to `true` (**BREAKING** for value-reset installs); new `persistence.workspace.*` block; `chart/templates/pvc.yaml` renders a second claim; `chart/templates/runtime.yaml` wires it; chart minor bump plus a `CHANGELOG.md` entry with the opt-out for clusters without RWX.
- **Operator**: `internal/chat/ops.go` (delivery markers), `internal/controller/conversation_controller.go` (reply re-derivation), `internal/httpapi/server.go` (`/work/done` marks rather than solely enqueues), `internal/ingest/grouping.go` + `internal/httpapi/signals.go` (cooldown load/store), `internal/runtimepod/podspec.go` (workspace volume + subPath).
- **Console**: resync-boundary rendering in the timeline; no storage, no new RBAC.
- **Docs**: `docs/concepts.md` (new CRD fields, capability resolution unchanged), `docs/contracts.md` (`/work/done` delivery semantics), `docs/console.md` (activity gap), `CLAUDE.md` (the durability model as an invariant), `CHANGELOG.md` (upgrade steps).
- **Non-goals**: manager HA / multi-replica (leader election stays as-is), moving activity telemetry to external storage, and any write path from the console to the Kubernetes API.
