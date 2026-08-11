## Context

Three records exist for one conversation, and conflating them is the first way
to get this design wrong:

| Record | Where | Holds | Authoritative for |
|---|---|---|---|
| Agent session | claude-code files under `$HOME/.claude/projects/-data-workspace/` on `/data/home` | every message, tool call and model response | **the agent's context** |
| Thread transcript | the chat surface, via bound channels | what a human said and was told | the human-visible history |
| Run history | `Conversation.status.runs[]` | outcome + result, truncated to 2000 chars | what the operator knows |

The user-visible guarantee — "continue with full context" — is a property of the
**first** row only. The others are summaries and neither can reconstruct it.

Today's flow: dispatch sets `ResumeSessionID` from `status.sessionId` for `reply`
and `recurrence` inputs; the runtime passes `--resume`; a resume whose session is
missing is retried once without it, which produces a **new** session with a new
id. Ingest separately converts a repeat signal to `recurrence` only when
`status.sessionId` is non-empty.

So that one field is load-bearing three times over: it decides whether an input
is classified as continuing work, whether the runtime is told to continue, and
which context it continues. It is written once and never corrected — and it is
named after one runtime's implementation, which is where D0 starts.

## Goals / Non-Goals

**Goals:**

- A conversation continues the context that exists, not the one it first saw.
- A single unrecoverable loss does not become a permanent one.
- Context loss is a recorded fact, not a sentence in a reply.
- The operator can tell "answered with memory" from "answered without" — and
  nobody is answered without memory in the first place.

**Non-Goals:**

- Storing the transcript in Kubernetes objects. Unbounded text in etcd is the
  thing `ConversationInput` exists to prevent.
- Reconstructing a lost session from run history. The 2000-character results are
  a summary; replaying them as context would fabricate a history that reads
  authentic and is not.
- Managing the model's context window. Compaction is the CLI's job.
- Cross-conversation memory. A conversation is the unit of context by design.

## Decisions

### D0: The handle is named in agent-ops' vocabulary, not the runtime's

`Conversation.status.sessionId` becomes `status.runtimeContextId`, and the work
contract's `resumeSessionId` becomes `runtimeContextId` to match.

*Why:* "session" is a runtime's word. claude-code has sessions; another backend
has threads, or conversations of its own, or no such concept at all. Agent-ops'
bounded context has **Conversations**, and what a runtime returns is its own
handle for that conversation's accumulated context. Carrying one vendor's noun in
the operator's API is the same leak as carrying its flags — it just happened
earlier and stopped being visible.

*Why not `runtimeConversationId`:* it is the more literal name, but it would sit
in `Conversation.status` beside a Conversation whose `metadata.name` is also an
identifier. Two ids, both called conversation-something, one of them not
agent-ops'. `context` says what is actually preserved and matches the
`ContextContinuity` condition.

*Alternatives:* keep `sessionId` and document that it is opaque (rejected — the
name is what teaches the next reader, and the last two decisions here exist
because vendor vocabulary had already leaked); `contextRef` (rejected — `Ref` in
this API means a Kubernetes object reference, and this is an opaque string).

**The rename must not do the harm this change prevents.** A status field that
simply changes name is a handle every in-flight conversation loses on upgrade —
every one of them restarting its context once, which is precisely the failure
being fixed. So the manager READS BOTH for one release, preferring the new field
and adopting the old one when only it is present, and WRITES only the new. The
work unit carries both fields for the same period so a runtime image can be
upgraded independently of the manager.

### D1: The context id is latest-wins

`/work/done` records whatever context id the run reports, replacing the stored
one.

*Why:* the current write-once rule assumes the first context is the only one,
which is false the moment a resume falls back. The stored id then names a session
that does not exist, and because dispatch and ingest both key off it, every later
message repeats the same failed resume. The bug is not that context was lost — an
evicted volume can do that — but that the system never recovers, and that recovery
costs one changed condition.

*Alternative:* keep write-once and clear the id when a resume fails (rejected —
same number of writes, and clearing loses the id of the session that is now
carrying the context).

### D2: The contract speaks CONTEXT HANDLES, never sessions or resume mechanics

`--resume` is claude-code's implementation, and `session` is claude-code's noun.
The contract names only an **opaque context handle** and an obligation about it:

- the manager stores whatever handle a run reports and hands it back on the next
  unit for that conversation;
- given a handle, a runtime SHALL either continue that context or report that it
  could not;
- **where that context lives is entirely the runtime's business** — session files
  under `$HOME`, a thread id at a vendor API, rows in a database. The manager
  never interprets the handle and never assumes a storage medium.

