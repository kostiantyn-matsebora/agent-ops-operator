# builtin-toolset-catalog

## Purpose

The chart-shipped `MCPToolset` CRs covering the runtime's built-in tools, split by risk — observation, execution, workspace mutation — so a Pipeline can bind them to widen or narrow a route's tool access with no AgentProfile involved at all — profiles carry no capabilities.

## Requirements

### Requirement: The chart ships built-in tools as risk-split toolsets

The chart SHALL render `MCPToolset` CRs covering the runtime's built-in tools,
split by risk rather than as one list: observation (`Read`, `Grep`, `Glob`),
execution (`Bash`), and workspace mutation (`Edit`, `Write`). Default names SHALL
be values-overridable and each tool list values-extendable, so adding a built-in
the runtime gains needs no new CR kind and no chart change. Rendering SHALL be
gated on a values flag defaulting to on; the toolsets carry no status and no
controller, so shipping them costs nothing beyond the objects themselves. They
are referencable from `Pipeline.spec.toolsets` only — AgentProfiles declare no
capabilities.

These names are RUNTIME-INTERPRETED. The manager passes tool patterns through
opaquely and holds no definition of what `Read` or `Bash` does; each runtime
image decides what it can provide. More than one runtime may therefore implement
this vocabulary, and each SHALL provide what it can and REPORT what it cannot —
naming an unimplemented tool in the run rather than dropping it silently or
substituting another. Silence would make a Pipeline look correctly wired while
its agent cannot do what the binding granted.

A binding is therefore never rejected for naming a tool some runtime lacks:
which tools exist is a property of the runtime executing the conversation, and
it is knowable only there.

#### Scenario: A fresh install exposes the vocabulary

- **WHEN** the chart is installed with defaults
- **THEN** the observation, execution, and mutation toolsets exist and are
  referencable from any Pipeline's `toolsets` binding

#### Scenario: The catalog is extendable without a chart change

- **WHEN** an operator adds a tool name to the observation toolset's values list
- **THEN** the rendered `MCPToolset` grants it, and every binding referencing
  that toolset picks it up at the next work unit

#### Scenario: The catalog can be omitted entirely

- **WHEN** the built-in toolsets values flag is disabled
- **THEN** no toolset objects render, and any Pipeline whose bindings referenced
  them reports `Ready=False` naming the missing refs

#### Scenario: Two runtimes serve one binding

- **WHEN** two Pipelines bind the same built-in toolset but route to profiles on
  different runtime images
- **THEN** each conversation gets the tools its own runtime implements, and the
  binding is unchanged

#### Scenario: A runtime lacks a granted tool

- **WHEN** a conversation runs on a runtime that does not implement a tool its
  allowlist carries
- **THEN** the run states that the tool is unavailable on that runtime
- **AND** neither the manager nor the Pipeline reports an error, because the
  binding is correct and the gap is the runtime's

### Requirement: A route can observe without executing
Binding only the observation toolset on a Pipeline SHALL yield conversations whose allowlist contains the observation tools and NOT the execution tools. Because a Pipeline's bindings ARE its conversations' capabilities — profiles contribute nothing, and nothing supplies a default — this needs no profile edit and no change of composition mode: a profile serving several routes grants shell on one and withholds it on another purely by what each Pipeline binds. This holds for every route uniformly, including a Pipeline reached by a posted task through a source it claims and one named by a chat command.

#### Scenario: One profile, two routes, different shell access
- **WHEN** two Pipelines route to one profile, one binding the observation toolset and one binding observation plus execution
- **THEN** conversations from the first cannot use `Bash` while conversations from the second can, and the AgentProfile is identical for both

#### Scenario: Withholding shell needs no profile edit
- **WHEN** an operator removes execution from a route by changing only that route's Pipeline binding
- **THEN** other Pipelines sharing the profile keep their previous capabilities

#### Scenario: A reached Pipeline governs what a task or command may do
- **WHEN** a Pipeline that binds observation only is reached by a `kind: task` signal on a source it claims, or by a chat command
- **THEN** the resulting conversation can observe but not execute, exactly as for a signal-routed one
