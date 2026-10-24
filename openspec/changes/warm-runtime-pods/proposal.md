## Why

A conversation's first answer waits on a COLD runtime pod — schedule, init
containers, boot, `git clone`, first `/work` — every time a signal opens one.
The install already pays for that capacity: with `MAX_ACTIVE_CONVERSATIONS`
slots and fewer conversations than slots, the idle slots hold nothing. A
Pipeline should be able to keep a few pods past that cold path, so a new
conversation on it starts at the first `/work`, and a slot given to a warm pod
must cost a real conversation NOTHING — the pod is evicted first, before any
idle conversation, whenever the cap is reached.

## What Changes

- **`Pipeline.spec.warm: N`** keeps up to N pre-provisioned runtime pods for
  that route. The pool is PER PIPELINE — the Pipeline's snapshot (ServiceAccount,
  toolsets, MCP servers, profile repository and its credential, persistence)
  is everything a pod bakes in at creation and none of it can travel with a
  work unit: repo auth and MCP secrets are kubelet-resolved and the manager
  reads none. A per-runtime pool was considered and rejected on those grounds.
- **A RESERVATION IS A `Conversation` IN PHASE `Reserved`**, not a name or a
  bare pod. The manager creates the Conversation with the Pipeline's snapshot
  materialised, NO inputs, no signature, no title, and its ordinary runtime pod
  and MCP ConfigMap follow through the existing path — owned by the
  Conversation, workspace `subPath` named for it, repo cloned, sidecar and
  egress proxy up, runtime long-polling `/work`. Nothing dispatches, no topic
  is ensured, admission's FIFO ignores it. No runtime, sidecar or proxy image
  changes.
- **ADOPTION**: a signal opening a NEW conversation on a Pipeline with a
  reservation takes the OLDEST Reserved one — the pool is a FIFO — and writes
  the signal's input, signature hash, title and phase onto it, instead of
  creating a fresh object. The pod already carries that conversation's name
  and starts answering. Signature REUSE and resurrection of an idle
  conversation never touch the pool.
- **THE RESERVED CONVERSATION IS THE QUEUE ENTRY, AND BOTH VERBS ON IT ARE
  CONDITIONAL.** Adopt is an update carrying the object's `resourceVersion`;
  evict is a delete with a precondition on it. Exactly one wins; the loser
  re-reads and takes the next reservation (or falls back to a cold pod). That
  is what stops a conversation that was JUST adopted being deleted by a
  reconciler that still saw it as warm.
- **Eviction order**, when a conversation needs a slot and the cap is reached:
  a Reserved pod of any pipeline first (it costs nothing), then the
  longest-idle conversation (it costs a restore), then `Pending` as today. A
  reservation is never a reason for `Pending`: the free-capacity count treats
  it as free. A workspace-persistent conversation at the head of the queue
  takes a warm pod's slot on its next pass.
- **Refill fills LEFTOVER capacity only**: up to `min(N, cap − active)` after
  admission, appended at the tail, after a short delay so a slot freed by a
  TTL is not filled and evicted within the same minute.
- **Idle release stays within the runtime TTL.** A reserved pod must not exit
  on the runtime's idle clock while it waits, so it is created with the
  runtime's TTL disabled and the MANAGER reaps a warm-born pod that goes idle
  after adoption, on the same `NeedsWorker` predicate and the runtime's own
  `idleTTLMinutes`. `/exit` and cap eviction are unchanged.
- The chart renders `warm` on the `pipelines:` values and on the bundle-shipped
  routes; the console and `kubectl` show phase `Reserved`.

## Capabilities

### New Capabilities
- `warm-runtime-pods`: the reservation object, its lifecycle (create, adopt,
  evict, reap, refill), the FIFO and the conditional-write serialisation, and
  what a reservation is excluded from.

### Modified Capabilities
- `pipeline-model`: `spec.warm` joins the wiring; a Reserved conversation
  carries `spec.pipelineRef` and the snapshot like any other.
- `conversation-capacity`: reservations count as active, are the FIRST evicted
  and never a cause of `Pending`; refill is bounded by the cap; an adopted
  warm-born pod's idle release is manager-side.
- `chat-signal-origination`: a NEW conversation on a Pipeline holding a
  reservation adopts it; reuse is unaffected.
- `conversation-housekeeping`: the orphan rule holds unchanged because the
  Conversation predates its directory, and this change STATES that the
  Reserved phase is what keeps it true; a reservation evicted unused is deleted
  as an object, and its directory becomes an ordinary orphan.

## Impact

- `platform/manager/`: `api/v1alpha1` (`Pipeline.spec.warm`, phase
  `Reserved`), `internal/controller` (pipeline reconciler refills, conversation
  reconciler eviction order, `evictableCount`, manager-side idle reap, the
  Reserved gate on topics and dispatch), `internal/httpapi/signals.go`
  (adoption before `GenerateName`), `internal/runtimepod` (TTL off for a
  reservation), CRDs under `chart/crds/`, envtest coverage.
- `chart/`: `pipelines[].warm`, bundle routes' values, NOTES.
- **Reference docs**: `docs/concepts.md` (the phase, the pool, the eviction
  order), `docs/cr-reference.md` and every generated Pipeline block
  (`docs-generate.py`), `docs/CHANGELOG.md`.
- **Adopter site**: `docs/guides/pipeline.md` (the field and when to set it),
  `docs/installation.md` (the values key and the capacity it costs),
  `docs/console-guide.md` (a Reserved conversation in the list),
  `docs/introduction.md` / `getting-started.md` only if they state "a signal
  creates a pod" as the whole story.
- Deferred, on record: a GENERIC late-bound pod that could also serve a
  RESURRECTED conversation (the sidecar already restores lazily at first
  `/work`) — a later change, since it redefines workspace durability as
  checkpoint-based.
