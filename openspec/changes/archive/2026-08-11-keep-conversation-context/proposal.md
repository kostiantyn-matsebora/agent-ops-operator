## Why

The machinery for continuous context already exists: `Conversation.status.sessionId`
holds the agent session, `runtime-claude` passes `--resume` for reply and
recurrence inputs, session files live on `/data/home`, and that volume now
persists by default. A conversation is *supposed* to keep every message, tool
call and model response across pod restarts.

It is not a guarantee, because three things can break it and all three break
**silently**.

**A failed resume is permanent.** `POST /work/done` records the session id only
once:

```go
if d.SessionID != "" && conv.Status.SessionID == "" {
    conv.Status.SessionID = d.SessionID
}
```

When a resume fails — session files gone after an eviction, a node move, an
install without the home volume — the runtime correctly retries once without
`--resume` and gets a **new** session. That new id is never recorded, because the
field is already set. The conversation now points at a session that does not
exist, so every subsequent message tries to resume it, fails, and starts another
fresh session. Context is lost not once but on **every message from then on**,
and the stored id stays wrong forever. One transient loss becomes a permanent
one.

**Losing context is invisible.** The runtime prefixes its answer with a warning
and the pod logs say so, but nothing reaches the `Conversation`. Nothing in
`kubectl get conversation`, nothing in the console, no condition. The
conversation looks healthy while answering every message with no memory.

**Whether an input resumes at all depends on that same id.** Ingest converts a
repeat signal to a `recurrence` only `if conv.Status.SessionID != ""`, and
dispatch resumes only for `reply` and `recurrence`. So a conversation whose id
is missing or stale does not merely fail to resume — its inputs are classified as
new work, taking the fresh-session path by design. The failure compounds quietly.

## What Changes

- **The handle is renamed out of the runtime's vocabulary.** `status.sessionId`
  becomes `status.runtimeContextId`, and the work unit's `resumeSessionId` follows.
  `session` is claude-code's noun; another backend calls it a thread and another
  has none. Agent-ops has Conversations, and what a runtime returns is its own
  handle for that conversation's accumulated context. **The rename reads BOTH
  fields for one release**, because a rename that simply moved the field would
  strand every in-flight handle on upgrade — inflicting the exact loss this change
  exists to prevent.
- **The handle is kept CURRENT, not first-write-wins.** Every `/work/done` that
  reports one records it. A run that fell back updates the conversation, so the
  next message continues from the context that actually exists rather than the one
  that stopped existing.
- **Continuity becomes a stated contract, not one runtime's mechanics.**
  `--resume` is claude-code's implementation; the contract names only an opaque
  handle and an obligation — continue the context this handle names, or report
  that you could not. Where the session lives is the runtime's business: files on
  a volume, a thread id at a vendor API, rows in a database. The manager stores
  the handle, returns it, and interprets nothing.
- **A run reports whether it continued what it was asked to**, so the manager
  knows the difference between "continued" and "started over" instead of
  inferring it from a handle it cannot verify. A runtime that cannot continue
  anything conforms by always reporting a new context, rather than silently
  starting over and looking identical to one that lost it.
- **Continuity is promised only where it is possible.** `AgentRuntime` declares
  where its context lives — on its home volume, externally, or nowhere — and the
  manager checks that before promising anything. An install with no durable home
  volume is single-run BY DECLARATION, stated from the first message, and answers
  each one fresh. Never-promised is a configuration; promised-and-lost is a loss,
  and only the second fails.
- **Unavailability is treated as an outage before it is treated as a loss.**
  Bounded retry in the runtime for a blip; a manager-side circuit breaker across
  conversations for an outage, which HOLDS affected work instead of failing it.
  Without the breaker, a two-minute storage incident would permanently destroy the
  context of every active conversation at once — worse, and less reversible, than
  the silent degradation this change replaces.
- **A context that cannot be continued FAILS the run** — once retries are
  exhausted and the breaker is closed, so the evidence points at this conversation
  rather than at the infrastructure. The runtime stops answering without memory: it reports an explicit reason, the bound threads are
  told what happened and that a new conversation is the remedy, and the
  conversation records that it can no longer be continued. A conversation without
  its context is not that conversation — it is a new one wearing its name and its
  thread, and presenting the second as the first is what this stops. It also saves
  the second agent invocation the current fallback spends on an answer that should
  not be given.
- **Lost context becomes a visible fact**, as a `ContextContinuity` condition on
  the Conversation naming when continuity broke and the reason the runtime gave.
- **The handle is recorded even when a run fails**, provided the agent got far
  enough to establish one. Today a crashed run leaves a session file with no
  reference, so the next message cannot continue it and the work is orphaned on
  disk.
- **The failure reason comes from the runtime, recorded verbatim.** The manager
  does not know where a given runtime keeps sessions, so it must not diagnose. The
  reference runtime is what reports "session files under /data/home were not
  found; this install has no home volume" — which matters because with the default
  idle TTL of one minute, an ephemeral install loses context every few minutes as
  normal operation, and that should read as a configuration choice rather than a
  malfunction.

## Capabilities

### New Capabilities

- `conversation-context-continuity`: the guarantee itself — what carries an
  agent's context between runs, when it can be lost, how loss is recorded, and
  what the system does next so a single loss does not become permanent.

### Modified Capabilities

- `runtime-workspace-persistence`: its requirement that "a resume whose session
  files are gone SHALL say so" gains a place to say it — the reply is not the
  only audience, and a warning that scrolls past is not a record.

## Impact

- **Contract**: `docs/contracts.md` — the session handle's semantics and the
  continuity report, stated so a non-claude runtime can implement them.
- **API**: `Conversation.status.sessionId` renamed to `runtimeContextId`, read
  dual-field for one release; **BREAKING** for anything reading the old field,
  including the console.
- **Manager**: `internal/httpapi/server.go` (handle update rule, condition),
  `internal/dispatch/dispatch.go` (unchanged resume decision, but it now acts on
  an id that is kept correct), `api/v1alpha1/conversation_types.go` (the
  condition; no new spec fields).
- **Runtime**: `runtime-claude/runtime.js` reports which context it actually ran
  and whether it fell back, reports the reason when it could not continue, and
  surrenders the handle even on failure.
- **Console**: `console/conversations.go` and the SPA read the renamed field.
- **Docs**: `docs/contracts.md` (the work contract's session semantics),
  `docs/concepts.md` (what continuity depends on).
- **Depends on**: `persistence-in-chart` (archived) — the home volume is what
  makes continuity possible FOR THE REFERENCE RUNTIME; this change makes continuity
  observable and self-correcting for any runtime, whatever its storage.
- **Out of scope — deliberately**: mirroring the agent's transcript into
  Kubernetes objects. Tool calls and model responses live in the session file on
  the volume and stream to pod logs; copying them into a CR would put unbounded
  text in etcd, which is the problem `ConversationInput` already exists to avoid.
  This change guarantees the agent keeps its context, not that the API server
  stores a copy of it.
- **Also out of scope**: context-window exhaustion on a very long conversation.
  Compaction belongs to the agent CLI, and the operator should not second-guess it.
