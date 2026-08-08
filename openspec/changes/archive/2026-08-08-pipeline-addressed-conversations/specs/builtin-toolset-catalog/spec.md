# builtin-toolset-catalog — delta

## MODIFIED Requirements

### Requirement: The chart ships built-in tools as risk-split toolsets
The chart SHALL render `MCPToolset` CRs covering the runtime's built-in tools, split by risk rather than as one list: observation (`Read`, `Grep`, `Glob`), execution (`Bash`), and workspace mutation (`Edit`, `Write`). Default names SHALL be values-overridable and each tool list values-extendable, so adding a built-in the runtime gains needs no new CR kind and no chart change. Rendering SHALL be gated on a values flag defaulting to on; the toolsets carry no status and no controller, so shipping them costs nothing beyond the objects themselves. They are referencable from `Pipeline.spec.toolsets` only — AgentProfiles declare no capabilities.

#### Scenario: A fresh install exposes the vocabulary
- **WHEN** the chart is installed with defaults
- **THEN** the observation, execution, and mutation toolsets exist and are referencable from any Pipeline's `toolsets` binding

#### Scenario: The catalog is extendable without a chart change
- **WHEN** an operator adds a tool name to the observation toolset's values list
- **THEN** the rendered `MCPToolset` grants it, and every binding referencing that toolset picks it up at the next work unit

#### Scenario: The catalog can be omitted entirely
- **WHEN** the built-in toolsets values flag is disabled
- **THEN** no toolset objects render, and any Pipeline whose bindings referenced them reports `Ready=False` naming the missing refs

### Requirement: A route can observe without executing
Binding only the observation toolset on a Pipeline SHALL yield conversations whose allowlist contains the observation tools and NOT the execution tools. Because a Pipeline's bindings ARE its conversations' capabilities — profiles contribute nothing, and nothing supplies a default — this needs no mode and no profile edit: a profile serving several routes grants shell on one and withholds it on another purely by what each Pipeline binds. This holds for every route uniformly, including the Pipelines a task or a chat command addresses directly.

#### Scenario: One profile, two routes, different shell access
- **WHEN** two Pipelines route to one profile, one binding the observation toolset and one binding observation plus execution
- **THEN** conversations from the first cannot use `Bash` while conversations from the second can, and the AgentProfile is identical for both

#### Scenario: Withholding shell needs no profile edit
- **WHEN** an operator removes execution from a route by changing only that route's Pipeline binding
- **THEN** other Pipelines sharing the profile keep their previous capabilities

#### Scenario: An addressed Pipeline governs what a task or command may do
- **WHEN** a Pipeline that binds observation only is addressed by `POST /task` or by a chat command
- **THEN** the resulting conversation can observe but not execute, exactly as for a signal-routed one
