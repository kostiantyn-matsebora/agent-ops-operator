## Why

Every manager operation does its work inside the HTTP request that asked for
it.

- `delete` walks and deletes a whole `causedBy` subtree before it answers.
- A console bulk close loops over a page of conversations in one request.
- An inbound chat message is routed, parsed and appended before the adapter
  hears back.

A caller waits for work it cannot observe. A failure partway through leaves no
record. A manager restart mid-request loses what was accepted.

The one queue the manager has, `OpQueue` (channel ops), is weak in four ways:

- It is in memory.
- It is leader-only.
- It has no attempt count, no backoff and no dead-letter record.
- Recovery is whatever the reconciler re-derives on its next pass. That once
  left 22 Telegram topics empty for four and a half hours behind a rate limit.

Two pieces of groundwork now exist:

- The state store sits behind a generic contract with a conformance suite
  (`state-store`).
- A clustered NATS server chart is arriving with `nats-state-store-default`,
  which the bus can reuse as a template.

Accepting work durably and doing it later is the next step. It is a manager
capability that does not depend on where state is kept.

## What Changes

### The bus

- **New command bus**, its own abstraction in
  `platform/manager/internal/commandbus/`, independent of the state store.
  - It carries accepted work to a consumer and records what became of it.
  - Two backends, `nats` and `memory`, pass one conformance suite and a bench.
  - `memory` is for tests and local development only. A production install
    selects `nats`.
- **Every queue is FIFO**, claimed oldest-eligible first.
  - A queue may declare an **ordering key**.
  - Tasks sharing a key are serialized. The next is not claimable until the
    previous has settled.
  - The rule is enforced at claim time through the ledger, never by
    partitioning.
- **A task ledger** records each attempt (state, attempt, error code, lease)
  and feeds a gap-free change stream.
  - The lease is held in the ledger, so a claim made on one replica can be
    completed on another.
  - The ledger is declared bounded, lossy telemetry. The conversation record
    stays the only durable truth.
- **Bus-owned retry.** Attempts are bounded, with backoff and jitter per kind.
  - After the last attempt a task goes `dead`, leaves a read-only dead-letter
    record, and its id is parked for a cool-off.
  - Failures use a closed error-code vocabulary.

### The operations

- **These operations become asynchronous:**
  - `close`: console single and bulk, chat, coordinated, the idle timer, a member's `escalate` and budget-exceeded.
  - `exit`, `reopen`, `delete`, `reset-context` and `escalate`.
  - Inbound chat messages and inbound signals.
- **Each returns `202`** with an `invocationId` and task ids. It first
  validates and durably enqueues. The work then runs on a manager consumer, on
  any replica.
- **Tasks are atomic.** One verb, one target.
  - A batch is only an `invocationId` tag shared by its tasks.
  - A cascade enqueues one task per member, members before the parent, on the
    root conversation's key.
- **`OpQueue` moves onto the bus** as the `out.<channelType>` queues, keyed by
  conversation and channel.
  - Its method surface is kept, so the router, reconciler and handlers do not
    change.
  - Fire-and-forget sends gain a TTL, so a restart cannot deliver a stale
    notice.
- **Read surface:** `GET /state/tasks` (with a stream) and
  `GET /state/dead-letters`. Both are read-only and use the existing `/state/*`
  token scheme.

### The chart

- A `commandBus.backend` selector, independent of `stateStore.backend`.
- Its own connection variables, credential and NetworkPolicy rule.
- A server bundle choice, recorded in the design.

### What stays synchronous

- Reads and streams, sessions.
- `invoke` and the coordinator reads.
- The runtime's `/work*` protocol, including move-per-turn handling inside
  `/work/done`.
- The consumer ends of `/channel/ops`, adapter state and status, `/activity`.
- Mark read and mark unread.
- The accept step of every submit.

### Boundaries

- **No component changes how it calls the manager API.** The adapter contract
  and the runtime contract are not edited.
- **BREAKING (semantic).** A `200` or `202` from `close`, `exit`, `reopen`,
  `delete`, `reset-context` and `escalate` now means *accepted*, not
  *performed*. A caller that needs the outcome reads the task.
