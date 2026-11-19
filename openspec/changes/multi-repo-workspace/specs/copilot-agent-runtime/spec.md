## MODIFIED Requirements

### Requirement: A third reference runtime implements the same work contract
The project SHALL ship a third reference `AgentRuntime` image, `runtime-copilot`, driving GitHub Copilot through its SDK. It SHALL be a self-contained component under `runtimes/copilot/` with no dependency on any other module here, and SHALL implement the work contract unchanged: long-poll `GET $CONTROL_URL/work`, execute the returned unit in the populated workspace at `/workspace` starting in `START_DIR` when set, stream a human-readable transcript to stdout, `POST $CONTROL_URL/work/done` carrying `continuity`/`continuityReason`, and exit `0` after `RUNTIME_IDLE_TTL_M` minutes without work.

No CRD field, manager behaviour, `Pipeline`, `AgentProfile` or `MCPToolset` SHALL need to change for a conversation to run on it. `Pipeline.spec.runtimeRef` SHALL be the entire switch between vendors.

The image SHALL stay generic by construction — git and generic shell utilities for the agent's own use, with no clone routine and no git credential, and no domain tooling. What an agent may reach is wiring.

#### Scenario: The same wiring runs on any vendor
- **WHEN** a `Pipeline`'s `runtimeRef` is pointed from a claude or ollama runtime at the copilot runtime, changing nothing else
- **THEN** conversations it starts continue to run, with the same profile, the same channels, the same toolsets and the same MCP servers

#### Scenario: The contract is honoured end to end
- **WHEN** the copilot runtime receives a work unit
- **THEN** it executes it against the workspace, streams the transcript to stdout, reports the outcome to `/work/done`, and exits `0` once idle for the configured TTL
- **AND** the manager handles the report with no runtime-specific branch

#### Scenario: No domain tooling is baked in
- **WHEN** an agent on this runtime needs to reach a system outside its workspace
- **THEN** it reaches it through MCP servers its Pipeline binds, not through a CLI shipped in the image

#### Scenario: The runtime does not clone

- **WHEN** a runtime pod starts
- **THEN** the copilot runtime finds `/workspace` already populated and performs no clone and reads no `GIT_*` variable

