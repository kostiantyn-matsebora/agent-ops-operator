## MODIFIED Requirements

### Requirement: The HA signal adapter is a standalone, dependency-free module

`signals/ha/` SHALL be its own Go module with no dependencies outside this
repository, reaching Home Assistant over plain HTTP and the manager over the
`/signal/*` contract. It SHALL hold no Kubernetes client and SHALL NAME NO
ServiceAccount, because its data source is Home Assistant, not the cluster —
so it runs as the release floor, an account bound to nothing.

It SHALL normalize what it observes into signals — `fingerprint`, `labels`,
optional `title`, `payload`, `kind` — and post them to the manager's inbound
endpoint. It SHALL apply no grouping, cooldown or recurrence logic: those stay
manager-side, driven by the `SignalSource`.

#### Scenario: Adapter normalizes and posts
- **WHEN** the adapter observes a Home Assistant condition matching its configuration
- **THEN** it posts a normalized signal to the manager and performs no grouping of its own

#### Scenario: No cluster access
- **WHEN** the adapter's workload is rendered
- **THEN** it names no account of its own, runs as the release floor, and holds no Kubernetes permissions
