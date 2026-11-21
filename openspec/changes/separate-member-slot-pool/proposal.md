## Why

One pool of slots (`maxActiveConversations`, default 5) is shared by every
conversation, whether a person's or a Coordinator's member. A Coordinator root
holds its slot for as long as its members run, so five busy roots can fill the
pool and leave every member `Pending` behind them — the roots wait on work that
cannot be admitted. Members and roots have different shapes and different cost
profiles; they should be budgeted separately.

## What Changes

- Split runtime-pod capacity into TWO independent pools: **conversations**
  (an uncaused conversation — no `spec.causedBy`) and **members** (a conversation
  an `invoke` created, `causedBy` set).
- New chart value `maxActiveMembers`, default **5**, rendered as env
  `MAX_ACTIVE_MEMBERS`. `maxActiveConversations` stays at **5** and now counts
  conversations only.
- Admission, FIFO ordering, idle eviction and the freed-slot wake-up run PER
  POOL: a full members pool never blocks a conversation, and the reverse. Eviction
  never takes a pod from the other pool.
- `/status`, the Prometheus slot gauges and the console's "at runtime ceiling"
  reading report both pools.
- **BREAKING (capacity)**: total simultaneous pods can now reach 10 where it
  was 5. `maxActiveConversations` no longer bounds Coordinator members; an
  install sizing node resources against the old single figure must add the new one.
- The deprecated `maxRuntimes`/`MAX_RUNTIMES` alias still feeds the
  conversations pool only.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `conversation-capacity`: the cap becomes two per-pool caps; admission, FIFO and eviction are scoped to a pool.
- `manager-introspection`: `/status` and the slot metrics report slots in use against each pool's ceiling.

## Impact

- Code: `platform/manager/internal/controller/conversation_controller.go`
  (admit, evictIdleWorker, createRuntimePod, the pod-deletion wake-up),
  `cmd/manager/main.go`, `internal/httpapi/status.go`/`server.go`,
  `internal/metrics/metrics.go`; `platform/console/ui/src/model/queues.ts`
  and the Queues view.
- Chart: `chart/values.yaml`, `chart/templates/deployment.yaml`,
  `chart/templates/metrics.yaml` (the alert text), render tests.
- Reference docs made untrue: `docs/concepts.md` (Capacity), `docs/configuration.md`
  (Capacity table, ephemeral-storage budget note), `docs/contracts.md` (`/status`),
  `docs/console.md` (ceiling reading), `docs/CHANGELOG.md`, `.claude/rules/invariants.md`.
- Adopter site: the landing page and `getting-started.md` state no cap; `installation.md`
  carries the values table row and the chart version; `docs/guides/*` for the Coordinator
  guide names the slot cost of a member.
- Interacts with the in-flight `warm-runtime-pods` change, whose reservations count against
  `MAX_ACTIVE_CONVERSATIONS`: they count against the pool of the conversation they warm for.
