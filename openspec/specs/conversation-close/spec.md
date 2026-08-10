# conversation-close Specification

## Purpose
TBD - created by archiving change limit-active-conversations. Update Purpose after archive.
## Requirements
### Requirement: /close ends a conversation from its thread
A message whose text is the command `/close` (optionally bot-suffixed, e.g.
`/close@SomeBot`) sent in a conversation's thread SHALL end that conversation:
the manager SHALL intercept it on the reply path BEFORE it becomes a reply
input, post a farewell message to every bound thread, and delete the
`Conversation`. The command SHALL be accepted from any sender able to post in
the thread — no additional authorization is required.

#### Scenario: Close deletes the conversation
- **WHEN** a user sends `/close` in a conversation's thread
- **THEN** the `Conversation` object is deleted and no reply input is appended to it

#### Scenario: Close is not handed to the agent
- **WHEN** a user sends `/close` in a thread
- **THEN** no runtime work unit is dispatched for that text

#### Scenario: Bot-suffixed form works
- **WHEN** a user sends `/close@AgentOpsBot` in a thread
- **THEN** the conversation is closed exactly as for `/close`

### Requirement: Closing tears down the conversation's resources
Deleting a `Conversation` SHALL remove its runtime pod and its
`agentops-mcp-conv-<name>` ConfigMap through existing owner references, and its
`ConversationInput` payload objects SHALL be cleaned up. The freed capacity
SHALL become available to any waiting `Pending` conversation.

#### Scenario: Runtime pod and ConfigMap are garbage collected
- **WHEN** a conversation with a runtime pod and an MCP ConfigMap is closed
- **THEN** both objects are removed

#### Scenario: Closing admits waiting work
- **WHEN** the cap is full, a conversation is closed, and a `Pending` conversation is waiting
- **THEN** the pending conversation is admitted

### Requirement: Closing a working conversation abandons its inflight work
`/close` SHALL be honored immediately even when the conversation is inflight:
the runtime pod is removed mid-run and the farewell message SHALL state that
in-progress work was abandoned.

#### Scenario: Inflight run is abandoned with notice
- **WHEN** `/close` is sent while a work unit is inflight
- **THEN** the conversation is deleted and the farewell message says the in-progress work was abandoned

### Requirement: Topics are archived before the conversation object disappears
The manager SHALL hold the deleting conversation with a finalizer while it
enqueues one `close-topic` operation per bound thread, and SHALL release the
finalizer once those operations complete or after a bounded grace period of 2
minutes, whichever comes first. Deletion SHALL therefore never wedge on an
adapter that is down or that does not implement the operation. Deletion by any
means — the `/close` command or a direct `kubectl delete` — SHALL take this
path.

`close-topic` SHALL be the ONLY operation kind that is not re-derivable from CR
state: its failure is logged rather than written as a condition, and it is never
regenerated, because the object that would carry the obligation is on its way
out. The finalizer is what keeps the derivability rule true while one is
outstanding. Every other operation kind, `send` included, SHALL be derivable and
SHALL be re-enqueued by reconciliation after a manager restart.

#### Scenario: Topic archived on close
- **WHEN** a conversation bound to a channel with a thread is closed
- **THEN** a `close-topic` operation carrying that thread id is delivered to the serving adapter

#### Scenario: Down adapter cannot block deletion
- **WHEN** no adapter claims the `close-topic` operation within the grace period
- **THEN** the finalizer is removed and the conversation object is deleted anyway

#### Scenario: Manual deletion archives too
- **WHEN** an operator runs `kubectl delete conversation <name>`
- **THEN** the bound threads receive `close-topic` operations before the object disappears

#### Scenario: Close-topic is not regenerated after a restart
- **WHEN** the manager restarts while a `close-topic` operation is outstanding and the grace period has expired
- **THEN** the operation is not re-enqueued and the conversation object is deleted, leaving at most an open topic a person can close by hand

### Requirement: /close on a general surface answers with usage
`/close` sent on a channel's general surface (no thread) SHALL be answered with
usage explaining that the command works inside a conversation's thread, and
SHALL NOT be reported as an unknown pipeline and SHALL NOT create a
conversation.

#### Scenario: General-surface close is explained
- **WHEN** a user sends `/close` on a channel's general surface
- **THEN** the reply explains that `/close` is used inside a conversation's thread and no conversation is created

