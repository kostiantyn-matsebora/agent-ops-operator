## Purpose

Chart-level `runtimeClassName` for every component workload the chart
renders outside the AgentRuntime/Conversation chain, so a cluster operator
can place the manager, the console, every adapter, and every bundle's own
workload under a kernel-isolating `RuntimeClass` such as gVisor's `runsc`,
globally or one component at a time.

## ADDED Requirements

### Requirement: A global default reaches every chart-rendered workload

The chart SHALL expose one `runtimeClassName` default at
`global.agentops.runtimeClassName`, defaulting to unset. Unset SHALL mean
exactly what it means today: no `RuntimeClassName` on the rendered pod,
the cluster's own default kernel.

This default SHALL reach every workload the chart renders outside the
AgentRuntime/Conversation chain:

- the manager
- the console and every signal/channel adapter, through the single
  `ensureAdapterWorkload` code path
- housekeeping, the context-probe CronJob, `mcp-aops`
- every bundle's own MCP server (`kubernetes`, `prometheus`,
  `home-assistant`), and the Telegram router

#### Scenario: The global default reaches console with no per-component setting
- **WHEN** an install sets `global.agentops.runtimeClassName` and no component overrides it
- **THEN** console's workload, and every signal/channel adapter's workload, render with that `runtimeClassName`

#### Scenario: Unset changes nothing
- **WHEN** no install sets `global.agentops.runtimeClassName` and no component overrides it
- **THEN** every workload in this capability's scope renders with no `RuntimeClassName`, exactly as before this capability existed

### Requirement: Any component may override the global default

Each component in this capability's scope SHALL accept its own
`runtimeClassName` value, overriding the global default for that
component alone. Every other component SHALL keep inheriting the global
default unaffected.

#### Scenario: One component opts out of the global default
- **WHEN** an install sets `global.agentops.runtimeClassName` and also sets a different value on the manager's own workload
- **THEN** the manager renders with its own value, and every other in-scope workload still renders with the global default

#### Scenario: One component opts in without a global default
- **WHEN** no global default is set and one bundle's MCP server sets its own `runtimeClassName`
- **THEN** only that server's workload renders with a `RuntimeClassName`, and every other workload renders with none

### Requirement: The value is never validated against the cluster

The chart and the manager SHALL NOT check that a named `RuntimeClass`
exists. Naming a runtime class is a reference, the same posture naming a
`serviceAccountName` already has.

A workload naming a `RuntimeClass` the cluster does not have SHALL fail
at pod admission, never at render time and never at reconcile time.

#### Scenario: An unknown runtime class is not caught early
- **WHEN** a component's `runtimeClassName` names no `RuntimeClass` the cluster has
- **THEN** the chart renders successfully, the workload's controller reports no error, and the pod is refused at admission naming the class

### Requirement: No gVisor-specific or vendor-specific logic exists anywhere

`runtimeClassName` SHALL be a plain string passthrough. Nothing in the
chart or the manager SHALL branch on its value, check it against a list
of known sandbox runtimes, or assume what kernel it selects.

#### Scenario: An operator uses a runtime class this project never named
- **WHEN** an install sets `runtimeClassName` to a value naming a `RuntimeClass` this project's own docs never mention
- **THEN** every workload in this capability's scope renders and behaves identically to how it would with `runsc` or any other value
