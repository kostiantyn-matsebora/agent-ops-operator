## MODIFIED Requirements

### Requirement: The specification is exhaustive

The specification SHALL cover every field, every enum value, every bound and
every path the contract carries. A field present in the wire types and absent
from the specification is a specification defect.

Every work-unit field SHALL be stated: `runId`, `convo`, `threadId`,
`runtimeContextId`, `resumeSessionId` (deprecated, replaced by
`runtimeContextId`), `promptText`,
`promptFile`, `promptVars`, `agent`, `agentFile` (the absolute path of the resolved agent file, mutually exclusive with `agent`), `systemPrompt`, `allowedTools`,
`toolsMode`, `maxTurns`, `repoContext` (`omit` only, absent reads as `load`).

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
| `repoContext` | `load`, `omit` — absent reads as `load` |

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

#### Scenario: The repoContext field is looked up

- **WHEN** a reader looks up `repoContext`
- **THEN** the specification states it is optional, that only `omit` is ever sent, that absent reads as `load`, and which obligation governs it

## ADDED Requirements

### Requirement: A runtime honours repoContext

The specification SHALL state one new runtime obligation, `RT-4`: when the work unit carries `repoContext: omit`, the runtime SHALL load none of the repository context into the run, and when the field is absent or `load` it SHALL behave as before.

Repository context is the checkout's own `CLAUDE.md` and `AGENTS.md`, its rules, skills, commands, hooks and settings, and every agent definition other than the one named by `agent` or `agentFile`. That definition, the unit's `systemPrompt` and `allowedTools` are NOT repository context and SHALL still be applied.

The specification SHALL state a second item, `RT-4.1`: a runtime that cannot guarantee `omit` SHALL fail the run with a reason naming `repoContext`, and SHALL NOT load the context silently.

The addition is a MINOR change under the compatibility rules, since a runtime ignoring an unknown field keeps working. The version SHALL move by one MINOR step, the changelog SHALL record it, a numbered item SHALL be added to the conformance checklist for each of `RT-4` and `RT-4.1`, and every shipped runtime and runtime page SHALL conform and claim the new version in the same change, as `A specification change carries conformance` requires.

#### Scenario: Omit is honoured

- **WHEN** a work unit carries `repoContext: omit` for a checkout containing a `CLAUDE.md` and skills
- **THEN** the run's context contains neither, and does contain the unit's agent definition and system prompt

#### Scenario: Absent behaves as before

- **WHEN** a work unit carries no `repoContext`
- **THEN** the runtime loads the checkout's context exactly as before this change

#### Scenario: An unsuppressable runtime fails loudly

- **WHEN** a runtime receives `omit` and cannot stop its vendor loading the checkout's context
- **THEN** it reports `status: failed` with a reason naming `repoContext`, and loads nothing silently

#### Scenario: Every shipped runtime conforms

- **WHEN** the specification's version moves for this field
- **THEN** `runtimes/claude`, `runtimes/claude-cli`, `runtimes/ollama`, `runtimes/copilot` and `runtimes/pi` each honour or fail `omit`, and each runtime page claims the new version
