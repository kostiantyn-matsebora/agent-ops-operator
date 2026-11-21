## Purpose

Every action a component performs is either a command that changes state or a query
that returns it, whatever triggers it. This capability defines that split and what a
trigger does.

## ADDED Requirements

### Requirement: Every action is a command or a query, whatever triggers it
Each action a component performs SHALL be exactly one of a command or a query. A
command SHALL change state and SHALL NOT return state beyond its own outcome. A query
SHALL return state and SHALL NOT change any.

The kind of an action SHALL NOT depend on its trigger. An HTTP route, a reconcile
loop, a sweep, a timer and a queue message are triggers.

The actions and the trigger of each SHALL be written down in the action table, `docs/contracts/actions.md`. A test SHALL compare the table with the routes the manager serves and the reconcilers it registers, and SHALL fail on any action missing from the table.

#### Scenario: A query changes nothing
- **WHEN** any query is executed repeatedly with the same arguments
- **THEN** the state read afterwards equals the state before the first execution

#### Scenario: A read with a side effect is a command
- **WHEN** an HTTP `GET` changes state, as a claim of work does
- **THEN** it is registered as a command, whatever its verb

#### Scenario: A new action has a kind
- **WHEN** an action is added
- **THEN** `docs/contracts/actions.md` names its kind and its trigger, and the table test fails if it does not

### Requirement: A trigger only calls a processor
A trigger SHALL build the request, call the command processor or the query processor,
and map the outcome to its own form. The effect SHALL run in the registered handler
and SHALL NOT run in the trigger.

A reconcile loop SHALL call a command and SHALL map the command's result to a requeue.
Leader election SHALL still gate the trigger.

#### Scenario: The HTTP handler does not do the work
- **WHEN** a command route receives a request
- **THEN** the HTTP handler sends the command, and the state change happens in the command's handler

#### Scenario: A reconcile loop calls a command
- **WHEN** a reconciler is triggered by a watch event
- **THEN** it sends a reconcile command carrying the key, and the reconcile logic runs in the command's handler

#### Scenario: A non-leader replica does not reconcile
- **WHEN** a replica does not hold the leader lease
- **THEN** its reconcile triggers send no command

### Requirement: A query reads a read model or a named source of truth
A query SHALL read a read model that commands keep current, or a source of truth the
action table (`docs/contracts/actions.md`) names, such as the Kubernetes API.

#### Scenario: A query that reads a source of truth is named
- **WHEN** a query reads the Kubernetes API directly
- **THEN** `docs/contracts/actions.md` names it as reading that source
