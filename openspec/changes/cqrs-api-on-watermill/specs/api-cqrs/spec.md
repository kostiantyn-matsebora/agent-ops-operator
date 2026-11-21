## Purpose

Every route the manager serves either changes state or returns it, and the two are
kept apart. This capability defines that split, how a command runs, and what a query
reads.

## ADDED Requirements

### Requirement: Every route is a command, a query or a claim
Each route the manager serves SHALL be exactly one of a command, a query or a claim.

A command SHALL change state and SHALL NOT return state beyond its own outcome.
A query SHALL return state and SHALL NOT change any.
A claim SHALL be the pull end of a queue, which reads and hands over one message.

The set of routes and the kind of each SHALL be written down in one place.

#### Scenario: A query changes nothing
- **WHEN** any query route is called repeatedly with the same arguments
- **THEN** the state read afterwards equals the state before the first call

#### Scenario: A new route has a kind
- **WHEN** a route is added
- **THEN** the route table names its kind, and the build fails if it does not

### Requirement: A command runs in a handler behind the command bus
A command SHALL be a typed message with a name. Its effect SHALL run in the handler
registered for that name, and SHALL NOT run in the HTTP handler that received it.

A command SHALL carry the ordering key its queue declares. Commands sharing a key
SHALL run one at a time and in the order sent.

#### Scenario: The HTTP handler does not do the work
- **WHEN** a command route receives a request
- **THEN** the request handler builds and sends the command, and the state change happens in the command's handler

#### Scenario: Commands of one key run in order
- **WHEN** two commands with the same key are sent in order
- **THEN** their handlers run one at a time in that order

### Requirement: A command answers as accepted or with its result
A command route SHALL answer in one of two ways, and the route table SHALL say which.

- **Accepted.** The reply means the command is durably sent, not performed.
- **With a result.** The caller waits for the handler's result and gets it in the same call.

A route that answers with a result SHALL refuse in the same call when the handler
refuses, and SHALL NOT answer accepted for a command that failed.

#### Scenario: An answered command returns the handler's result
- **WHEN** a command that answers with a result is sent and its handler succeeds
- **THEN** the call returns the handler's result

#### Scenario: An answered command refuses in the call
- **WHEN** such a command's handler refuses it
- **THEN** the same call returns the refusal and no state changed

#### Scenario: An accepted command is not yet performed
- **WHEN** a command that answers accepted is sent
- **THEN** the reply is returned before the handler's effect is guaranteed, and the effect is delivered at least once

### Requirement: A change publishes an event after it lands
A command handler SHALL publish an event naming what changed, after the change is
written and not before. An event SHALL NOT be the record of the change.

A lost event SHALL NOT leave a read surface wrong for ever. The state a surface shows
SHALL be re-derivable without the event.

#### Scenario: An event follows the write
- **WHEN** a command changes a conversation
- **THEN** an event naming the change is published after the conversation record shows it

#### Scenario: A lost event is recovered
- **WHEN** an event is lost and nothing else touches the conversation
- **THEN** the read surface shows the conversation's true state after the next reconciliation

### Requirement: A query reads a read model or a named source of truth
A query SHALL read a read model that commands and their events keep current, or a
source of truth the route table names, such as the Kubernetes API. A query SHALL NOT
run a command.

#### Scenario: A query that reads a source of truth is named
- **WHEN** a query reads the Kubernetes API directly
- **THEN** the route table names it as reading that source
