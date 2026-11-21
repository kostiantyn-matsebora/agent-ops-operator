## Purpose

Every route the manager serves either changes state or returns it, and the two are
kept apart. This capability defines that split, the two processors that run a
command or a query, and what a query reads.

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

### Requirement: A command runs in its handler through the command processor
A command SHALL be a typed request with one registered handler. The effect of a
command SHALL run in that handler and SHALL NOT run in the HTTP handler that
received it. The HTTP handler SHALL only parse the request, call the command
processor and write the response.

#### Scenario: The HTTP handler does not do the work
- **WHEN** a command route receives a request
- **THEN** the HTTP handler calls the command processor, and the state change happens in the command's handler

### Requirement: A command's mode is fixed by its registration
A command SHALL be registered as synchronous or as asynchronous. The caller SHALL NOT
choose the mode.

A synchronous command SHALL run in the process that received the request and SHALL
return its handler's result or refusal in the same call. It SHALL NOT depend on the
command bus.

An asynchronous command SHALL be durably submitted to its queue and SHALL answer
accepted. Its effect SHALL run in the consumer pool and SHALL be delivered at least
once. Commands of one key SHALL run one at a time and in the order submitted.

#### Scenario: A synchronous command returns the handler's result
- **WHEN** a synchronous command's handler succeeds
- **THEN** the call returns the handler's result

#### Scenario: A synchronous command refuses in the call
- **WHEN** a synchronous command's handler refuses it
- **THEN** the same call returns the refusal and no state changed

#### Scenario: A synchronous command survives a bus outage
- **WHEN** the command bus cannot accept messages and a synchronous command is sent
- **THEN** the command runs and returns its result

#### Scenario: An asynchronous command answers accepted
- **WHEN** an asynchronous command is sent
- **THEN** the reply is returned once it is durably submitted, before its effect is guaranteed

#### Scenario: Commands of one key run in order
- **WHEN** two asynchronous commands with the same key are sent in order
- **THEN** their handlers run one at a time in that order

### Requirement: A query runs through the query processor and changes nothing
A query SHALL be a typed request with one registered handler. The query processor
SHALL return the handler's data. A query handler SHALL NOT send a command and SHALL
NOT write state.

A query SHALL read a read model that commands and their events keep current, or a
source of truth the route table names, such as the Kubernetes API.

#### Scenario: A query handler cannot write
- **WHEN** a query handler is registered
- **THEN** it has no access to the command processor

#### Scenario: A query that reads a source of truth is named
- **WHEN** a query reads the Kubernetes API directly
- **THEN** the route table names it as reading that source

### Requirement: One pipeline wraps every handler
Every command handler and every query handler SHALL run inside one pipeline. The
pipeline SHALL recover a panic into an error, SHALL bound a synchronous handler with
a timeout, and SHALL log the request's name, key and outcome.

The pipeline SHALL NOT retry a command.

#### Scenario: A panic becomes an error
- **WHEN** a handler panics
- **THEN** the caller receives an error, the process keeps running, and one line is logged

#### Scenario: A slow synchronous handler is bounded
- **WHEN** a synchronous handler runs longer than its timeout
- **THEN** its context is cancelled and the caller receives a timeout error

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
