## Why

Two problems, and they turn out to be the same one seen from opposite ends.

**Nothing ever reclaims what a conversation leaves behind.** `/close` and
`kubectl delete` are the only ways a `Conversation` disappears, so on any install
with an observing lane they accumulate without bound — the reference cluster
holds **31 after 35 hours**, which is thousands over a year, each with its
`ConversationInput` objects and a compiled MCP ConfigMap. Chart 5.1.0 made the
storage half concrete: workspace persistence gives every conversation its own
directory in a shared claim and nothing deletes it, while the home claim is
mounted with **no `subPath`**, so every conversation's claude-code transcripts
pile into one shared `$HOME/.claude/projects/-data-workspace/`.

**And closing is too expensive to use.** Closing today is deletion: the
`Conversation`, its recorded runs, its context handle and — once the directory is
reclaimed — its workspace all go at once, irreversibly. That makes `/close` a
destructive act rather than a tidying one, which is precisely why nobody closes
anything and why the backlog above exists. An operator who wants a lane quiet has
to choose between keeping everything and losing everything.

So closing and reclaiming are split into **two stages with two clocks**:

1. **Close** — the conversation goes **inert but intact**. Its runtime pod and MCP
   ConfigMap are torn down, its topics are archived with a farewell, its capacity
   is released, and it participates in no pipeline and no conversation reuse. The
   `Conversation` object REMAINS, at phase `Closed`, and its volume state is
   untouched. It can be **reopened**.
2. **Delete** — the conversation and its state are reclaimed for good: the object
   from etcd, the workspace directory and the session transcripts from disk.

Each stage gets its own flag and its own window, both off by default. The
manager cannot perform stage 2's disk half — it **mounts no PersistentVolume**,
by invariant, and that invariant is load-bearing — so the split by clock is also
the split by workload.

## What Changes

**Closing stops being deletion.** A closed conversation is one at phase `Closed`:

- no runtime pod, no MCP ConfigMap, no dispatch, no capacity consumed;
- absent from conversation REUSE, so a signal with a matching signature opens a
  new conversation rather than waking a closed one;
- absent from every pipeline — a closed conversation is not a place work can land;
- `status.closedAt` stamped, which is what the second clock is measured from;
- its workspace directory, its session transcripts and its `runtimeContextId`
  retained, which is what makes reopening mean anything.

The normative close path — the farewell, the teardown, the ONE implementation
every originator calls — is unchanged except in its final step: it sets a phase
instead of issuing a delete.

**Topic archiving moves to the close transition.** `close-topic` ops are enqueued
when the conversation closes, not when the object is deleted. The
`agentops.dev/close-topics` finalizer stays, demoted to what it always guarded:
a direct `kubectl delete` of a conversation nobody closed first.

**A closed conversation can be reopened, from the console, back to `Idle`.** The
materialized refs it already carries (`profileRef`, `channelRefs`, `toolsets`,
`mcpConfigs`) are re-read as they always are, so no wiring is re-resolved and no
Pipeline edit leaks in. Continuity is restored **where it was promised** — the
workspace and the context handle survive under `contextStorage: volume`, and
under `none` a reopen answers fresh and says so, exactly as a resume does.

**Reopening re-establishes threads through `ensure-topic`, carrying the archived
thread id as a HINT.** An adapter that can un-archive returns the same thread id
and the conversation continues where it left off; an adapter whose transport has
no such notion ignores the hint and returns a new one. What a reopen looks like
on a surface is the adapter's decision, which is the only place that knowledge
exists. No new operation kind, and no existing adapter breaks.

**Two windows, two flags, named for what each measures:**

- `retention.autoclose.enabled` / `retention.autoclose.idleAge` — a finished
  conversation is closed once it has been **inactive** that long.
- `retention.autodelete.enabled` / `retention.autodelete.closedAge` — a closed
  conversation is deleted once it has been **closed** that long.

**The autoclose window is idle time, never lifetime.** It is measured from last
activity — the most recent run or input — not from creation. A conversation
created ten days ago that answered an hour ago is not old; it is busy.

"Finished" means all of: phase `Idle`, no pending inputs, no inflight run, no
runtime pod, and every recorded run marked delivered to every bound channel. That
last clause is not decoration: a run goes `Idle` the moment its result is
recorded, while the reply may still be an unclaimed `send` op, so closing on
`Idle` alone can archive the thread out from under the answer.

**There is deliberately no "close as soon as the answer is delivered" mode**, and
the reasoning now lands on the DELETE window instead. `status.runs[].result` is
where an answer lives in the Kubernetes API and nowhere else, so it is deletion
that destroys the record. A short close window is cheap — the result stays
readable and the conversation stays reopenable. A short DELETE window is the
expensive one, and the values comment says so at the setting.

**The console gains bulk delete beside bulk close, and per-row reopen.** Delete
is refused on anything that is not already `Closed` — a live conversation named
in a delete batch is reported `skipped` with "close it first", never closed
implicitly. Reach for both verbs is the **binding**: a surface may delete or
reopen a conversation whose `spec.channelRefs` names its channel. That is the
amendment the archived-thread case forces on "no remote close verb exists": the
rule protected *you may only end a conversation you are part of*, and holding a
live thread was how membership was proven while a thread existed. Once topics are
archived there is no thread left to hold, so membership is read from the binding
that put the thread there. The console still performs no Kubernetes write.

**A chart-shipped, opt-in CronJob reclaims disk**, because only something that
mounts the claim roots can:

- **Orphan workspace directories** — `<claim>/<name>` where no `Conversation` of
  that name exists.
- **Orphan session files** — transcripts on the home claim whose session id no
  longer appears in any live `Conversation`.

