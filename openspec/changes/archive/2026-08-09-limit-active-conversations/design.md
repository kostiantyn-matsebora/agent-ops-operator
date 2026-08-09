## Context

Conversation concurrency is bounded today only inside
`ConversationReconciler.createRuntimePod`: it lists runtime pods, and when
`len(live) >= MaxRuntimes` (default 8) it either evicts the longest-idle worker
or returns `created=false`, leaving the conversation in `Queued` with a 30 s
requeue. Three things follow from that shape:

- The limit is named and reported in **pod** terms. An operator who wants "at
  most five conversations at a time" has to know that one conversation owns at
  most one runtime pod (strictly serial per conversation) to translate it.
- A conversation that cannot get a pod has already had its **chat topic
  created** and its MCP ConfigMap resolved — the expensive, user-visible parts
  happen before the capacity decision.
- A finished conversation holds its slot for `RUNTIME_IDLE_TTL_M` (default
  10 min) before the runtime exits 0. Under a signal burst, capacity is
  reclaimed mostly by eviction rather than by work actually completing.

Nothing ever deletes a `Conversation`. There is no "this one is done" — the CR,
its chat topic, and its input objects persist indefinitely.

Constraints this design has to respect:

- **Strictly serial per conversation**, parallelism across conversations —
  one conversation ↔ at most one runtime pod stays true, so "active
  conversations" and "live runtime pods" are the same number.
- **The HTTP API is not leader-gated**; `/signal/inbound` may run in a replica
  that is not the active reconciler. Admission decisions therefore belong in
  the reconciler, not in the ingest path.
- **Ops are at-least-once and the OpQueue is in-memory by design**, justified by
  every op being derivable from CR state. A close-topic op is the first op whose
  originating CR is on its way out — that tension is decided below.
- **The manager reads no Secrets** and **grants adapters no RBAC**; nothing here
  changes either.

## Goals / Non-Goals

**Goals:**

- A configurable ceiling on simultaneously active conversations, default 5,
  expressed in conversation terms.
- Over-cap work waits durably and is admitted in arrival order, without a new
  CRD and without an in-memory backlog.
- Capacity is reclaimed promptly when an agent stops working: runtime idle TTL
  default 1 minute.
- A person can end a conversation from chat with `/close` — the CR is deleted
  and every bound thread is archived.

**Non-Goals:**

- Retention/GC of *closed* conversations: closing deletes, so there is nothing
  left to retain. Bulk cleanup of the conversations that already exist in a
  running cluster is an operator action, not part of this change.
- Priority or fairness classes between pipelines. Admission is FIFO by creation
  time, full stop.
- Per-user authorization for `/close`. No surface in this system authorizes
  individual senders today, and inventing it here would be the only such check.
- Changing `AgentRuntime.spec.idleTTLMinutes` semantics — only the manager-wide
  default moves.

## Decisions

### 1. "Active" means pod-backed; the cap counts live runtime pods

A conversation is **active** while a runtime pod exists for it
(`Running` or `Pending`). Idle conversations — the agent finished, the pod
exited after its TTL — cost nothing and do not count. This is what makes a
default of 5 safe: a conversation that is merely *open* never starves the queue,
and the only thing a slot ever represents is a real pod on a real node.

The counter stays the pod list already used by `createRuntimePod` (label
`runtimepod.LabelApp`), not a count of Conversations in some phase: pods are the
thing that consumes the cluster, and a status field can lag the pod it describes.

*Alternative rejected —* count conversations whose phase is `Working`/`Queued`:
cheaper to read, but it drifts from reality exactly when it matters (pod stuck
`Pending` on an unschedulable node, status patch lost to a conflict).

### 2. Admission happens before topic creation, in a new `Pending` phase

`ConversationPhase` gains `Pending`: *created, waiting for a capacity slot;
nothing has been provisioned yet.* A `Pending` conversation gets **no runtime
pod, no chat topic, no MCP ConfigMap, no dispatch**. It keeps its inputs, its
wiring snapshot, and its signature label, so grouping and window reuse work on
it exactly as on an admitted one.

The reconciler orders itself accordingly: the admission check moves to the top
of `Reconcile`, before `ensureTopics`. `Queued` keeps its current meaning —
*admitted, work waiting its turn behind the serial-per-conversation rule* — so
the two phases answer different questions and neither is overloaded.

