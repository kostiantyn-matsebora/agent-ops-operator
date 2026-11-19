## MODIFIED Requirements

### Requirement: The specification states both parties

The specification SHALL state the manager's provisions and the runtime's
obligations as two named sections, each exhaustive for its party.

The manager's provisions SHALL include at least:

| Provision | Content |
|---|---|
| one pod per conversation | the manager creates one runtime pod per conversation and owns its lifecycle |
| serial dispatch | one inflight work unit per conversation, never two |
| context restoration | where the deployment checkpoints context, durable context is restored before the first `GET /work` answers |
| checkpoint ordering | the context is checkpointed before `POST /work/done` reaches the manager, so a recorded handle always names persisted context |
| start-deadline reaping | a pod that never starts is reaped at a deadline, freeing its admission slot |
| populated workspace | `/workspace` holds the resolved repositories, cloned by an init container before the runtime starts, with no git credential in the agent container |
| start directory and agent file | the directory the agent starts in, and the absolute path of the resolved agent file when one is declared |
| environment | every variable in the environment table |

The runtime's obligations SHALL include at least: long-poll
`GET $CONTROL_URL/work?convo=$CONVO_ID&pod=$POD_NAME&wait=25`, execute the
unit in the start directory, compose the tool allowlist as `toolsMode`
directs, report on `POST $CONTROL_URL/work/done` within the bounds, and exit
`0` after `RUNTIME_IDLE_TTL_M` minutes without work.

**Repository checkout is neither party's obligation at run time.** An init
container of the `workspace-sync` capability populates `/workspace`
before the runtime starts. A runtime SHALL NOT clone, SHALL NOT read a git
credential and SHALL NOT expect `REPO_URL`, `REPO_REF` or any `GIT_*`
variable.

#### Scenario: The checkout obligation is looked up

- **WHEN** a reader asks which party puts the repositories at `/workspace`
- **THEN** the specification names the workspace-sync init container as the party that clones them
- **AND** no page claims the runtime or the manager checks a repository out

#### Scenario: A continuation is promised

- **WHEN** a work unit carries a `runtimeContextId` in a deployment that
  checkpoints context
- **THEN** the specification guarantees the durable context was restored
  before the first `GET /work` answered, so the handle names context the
  runtime can reach

### Requirement: The specification is exhaustive

The specification SHALL cover every field, every enum value, every bound and
every path the contract carries. A field present in the wire types and absent
from the specification is a specification defect.

Every work-unit field SHALL be stated: `runId`, `convo`, `threadId`,
`runtimeContextId`, `resumeSessionId` (deprecated, replaced by
`runtimeContextId`), `promptText`,
`promptFile`, `promptVars`, `agent`, `agentFile` (the absolute path of the resolved agent file, mutually exclusive with `agent`), `systemPrompt`, `allowedTools`,
`toolsMode`, `maxTurns`.

Every report field SHALL be stated: `convo`, `runId`, `status`, `exitCode`,
`runtimeContextId`, `sessionId` (deprecated, replaced by
`runtimeContextId`), `continuity`,
`continuityReason`, `result`, `brief`, `turns[]` and `toolCalls[]`.

Every entry field of the telemetry lists SHALL be stated: `model`,
`tokensIn`, `tokensOut`, `cacheReadTokens` and `stopReason` on a turn, and
`tool`, `server`, `durationMs` and `resultBytes` on a tool call.

Every enum SHALL be stated with every value:

| Enum | Values |
|---|---|
| `status` | `succeeded`, `failed` — the manager treats any value other than `succeeded` as a failed run |
| `continuity` | `continued`, `new`, `unavailable` — absent means no claim |
| `toolsMode` | `merge`, `overwrite` |

The telemetry bounds SHALL be stated with their refusal:

| Bound | Value |
|---|---|
| entries in `turns[]` | at most 200 |
| entries in `toolCalls[]` | at most 200 |
| any string field of either | at most 200 bytes |
| any integer field | not negative |

A report over any bound is refused WHOLE with `400` naming the field and the
bound it broke, never trimmed.

The environment table SHALL be split into always-present and conditional
variables, each conditional one naming its condition:

| Kind | Variables |
|---|---|
| always present | `ROLE`, `CONVO_ID`, `CONTROL_URL`, `WORKSPACE`, `RUNTIME_IDLE_TTL_M`, `HOME`, `MCP_CONFIG`, `POD_NAME` |
| conditional | `START_DIR` (the conversation's `workingDir` resolves, otherwise the runtime starts in `WORKSPACE`), `AOPS_MCP_TOKEN` (the aops MCP server is bound and a coordinator resolves) |

`REPO_URL`, `REPO_REF`, `GIT_AUTH_TYPE`, `GIT_SSH_KEY` and `GIT_TOKEN` SHALL NOT
appear in the table. They are retired from the runtime contract.

The filesystem contract SHALL be stated: the workspace lives at `/workspace`
and is the value of `WORKSPACE`, the repositories are laid out as
`multi-repo-workspace` specifies, `$HOME` points at the live
context mount, and the composed MCP configuration is read from the path
`MCP_CONFIG` names.

#### Scenario: A field is looked up

- **WHEN** a reader looks up any field of the work unit or the report
- **THEN** the specification states its name, its type, its meaning and when
  it is present

#### Scenario: A report breaks a bound

- **WHEN** a reader asks what an over-bound report costs
- **THEN** the specification states the whole report is refused with `400`
  naming the field and the bound, result included, so a conformant runtime
  trims before posting

#### Scenario: A variable's presence is asked

- **WHEN** a runtime author asks whether an environment variable is always
  set
- **THEN** the environment table answers always-present or names the exact
  condition under which it appears

#### Scenario: A retired variable is looked up

- **WHEN** a runtime author asks for `REPO_URL` or `GIT_TOKEN`
- **THEN** the specification states they are retired and the runtime receives a populated `/workspace`

#### Scenario: The start directory is asked

- **WHEN** a runtime author asks where to start the agent
- **THEN** the specification answers `START_DIR` when present and `WORKSPACE` otherwise

