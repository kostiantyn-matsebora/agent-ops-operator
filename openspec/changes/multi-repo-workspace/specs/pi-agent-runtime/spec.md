## MODIFIED Requirements

### Requirement: A fifth reference runtime implements the same work contract
The project SHALL ship a fifth reference `AgentRuntime` image, `runtime-pi`, driving pi through its SDK in-process. It SHALL be a self-contained component under `runtimes/pi/` with no dependency on any other module here.

It SHALL implement the work contract unchanged:

- long-poll `GET $CONTROL_URL/work`
- execute the returned unit in the populated workspace at `/workspace`, starting in `START_DIR` when set
- clone nothing and read no git credential, since the init container owns cloning
- stream a human-readable transcript to stdout
- `POST $CONTROL_URL/work/done` carrying `continuity`/`continuityReason`
- exit `0` after `RUNTIME_IDLE_TTL_M` minutes without work

No CRD field, manager behaviour, `Pipeline`, `AgentProfile` or `MCPToolset` SHALL need to change for a conversation to run on it. `Pipeline.spec.runtimeRef` SHALL be the entire switch between vendors.

The image SHALL stay generic by construction: the workspace it is handed, generic shell utilities and the MCP extension it needs, and no domain tooling. What an agent may reach is wiring.

The runtime SHALL conform to the version of `docs/contracts/runtime.md` its page claims, walked item by item against the numbered conformance checklist.

#### Scenario: The same wiring runs on any vendor
- **WHEN** a `Pipeline`'s `runtimeRef` is pointed from another runtime at the pi runtime, changing nothing else
- **THEN** conversations it starts continue to run, with the same profile, the same channels, the same toolsets and the same MCP servers

#### Scenario: The contract is honoured end to end
- **WHEN** the pi runtime receives a work unit
- **THEN** it executes it in the workspace, streams the transcript to stdout, reports the outcome to `/work/done`, and exits `0` once idle for the configured TTL
- **AND** the manager handles the report with no runtime-specific branch

#### Scenario: No domain tooling is baked in
- **WHEN** an agent on this runtime needs to reach a system outside its workspace
- **THEN** it reaches it through MCP servers its Pipeline binds, not through a CLI shipped in the image

#### Scenario: Missing configuration is a crash, not an idle exit
- **WHEN** the runtime starts without `CONTROL_URL`, `CONVO_ID` or the model credential it needs
- **THEN** it exits nonzero before the first poll, naming what is missing
