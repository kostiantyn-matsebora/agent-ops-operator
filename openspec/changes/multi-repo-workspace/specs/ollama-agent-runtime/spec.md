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


### Requirement: The image stays generic

The runtime image SHALL carry what it needs to run a shell, ordinary shell utilities, and SHALL carry NO domain
tooling: no `kubectl`, no cloud CLI, no bundled MCP server. It does not need `git` or `openssh-client` to clone, since the init container owns cloning. Either MAY be present so an agent can run git, and neither is required by the contract. What an agent may
reach is wiring, expressed by `MCPConfig` and `MCPToolset` on a Pipeline.

An adopter needing a CLI in the sandbox SHALL derive an image and point
`AgentRuntime.spec.image` at it. The image SHALL NOT hold any channel
credential or git credential and SHALL post to no transport — agent output reaches chat through
the manager and its adapters.

#### Scenario: Cluster access is wanted

- **WHEN** an adopter wants the agent to reach Kubernetes
- **THEN** it is bound as MCP tooling on the Pipeline, not baked into this image

#### Scenario: The image is inspected

- **WHEN** the built image is inspected
- **THEN** it contains no cluster or cloud CLI, no channel credential and no git credential
