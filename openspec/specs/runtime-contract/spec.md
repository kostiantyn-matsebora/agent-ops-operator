# runtime-contract Specification

## Purpose
The versioned specification of the runtime work contract: the one document a
stranger builds a conformant `AgentRuntime` image from, stating what the
manager provides and what the runtime owes, without reading any shipped
runtime's source.

## Requirements

### Requirement: The specification is a versioned reference page

The runtime specification SHALL live at `docs/contracts/runtime.md` as a
published reference page. It SHALL carry the site's standard front matter —
title, permalink, description — which the site's own layout rules own.

Front matter is PRESENTATION, never a normative statement. A front-matter
change with no normative statement touched moves no version.

It SHALL carry a semver version, starting at `1.0.0`, and its own changelog
recording every version with what changed in it.

It SHALL replace the work-contract narrative in `docs/contracts.md` as the
normative statement of the contract. The narrative SHALL NOT survive as a
second statement beside it.

#### Scenario: A runtime author needs the contract

- **WHEN** an author sets out to build an `AgentRuntime` image
- **THEN** `docs/contracts/runtime.md` states every obligation of both
  parties
- **AND** the author needs no shipped runtime's source to build against it

#### Scenario: A normative statement changes

- **WHEN** a normative statement in the specification is added, changed or
  removed
- **THEN** the version moves as the compatibility rules direct, and the
  changelog records the change under the new version

#### Scenario: Front matter moves no version

- **WHEN** the page's front matter is edited and no normative statement changes
- **THEN** the specification version is unchanged

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
| environment | every variable in the environment table, and the git credentials for the profile's repository |

The runtime's obligations SHALL include at least: long-poll
`GET $CONTROL_URL/work?convo=$CONVO_ID&pod=$POD_NAME&wait=25`, clone the
repository, execute the unit, compose the tool allowlist as `toolsMode`
directs, report on `POST $CONTROL_URL/work/done` within the bounds, and exit
`0` after `RUNTIME_IDLE_TTL_M` minutes without work.

**Repository checkout is the RUNTIME's obligation.** The manager provides
`REPO_URL`, `REPO_REF` and the git credentials. It never checks the
repository out itself.

#### Scenario: The checkout obligation is looked up

- **WHEN** a reader asks which party puts the repository at `/data/workspace`
- **THEN** the specification names the runtime as the party that clones it
- **AND** no page claims the manager checks the repository out

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
`promptFile`, `promptVars`, `agent`, `systemPrompt`, `allowedTools`,
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
| always present | `ROLE`, `CONVO_ID`, `CONTROL_URL`, `REPO_URL`, `REPO_REF`, `RUNTIME_IDLE_TTL_M`, `HOME`, `MCP_CONFIG`, `POD_NAME` |
| conditional | `GIT_AUTH_TYPE` with `GIT_SSH_KEY` (profile declares ssh repository auth), `GIT_AUTH_TYPE` with `GIT_TOKEN` (profile declares https repository auth), `AOPS_MCP_TOKEN` (the aops MCP server is bound and a coordinator resolves) |

The filesystem contract SHALL be stated: the repository checkout lives at
`/data/workspace` and that path never moves, `$HOME` points at the live
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

### Requirement: Schema tables are generated from the Go structs

The work-unit and report field tables in the specification SHALL be rendered
by the docs generator from the Go structs' json tags, under a declared
marker, never typed by hand.

CI SHALL regenerate the tables and fail on a difference, so a field added in
code without a specification row fails the build.

#### Scenario: A field is added in code only

- **WHEN** a json-tagged field is added to the work unit or the report struct
  and the specification is not regenerated
- **THEN** CI fails on the stale generated block

#### Scenario: A generated table is hand-edited

- **WHEN** someone edits a generated field table by hand
- **THEN** the next generator run reverts the edit and CI fails on the
  difference

### Requirement: Compatibility rules make additive change safe

A runtime MUST ignore work-unit fields it does not know. The manager MUST
ignore report fields it does not know. Both codify the decoding behavior
already shipped, and together they make an additive field a MINOR change.

| Version part | Moves when |
|---|---|
| MAJOR | a runtime conformant with the previous version stops conforming |
| MINOR | a new optional capability or field is added |
| PATCH | a clarification that changes no obligation |

Section anchors SHALL be stable names. The guide and the shipped runtime
pages link by anchor, so renaming a published anchor is a breaking change to
the document and moves MAJOR.

#### Scenario: An unknown field arrives

- **WHEN** a work unit carries a field the runtime does not know, or a report
  carries one the manager does not know
- **THEN** the receiver ignores it and the run proceeds, so the change that
  added the field was MINOR

#### Scenario: A published anchor is renamed

- **WHEN** a change renames a section anchor the guide or a shipped runtime
  page links
- **THEN** the change is MAJOR, because a conformant reader's reference
  stopped resolving

### Requirement: A numbered conformance checklist is the spine

