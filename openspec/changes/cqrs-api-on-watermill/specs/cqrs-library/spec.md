## Purpose

A library the components share for running commands and queries. It holds the
command processor with its synchronous and asynchronous paths, and a separate query
processor.

## ADDED Requirements

### Requirement: The core of the library has no third-party dependency
The library's core SHALL import only the standard library. Anything that needs a
third-party package, such as the NATS backend and Watermill, SHALL be a separate
module.

A component that imports only the core SHALL NOT gain a third-party dependency from it.

#### Scenario: The core builds on the standard library alone
- **WHEN** the core module's dependencies are listed
- **THEN** the list is empty

#### Scenario: A stdlib-only component stays stdlib-only
- **WHEN** a component that had no third-party dependency imports the core
- **THEN** its module still lists no third-party dependency

### Requirement: Moving the bus into the library changes no behaviour
The bus, its consumer pool, its backends and its conformance suite SHALL pass the
same tests before and after they move into the library.

#### Scenario: The conformance suite passes on both backends after the move
- **WHEN** the conformance suite runs against the `memory` and the `nats` backends from the library
- **THEN** every scenario that passed before the move passes

### Requirement: A query runs through a separate query processor
A query SHALL be a typed request with one registered handler. The query processor
SHALL return the handler's data. A query handler SHALL NOT write state and SHALL NOT
be given the command processor.

A query SHALL NOT be delivered through the bus, retried or recorded as a task.

#### Scenario: A query returns the handler's data
- **WHEN** a registered query is executed
- **THEN** the call returns the handler's data

#### Scenario: A query handler cannot send a command
- **WHEN** a query handler is registered
- **THEN** it has no access to the command processor

#### Scenario: A query leaves no task
- **WHEN** a query is executed
- **THEN** the bus holds no task for it

### Requirement: A component imports the library, never another component's internals
A component SHALL use the library through its published module path. A component
SHALL NOT import another component's `internal` package.

#### Scenario: The library is a versioned module
- **WHEN** a component requires the library
- **THEN** it requires a tagged version, and a local build may use a workspace file that no image build reads
