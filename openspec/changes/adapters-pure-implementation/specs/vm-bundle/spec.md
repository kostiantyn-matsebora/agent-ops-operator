# vm-bundle — delta

## MODIFIED Requirements

### Requirement: The alertmanager component packages the adapter with its webhook Service
When active, the `alertmanager` component SHALL render the `SignalAdapter` CR only (values-configured image, `port: 8080`, singleton) — **the Service and `LISTEN_ADDR` are appliance owned by the SignalAdapter reconciler via `spec.port`; the chart ships no connectivity** — plus the optional default `SignalSource` and the `Pipeline` claiming it (gated on `defaultSource.enabled` with a strictly required `defaultSource.profileRef`). The default SignalSource's `spec.type` SHALL render from the adapter's name value, so configuration can never drift from the implementation it targets. The pod-label selector contract remains pinned by an integration-test assertion.

#### Scenario: One flag exposes a working webhook URL
- **WHEN** the chart is installed with `vm-bundle.enabled=true` and a served source exists
- **THEN** `http://agentops-signal-<name>.<ns>.svc:8080/webhook/<source>` accepts Alertmanager webhooks — the Service comes from the reconciler, not the chart

#### Scenario: Source type follows the adapter name
- **WHEN** the bundle renders with `alertmanager.name: vm-alertmanager` and the default source enabled
- **THEN** the SignalSource carries `spec.type: vm-alertmanager` and the adapter serves it

#### Scenario: Default source requires an explicit profile
- **WHEN** `alertmanager.defaultSource.enabled=true` and `defaultSource.profileRef` is empty
- **THEN** rendering fails with a message naming the missing value rather than emitting an unwired SignalSource whose signals would drop
