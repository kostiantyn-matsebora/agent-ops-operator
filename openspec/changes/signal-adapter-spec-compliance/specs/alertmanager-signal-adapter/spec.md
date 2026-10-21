# alertmanager-signal-adapter delta

## MODIFIED Requirements

### Requirement: The adapter self-registers with the sender from source config

For each served SignalSource whose opaque config carries a `register` block,
the adapter SHALL ensure a `VMAlertmanagerConfig` named `agentops-<source>`
in the block's target namespace.

The object SHALL contain a webhook receiver pointing at the adapter's own
endpoint
(`http://agentops-signal-<adapter>.<POD_NAMESPACE>.svc:<port>/webhook/<source>`)
and a route with `continue: true`, never diverting alerts from existing
receivers.

The block's fields, every one optional:

| Field | Means |
|---|---|
| `namespace` | the namespace to register in — absent, the adapter's own namespace |
| `matchers`, `groupWait`, `groupInterval`, `repeatInterval`, `maxAlerts`, `sendResolved` | route and receiver knobs, together enough to fully REPLACE a hand-written receiver plus route |

There is no `name` field. The registered object's name is always the derived
`agentops-<source>`. Unset knobs SHALL be omitted from the rendered object
rather than written as zero values, so the sender's own default applies.

The ensure SHALL use the in-cluster Kubernetes REST API with the mounted
ServiceAccount token, implemented with the standard library only — the module
stays dependency-free.

It SHALL run on every registry poll: created when absent, updated on drift.
Sources without `register` SHALL behave exactly as before. On success the
source's Ready condition SHALL report `reason: AdapterReady` naming the
registered object.

#### Scenario: Registration from source config
- **WHEN** a served source's config carries `register: {namespace: monitoring}` and the adapter pod has a token with write access to vmalertmanagerconfigs in `monitoring`
- **THEN** a `VMAlertmanagerConfig` `monitoring/agentops-<source>` exists with the adapter's webhook URL and a continue-route, and the source's Ready message names it

#### Scenario: An empty register block registers beside the adapter
- **WHEN** a served source's config carries `register: {}` with no `namespace`
- **THEN** the object is ensured in the adapter's own namespace, under the same derived name

#### Scenario: Drift is repaired
- **WHEN** the registered object is edited to point elsewhere
- **THEN** a subsequent poll restores the adapter's webhook URL

#### Scenario: Source-owned routing replaces a hand-written receiver
- **WHEN** a source's `register` block carries matchers plus `groupWait`, `repeatInterval` and `maxAlerts`
- **THEN** the rendered route carries those matchers and timings and the webhook carries `max_alerts`, so the equivalent hand-written receiver and route can be deleted from the sender's own config

#### Scenario: An unset knob is omitted, not zeroed
- **WHEN** a source's `register` block leaves `sendResolved` unset
- **THEN** the rendered webhook carries no `send_resolved` key at all, so the sender's own default governs

#### Scenario: No register block, no registration
- **WHEN** a served source's config carries no `register`
- **THEN** the adapter performs no Kubernetes API calls for it and the Ready message names the webhook path as before

### Requirement: Registration failure degrades to instructions, never to an unserved source

When registration cannot be performed, the adapter SHALL keep serving the
source (the webhook stays live) and SHALL report the Ready condition True
with `reason: RegistrationManual`.

The message SHALL state the cause plus the exact manual action: the full
webhook URL and the object to create (`VMAlertmanagerConfig
<ns>/agentops-<source>` or an equivalent `webhook_configs` entry).

The degradation cases, each loud and none silent:

| Cause | The message additionally states |
|---|---|
| token not mounted, or the API forbidden (RBAC missing) | the grant to bind — against the account `spec.serviceAccountName` names |
| `VMAlertmanagerConfig` CRD absent, or target unreachable | the manual `webhook_configs` form |
| a malformed `register` block | what failed to parse — never treated as an absent block |
| the adapter cannot determine its own webhook URL (hand-deployed with no namespace fact) | the URL shape to fill in — never a placeholder written into the sender's live config |

A placeholder URL in a registered object looks registered and delivers
nothing, so computing no URL SHALL mean registering no object.

The message SHALL name only fields the current CRD carries, so following it
can actually heal the condition.

The attempt SHALL be retried on every registry poll, so externally granting
the missing permission or correcting the block heals the condition without a
restart.

#### Scenario: Missing RBAC yields instructions
- **WHEN** the adapter's ensure call returns 403
- **THEN** the source stays served and its Ready message contains the webhook URL and the manual registration step naming the target namespace

#### Scenario: A malformed register block is named, not ignored
- **WHEN** a served source's `register` block cannot be parsed
- **THEN** the source stays served, Ready reports `RegistrationManual` naming the parse problem, and no registration call is attempted

#### Scenario: An uncomputable webhook URL registers nothing
- **WHEN** the adapter holds a token but cannot determine its own namespace, so it cannot compute its webhook URL
- **THEN** no object is written, and Ready reports `RegistrationManual` with the URL shape the operator must fill in

#### Scenario: The remediation can be followed
- **WHEN** registration fails for a missing grant
- **THEN** the message names `spec.serviceAccountName` and the binding to create, and names no field the CRD does not carry

#### Scenario: Granting permission self-heals
- **WHEN** the operator later binds the required Role to the adapter's ServiceAccount
- **THEN** a subsequent poll registers successfully and the Ready message flips to the registered-object form
