## MODIFIED Requirements

### Requirement: The bundle ships as a self-gated subchart named for the protocol, not a vendor
The bundle SHALL be named `prometheus`, dropping the `-bundle` suffix, matching
`kubernetes`, `home-assistant` and `telegram`.

Naming it for the PROTOCOL rather than a vendor is unchanged, and is why the name
works: it reads the STANDARD Alertmanager payload, which vanilla Alertmanager and
VictoriaMetrics both send.

It SHALL remain self-gated and off by default, and demo mode SHALL still not
enable it. **Every retired key SHALL FAIL the render**, naming its replacement.

#### Scenario: The bundle is named for the protocol, without a suffix
- **WHEN** an install enables the Prometheus bundle
- **THEN** it does so under a key naming the protocol, with no suffix

#### Scenario: Default install renders nothing from the bundle
- **WHEN** the chart is installed with default values
- **THEN** no SignalAdapter, SignalSource, MCPConfig, MCPToolset, AgentProfile, Pipeline, route ServiceAccount or workload from the bundle is rendered

#### Scenario: The route's identity is the bundle's, the substrate is not
- **WHEN** the wiring component renders with no `serviceAccountName` set on it
- **THEN** a ServiceAccount for that route is rendered with no Kubernetes RBAC bound to it, while the runtime defaults, the floor account and the context volume still come from the parent chart

#### Scenario: Demo mode does not enable this bundle
- **WHEN** the chart is installed with `global.demo.enabled=true` and default bundle values
- **THEN** no object from this bundle renders, because it consumes endpoints a demo cluster does not have

#### Scenario: The retired values key fails the render
- **WHEN** an install upgrades while still carrying a retired key for this bundle
- **THEN** the render FAILS naming the replacement, rather than succeeding with a bundle that silently renders nothing

#### Scenario: A Prometheus install needs no VictoriaMetrics knowledge
- **WHEN** an operator enables the bundle against Prometheus and Alertmanager
- **THEN** the ingest lane, the metrics tooling, the profile and the wiring all render and function, and the only VictoriaMetrics-specific value is the self-registration component, which stays off
