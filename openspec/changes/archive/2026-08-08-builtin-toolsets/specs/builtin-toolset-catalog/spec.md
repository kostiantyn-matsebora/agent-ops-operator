# builtin-toolset-catalog

## ADDED Requirements

### Requirement: The chart ships built-in tools as risk-split toolsets
The chart SHALL render `MCPToolset` CRs covering the runtime's built-in tools, split by risk rather than as one list: observation (`Read`, `Grep`, `Glob`), execution (`Bash`), and workspace mutation (`Edit`, `Write`). Default names SHALL be values-overridable and each tool list values-extendable, so adding a built-in the runtime gains needs no new CR kind and no chart change. Rendering SHALL be gated on a values flag defaulting to on; the toolsets carry no status and no controller, so shipping them costs nothing beyond the objects themselves.

#### Scenario: A fresh install exposes the vocabulary
- **WHEN** the chart is installed with defaults
- **THEN** the observation, execution, and mutation toolsets exist and are referencable from any Pipeline's or AgentProfile's `toolsets` binding

#### Scenario: The catalog is extendable without a chart change
- **WHEN** an operator adds a tool name to the observation toolset's values list
- **THEN** the rendered `MCPToolset` grants it, and every binding referencing that toolset picks it up at the next work unit

#### Scenario: The catalog can be omitted entirely
- **WHEN** the built-in toolsets values flag is disabled
- **THEN** no toolset objects render and profiles declaring `allowedTools` inline are unaffected

### Requirement: A route can observe without executing
Binding the observation toolset in `overwrite` mode on a Pipeline SHALL yield conversations whose allowlist contains the observation tools and NOT the execution tools, without editing the AgentProfile — so a profile serving several routes can grant shell on one and withhold it on another. `overwrite` is required for this: `merge` would union the profile's own `allowedTools` back in, re-granting exactly what the route withheld.

#### Scenario: One profile, two routes, different shell access
- **WHEN** two Pipelines route to one profile, one binding the observation toolset in `overwrite` mode and one binding nothing
- **THEN** conversations from the first cannot use `Bash` while conversations from the second can, and the AgentProfile is identical for both

#### Scenario: Withholding shell needs no profile edit
- **WHEN** an operator removes execution from a route by changing only that route's Pipeline binding
- **THEN** other Pipelines sharing the profile keep their previous tools
