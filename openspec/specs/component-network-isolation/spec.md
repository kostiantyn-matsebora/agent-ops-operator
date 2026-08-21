# component-network-isolation Specification

## Purpose
Chart-level restriction of which pods may reach agent-ops components, so that a component's assumption that its callers are wired stops being an assumption, together with the honest statement of what that restriction cannot guarantee.

## Requirements

### Requirement: The chart can restrict reach to every component it deploys
The chart SHALL be able to restrict, at the network level, which pods may reach the components it deploys — the manager, the channel and signal adapters, the console, the runtime pods, and the MCP server workloads shipped by bundles. Restriction SHALL default to DENYING ingress to those components, with allowances added only for the flows the installation actually wires.

#### Scenario: An unwired pod cannot reach a component
- **WHEN** restriction is enabled and a pod that is not part of the installation connects to a deployed component
- **THEN** the connection does not reach that component

#### Scenario: Bundle-shipped MCP servers are covered
- **WHEN** restriction is enabled and a bundle deploys an MCP server workload
- **THEN** that workload is restricted on the same terms as the components the parent chart deploys

### Requirement: Every wired flow keeps working when restriction is enabled
Enabling restriction SHALL NOT break any flow the installation declares. Runtime pods SHALL keep reaching the manager and the MCP servers they are wired to, adapters SHALL keep reaching the manager, the ingest router SHALL keep reaching the adapters it pushes to, the console SHALL keep serving through its configured ingress, and metrics collection SHALL keep reaching the manager's metrics endpoint.

#### Scenario: A conversation completes end to end under restriction
- **WHEN** restriction is enabled and a signal opens a conversation that answers on a bound channel
- **THEN** the conversation dispatches, uses its bound MCP servers, and delivers its answer

#### Scenario: Metrics collection survives
- **WHEN** restriction is enabled and a metrics collector scrapes the manager
- **THEN** scraping succeeds

### Requirement: Restriction is opt-in and its scope is an install decision
Restriction SHALL be off unless the installation asks for it, and asking for it SHALL be a single decision rather than a per-component chore. An installation SHALL be able to permit additional callers it knows about — a collector, an ingress controller, an operator's own tooling — without editing templates.

#### Scenario: Default install is unchanged
- **WHEN** the chart is installed without asking for restriction
- **THEN** no network restriction object is created and connectivity is exactly as before

#### Scenario: An extra caller can be permitted
- **WHEN** an installation must allow a component of its own to reach the manager
- **THEN** it can express that in values, without modifying chart templates

### Requirement: The install is told that enforcement cannot be verified
Because enforcement depends on cluster infrastructure the chart can neither require nor detect, the post-install output SHALL state plainly that these objects apply successfully on a cluster that does not enforce them and protect nothing in that case, and SHALL name what remains exposed. The output SHALL NOT describe the components as protected. It SHALL say what to check to establish whether enforcement is real.

#### Scenario: Enabled restriction is reported with its caveat
- **WHEN** the chart is installed with restriction enabled
- **THEN** the output states that unenforced objects protect nothing, names what would remain reachable, and tells the operator how to verify enforcement

#### Scenario: Disabled restriction is reported as exposure
- **WHEN** the chart is installed without restriction and components that accept unauthenticated callers are deployed
- **THEN** the output names that exposure rather than staying silent

### Requirement: Restriction is not presented as authentication
Restriction bounds WHO MAY CONNECT and SHALL NOT be documented, named or reported as authenticating anyone. Components that accept unauthenticated callers SHALL continue to be described as doing so, for every caller still permitted to reach them.

#### Scenario: Documentation keeps the distinction
- **WHEN** the isolation feature is documented
- **THEN** it states which components still accept unauthenticated callers within the permitted set
