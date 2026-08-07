# vm-alertmanager-signal-adapter — delta

## ADDED Requirements

### Requirement: The adapter self-registers with the sender from source config
For each served SignalSource whose opaque config carries a `register` block
(`register.vmalertmanager: {name, namespace}` required; `matchers`,
`groupWait`, `groupInterval`, `repeatInterval`, `maxAlerts` and
`sendResolved` optional — together enough to express everything a
hand-written receiver + route did, so a SignalSource can fully REPLACE one
rather than sit alongside it; unset knobs are omitted from the rendered
object rather than written as zero values), the adapter SHALL ensure a `VMAlertmanagerConfig`
named `agentops-<source>` in the target namespace containing a webhook
receiver pointing at its own endpoint
(`http://agentops-signal-<adapter>.<POD_NAMESPACE>.svc:<port>/webhook/<source>`)
and a route with `continue: true` (never diverting alerts from existing
receivers), using the in-cluster Kubernetes REST API with the mounted
ServiceAccount token — implemented with the standard library only (the
module stays dependency-free). The ensure SHALL run on every registry poll:
created when absent, updated on drift. Sources without `register` SHALL
behave exactly as before. On success the source's Ready condition SHALL
report `reason: AdapterReady` naming the registered object.

#### Scenario: Registration from source config
- **WHEN** a served source's config carries `register.vmalertmanager: {name: vmks, namespace: monitoring}` and the adapter pod has a token with write access to vmalertmanagerconfigs in `monitoring`
- **THEN** a `VMAlertmanagerConfig` `monitoring/agentops-<source>` exists with the adapter's webhook URL and a continue-route, and the source's Ready message names it

#### Scenario: Drift is repaired
- **WHEN** the registered object is edited to point elsewhere
- **THEN** a subsequent poll restores the adapter's webhook URL

#### Scenario: Source-owned routing replaces a hand-written receiver
- **WHEN** a source's `register` block carries matchers plus `groupWait`, `repeatInterval` and `maxAlerts`
- **THEN** the rendered route carries those matchers and timings and the webhook carries `max_alerts`, so the equivalent hand-written receiver and route can be deleted from the sender's own config

#### Scenario: No register block, no registration
- **WHEN** a served source's config carries no `register`
- **THEN** the adapter performs no Kubernetes API calls for it and the Ready message names the webhook path as before

### Requirement: Registration failure degrades to instructions, never to an unserved source
When registration cannot be performed — token not mounted, API forbidden
(RBAC missing), `VMAlertmanagerConfig` CRD absent, or target unreachable —
the adapter SHALL keep serving the source (webhook stays live) and SHALL
report the Ready condition True with `reason: RegistrationManual` and a
message stating the cause plus the exact manual action: the full webhook
URL and the object to create (`VMAlertmanagerConfig <ns>/agentops-<source>`
or an equivalent `webhook_configs` entry). The attempt SHALL be retried on
every registry poll so externally granting the missing permission heals the
condition without a restart.

#### Scenario: Missing RBAC yields instructions
- **WHEN** the adapter's ensure call returns 403
- **THEN** the source stays served and its Ready message contains the webhook URL and the manual registration step naming the target namespace

#### Scenario: Granting permission self-heals
- **WHEN** the operator later binds the required Role to the adapter's ServiceAccount
- **THEN** a subsequent poll registers successfully and the Ready message flips to the registered-object form