The work-done payload therefore distinguishes "continued the context you named",
"started a new one", and "started a new one AFTER the named context was
unavailable" — vocabulary that holds for any backend, naming neither a session
nor a flag.

*Why the report is needed at all:* the manager cannot infer this. It sends a
handle and receives a handle; when they differ it has no way to know whether the
agent branched deliberately or was forced to. The runtime knows, because it is
the component that tried.

One optional field, so a runtime that does not set it keeps working and is
treated as making no claim.

### D2a: A context that cannot be continued FAILS the run

On discovering that the named context is unavailable, the runtime SHALL NOT
start a fresh one and answer anyway. It reports the run as failed with an
explicit reason, and the manager marks the conversation as no longer continuable.

*Why:* a conversation without its context is not that conversation. It is a new
one wearing the same name and the same chat thread, and every party involved —
the person replying, the operator reading `kubectl`, the agent itself — is
entitled to know which one they are talking to. An agent asked to "roll that
back" with no memory of what it did will either guess or ask; on an install where
the runtime holds cluster-admin, the guess is the expensive outcome.

The reference runtime does the opposite today, and its reason is recorded in its
own comment: *"failing the reply outright is a worse [cost] ... it fails with an
EMPTY result because claude never reaches its result event, so the person who
typed sees 'failed' and no reason at all."* That objection is about the failure
being **inarticulate**, not about failing being wrong. This change makes the
failure explain itself — a stated reason, a message on the thread naming the
remedy — which removes the reason the fallback existed.

It also stops spending a second agent invocation on an answer that should not be
given: today the fallback re-runs the agent from scratch.

*The cost, and how much of it is avoidable:* a TRANSIENT unavailability would
fail a conversation the fallback would have limped through. That cost is real but
it is smaller than "the runtime cannot tell transient from permanent", because
the two look different at the storage layer:

| | shape | example |
|---|---|---|
| **Transient** | the store did not ANSWER | shared-filesystem hiccup — a Longhorn share-manager restart, a stale NFS handle after a pod moves, an attribute-cache lag that briefly hides a file written seconds ago on another node |
| **Permanent** | the store answered, and the context is NOT THERE | no home volume at all, the claim replaced, files pruned, a session format the CLI no longer accepts |

So the rule is **not** fail-on-first-miss. A runtime SHALL distinguish "the
context store says it is gone" from "the context store did not answer", and
SHALL retry briefly before declaring a context unavailable. For the reference
runtime that is a cheap check: the session files either exist on the mounted path
or they do not, and a lag resolves within seconds.

This matters most on exactly the deployment this project recommends. With an idle
TTL of one minute, runtime pods recycle constantly and land on arbitrary nodes, so
consecutive runs of one conversation routinely read a shared volume from different
nodes. Without the retry, a two-second visibility lag would end a conversation
permanently — turning a storage nicety into a correctness bug.

Note also what does NOT reach this path: a volume that fails to MOUNT never
starts the container, so there is no run, no report, and no continuity decision —
that surfaces as a pod-level failure instead.

What remains after the retry is genuine: the context is gone, and the run fails
rather than answering without it. The fallback made a permanent loss silent; this
makes it loud, and a loud failure is recoverable by starting a new conversation.

