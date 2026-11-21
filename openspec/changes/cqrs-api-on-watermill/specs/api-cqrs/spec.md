## Purpose

Every route the manager serves either changes state or returns it, and the two are
kept apart. This capability defines that split and what each kind calls.

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

### Requirement: A route's handler only calls a processor
The HTTP handler of a command route SHALL parse the request, send the command and
write the response. The HTTP handler of a query route SHALL parse the request, execute
the query and write the response. The effect SHALL run in the registered handler and
SHALL NOT run in the HTTP handler.

#### Scenario: The HTTP handler does not do the work
- **WHEN** a command route receives a request
- **THEN** the HTTP handler sends the command, and the state change happens in the command's handler

### Requirement: A query reads a read model or a named source of truth
A query SHALL read a read model that commands keep current, or a source of truth the
route table names, such as the Kubernetes API.

#### Scenario: A query that reads a source of truth is named
- **WHEN** a query reads the Kubernetes API directly
- **THEN** the route table names it as reading that source
