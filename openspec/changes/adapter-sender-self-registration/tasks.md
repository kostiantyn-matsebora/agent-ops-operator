# Tasks: adapter-sender-self-registration

## 1. API

- [x] 1.1 `SignalAdapterSpec.KubernetesAccess *bool` (implementation property, default false); regen deepcopy + CRDs

## 2. Manager

- [x] 2.1 SignalAdapter reconciler: `kubernetesAccess` flips pod `automountServiceAccountToken` true + injects `POD_NAMESPACE` (downward API); nothing else — no RBAC creation. Integration tests: token+namespace when set, unchanged posture when unset

## 3. signal-vmalertmanager 0.3.0

- [x] 3.1 Stdlib in-cluster k8s REST client (token/CA/namespace from the SA mount); parse `register` from source config in the listing; ensure `VMAlertmanagerConfig agentops-<source>` (webhook receiver + continue-route, matchers, sendResolved) each poll — create/update on drift
- [x] 3.2 Status: success → Ready `AdapterReady` naming the object; failure (no token/403/CRD absent/target bad) → Ready `RegistrationManual` with cause + webhook URL + manual step; retried each poll; unit tests with a fake API server

## 4. Chart, docs

- [x] 4.1 vm-bundle `alertmanager.registration` block: `kubernetesAccess` on the adapter, Role+RoleBinding in the target namespace, `register` in defaultSource config, render-fail without target, NOTES.txt; parent chart 1.8.0 / manager 0.10.0; README (registration + namespace-matcher caveat) + CLAUDE.md invariant refinement
- [x] 4.2 Build + push manager 0.10.0 and signal-vmalertmanager 0.3.0; commit

## 5. Live (home-data-center)

- [ ] 5.1 Deploy: CRDs, helm upgrade 1.8.0 with registration enabled (delete `vm-alerts` source first — immutable type change from the pending 0.9.0 cutover rides along); verify self-registration → VMAlertmanagerConfig exists → alerts flow via adapter; then retire old built-in source, its pipeline claim, and hand the user the one-line removal of the stale receiver from the vmks-owned config secret
