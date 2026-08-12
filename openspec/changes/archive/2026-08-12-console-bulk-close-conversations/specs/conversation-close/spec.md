## ADDED Requirements

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