*Alternatives:* fail only for `reply` inputs and let a re-firing alert start
fresh (a real distinction — a repeat alert does not depend on prior context the
way a human's follow-up does — left as an open question rather than built);
a values flag to restore the old behaviour (rejected for now: the escape hatch
for "I want an answer without context" is to start a new conversation, which
costs nothing and is honest about what it is).

### D2c: Transient unavailability gets availability tactics, not a guess

Distinguishing "gone" from "did not answer" is a known problem class, and it is
solved with the standard tactics rather than with bespoke checks. Three apply
here, at different scopes, and fail-fast is the LAST of them rather than the
first.

**Retry — bounded, with backoff, in the runtime.** For a blip affecting one run:
a restarting share-manager, a stale handle, a file not yet visible from this
node. Bounded because a person is waiting on the other end of a chat reply, so
the budget is seconds, not minutes. This is the per-run tactic and it is all that
a single pod can meaningfully do — it is short-lived and sees only its own unit.

**Circuit breaker — in the manager, across conversations.** A pod cannot tell a
lost context from a broken filesystem; the manager can, because it sees every
run. Many conversations reporting a context unavailable inside one window is
evidence about the INFRASTRUCTURE, not about each conversation's context.

This is the tactic the design was missing, and its absence was a real hazard:
under fail-fast alone, a two-minute storage outage would report unavailability
for every active conversation and **permanently kill all of them**, irreversibly,
in the time it takes each to receive one message. A recoverable infrastructure
incident would become mass destruction of state, which is a strictly worse
failure than the silent-fallback behaviour this change set out to replace.

So: past a threshold of unavailability reports in a window, the breaker OPENS and
the manager stops treating them as loss. Affected inputs are HELD — requeued, not
failed — the condition says the breaker is open and why, and dispatch of
continuations pauses. It closes after a probe succeeds, and the held work
proceeds with its context intact.

**Fail fast — only once the other two have spoken.** A run fails for an
unavailable context when the runtime's retries were exhausted AND the breaker is
closed, meaning the rest of the system is continuing conversations normally. Then
this really is one conversation's loss, and D2a applies.

*Why the breaker is manager-side and windowed:* per-pod state is useless — a
runtime pod handles one unit and exits, so it has nothing to accumulate. The
manager already holds cross-conversation state for admission and cooldown, and
this is the same shape.

*Alternatives:* retry alone (rejected — it cannot see a systemic outage, only its
own unit); failing everything and relying on humans to restart conversations
(rejected — that is the mass-destruction case, and the work was recoverable);
holding work indefinitely with no breaker (rejected — an install whose volume is
genuinely gone would queue forever instead of telling anyone).

### D2b: Continuity has a declared PREREQUISITE, checked before it is promised

`AgentRuntime.spec` gains `contextStorage`: `volume` (the runtime keeps context
on its home volume), `external` (somewhere the operator does not provide, such as
a vendor API), or `none` (it cannot continue anything). The reference runtime
declares `volume`.

The manager checks it before promising continuity. When a runtime declares
`volume` and no durable home volume is configured, continuity is **impossible in
this deployment** — so the manager does not hand out a handle at all, and the
conversation says up front that it has no continuity.

*Why this is not the same failure as D2a:* there are two different situations that
fail-fast alone would collapse into one.

| | what happened | right response |
|---|---|---|
| **Never promised** | the deployment cannot carry context — ephemeral home, runtime declares `none` | run fresh, say so from the first message |
| **Promised and lost** | continuity was possible and the context is gone | FAIL the run (D2a) |

Without the distinction, an install with `persistence.enabled: false` — a
legitimate choice on a cluster with no RWX provisioner, and one the chart
explicitly supports — would fail **every** follow-up message, because with an idle
TTL of one minute the pod exits between almost any two messages. A supported
configuration would look like a broken product.

With it, that install is honest instead: every conversation is single-run by
declaration, visible before anyone types a second message, and no run fails for a
reason the operator already chose.

*Why the RUNTIME declares it rather than the chart inferring it:* the chart would
have to know which images need a volume, which is the same vendor knowledge D2
took out of the manager. A runtime that keeps context at a vendor API needs no
volume at all and must not be told it does.

*Chart consequence:* rendering a runtime that declares `volume` while persistence
is off is legal and reported — `NOTES.txt` states that conversations cannot be
continued in that configuration. It is NOT a render failure: the operator may
genuinely want a one-shot-per-message install, and refusing to render would take
that away.

### D2d: Making continuity matter obliges us to document how to have it

This change makes durable context load-bearing: with the prerequisite check
(D2b), an install without it is single-run by declaration. That is only fair if
having it does not require distributed storage, which many clusters do not have.

It already does not. Two supported topologies exist today and neither needs new
API:

| topology | how | who it is for |
|---|---|---|
| **Shared** | RWX claim, pods anywhere | Longhorn, EBS-backed RWX, NFS |
| **Single-node** | RWO claim (or a `local` PersistentVolume) + `nodeSelector` pinning runtime pods to one node | anything without a distributed provisioner |

`AgentRuntime.spec.nodeSelector` and the chart's `runtime.nodeSelector` already
exist, and `persistence.accessModes` is already a value. The single-node topology
is therefore a documentation and guard-rail gap, not a capability gap.

**The trap worth guarding:** an RWO claim with runtime pods NOT pinned works
until a second conversation schedules on another node, then fails with
`Multi-Attach` errors — at the moment of concurrency, far from the storage
setting that caused it. The notes should say so when RWO is configured without a
selector. Not a render failure: on a single-node cluster the selector is
unnecessary, and refusing to render there would be wrong.

*Why not add `hostPath` to `HomeVolume`:* it mounts a host filesystem path into
the pod that executes agent code — which in a `rbacMode: full` install already
holds cluster-admin, and in any install runs arbitrary tools. A `local`
PersistentVolume reaches the same disk with node affinity expressed through the
PV machinery instead, so the escape hatch for provisioner-less clusters exists
without handing an agent the node.

### D3: Loss is a condition, not a log line

An unavailable context sets a `ContextContinuity` condition on the Conversation,
naming when continuity broke and the reason the runtime gave. It clears if a
later run continues successfully.

*Why:* everything else about a conversation's health is a condition, and this is
the one failure that leaves the object looking perfectly healthy — same phase,
same runs, answers still arriving. The console and `kubectl` both read
conditions, so this makes "why does it not remember?" answerable without reading
a runtime pod's logs, which by then has probably exited.

The thread is still told, and now it is the failure that speaks: the person
reading it is the one most affected, they are not looking at conditions, and
"this conversation cannot be continued — start a new one" is more useful to them
than an answer given without memory.

### D4: Record the context id even when the run fails

A failed run still reports the context it established, if it got that far.

*Why:* today a crash after the session started leaves a session file that nothing
references. The next message cannot resume it, so the work the agent already did
is stranded on disk while the conversation starts over. Recording the id makes
the next message continue from where the failure happened, which is exactly what
a person retrying after an error expects.

*Alternative:* only record on success (rejected — that is the current behaviour
and it discards recoverable context precisely when the conversation is going
badly).

### D5: Continuity is not repaired by replaying run history

When context is lost, the next run starts genuinely fresh. The manager does NOT
seed it with previous results.

*Why:* there is no case in which run history is the right tool, which is clearest
split in two:

- **The session file exists** — `--resume` restores the entire transcript: every
  message, tool call, tool result and model response. `runs[].result` is a strict
  subset of what the agent already has, so adding it is redundant.
- **The session file is gone** — `--resume` is impossible, and run history cannot
  stand in for it. It is 2000 characters of final answers, with no tool calls and
  no intermediate reasoning.

Either the real context is available, or the substitute is inadequate — and the
substitute is worse than nothing, because an agent handed its own truncated
summaries treats them as memory and gives a confident, plausible, wrong account
of what it did. An agent that knows it lost the thread is safer than one that
half-remembers, and the condition plus the reply warning tell both the operator
and the human.

This is also why the change is not about *restoring* context. Resume is already
sufficient wherever it is possible; what fails today is the POINTER to the
session, not the session.

*Alternative:* hand the agent a summary **explicitly framed as not its own
memory** — "your prior context was lost; the following is an external summary you
did not write". This is a genuinely different proposal from silently seeding, and
on a long incident it might beat starting cold. It is left out because the
framing has to be exactly right or it collapses into the failure above, and
because the material available to summarise is thin. Worth revisiting only if
there is ever a real transcript to summarise, which the non-goals rule out.

### D6: The RUNTIME supplies the reason; the manager records it verbatim

When a run reports that it could not continue a session, it may also report WHY,
as free text. The manager stores that on the condition and adds nothing of its
own.

*Why:* an earlier draft of this design had the manager name "an ephemeral home
volume" as the likely cause. That is claude-code's storage model leaking into the
operator: a runtime whose sessions live at a vendor API has no home volume, and
the diagnosis would be confidently wrong. The manager does not know where any
given runtime keeps sessions, and it must not guess.

So the home-volume explanation moves to `runtime-claude`, which is the component
that knows its sessions are files under `$HOME` and can see whether that path is
backed by a claim. The reference runtime says "session files under /data/home
were not found; this install has no home volume", and a different runtime says
whatever is true for it.

The point being preserved is worth keeping: with the default idle TTL of one
minute, an ephemeral install loses context every few minutes as normal operation.
That is a legitimate configuration, and the operator should see it named rather
than conclude the agent is broken. It is simply the reference runtime's sentence
to write, not the manager's.

*Alternative:* a fixed enum of causes in the contract (rejected — it would have to
enumerate storage models the manager knows nothing about, and every new runtime
would need a new enum value or would misreport itself as the closest wrong one).

## Risks / Trade-offs

- **Latest-wins hides a resume that silently branched** → D2's explicit report is
  what distinguishes branch from fallback; without it, latest-wins alone would
  make the two indistinguishable.
- **A condition that flaps on a flaky volume** → it is set on fallback and cleared
  on the next successful resume, so it reflects the current state rather than
  accumulating history; the run that lost context is already recorded in `runs[]`.
- **Third-party runtimes do not report the new field** → absent means "no claim",
  and the manager keeps today's behaviour for those: record the id, set no
  condition. The contract addition must not make an existing runtime look broken.
- **Recording a session id from a failed run points at a partial session** →
  resuming a partial session is the desired outcome; the alternative is starting
  over, which is strictly worse and is today's behaviour.
- **Nothing here makes an evicted volume recoverable** → true, and the change
  does not claim to. It bounds the damage to one message instead of all of them.

## Migration Plan

The rename is the only step that can destroy what this change protects, so it is
sequenced to make that impossible rather than unlikely.

1. **Manager first.** It reads `runtimeContextId` and falls back to `sessionId`,
   writes only the new field, and sends BOTH on the work unit. Latest-wins and the
   condition ship here too. Every existing conversation is adopted on its next
   run with no restart, and the current runtime image keeps working unchanged
   because the unit still carries the old field.
2. **Runtime next, on its own schedule.** The image reads the new field, reports
   continued/new/new-after-unavailable with a reason, and surrenders the handle
   on failure. An older manager ignores the extra fields.
3. **Console with or after the manager** — it only reads the field for display.
4. **Drop the dual read one release later**, once no conversation can still carry
   only the old field. Removing it earlier is what would strand handles.
5. CRD regeneration is required: a renamed status field plus the new condition.
6. Rollback: revert either side independently — that is the point of ordering
   them this way. Reverting the manager after step 4 would lose handles, so that
   step is the point of no return and should be a separate release.

## Open Questions

- **A human-initiated recovery for a failed conversation — the strongest follow-up.**
  Fail-fast currently leaves the person with one option: start a new conversation
  and lose the thread. Two distinct actions would change that, and they are not
  the same feature:
  - **Retry the continuation** — a probe for the case the automatic tactics miss:
    an isolated unavailability that outlives the runtime's seconds-long retry but
    never trips the manager's breaker because it affected one conversation. Narrow
    but real. Against a context that is genuinely gone it fails identically every
    time, so it must present as a probe rather than a fix.
  - **Continue anyway, fresh** — accept the loss, KEEP THE THREAD, and start a new
    context inside it. This is the one with reach, because it hands the decision
    to the person the failure happened to.

  This revises a rejection made in D2a. A values flag for "answer without context"
  is wrong: it is set blanket, ahead of time, by someone who is not present when
  it matters, and it silently converts every future loss into a degraded answer.
  The same capability as a per-conversation action is right: informed, at the
  moment of failure, by the affected person, and recorded as their choice. The
  mechanism to reject was the flag, not the capability.

  The machinery exists — `/close` already proves reply-path command interception,
  and the console is a channel and could offer both as buttons. Deliberately not
  in this change: it is a user-facing interaction surface, and this change is
  about making continuity correct first. A conversation that can be recovered by
  hand is worth little if the system still loses context on every message.

  **Seeding that fresh context with a briefing** is the natural companion, and is
  the D5 alternative rather than the D5 rejection: acceptable only because it is
  post-failure, human-chosen, and labelled as something the agent did not write.
  Three limits shape what such a briefing could honestly contain:

  - **It is one side of the dialogue.** The operator does not retain user
    messages — `spec.inputs[]` is pruned once processed and the `ConversationInput`
    objects go with it, deliberately, to keep Conversation objects small. What
    survives is `runs[].result`: the agent's own answers, truncated to 2000
    characters, bounded to the last ten runs. The questions live in the chat
    thread, which is the channel's record and which no adapter contract call
    exposes to the manager.
  - **Results are CLAIMS, not records of action.** With no tool outcomes, "I
    scaled `api` to 3" cannot be distinguished from "I proposed scaling `api` to
    3". An agent holding broad cluster rights that treats the first reading as
    established fact is the specific hazard, so a briefing must be labelled as
    unverified and instruct the agent to confirm current state before acting on
    anything in it.
  - **It is the tail, not the history.** Ten truncated results from a long
    incident are its most recent surface, which may be the least useful part.

  A better briefing would need the operator to retain a bounded, truncated record
  of BOTH sides — a deliberate new record, weighed against the etcd-size reasoning
  that made inputs prunable in the first place. Worth deciding on its own merits
  rather than acquiring by accident as a side effect of recovery.



- **Should a conversation whose context was lost be renamed or split?** It is now
  two conversations wearing one name, and the thread reads as continuous. Leaning
  no — the thread IS the human's continuity even when the agent's broke — but it
  is the strongest argument the other way.
- **Should dispatch resume for `task` inputs on an existing conversation?**
  Today only `reply` and `recurrence` do. Task and chat are one-shot lanes that
  each open their own conversation, so it does not currently arise; it would if
  a future lane appended a task to an existing conversation.
- **Should the manager verify the session file exists before dispatching a
  resume?** It would turn a failed resume into a prevented one, but the manager
  does not mount the volume and must not start.