Suppressing the topic is the point of the phase: it is what stops a thousand
signals from becoming a thousand chat threads before anyone has looked at the
first one.

*Alternative rejected —* a separate `PendingSignal` CRD holding the normalized
signal until admission. It keeps the Conversation list "genuinely active", but
costs a CRD, RBAC, a drain loop, and — decisively — a second place that has to
snapshot pipeline wiring and a second list that signature grouping has to
consult for window reuse.

*Alternative rejected —* an in-memory backlog: lost on restart, and with a
non-leader-gated HTTP API each replica would hold a different one.

### 3. Admission is FIFO by creation time, driven by a pod watch

A `Pending` conversation admits itself only when it is the **oldest** pending
conversation that needs a worker and a slot is free. Every conversation makes
that decision from the same two lists (pods, pending conversations), so the
order is stable without a leader-elected scheduler or a queue object.

Reconciles are triggered by a new watch on runtime pods mapping **any** runtime
pod deletion to the oldest pending conversations, rather than by polling: the
existing `Owns(&corev1.Pod{})` only routes events to the pod's own conversation,
which is precisely the conversation that no longer needs the slot. A
`RequeueAfter` remains as a backstop for missed events.

*Alternative rejected —* let every pending conversation retry on its own 30 s
timer. Simple, but admission order becomes whichever timer fires first, and
under sustained pressure a late arrival can jump a long-waiting one repeatedly.

### 4. Idle eviction stays, and stays harmless

The existing behavior — at cap, evict the longest-idle pod whose conversation
has nothing inflight and nothing queued — is kept. It does not close anything:
the conversation returns to `Idle`, its session survives in `/data/home`, and it
gets a fresh pod on its next input. With the TTL at 1 minute this fires rarely;
keeping it means a burst never blocks behind a pod that is doing nothing.

### 5. The idle TTL default drops to 1 minute

`RUNTIME_IDLE_TTL_M` / `runtimeIdleTtlMinutes` default 10 → 1.
`AgentRuntime.spec.idleTTLMinutes` still overrides per runtime, so a runtime
with expensive startup can opt back out.

Trade: a follow-up message a few minutes later pays pod startup again. It does
**not** lose context — claude-code sessions key on cwd (`/data/workspace`) and
the home volume persists — so the cost is latency, not memory.

### 6. `/close` deletes the CR; a finalizer archives the topics first

`/close` in a conversation's thread:

1. is intercepted in `Router.HandleMessage` **before** the text becomes a reply
   input (parsed with `addressing.Parse`, so `/close@SomeBot` works too);
2. fans out a farewell message to every bound thread — naming abandoned work if
   the conversation was inflight;
3. deletes the `Conversation`.

Deletion does the rest through machinery that already exists: ownerRefs GC the
runtime pod and the `agentops-mcp-conv-<name>` ConfigMap; `ConversationInput`
objects are cleaned by the same path that prunes processed inputs.

Topic archiving hangs on a **finalizer** (`agentops.dev/close-topics`). On
deletion the reconciler enqueues one `close-topic` op per bound thread and
removes the finalizer once they complete — or after a bounded grace of 2
minutes, so a channel whose adapter is down can never wedge a deletion. This is
what keeps the "ops are derived from CR state" invariant intact: while the
finalizer holds, the CR is still there to regenerate the op from after a manager
restart. It also means `kubectl delete conversation` archives topics too, which
is the behavior an operator would assume anyway.

*Alternative rejected —* enqueue the ops and delete immediately. Three lines
shorter, and every manager restart in that window leaves an orphaned open topic
with no CR left to derive a retry from.

*Alternative rejected —* a terminal `Closed` phase that keeps the CR. It
preserves history, but the CRs then accumulate forever — the problem this
change exists to stop — and it needs a retention sweep to finish the job.

`/close` on a channel's **general surface** (no thread) reaches
`Router.HandleCommand` as an unknown pipeline named `close`; it answers with
usage rather than "unknown agent", because typing it there is an obvious
mistake, not a typo'd pipeline name.

`/close` while the conversation is **working** is honored immediately: the pod
is GC'd mid-run and the farewell says so. Refusing would make `/close` useless
for the case an operator most wants it — an agent that has gone off the rails.

### 7. `close-topic` is a new op kind, and it is fire-and-forget on failure

