## MODIFIED Requirements

### Requirement: The runtime is the harness; Ollama is only inference

`runtime-ollama` SHALL implement the work contract in full: long-poll
`GET $CONTROL_URL/work`, execute the returned unit, stream progress to stdout,
`POST $CONTROL_URL/work/done`, and exit 0 after `RUNTIME_IDLE_TTL_M` minutes
without work. It SHALL run in the populated workspace at `/workspace`, starting in `START_DIR`
when set. It SHALL NOT clone and SHALL read no git credential.

The agent loop, tool dispatch, transcript and context handle SHALL be owned by
this process. Ollama SHALL be called only to produce the next assistant message.
No behavior the contract specifies SHALL be delegated to the model server,
because it implements none of it.

This SHALL require no change to the manager, to any CRD, or to the work
contract. If it does, the contract was vendor-specific and that is the finding
to report before the code is written.

#### Scenario: A unit is executed end to end

- **WHEN** the manager dispatches a work unit to a `runtime-ollama` pod
- **THEN** the runtime executes it and reports `{convo, runId, status,
  runtimeContextId, result}` to `/work/done`
- **AND** the manager handles the report with no runtime-specific branch

#### Scenario: The pod goes idle

- **WHEN** no work arrives for `RUNTIME_IDLE_TTL_M` minutes
- **THEN** the process exits 0 and the pod is not restarted until the manager
  needs it

#### Scenario: The contract would need widening

- **WHEN** implementing any part of this runtime requires a new field on the
  work unit or the done report
- **THEN** that requirement is raised as a contract change on its own, not
  worked around inside this image

#### Scenario: The runtime does not clone

- **WHEN** a runtime pod starts
- **THEN** the ollama runtime finds `/workspace` already populated and performs no clone
- **AND** its built-in file tools are scoped to `/workspace`