- **Not in this change:**
  - The console's task-based UI. It is a later proposal, once this is
    implemented.
  - A retry or dismiss action on dead letters.
  - Making `invoke` asynchronous.

## Capabilities

### New Capabilities

- `command-bus`: the contract.
  - Durable accept and idempotent submit.
  - FIFO claim and per-key serialization.
  - The ledger-held lease.
  - Bus-owned backoff, and the dead and parked states.
  - Per-kind TTL, the ledger feed and per-queue stats.
  - The closed queue inventory, the `nats` and `memory` backends, the
    conformance suite and the chart selector.
- `async-operations`: the manager's use of it.
  - The command inventory, and which queue and key each command belongs to.
  - The accept and process split.
  - The error-code vocabulary and the escalate decomposition.
  - The `/state/tasks` and `/state/dead-letters` surfaces.
  - The operations that stay synchronous.

### Modified Capabilities

- `durable-chat-ops-broker`: a channel op's claim and lease move from the
  leader-only in-memory queue and the Conversation's claim placeholder to the
  bus. Any replica serves the poll.
- `conversation-close`: closing, and its cascade, is performed by tasks. The
  reply means accepted.
- `conversation-exit-command`: `/exit` is a task and is refused mid-run with a
  typed code.
- `coordination-escalation`: `escalate` is a task. The member branch is a
  close followed by a result append on one key. The append id is derived from
  the task.
- `state-durability`: the ledger and dead-letter record are a third declared
  home, bounded and lossy.
- `component-network-isolation`: the manager may reach the command-bus server.
  No other component may.
- `chart-managed-secrets`: the command-bus credential is chart-generated and
  stable across upgrades.
- `end-to-end-testing`: the system pack gains the bus lanes, and the full tier
  gains the end-to-end flows.

## Impact

### Code

| Area | Change |
|---|---|
| `platform/manager/internal/commandbus/` | new: contract, `nats` and `memory` backends, conformance suite, bench |
| `internal/chat/ops.go` | facade over the bus |
| `internal/httpapi/` | accept and process split, `/state/tasks`, `/state/dead-letters` |
| `internal/chat/coordinate.go` | escalate and close as tasks |
| `cmd/manager/` | consumer roles, backend selection, readiness |
| `go.mod` | `nats.go` moves from 1.39.1 to the version the chosen library requires. The state backend's suite is re-run against it |
| `chart/`, `chart/templates/_helpers.tpl` | selector, bundle or external URL, credential, NetworkPolicy, deployment env, guard |
| `platform/manager/test/system/` | bus lanes, multi-replica claim-and-complete, restart durability |
| full-tier end-to-end flows | new set, see the tasks |

### Reference docs made untrue

- `docs/concepts.md`: where work and state live, and what a reply means.
- `docs/contracts.md`: the accept semantics of each endpoint, `/state/tasks`
  and `/state/dead-letters`.
- `docs/configuration.md`: `commandBus.*`.
- `docs/security.md`: a new trust boundary and its control.
- `docs/testing.md`: the bus conformance suite and the new lanes.
- `docs/CHANGELOG.md`: the semantic change and upgrade notes.
- A `docs/guides/` page for the bundle.
- `.claude/rules/structure.md`, `invariants.md`, `gotchas.md` and
  `terminology.md`: the ledger as a declared state home, the ordering and
  idempotency rules, and the words task, invocation, ledger and dead letter.

### Adopter site

- `docs/installation.md`: the new workload and volumes, and uninstall residue.
- `docs/getting-started.md`, `docs/introduction.md` and the landing page,
  wherever they say a close or delete returns when the work is done.
- `README.md`: the component list.

### Dependencies

- The bus renders its own server bundle and owns its credential (design D-13).
  It reuses the NATS chart template that `nats-state-store-default` (PR #471,
  in final verification) adds. The two bundles are independent installs of one
  template. Implementation follows that merge.
- The change does not depend on the state-store contract. It adds nothing to
  `state.Stores`, and `kube`-store installs can run the bus.
