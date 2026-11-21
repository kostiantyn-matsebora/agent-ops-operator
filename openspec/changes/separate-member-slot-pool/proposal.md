## Why

One pool of slots (`maxActiveConversations`, default 5) is shared by every
conversation, whether a person's or a Coordinator's member.

A Coordinator root holds its slot for as long as its members run. Five busy
roots can fill the pool and leave every member `Pending` behind them.

Roots and members have different shapes and costs, so they should be budgeted
separately.

## What Changes

- Split runtime-pod capacity into TWO independent pools:
  - **conversations**: an uncaused conversation, with no `spec.causedBy`.
  - **members**: a conversation an `invoke` created, `causedBy` set.
- Add chart value `maxActiveMembers`, default **5**, rendered as env `MAX_ACTIVE_MEMBERS`.
- Keep `maxActiveConversations` at **5**. It now counts conversations only.
- Run admission, FIFO ordering, idle eviction and the freed-slot wake-up PER POOL. A full pool never blocks the other, and eviction never takes a pod from the other pool.
- Report both pools in `/status`, the Prometheus slot gauges and the console's "at runtime ceiling" reading.
- **BREAKING (capacity)**: total simultaneous pods can reach 10 where it was 5.
  - `maxActiveConversations` no longer bounds Coordinator members.
  - An install sizing node resources against the old single figure must add the new one.
- The deprecated `maxRuntimes` / `MAX_RUNTIMES` alias still feeds the conversations pool only.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `conversation-capacity`: the cap becomes two per-pool caps, and admission, FIFO and eviction are scoped to a pool.
- `manager-introspection`: `/status` and the slot metrics report slots in use against each pool's ceiling.

## Impact

**Code**

- `platform/manager/internal/controller/conversation_controller.go`: admit, evictIdleWorker, createRuntimePod, the pod-deletion wake-up.
- `platform/manager/cmd/manager/main.go`, `internal/httpapi/status.go`, `internal/httpapi/server.go`, `internal/metrics/metrics.go`.
- `platform/console/ui/src/model/queues.ts` and the Queues view.
- Chart: `chart/values.yaml`, `chart/templates/deployment.yaml`, `chart/templates/metrics.yaml` (the alert text), render tests.

**Reference docs made untrue**

- `docs/concepts.md` (Capacity).
- `docs/configuration.md` (Capacity table, ephemeral-storage note).
- `docs/contracts.md` (`/status`).
- `docs/console.md` (ceiling reading).
- `docs/CHANGELOG.md`.
- `.claude/rules/invariants.md`.

**Adopter site**

- The landing page and `getting-started.md` state no cap.
- `installation.md` carries the values table row and the chart version.
- The Coordinator guide under `docs/guides/` names the slot cost of a member.

**In-flight change**

`warm-runtime-pods` reservations count against `MAX_ACTIVE_CONVERSATIONS`. They now count against the pool of the conversation they warm for.
