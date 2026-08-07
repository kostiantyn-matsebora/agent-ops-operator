# vm-bundle — delta

## ADDED Requirements

### Requirement: The registration component wires the in-cluster VMAlertmanager declaratively
When `alertmanager.registration.enabled=true` (default false) with a
required `registration.vmalertmanager: {name, namespace}`, the bundle SHALL:
set `kubernetesAccess: true` on the rendered SignalAdapter; render a Role
scoped to `vmalertmanagerconfigs.operator.victoriametrics.com`
(get/list/create/update/patch) plus a RoleBinding for the adapter's
deterministic ServiceAccount (`agentops-signal-<name>`) into the
VMAlertmanager's namespace; and put the `register` block (target, optional
`matchers`/`sendResolved`) into the default source's opaque config — so one
flag yields an end-to-end path where the sender is configured by the
adapter, with no manual alertmanager edits. Rendering SHALL fail loudly when
`registration.enabled` is set without the target reference. Install notes
SHALL state whether registration is automatic or print the manual webhook
URL. The documentation SHALL note the vm-operator namespace-matcher caveat
(`VMAlertmanager.spec.disableNamespaceMatcher` for cluster-wide routing).

#### Scenario: One flag wires the sender
- **WHEN** the bundle renders with `registration.enabled=true` and a valid `vmalertmanager` reference alongside the default source
- **THEN** the SignalAdapter carries `kubernetesAccess: true`, the Role+RoleBinding land in the target namespace bound to `agentops-signal-<adapter>`, and the SignalSource config carries the `register` block

#### Scenario: Registration without a target fails at render time
- **WHEN** `registration.enabled=true` but `registration.vmalertmanager` is empty
- **THEN** rendering fails naming the missing value

#### Scenario: Disabled registration changes nothing
- **WHEN** the bundle renders with registration disabled
- **THEN** no RBAC objects render, `kubernetesAccess` is unset, and the source config carries no `register` block
