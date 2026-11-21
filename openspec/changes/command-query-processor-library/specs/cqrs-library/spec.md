## Purpose

A library the components share for running commands and queries. It holds the
command processor with its synchronous and asynchronous paths, and a separate query
processor.

## ADDED Requirements

### Requirement: The core of the library has no third-party dependency
The library's core SHALL import only the standard library. Anything that needs a
third-party package, such as a messaging backend, SHALL be a separate
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

#### Scenario: A cross-component internal import fails the build
- **WHEN** a component imports another component's `internal` package
- **THEN** a check fails and names the import

#### Scenario: The library is a versioned module
- **WHEN** a component requires the library
- **THEN** it requires a tagged version, and a local build may use a workspace file that no image build reads

### Requirement: The library defines the base command, query and response
The library SHALL define a base command, a base query, a response for a command and a
response for a query.

A trigger SHALL build the command or the query and SHALL read the response. A handler
SHALL receive a typed request and return a typed result.

A command SHALL carry its kind, a command id the caller can repeat, an ordering key and
a correlation id.

A query SHALL carry its name, its parameters and a page request. A page request SHALL
carry a limit and an offset.

A command response SHALL say whether the command succeeded or was accepted. A
succeeded command SHALL carry its result. An accepted command SHALL carry the id of the
task to read back. A refusal SHALL carry an error and SHALL NOT carry a result.

A query response SHALL carry its data and the page it returns, with the total. A
refusal SHALL carry an error and SHALL NOT carry data.

An error SHALL carry a code and a detail. The code SHALL be one of: `not_found`, `precondition_failed`, `forbidden`, `conflict`, `unavailable`, `invalid_command`, `internal`.

#### Scenario: A sync command response carries its result
- **WHEN** a sync command succeeds
- **THEN** its response says it succeeded and carries the handler's result

#### Scenario: An async command response carries a task id
- **WHEN** an async command is accepted
- **THEN** its response says it was accepted and carries the id of the task to read back

#### Scenario: A refusal carries an error and no result
- **WHEN** a handler refuses a command
- **THEN** the response carries an error with a code from the closed set and no result

#### Scenario: A query response carries a page
- **WHEN** a query returns a list
- **THEN** the response carries the data, the limit, the offset and the total

#### Scenario: The error code set is closed
- **WHEN** a handler returns an error
- **THEN** its code is one of the closed set, and the library refuses a code outside it