A **closed conversation still has a CR**, so its state is protected by the same
listing rule that identifies an orphan — the job needs no knowledge of phases at
all. Autodelete produces orphans deliberately; the job reclaims them on its next
run. The two never coordinate.

It runs no agent code, under its **own** ServiceAccount with read-only access to
conversations — never the runtime SA, which would hand agent workloads the claim
root that `subPath` isolation exists to deny them.

**Safety is the design, not a flag.** An orphan is only reclaimed if it is absent
from a listing taken *after* the directory scan AND older than a grace period.
Every run is bounded (`maxDeletions`) so a first run on an old install cannot
reclaim a thousand trees at once, and a `dryRun` mode reports what it would
remove.

**BREAKING, behaviourally, for everyone** — not because a value changed default,
but because `/close` means something different. A closed conversation now stays
in `kubectl get conversations` as a `Closed` row until autodelete is enabled or
an operator deletes it. Both windows are off by default, so nothing is reclaimed
that was not reclaimed before; what changes is that nothing is DESTROYED that was
destroyed before either.

## Capabilities

### New Capabilities

- `conversation-housekeeping`: the two-stage conversation lifecycle and what
  reclaims each stage — autoclose of finished, idle conversations into an inert
  `Closed` state, reopening back to `Idle`, autodelete of long-closed
  conversations, orphan workspace directories and orphan session files, the
  safety rules that make deletion sound (listing order, grace period, per-run
  bound, dry run), and the identity separation that keeps the claim root away
  from agent workloads.

### Modified Capabilities

- `conversation-close`: closing sets phase `Closed` instead of deleting. The
  farewell, the teardown, the capacity release and the ONE-implementation rule
  survive intact; what changes is the final step, where the `close-topic` ops are
  enqueued (the transition, not the deletion), and what the close-topics
  finalizer is now for. Deletion becomes a second verb with its own reach rule.
  **`close-topic` stops being the one operation not re-derivable from CR state**,
  and that is a fix rather than a tidy: with the object surviving the close, a
  restart that dropped an outstanding archive used to lose it invisibly along with
  the object, and would now leave a `Closed` conversation sitting forever beside a
  topic nobody archived. A per-thread archived marker, mirroring
  `status.runs[].delivered[]`, makes it re-derivable. The finalizer and its
  two-minute grace remain for the one path where the object really does go away —
  a direct `kubectl delete`.
- `console-bulk-close`: bulk delete beside bulk close, per-row reopen, `Closed`
  rows presented as closed-and-reopenable rather than gone, and the reach rule
  restated over bindings now that a closed conversation holds no thread.
- `channel-adapter-contract`: `ensure-topic` gains an optional previous-thread-id
  hint, and the adapter's freedom to honour it or open a fresh entity is stated
  as contract rather than left to each implementation to guess.
- `signal-self-exclusion`: the housekeeping workload must be excluded by NAME
  PREFIX. Today's prefixes are `agentops-conv-`, `agentops-adapter-` and
  `agentops-signal-`, and a housekeeping pod matches none of them — so a failed
  cleanup Job would emit a Warning event, become a signal, and wake an agent
  about agent-ops' own maintenance. That is the "own health is STATUS, not
  SIGNAL" rule, and the prefix mechanism is the one that holds with a cold cache.

## Impact

- **API**: `Conversation` gains phase `Closed` and `status.closedAt`. No spec
  field is added — closing is a state the manager reaches, never something an
  author writes.
- **Manager**: the close path in `internal/controller` sets a phase rather than
  deleting, and enqueues `close-topic` at the transition; both TTLs are
  self-scheduled requeues configured by env (`CONVERSATION_AUTOCLOSE_ENABLED` /
  `_IDLE_AGE`, `CONVERSATION_AUTODELETE_ENABLED` / `_CLOSED_AGE`); dispatch,
  admission and conversation reuse all learn to skip `Closed`; two new
  console-reachable verbs (delete, reopen) bounded by the binding. No new RBAC —
  `delete` on conversations is already granted.
- **Console**: bulk delete, per-row reopen, `Closed` presented as a state rather
  than an absence. Still no Kubernetes write path.
- **Adapters**: `ensure-topic` may carry a previous thread id;
  `channel-telegram` honours it by un-archiving, and ignoring it stays valid.
  `signal-k8s-events/selfexclude.go` gains the housekeeping prefix.
- **New module + image**: a dependency-free `housekeeping/` module talking to the
  in-cluster API over `net/http`, the same technique `signal-k8s-events` and
  `console` already use; no client-go, no third-party image.
- **Chart**: a `housekeeping` component (CronJob, SA, read-only Role) plus values
  for schedule, both retention blocks, per-run bound and dry run; both claims
  mounted at their ROOT, which is why the identity is separate.
- **Docs**: `docs/concepts.md` (the two-stage lifecycle, both windows, what
  reclaims what), `docs/contracts.md` (the `ensure-topic` hint and the two console
  verbs), `docs/console.md` (delete, reopen, `Closed` rows), `CHANGELOG.md` for
  the changed close semantics and the new values.
- **Depends on**: `persistence-in-chart` (archived 2026-08-10), which introduced
  the workspace claim and the per-conversation `subPath` this reclaims — and
  which is what makes a reopen restore anything at all.
- **Supersedes part of**: `console-bulk-close-conversations` (archived
  2026-08-12). Its batch close is unchanged in mechanism; what changes underneath
  it is that the close it orders no longer deletes.
- **Out of scope**: reclaiming anything from etcd other than conversations;
  bulk reopen (reopening is a per-conversation decision); exporting or archiving
  a conversation's transcript before deletion; and any autoclose that ends a
  conversation a human might still reply to — the gate is finished-and-idle,
  never "old" alone.
