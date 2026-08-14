## Why

A runtime pod holds its slot until it times out. The default TTL is one minute,
but the installs that most need the slot back are the ones that raise it — a
long TTL is how you avoid re-cloning a large repo and re-warming a model on
every message, and the price is that an agent that answered ten minutes ago is
still holding a pod, a workspace checkout, and — on a local-model runtime — the
GPU memory its model is resident in.

Eviction already covers ONE half of this: when a conversation is waiting for
capacity, the manager evicts the longest-idle pod to admit it. The half with no
answer is when nothing is waiting yet. Nobody is blocked, so nothing evicts, and
an operator who knows a conversation is finished has no way to say so short of
`/close`, which DELETES the conversation and archives its thread. Ending a
thread to reclaim memory is the wrong trade, and it is currently the only one on
offer.

`/exit` is the missing half: release the runtime, keep the conversation.

## What Changes

- **A new chat command, `/exit`**, intercepted on the reply path exactly as
  `/close` is — before the text can become an input for the agent. It deletes
  the conversation's runtime pod and nothing else.
- **The conversation survives**: its object, its thread bindings, its inputs and
  its context handle are untouched. The next message admits it again, gets a
  fresh pod, and resumes where it left off — the same behavior an eviction
  already produces, now reachable by a person.
- **The slot frees immediately**, through machinery that already exists:
  capacity is counted from live pods, and the runtime-pod DELETE watch promotes
  the FIFO-first waiting conversation. No new scheduling path.
- **`/exit` acts only when the conversation needs no worker** — nothing
  inflight, nothing queued. That is the SAME predicate that already defines an
  evictable pod, and it is shared rather than restated. A `/exit` during a run
  is refused with the run named and `/close` offered; a `/exit` with queued
  input is refused because the pod would come straight back.
- **The reply tells the truth about continuity.** The manager already knows
  whether a conversation's runtime can carry context across a pod loss
  (`ContinuityPossible()`: `contextStorage` against the configured home volume).
  Where it can, `/exit` says the conversation keeps its memory. Where it cannot,
  it warns that the next message starts fresh — the loss the idle TTL would have
  caused anyway, stated at the moment someone chooses it rather than discovered
  later.
- **`/exit` on a general surface answers with usage**, like `/close`: there is
  no conversation there to release.
- **The command is discoverable**: `/agents` names `/exit` and `/close` with
  the one-line difference between them.

Not in scope: a console button (the console is a channel adapter, so `/exit`
typed there works through the same path), an HTTP endpoint, an exit-when-idle
deferral for busy conversations, and any change to eviction or the cap.

## Capabilities

### New Capabilities

- `conversation-exit-command`: the `/exit` command — what it releases, what it
  preserves, when it refuses, what it reports about continuity, and how it
  behaves off a conversation thread.

### Modified Capabilities

- `conversation-capacity`: capacity release is no longer only automatic. The
  requirement covering idle release gains the user-triggered form — same
  predicate, same "delete only the pod" semantics, same resumption — so the two
  paths cannot drift into two different meanings of "idle".

## Impact

- `internal/chat/router.go`: `/exit` recognition beside `isCloseCommand`, a
  `ReleaseRuntime` path, and the general-surface usage answer; `/agents` help
  text gains a line.
- `internal/dispatch`: the "needs a worker" predicate becomes an exported
  helper so the router and the reconciler share ONE definition of busy. The
  controller's private `needsWorker` is replaced by it — behavior identical,
  definition singular.
- `internal/chat`: the `Router` gains the bootstrap runtime `Config` (the same
  value `httpapi.Server` already holds) so it can answer the continuity
  question honestly, wired in `cmd/manager/main.go`.
- **No CRD change, no work-contract change, no new activity event kind** — idle
  eviction emits none today, and a user-triggered eviction should not be the one
  thing that does.
- Docs: `docs/concepts.md` (capacity section, beside eviction) and
  `docs/contracts.md` only where the chat command list lives; `CLAUDE.md`
  terminology gains the `/exit` vs `/close` distinction, since confusing them
  costs a thread.
- Nothing changes for an install that never types the command.
