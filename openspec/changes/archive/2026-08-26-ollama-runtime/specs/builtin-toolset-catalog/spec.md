## MODIFIED Requirements

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
