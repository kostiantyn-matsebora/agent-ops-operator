# signal-adapter-lifecycle delta

## MODIFIED Requirements

### Requirement: SignalAdapter CRD declares an implementation, never credentials

The `SignalAdapter` CRD SHALL mirror `ChannelAdapter` as pure implementation: required `spec.image`, workload-run properties (`singleton` defaulting true, `resources`), and an optional `spec.port` declaring the port the image's own HTTP surface listens on (webhook-receiving implementations). It SHALL carry no `env`, no `type`, and no credentials.

Kubernetes reach is `spec.serviceAccountName` alone. The retired `spec.kubernetesAccess` toggle is DELETED with no alias, per the identity requirement below — this spec SHALL state that nowhere else does it still exist.

**The CR name IS the routing key**: SignalSources whose `spec.adapter` equals the adapter's name are served by it, and one adapter per implementation holds by construction.

#### Scenario: Plug a new signal kind with CRs only
- **WHEN** a `SignalAdapter` named `cron` naming an image is applied, followed by a `SignalSource` with `spec.adapter: cron`
- **THEN** the adapter workload is created and the source is served without any operator, chart, or helm change

#### Scenario: Duplicate implementation impossible by construction
- **WHEN** a second `SignalAdapter` with the same name is applied
- **THEN** the API server rejects it as an ordinary name conflict — no TypeConflict condition exists
