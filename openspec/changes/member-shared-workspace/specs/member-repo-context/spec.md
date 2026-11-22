## ADDED Requirements

### Requirement: An agents entry declares whether its member loads the checkout's context

An `agents[]` entry SHALL accept a `repoContext` field with two values:

| Value | Meaning |
|---|---|
| `load` | the runtime loads the checkout's agent context, as today |
| `omit` | the runtime loads none of it into this member's context |

An absent field SHALL read as `load`. The field SHALL be valid on an entry naming a `capabilityRef` and on one naming a `coordinatorRef`.

**Repository context** is the checkout's own `CLAUDE.md` and `AGENTS.md`, its rules, skills, commands, hooks, settings and every agent definition other than the one the conversation was started with. It is not the conversation's own agent definition and not its prompt.

With `omit`, the member SHALL still receive its capability's system prompt, its resolved agent definition (`agent` or `agentFile` on the work unit) and its tool allowlist.

```yaml
agents:
  - name: tester
    description: Runs the suite and reports failures
    capabilityRef: tester
    workspace: shared
    repoContext: omit
```

#### Scenario: The default loads
- **WHEN** an entry omits `repoContext`
- **THEN** its member's runtime loads the checkout's context as it does today

#### Scenario: A value outside the enum is refused
- **WHEN** an entry carries `repoContext: none`
- **THEN** the API server rejects the manifest

#### Scenario: The member keeps its own definition
- **WHEN** an entry declares `repoContext: omit` and its capability names an agent definition in the checkout
- **THEN** the member runs with that definition and the capability's prompt, and with none of the checkout's other context

### Requirement: The setting rides the work unit and is fixed per conversation

The manager SHALL snapshot the entry's `repoContext` onto the member's conversation when it is created, and SHALL NOT resolve it again. Every work unit of that conversation SHALL carry the value, so the setting is per conversation and not per runtime image or pod.

The work unit SHALL carry `repoContext` only when it is `omit`. An absent field reads as `load`. A root conversation and any conversation not opened through an entry SHALL carry none.

An `invoke` that attaches to a live member SHALL keep that member's recorded value.

```json
{"convo": "c-7", "runId": "r-1", "promptText": "run the suite",
 "agent": "tester", "toolsMode": "overwrite", "repoContext": "omit"}
```

#### Scenario: An omit member's units carry the field
- **WHEN** a member opened through an entry with `repoContext: omit` is dispatched a unit
- **THEN** the unit carries `repoContext: omit`

#### Scenario: A load member's units carry nothing
- **WHEN** a member opened through an entry that omits the field is dispatched a unit
- **THEN** the unit carries no `repoContext` field

#### Scenario: An edit does not change a running member
- **WHEN** an entry is edited from `omit` to `load` while a member opened under it is open
- **THEN** that member's later units still carry `omit`

### Requirement: A runtime honours omit or fails the run

A runtime that receives `repoContext: omit` SHALL load none of the repository context into the run. A runtime that cannot guarantee it SHALL fail the run with an explicit reason naming `repoContext`, and SHALL NOT load the context silently.

The obligation is stated for every runtime by `runtime-contract` and its per-runtime consequences by the runtime specifications.

#### Scenario: A runtime that cannot honour it says so
- **WHEN** a runtime receives `repoContext: omit` and has no way to suppress a vendor's own loading
- **THEN** it reports the run failed with a reason naming `repoContext`

#### Scenario: The chart-wide switch is a ceiling
- **WHEN** a runtime's image-level switch for loading checkout context is off and a unit carries no `repoContext`
- **THEN** nothing is loaded, and a unit carrying `load` cannot turn it on
