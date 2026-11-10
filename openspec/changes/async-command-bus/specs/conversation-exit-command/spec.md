## RENAMED Requirements

- FROM: `### Requirement: /exit acts only when the conversation needs no worker`
- TO: `### Requirement: /exit acts only when the conversation needs no agent runtime`

## MODIFIED Requirements

### Requirement: /exit acts only when the conversation needs no agent runtime

`/exit` SHALL release the pod only when the conversation has nothing inflight
and nothing queued — the SAME predicate that makes a pod evictable.

That predicate SHALL have ONE definition in the codebase, shared by the
eviction path and this command, so the two cannot drift into different
meanings of "idle".

The predicate SHALL be evaluated when the `exit` task runs, not when the
command was accepted.

A refusal SHALL be reported by the task with the code `precondition_failed`,
which is not retried. A run that ends later never turns an old refusal into a
surprise release.

A `/exit` while a run is in progress SHALL be REFUSED, naming the run and
pointing at `/close` for the case where the intent is to abandon it.

Killing the pod mid-run would clear the inflight state and re-dispatch the
input. Work that may already have acted would run a second time, the one
outcome a release must never produce.

A `/exit` with queued input SHALL be REFUSED and say why.

The conversation needs an agent runtime (`dispatch.NeedsWorker`), so the pod would be
recreated immediately and nothing would be freed.

The refusal SHALL reach the person on the thread as a notice naming its reason,
exactly as before.

#### Scenario: A run is in progress

- **WHEN** an `exit` task runs while the conversation has an inflight run
- **THEN** no pod is deleted, the task is dead as `precondition_failed` without
  a retry, and the thread's notice names the run and offers `/close` for
  abandoning it

#### Scenario: Work is queued

- **WHEN** an `exit` task runs while inputs are waiting to be dispatched
- **THEN** no pod is deleted, the task is dead as `precondition_failed`, and the
  notice says the conversation still has work to do

#### Scenario: The predicate is singular

- **WHEN** the condition for releasing a pod is evaluated by `/exit` and by the
  eviction path
- **THEN** both call the same exported helper, and neither restates it

#### Scenario: No run is repeated

- **WHEN** any `/exit` is processed
- **THEN** no inflight run is cleared and no input is re-dispatched as a result

#### Scenario: A refusal is not retried into a release

- **WHEN** an `exit` task was refused for an inflight run and the run then ends
- **THEN** no pod is deleted, because the task is dead and is not retried

### Requirement: The reply states what happens to the conversation's memory

The reply on the thread SHALL distinguish releasing a runtime from ending a
conversation, and SHALL say what becomes of the context. It is posted when the
`exit` task has run, never at accept.

Where the conversation's runtime can carry context across a pod loss — decided
by the runtime's `contextStorage` against the configured context volume, which the
manager already computes — the reply SHALL say the conversation keeps its
context and resumes on the next message.

Where it cannot, the reply SHALL WARN that the next message starts fresh. This
is the loss the idle TTL would have caused anyway; stating it at the moment
someone chooses it is what stops it being discovered later as a fault.

When no runtime pod is running, `/exit` SHALL report that there was nothing to
release rather than erroring.

#### Scenario: Continuity survives the release

- **WHEN** `/exit` succeeds on an install whose runtime keeps context on a home
  volume that is provided
- **THEN** the reply says the conversation is still open and keeps its context

#### Scenario: Continuity does not survive the release

- **WHEN** `/exit` succeeds where the runtime keeps context on a volume this
  install does not provide, or declares `contextStorage: none`
- **THEN** the reply warns that the next message starts a fresh context

#### Scenario: Nothing is running

- **WHEN** `/exit` arrives for a conversation with no runtime pod
- **THEN** the reply says there was nothing to release, and no error is reported

#### Scenario: It is not mistaken for /close

- **WHEN** any `/exit` reply is read
- **THEN** it states that the conversation and its thread remain open

## ADDED Requirements

### Requirement: /exit is a task and the reply means accepted

A `/exit` typed in a conversation's thread SHALL arrive as an inbound chat
message. The manager SHALL accept it and enqueue one `exit` task on the
conversation's root key. The pod SHALL be deleted when the task runs.

The accept reply SHALL be `202` with the invocation id and the task id, as
`async-operations` states. It means accepted, not that the pod is gone. The
reply to the person on the thread, stating what happens to the conversation's
memory, SHALL be posted when the task has run.

The `exit` handler SHALL be idempotent. A task delivered twice on a conversation
with no pod SHALL report that there was nothing to release and succeed.

#### Scenario: An exit is accepted before it is done

- **WHEN** `/exit` is sent in an idle conversation's thread
- **THEN** the inbound message is accepted, an `exit` task is enqueued, and the pod is deleted when the task runs

#### Scenario: A redelivered exit changes nothing twice

- **WHEN** an `exit` task is delivered a second time after its pod was already deleted
- **THEN** the task succeeds, reports there was nothing to release, and no error is raised
