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

### Requirement: Closing has exactly one implementation
Every close — however it is ordered — SHALL take the same path: a farewell posted
to every bound thread, deletion of the `Conversation`, the close-topics finalizer
archiving those threads, owner references reclaiming the inputs and the MCP
ConfigMap, and the freed capacity admitting a waiting conversation. There SHALL
be exactly one implementation of that sequence, and every originator SHALL call
it rather than reproduce it.

The originators are the `/close` command on a thread, a surface closing several
conversations at once, and the manager itself closing on its own schedule. What
differs between them is only WHO decided and what the farewell says; a second
implementation would be free to drift from the first on any step above, and the
drift would be found in production.

**NO REMOTE CLOSE VERB EXISTS.** No HTTP endpoint, no channel adapter contract
operation and no CRD field ends a conversation: an external caller reaches
closing only by posting `/close` on a thread it holds. A manager-internal close
is not such a verb — nothing outside the manager can ask for it — which is what
lets the manager close on a timer without giving any caller a way to.

#### Scenario: A batch close is N ordinary closes
- **WHEN** a surface closes several conversations in one gesture
- **THEN** each conversation receives `/close` on that surface's thread with it and takes the ordinary close path

#### Scenario: Teardown is identical whatever ordered the close
- **WHEN** a conversation is closed by a batch, by a hand-typed `/close`, or by the manager on its own schedule
- **THEN** its threads are archived by the finalizer, its runtime pod and MCP ConfigMap are garbage collected, and freed capacity admits a waiting conversation — identically in all three cases

#### Scenario: Every close says goodbye
- **WHEN** a conversation is closed by any originator
- **THEN** a farewell reaches every bound thread before the object disappears, so an archived thread never reads as one that merely stopped

#### Scenario: No remote close verb exists
- **WHEN** an external caller looks for an endpoint or contract operation that ends a conversation
- **THEN** there is none: it can only post `/close` on a thread it holds

### Requirement: A surface's close reach is bounded by the threads it holds
A channel surface SHALL be able to close only the conversations it holds a thread
on. A conversation it merely observes SHALL NOT be closeable from it, because
`/channel/inbound` is reply-only and there is no thread to post the command on.
This SHALL be reported as a bounded reach — naming the binding that would extend
it — and never as a permission error.

The bound applies to SURFACES, which reach closing by posting a command. It does
not describe the manager, which holds no threads and closes conversations
directly; the distinction is the same one that makes a remote close verb absent
while a scheduled close is possible.

#### Scenario: An observed conversation cannot be closed
- **WHEN** a surface attempts to close a conversation it holds no thread on
- **THEN** the close does not happen and the reason names the missing channel binding

#### Scenario: Reach follows the binding
- **WHEN** a channel is added to a conversation's pipeline and a thread is bound
- **THEN** that surface can close the conversation