The specification SHALL end with a numbered conformance checklist. Each item
SHALL be individually verifiable against a running runtime or its source, and
every normative obligation SHALL be reachable from an item.

#### Scenario: A runtime is judged conformant

- **WHEN** someone claims a runtime conforms to a specification version
- **THEN** the claim is the checklist walked item by item, each with its own
  verdict, never an impression of the whole

### Requirement: The adopter guide is the station procedure

`docs/guides/agent-runtime.md` SHALL be the exact ordered procedure for
introducing a runtime. Each step SHALL cite the specification section it
implements by anchor, and each SHALL end in a verification.

References SHALL run one direction. The guide cites the specification, and
the specification never depends on the guide. The guide SHALL state no
requirement of its own.

#### Scenario: A step is followed

- **WHEN** a reader completes a step of the guide
- **THEN** the step's verification tells them it worked before the next step
  begins
- **AND** the cited anchor names where the obligation is normative

#### Scenario: The guide and the specification disagree

- **WHEN** a statement in the guide contradicts the specification
- **THEN** the specification wins and the guide is the defect, because the
  guide states no requirement of its own

### Requirement: Shipped runtime pages claim a specification version

Each shipped runtime's documentation page SHALL state the specification
version that runtime conforms to.

#### Scenario: An adopter checks a shipped runtime

- **WHEN** an adopter opens a shipped runtime's page
- **THEN** it names the specification version the runtime conforms to
- **AND** that version exists in the specification's changelog

### Requirement: A specification change carries conformance

A change that edits `docs/contracts/runtime.md` MUST, in the same change,
bring every shipped runtime — `runtimes/claude`, `runtimes/ollama`,
`runtimes/copilot` — into conformance with the new version.

The same change MUST move the version each shipped runtime page claims. The
change is not finished while any shipped runtime conforms only to the
previous version, so the specification never runs ahead of what ships.

#### Scenario: The specification version moves

- **WHEN** a change bumps the specification version
- **THEN** the same change updates every shipped runtime and every shipped
  runtime page's claimed version, or the change is not archivable

### Requirement: The specification governs the agent's own changes

A scoped context rule SHALL load for `runtimes/**` and the manager's
work-contract surface: `internal/dispatch/`, `internal/runtimepod/` and the
work endpoints in `internal/httpapi/`.

The rule SHALL state that a change there is verified against the
specification before it is claimed done. It SHALL state that the
specification itself is NEVER edited without the user's explicit approval.

#### Scenario: A governed surface changes

- **WHEN** an agent session changes a runtime or the manager's work-contract
  surface
- **THEN** the loaded rule directs it to verify the change against
  `docs/contracts/runtime.md` before claiming the work done

#### Scenario: A session reaches for the specification itself

- **WHEN** an agent session would edit `docs/contracts/runtime.md` without
  the user's explicit approval
- **THEN** the rule directs it to stop and ask, and the session reports the
  conflict instead of editing

### Requirement: A startup configuration failure is a crash, not a lifecycle exit

The idle TTL SHALL remain the runtime's only LIFECYCLE exit: exit `0` after
`RUNTIME_IDLE_TTL_M` minutes without work.

Missing or unusable required configuration, detected before the first
`GET /work` poll, SHALL end the process with a nonzero exit naming what is
missing. That is a CRASH the kubelet surfaces and restarts, not a lifecycle
exit, and it does not breach the single-exit rule.

Honouring the pod's own termination signal is likewise not a lifecycle exit.

The specification SHALL state this scoping, so a conformance reading of the
exit rule never condemns failing fast at startup.

#### Scenario: Missing required configuration fails fast

- **WHEN** a runtime starts without a required variable such as `CONTROL_URL`
- **THEN** it exits nonzero before its first poll, naming the missing variable
  in its log

#### Scenario: The lifecycle exit rule still holds after startup

- **WHEN** a runtime has answered its first poll and runs out of work
- **THEN** exiting `0` at the idle TTL remains its one lifecycle exit

### Requirement: A definitive report rejection is not retried

The specification SHALL state the delivery rule for `POST /work/done`:

| Response | Runtime behaviour |
|---|---|
| transport error, timeout, `408`, `429`, any `5xx` | bounded retry, then log the loss and resume polling |
| any other `4xx` | NO retry — log the status and body, resume polling |
| `2xx` | done |

A definitive rejection means the manager refused the report's content, and
resending the same bytes cannot change the verdict. Retrying it only blocks the
poll loop.

Every undelivered report SHALL leave a log line carrying the response status
and body, so a refused report is diagnosable from the pod log. A report SHALL
never be dropped silently.

#### Scenario: A 400 is not retried

- **WHEN** the manager refuses a report with `400` naming an over-bound field
- **THEN** the runtime logs the status and body, does not resend, and returns
  to polling

#### Scenario: A transient failure keeps its bounded retry

- **WHEN** the manager is unreachable while a report is due
- **THEN** the runtime retries within its bounded budget, and logs the loss if
  the budget exhausts
