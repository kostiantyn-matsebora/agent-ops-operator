## Invariants (do not break)

| Invariant | Rule |
|---|---|
| Parent owns defaults/floor | `global.agentops.runtimeDefaults` is the complete config every runtime inherits. `defaultRuntimeGuard` fails the render if nothing answers `default`. Per-route override via Pipeline (`wiring.md`) |
| No preset RBAC posture | `rbacMode` is DELETED, both levels. An account states its own rules or holds nothing. No built-in role, ever. `agentops.dev` is NEVER granted |
| Storage: operator may create, never destroy | PVC RBAC is `get`/`list`/`watch`/`create` only — no `delete`/`update`/`patch`. Created claims carry NO ownerRef on the Pipeline. Idempotent BY EXISTENCE, never patched |
| Manager reads NO secrets | Everything compiles to `valueFrom`/`envFrom` for the kubelet. `ADAPTER_TOKEN` is derived (HMAC), never minted or stored. No `secrets` RBAC verbs |
| Adapters get NO Kubernetes permissions from the operator | No RBAC, no ServiceAccount created by any reconciler. `spec.serviceAccountName` is a CHART-created reference — absent = release FLOOR |
| Strictly serial per conversation | ONE inflight unit. `MAX_ACTIVE_CONVERSATIONS` (default 5), idle-runtime eviction. `MAX_RUNTIMES` is the deprecated alias |
| Cap decided before provisioning | "Active" = POD-BACKED, never status. Unadmitted → `Pending`, no pod, no `ensure-topic`. FIFO over PODS. `MAX_QUEUED_CONVERSATIONS` (50) |
| `/close` = phase, deletion is a second verb | `Closed` + `closedAt`, tears down pod/ConfigMap/slot. Object and context SURVIVE. Closed = INERT. Reopen NEVER re-resolves refs |
| `/exit` releases the runtime, `/close` ends the conversation | `/exit` deletes only the pod. REFUSED mid-run — a replacement pod would re-run completed work |
| A pod that never starts is reaped | Start DEADLINE frees the slot via the DELETE watch. Condition carries the kubelet's reason verbatim |
| One storage breaker, two edges | fed by unreachable-context reports AND provisioning failures, never two breakers. Only STORAGE-attributable failures count |
| Condition taints are not drains | Only `spec.unschedulable` + taints outside the condition set mean deliberate drain. Gated on `rbac.drainAware`, off by default |
| Housekeeping's listing is phase-blind | A CLOSED conversation still has a CR, so no phase check, or it reclaims kept state |
| Filesystem state on a PVC, else Kubernetes API | THE MANAGER MOUNTS NOTHING. State is a cache, DERIVABLE, or declared-lossy telemetry |
| Conversations originate only from served sources | A channel CARRIES, never starts one. `/channel/inbound` is reply-only. No channel default profile |
| Adapter outbound URL is config, never a constant | `TELEGRAM_API_BASE`, default `https://api.telegram.org`. Precedence: Secret key, then env var, then real host. NEVER `Channel.spec.config` |
| HTTP API not leader-gated, with one exception | `/channel/ops` claims are written only by the current Lease holder (`status.threads[].claim`) — non-leader returns 503. Every other endpoint is an ordinary write |
| One getUpdates consumer per bot token, ever | `gateway-telegram`, one poll loop per Deployment, one Deployment per token. A second poller produces 409s and stolen updates |
| Channel config is opaque to the operator | Never parsed manager-side. `configSchema` applies mechanically (advisory `ConfigValid` only) |
| Manager composes meaning, adapters compose presentation | No transport dialect in `internal/`. Ops carry a TYPED message, never rendered text. Adapters parse the markdown+block-grammar body, not the manager |
| A thread opens with the event that caused it | Every input delivers to every bound channel EXCEPT the one it entered on. "Already seen" is a SURFACE fact, never a message fact |
| A conversation's messages are Kubernetes-API state | `status.runs[].inputs[]` is the permanent record, written before `spec.inputs[]` (the queue) is pruned |
| No relay loops | An adapter must never re-ingest its own outbound post as inbound |
| No signal loops | An observing signal adapter must NEVER emit a signal about agent-ops' own machinery — `selfexclude.go`: name prefix, owner/label, own-namespace |
| No coordination loops | `invoke` refuses a cycle by walking `causedBy` to the root before allowing a call. NO depth limit — only a repeat is refused |
| Coordinator budgets are per-level, never pooled | `maxAgents`/`maxTurns`/`deadline` enforced on the Coordinator-root conversation alone. A caused conversation binds NO human channel — only the uncaused root does |
| Runtime pods | ownerRef → Conversation, for GC. Checkout at `/data/workspace` — never move it |
| Dispatch/ingest semantics | pinned by test fixtures, change deliberately |
| Signal rule vocabulary | `.claude/rules/signal-rules.md` |