`OpCloseTopic = "close-topic"` carries channel, conversation and `threadId`.
Completion is `POST /channel/ops/{id}/done` with an empty body, or
`{"error":"…"}`.

Failures are **logged, not surfaced as a Conversation condition** — the
conversation is being deleted, so there is no object left to carry the
condition, and no regeneration after the finalizer grace expires. An adapter
that does not implement the kind should complete it with an error (or ignore it
and let the grace expire); the visible consequence is an open topic for a
conversation that no longer exists, which is recoverable by hand.

Adapters: `channel-telegram` calls `closeForumTopic`; the console marks the
thread archived and keeps its transcript for the session.

### 8. The backlog is bounded too

An unbounded `Pending` queue reproduces the original complaint one level down —
no pods, but unbounded Conversation CRs. `MAX_QUEUED_CONVERSATIONS` /
`maxQueuedConversations` (default 50) caps it. Beyond it, `/signal/inbound`
declines to create the conversation and reports the batch drop through the
existing drop-reason path: chat origins are told on the surface they typed on
(`tellOriginatingSurfaces`), alert/job origins are logged and counted.

This is the one check that must live in the ingest path rather than the
reconciler, because the point is not to create the object at all. It is a count
over the conversation list, not a scheduling decision, so a stale read at worst
admits one over the bound.

### 9. Config names follow the conversation vocabulary

- `MAX_ACTIVE_CONVERSATIONS` (chart `maxActiveConversations`), default **5**.
  `MAX_RUNTIMES` / `maxRuntimes` is honored as a deprecated alias when the new
  one is unset, with a startup log line, and removed after one release.
- `MAX_QUEUED_CONVERSATIONS` (chart `maxQueuedConversations`), default **50**.
- `RUNTIME_IDLE_TTL_M` (chart `runtimeIdleTtlMinutes`), default **1**.

The rename is worth its churn: the field is read by people who think in
conversations, and `maxRuntimes: 5` next to `AgentRuntime` CRs reads as a limit
on runtime *kinds*.

## Risks / Trade-offs

- **A default of 5 is lower than today's 8 — an upgrade silently reduces
  throughput.** → Called out in the proposal and release notes; the knob is a
  one-line values change, and queued work is delayed, never dropped (up to the
  backlog bound).
- **A 1-minute idle TTL trades pod churn for capacity.** A conversation with a
  chatty human pays startup repeatedly. → Session context survives in
  `/data/home`; operators who prefer stickiness raise `runtimeIdleTtlMinutes` or
  set `AgentRuntime.spec.idleTTLMinutes`.
- **Pending conversations are invisible in chat** — no topic exists yet, so a
  user who typed `/pipeline task` sees nothing. → On entering `Pending`, one
  message goes to the originating channel's general surface stating the
  conversation is queued; the topic appears on admission.
- **The finalizer can delay deletion by up to the grace period.** → Bounded at
  2 minutes and never blocks past it; the reconciler removes the finalizer even
  when no adapter ever claimed the op.
- **Backlog rejection loses a signal.** → Only above 50 queued conversations,
  which means the cluster is far past capacity; the drop is reported on the
  originating surface and counted, matching how unwired-source drops behave.
- **Cap enforcement races.** Two conversations reconciled around the same pod
  list could both admit. → Conversation reconciles are single-threaded
  (`MaxConcurrentReconciles` unset = 1) and `createRuntimePod` re-checks the cap
  against a fresh list before creating; worst case is one pod over, corrected on
  the next reconcile.

## Migration Plan

1. Ship the API change (`Pending` phase) with regenerated deepcopy + CRDs — an
   additive enum value; old managers never write it.
2. Ship manager + adapters together in one chart version: an adapter that does
   not know `close-topic` would otherwise leave topics open. Chart bumps
   `channel-telegram` and `console` image tags alongside the manager.
3. `maxRuntimes` keeps working for one release; upgrades that never touched it
   pick up the new default of 5.
4. Rollback is a chart rollback: `Pending` conversations reconciled by an older
   manager fall through to the existing pod-cap path and proceed normally, since
   phase is status-only and nothing keys behavior off it.

## Open Questions

- `maxQueuedConversations: 50` and the 2-minute finalizer grace are chosen
  defaults, not requirements from the request — worth a look before implementing.
- Should admission emit a Kubernetes Event (`AtCapacity`) in addition to the
  phase, for `kubectl describe`? Cheap to add, easy to omit.
