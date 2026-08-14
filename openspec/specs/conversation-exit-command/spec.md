# conversation-exit-command Specification

## Purpose
Releasing a conversation's runtime on demand: what `/exit` frees, what it
must leave standing, when it refuses, and what it reports about the
conversation's memory. The counterpart to automatic eviction, for the case
eviction cannot serve — nothing waiting, so nothing evicts.
## Requirements
### Requirement: /exit releases the runtime and keeps the conversation

`/exit`, sent in a conversation's own thread, SHALL delete that conversation's
runtime pod and nothing else. The Conversation object, its thread bindings, its
inputs, its run history and its stored context handle SHALL be untouched.

It SHALL be intercepted on the reply path BEFORE the text becomes an input, for
the same reason `/close` is: handing it to the agent would both dispatch a work
unit for a command and leave the pod running.

The next input SHALL admit the conversation again and give it a fresh runtime
pod, resuming from its context handle — the behavior an eviction already
produces. `/exit` SHALL NOT delete the conversation, archive its thread, or
cancel anything queued.

Recognition SHALL match `/close`'s: the bare command and its bot-suffixed form
(`/exit@SomeBot`), and trailing text SHALL disqualify it — "exit the
maintenance window when you are done" is an instruction for the agent, not a
command for the manager.

#### Scenario: An idle conversation releases its runtime

- **WHEN** a person sends `/exit` in the thread of a conversation whose runtime
  pod is running and which has nothing inflight and nothing queued
- **THEN** the runtime pod is deleted
- **AND** the Conversation still exists with its threads, inputs and context
  handle unchanged

#### Scenario: The conversation resumes afterwards

- **WHEN** a message arrives in that thread after an `/exit`
- **THEN** a fresh runtime pod is created and the run continues the same
  context, exactly as after an eviction

#### Scenario: The command never reaches the agent

- **WHEN** `/exit` is sent in a conversation thread
- **THEN** no input is appended and no work unit is dispatched for it

#### Scenario: Trailing text is not the command

- **WHEN** a person sends `/exit the node from the cluster once it drains`
- **THEN** it is treated as an ordinary reply to the agent and no pod is deleted

### Requirement: /exit frees the capacity slot through the existing path

Releasing the pod SHALL free the conversation's capacity slot through the
mechanisms that already exist: "active" is counted from live runtime pods, and
the runtime-pod DELETE watch wakes waiting conversations, which each make their
own FIFO admission decision.

No new scheduling path, priority, or reservation SHALL be introduced. A
conversation waiting in `Pending` SHALL be admitted by the ordinary FIFO rule
once the slot is genuinely free.

The value of the release is the case where NOTHING is waiting. Automatic
eviction already frees an idle pod for a conversation that IS waiting — at the
cap, admission evicts the longest-idle evictable pod — so promoting a waiter is
not a property `/exit` adds, and must not be claimed as one. What `/exit` adds is
the interval no waiter ends: with nobody blocked, nothing evicts, and the pod
holds its slot, its checkout and whatever its runtime keeps resident until the
idle TTL expires.

#### Scenario: A slot is released with nothing waiting

- **WHEN** an idle conversation holds the only slot, nothing is `Pending`, and
  `/exit` is sent in its thread
- **THEN** its pod is deleted immediately, and the next conversation created is
  admitted outright rather than parked

#### Scenario: Promotion is undisturbed

- **WHEN** the cap is reached, one conversation is `Pending`, and an idle
  conversation is released with `/exit`
- **THEN** the pending conversation is admitted by the ordinary path — a
  non-regression on the interaction, not evidence for the command

#### Scenario: FIFO order is unchanged

- **WHEN** several conversations are `Pending` and one slot is released
- **THEN** the oldest by creation time is admitted, exactly as for any other
  freed slot

#### Scenario: The slot is counted from pods

- **WHEN** the released pod has been deleted but has not finished terminating
- **THEN** it still counts as active until it is gone, so the cap is never
  exceeded by the release

### Requirement: /exit acts only when the conversation needs no worker

`/exit` SHALL release the pod only when the conversation has nothing inflight
and nothing queued — the SAME predicate that makes a pod evictable. That
predicate SHALL have ONE definition in the codebase, shared by the eviction path
and this command, so the two cannot drift into different meanings of "idle".

A `/exit` while a run is in progress SHALL be REFUSED, naming the run and
pointing at `/close` for the case where the intent is to abandon it. Killing the
pod mid-run would clear the inflight state and re-dispatch the input, running
work that may already have acted a second time — the one outcome a release must
never produce.

A `/exit` with queued input SHALL be REFUSED and say why: the conversation needs
a worker, so the pod would be recreated immediately and nothing would be freed.

#### Scenario: A run is in progress

- **WHEN** `/exit` arrives while the conversation has an inflight run
- **THEN** no pod is deleted, and the reply names the run and offers `/close`
  for abandoning it

#### Scenario: Work is queued

- **WHEN** `/exit` arrives while inputs are waiting to be dispatched
- **THEN** no pod is deleted, and the reply says the conversation still has work
  to do

#### Scenario: The predicate is singular

- **WHEN** the condition for releasing a pod is evaluated by `/exit` and by the
  eviction path
- **THEN** both call the same exported helper, and neither restates it

#### Scenario: No run is repeated

- **WHEN** any `/exit` is processed
- **THEN** no inflight run is cleared and no input is re-dispatched as a result

### Requirement: The reply states what happens to the conversation's memory

The reply SHALL distinguish releasing a runtime from ending a conversation, and
SHALL say what becomes of the context.

Where the conversation's runtime can carry context across a pod loss — decided
by the runtime's `contextStorage` against the configured home volume, which the
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

### Requirement: /exit off a conversation thread answers with usage

`/exit` reaching the manager anywhere other than a conversation's own thread —
a chat surface's general area, where there is no conversation to release —
SHALL answer with usage and SHALL release nothing and create nothing.

It SHALL NOT be answered as an unknown agent: typing it on a general surface is
an obvious mistake, not a mistyped pipeline name. It SHALL NOT originate a
conversation.

#### Scenario: Typed on a general surface

- **WHEN** a person sends `/exit` on a chat surface's general area
- **THEN** the reply explains that `/exit` releases a conversation's runtime and
  must be sent inside that conversation's thread
- **AND** no Conversation is created and no pod is deleted

#### Scenario: A pipeline is not shadowed accidentally

- **WHEN** `/exit` is parsed
- **THEN** it is handled as a manager command before any Pipeline lookup, the
  same way `/close` is

### Requirement: The command is discoverable

The `/agents` listing SHALL name `/exit` alongside `/close` with a one-line
statement of the difference: `/exit` releases the runtime and keeps the
conversation, `/close` ends the conversation and archives the thread.

Two commands one word apart, with one destroying a thread and one not, are
exactly the pair a person must not have to guess between.

#### Scenario: A person looks up the commands

- **WHEN** a person sends `/agents`
- **THEN** the reply names both `/exit` and `/close` and states what each does
