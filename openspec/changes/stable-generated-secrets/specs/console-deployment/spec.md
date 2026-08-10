## MODIFIED Requirements

### Requirement: Console ships as an opt-in chart bundle of CRs
The console SHALL ship as a chart bundle of CRs and RBAC — a ChannelAdapter, a Channel, an externally-served SignalAdapter, a SignalSource, the UI token Secret, and a read-only Role/RoleBinding — and SHALL be **enabled by default** (`console.enabled: true`). The chart SHALL ship no workload or connectivity of its own: the ChannelAdapter reconciler owns the Deployment and, because `spec.port` is set, the Service.

Setting `console.enabled: false` SHALL remove the CRs and with them the Deployment, pod and Service; Channels naming `adapter: console` SHALL report `Served=False`, conversations SHALL keep their other threads, and no other component SHALL be affected.

Because this default changes from `false`, an upgrade starts a workload that was not previously running and that reads every `agentops.dev` CR in the namespace. The chart major SHALL be bumped and `CHANGELOG.md` SHALL carry the migration entry naming the one-value opt-out.

The UI token Secret SHALL take its value from a defined source order — an explicitly configured token, then an existing Secret, then generation — and SHALL NOT change on an upgrade that configures none. Signing every browser out is a consequence an operator asks for, never one a redeploy causes.

#### Scenario: Default install has a console
- **WHEN** the chart is installed at defaults
- **THEN** the console pod runs and is reachable on its ClusterIP Service

#### Scenario: Enabling the console is CRs-only
- **WHEN** the console is enabled and the release upgraded
- **THEN** the ChannelAdapter/Channel/SignalAdapter/SignalSource/Secret are applied and the reconcilers bring up the console workload and Service with no chart-owned workload objects

#### Scenario: Opting out is clean
- **WHEN** `console.enabled=false` is applied to a running install
- **THEN** the Deployment, pod and Service are removed, console-served Channels report `Served=False`, and every other pipeline keeps delivering

#### Scenario: Disabling is non-destructive to conversations
- **WHEN** `console.enabled` is flipped to false
- **THEN** the console workload and Service are removed, referencing Channels report `Served=False`, and existing Conversations keep their other channel threads

#### Scenario: Redeploying does not sign anyone out
- **WHEN** an install whose UI token was generated is upgraded without configuring one
- **THEN** the token is unchanged and browser sessions established from it keep working
