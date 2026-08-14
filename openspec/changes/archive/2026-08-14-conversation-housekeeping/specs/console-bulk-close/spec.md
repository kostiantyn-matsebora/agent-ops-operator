## ADDED Requirements

### Requirement: Closed conversations are presented as a state, not an absence
The console SHALL list a `Closed` conversation, label it, and distinguish it
from one held by its close-topics finalizer on the way out. A closed
conversation keeps its recorded answers and costs no runtime pod and no
capacity, so it is a row worth showing rather than a gap in the list.

A closed conversation SHALL NOT be typeable. That decision SHALL be read from
the conversation's PHASE — which is on the CR and survives everything — and not
from console-process state alone. The console's own record that a thread was
archived lives in memory for one session, so a console restart would otherwise
put the composer back on every closed conversation and let a person type into
one that answers each message with "this conversation is closed".

#### Scenario: A closed conversation is listed and labelled
- **WHEN** the conversation list is rendered after a close
- **THEN** the conversation appears with a `Closed` label, distinct from the `closing` label of one being deleted

#### Scenario: A restarted console still refuses to type into a closed conversation
- **WHEN** a console process that never observed the close renders a `Closed` conversation
- **THEN** it is reported as archived and offers no composer

### Requirement: A closed conversation can be reopened from its row
The console SHALL offer reopen on a `Closed` conversation, per row. Reopening
SHALL be reached through a manager verb; the console SHALL perform no Kubernetes
write. The manager's own refusal — a missing profile or channel — SHALL be
passed through rather than flattened, so the reason names the missing object.

There SHALL be NO bulk reopen. A reopen re-materialises threads on every bound
channel, so a batch of them would announce itself on surfaces nobody is
watching; it is a decision about one conversation.

Once reopened, the conversation SHALL be typeable again from the console.

#### Scenario: A closed row offers reopen and comes back
- **WHEN** an operator reopens a closed conversation from its row
- **THEN** it returns to `Idle` and its console thread accepts messages again

#### Scenario: A failed reopen names what is missing
- **WHEN** the manager refuses a reopen because a referenced object is gone
- **THEN** the console shows that reason, naming the object

#### Scenario: No bulk reopen exists
- **WHEN** several closed conversations are selected
- **THEN** the console offers no reopen action over the batch

### Requirement: Bulk delete stands beside bulk close and refuses live conversations
The console SHALL offer a bulk delete over explicitly selected conversations,
mirroring bulk close in every mechanical respect: the same 50-name bound
enforced server-side, the same selection over the rows on screen, the same
per-item outcomes (`deleted` / `skipped` / `failed`) with reasons, the same
never-abort walk, and the same write gate, identity and logging.

A name that is not already `Closed` SHALL be reported `skipped` with a reason
naming the missing step, and SHALL NOT be closed on the way through. One call
doing the irreversible thing to a conversation that was still working, behind a
confirmation naming only the delete, is what the two-step prevents.

The confirmation SHALL name what is destroyed — the recorded answers, which are
the only durable copy of what the agent said, and the workspace on disk — and
SHALL state that it cannot be undone. It SHALL point at closing as the
reversible alternative.

Deleting SHALL be reached through a manager verb; the console SHALL perform no
Kubernetes write.

#### Scenario: A mixed delete batch reports per item
- **WHEN** a batch names both closed and live conversations
- **THEN** the closed ones are deleted, the live ones are `skipped` with "close it first", and the request succeeds with per-item outcomes

#### Scenario: A live conversation in a delete batch is untouched
- **WHEN** a live conversation is named for deletion
- **THEN** it is neither deleted nor closed, and nothing is posted to its threads

#### Scenario: The confirmation names the cost
- **WHEN** the delete confirmation is shown
- **THEN** it names the recorded answers and the workspace, states that it cannot be undone, and offers closing as the reversible alternative

#### Scenario: The bound is server-enforced
- **WHEN** a client sends more than the page size of names to delete
- **THEN** the request is refused regardless of what the selection UI allowed
