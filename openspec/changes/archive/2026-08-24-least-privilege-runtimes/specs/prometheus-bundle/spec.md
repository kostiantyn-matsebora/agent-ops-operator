## MODIFIED Requirements

### Requirement: Sender self-registration is a VictoriaMetrics-only sub-feature and says so
The `registration` sub-component SHALL keep its behavior: when enabled with a
required target reference it SHALL render the adapter's ServiceAccount BESIDE
its grant and NAME it on the rendered SignalAdapter — the operator creates no
account and binds no RBAC — plus a Role scoped to
`vmalertmanagerconfigs.operator.victoriametrics.com` and a RoleBinding for that
account in the target namespace, and place the
`register` block — target plus the optional routing knobs `matchers`,
`groupWait`, `groupInterval`, `repeatInterval`, `maxAlerts`, `sendResolved` —
into the source's opaque config. Rendering SHALL fail loudly when registration is
enabled without the target reference. Registration failure SHALL never unserve
the source: the webhook stays live and the cause plus the manual step are carried
on the source's Ready condition.

This sub-feature SHALL be documented as working ONLY against VictoriaMetrics, and
as being impossible to generalize: it configures the sender by writing a
`VMAlertmanagerConfig`, and vanilla Alertmanager's configuration is a file or a
Secret, not a Kubernetes object an adapter can write. Presenting it without that
statement inside a vendor-neutral bundle would read as a feature merely awaiting
implementation for other senders.

#### Scenario: One flag wires a VictoriaMetrics sender
- **WHEN** the bundle renders with registration enabled and a valid target
  alongside the default source
- **THEN** the SignalAdapter NAMES the account this component rendered, the Role
  and RoleBinding land in the target namespace bound to it, and the source's
  config carries the `register` block

#### Scenario: Registration without a target fails at render time
- **WHEN** registration is enabled with no target reference
- **THEN** the render fails naming the missing value

#### Scenario: A Prometheus install is told this does not apply to it
- **WHEN** an operator running vanilla Alertmanager reads the registration values
  or documentation
- **THEN** it states that registration is VictoriaMetrics-only because there is no
  sender-side object to write, and points at the printed receiver configuration
  instead
