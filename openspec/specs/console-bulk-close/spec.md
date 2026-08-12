# console-bulk-close Specification

## Purpose
TBD - created by archiving change console-bulk-close-conversations. Update Purpose after archive.
## Requirements
### Requirement: The console closes a selected batch of conversations
The console SHALL offer an action that ends several conversations in one
gesture. The action SHALL take an explicit list of conversation names supplied
by the operator's selection, and SHALL NOT accept a filter, a query or a
"everything matching" scope: what may be closed is what was selected.

The batch SHALL be bounded at 50 names — the list page size — so the blast
radius never exceeds one screen of conversations. The bound SHALL be enforced by
the server, not only by the selection UI.

#### Scenario: A selection of conversations is closed
- **WHEN** an operator selects three joined conversations and confirms the close action
- **THEN** all three conversations are closed and the result reports three closed

#### Scenario: An oversized batch is refused
- **WHEN** a close request carries more than 50 names
- **THEN** the request is rejected with 400 and nothing is closed

#### Scenario: An empty batch is refused
- **WHEN** a close request carries no names
- **THEN** the request is rejected with 400

### Requirement: Bulk close is the `/close` command, taking its own path
Each conversation in the batch SHALL be closed by posting the `/close` command
on that conversation's console thread through the console's existing inbound
write. The console SHALL NOT delete `Conversation` objects, SHALL NOT gain any
Kubernetes write path, and SHALL NOT require any new manager endpoint or channel
adapter contract operation.

Consequently every guarantee `/close` already carries SHALL hold unchanged for a
bulk close: the command is intercepted before it becomes a reply input, a
farewell is posted to every bound thread, the threads are archived by the
`agentops.dev/close-topics` finalizer, the runtime pod and MCP ConfigMap are
garbage collected, and freed capacity admits a waiting conversation.

#### Scenario: The closed conversation takes the ordinary close path
- **WHEN** a conversation is closed as part of a batch
- **THEN** its bound threads receive the farewell and the `close-topic` operations, exactly as for a typed `/close`

#### Scenario: No Kubernetes write is performed by the console
- **WHEN** a batch close runs
- **THEN** the console issues no write to the Kubernetes API and the conversations are deleted by the manager

#### Scenario: The close text is not handed to the agent
- **WHEN** a batch close posts `/close` on a conversation's console thread
- **THEN** no reply input is appended and no work unit is dispatched for that text

### Requirement: Reach is joined conversations; observed ones are reported as skipped
A conversation the console does not hold a thread on — an observed
conversation — SHALL be reported with outcome `skipped` and a reason naming the
fix (bind the console channel to that conversation's pipeline). It SHALL NOT be
hidden from selection, SHALL NOT be reported as closed, and SHALL NOT fail the
request.

#### Scenario: Observed conversation is skipped, not closed
- **WHEN** a batch includes a conversation with no console thread
- **THEN** that conversation is reported `skipped` with the reason that the console has no thread on it, and it is not closed

#### Scenario: One observed conversation does not fail the batch
- **WHEN** a batch of five contains one observed conversation and four joined ones
- **THEN** the four joined conversations are closed and the response reports four closed and one skipped

### Requirement: Working conversations are excluded unless explicitly included
A conversation in phase `Working` SHALL be reported `skipped` unless the request
explicitly opts in to including working conversations. The opt-in SHALL be a
distinct flag on the request, defaulting to excluded, and the UI control that
sets it SHALL state that including them abandons in-progress runs.

The phase SHALL be read server-side from the conversation's own state, never
taken from the request, so the decision cannot be made by the caller.

#### Scenario: Working conversation is skipped by default
- **WHEN** a batch includes a conversation in phase `Working` and the request does not opt in
- **THEN** that conversation is reported `skipped` because it is working, and it is not closed

#### Scenario: Opting in closes working conversations
- **WHEN** a batch includes a conversation in phase `Working` and the request opts in to including working conversations
- **THEN** that conversation is closed and its in-progress run is abandoned with the existing farewell notice

#### Scenario: The caller cannot assert a phase
- **WHEN** a request names a conversation that is `Working` without opting in, regardless of what the client believes its phase to be
- **THEN** the server reads the phase from conversation state and skips it

### Requirement: The batch reports a per-conversation outcome
The response SHALL carry one result per requested name with an outcome of
`closed`, `skipped` or `failed`, a reason for anything not closed, and totals for
each outcome. A batch in which some conversations were not closed SHALL still be
a successful request — a mixed result is a normal outcome, not a transport
failure. Names SHALL be processed in the order given, and a failure on one
SHALL NOT stop the rest.

#### Scenario: Mixed batch returns success with per-item detail
- **WHEN** a batch closes some conversations, skips others and fails on one
- **THEN** the response is 200 and lists each conversation with its own outcome and reason, plus the closed/skipped/failed totals

#### Scenario: A failure does not abort the batch
- **WHEN** closing one conversation in a batch fails
- **THEN** the remaining conversations are still attempted

#### Scenario: Re-running over an already-closed conversation is safe
- **WHEN** a batch names a conversation that has already been closed
- **THEN** it is reported `skipped` or `failed` and no second close is performed

### Requirement: Bulk close is a write, gated and attributed
The action SHALL be subject to the same controls as the console's other
writes: authentication, the install-wide write gate, and a forwarded identity to
attribute the write against. Each close SHALL be logged against the identity that
ordered it.

#### Scenario: A read-only console cannot bulk close
- **WHEN** the console is configured read-only and a close request arrives
- **THEN** the request is refused with 403 and nothing is closed

#### Scenario: An unattributable write is refused
- **WHEN** an authenticated close request carries no forward-auth identity
- **THEN** the request is refused with 403 explaining that the write cannot be attributed

#### Scenario: An unauthenticated request is refused
- **WHEN** an unauthenticated close request arrives
- **THEN** it is refused with 401

### Requirement: Selection is explicit and the confirmation names the cost
The conversation list SHALL offer per-row selection and a select-all control
scoped to the rows on the current page. The close action SHALL be disabled with
nothing selected, and SHALL require a confirmation that states how many
conversations will be closed, how many of them are working, and that closing
cannot be undone. There SHALL be no control that selects conversations beyond
the current page.

#### Scenario: Confirmation states the count
- **WHEN** an operator triggers the close action over four selected conversations
- **THEN** a confirmation names the four conversations to be closed and that the action cannot be undone

#### Scenario: Nothing selected disables the action
- **WHEN** no conversation is selected
- **THEN** the close action is not available

#### Scenario: Selection cannot escape the page
- **WHEN** an operator uses the select-all control
- **THEN** only the conversations on the current page are selected

### Requirement: A conversation being closed is shown as closing
A `Conversation` held by its deletion finalizer SHALL be presented as closing
rather than as an ordinary open conversation, and SHALL NOT be selectable for
closing. The console SHALL derive this from the object's deletion timestamp,
which it already watches.

#### Scenario: Closed conversations do not read as untouched
- **WHEN** a batch close succeeds and the closed conversations are still held by their finalizer
- **THEN** the list shows them as closing rather than as open conversations

#### Scenario: A closing conversation cannot be re-closed
- **WHEN** a conversation is closing
- **THEN** it cannot be selected for a close batch

