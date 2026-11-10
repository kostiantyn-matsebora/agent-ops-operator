## MODIFIED Requirements

### Requirement: /exit acts only when the conversation needs no worker

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

The conversation needs a worker (`dispatch.NeedsWorker`), so the pod would be
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

## ADDED Requirements

### Requirement: /exit is a task and the reply means accepted

A `/exit` typed in a conversation's thread SHALL arrive as an inbound chat
message. The manager SHALL accept it and enqueue one `exit` task on the
conversation's root key. The pod SHALL be deleted when the task runs.

The reply to the request SHALL mean the command was accepted, not that the pod
is gone. The reply to the person on the thread, stating what happens to the
conversation's memory, SHALL be posted when the task has run.

The `exit` handler SHALL be idempotent. A task delivered twice on a conversation
with no pod SHALL report that there was nothing to release and succeed.

#### Scenario: An exit is accepted before it is done

- **WHEN** `/exit` is sent in an idle conversation's thread
- **THEN** the inbound message is accepted, an `exit` task is enqueued, and the pod is deleted when the task runs

#### Scenario: A redelivered exit changes nothing twice

- **WHEN** an `exit` task is delivered a second time after its pod was already deleted
- **THEN** the task succeeds, reports there was nothing to release, and no error is raised
