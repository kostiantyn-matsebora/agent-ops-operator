# Proposal: adapter-sender-self-registration

## Why

The vm-alertmanager adapter is a push-model receiver: the *sender*
(VMAlertmanager) must be configured with the webhook URL. Today that last
mile is manual (edit the alertmanager config secret), which the user has
rejected: the adapter should tell the sender where to push, driven by
configuration residing on the SignalSource. The blocker was architectural:
adapter pods have zero Kubernetes authority by invariant, so no in-cluster
actor could write sender-side config.

## What Changes

- **`SignalAdapter.spec.kubernetesAccess`** (implementation property, default
  false): when true, the reconciler mounts the adapter SA token
  (`automountServiceAccountToken: true`) and injects `POD_NAMESPACE`. The
  operator still grants **zero RBAC** — permissions are deployment
  configuration, bound by the chart/user to the adapter's deterministic SA
  (`agentops-signal-<name>`). The invariant is refined, not dropped:
  *adapters get no permissions from the operator, ever*.
- **Registration config on the SignalSource** (`spec.config.register`,
  opaque to the manager, interpreted by the serving adapter):
  `{vmalertmanager: {name, namespace}, matchers?, sendResolved?}`.
- **signal-vmalertmanager self-registers**: for each served source carrying
  `register`, it ensures a `VMAlertmanagerConfig` named `agentops-<source>`
  in the target namespace (receiver + continue-route pointing at its own
  webhook URL) via the Kubernetes REST API — stdlib only, module stays
  dependency-free. vm-operator merges it into the running alertmanager.
- **Graceful degradation**: any registration failure (token not mounted,
  403, CRD absent, target missing) keeps the source served and reports the
  Ready condition with **exact manual instructions** (the webhook URL and a
  ready-to-paste snippet), retried every registry poll — granting the
  missing permission self-heals without restarts.
- **vm-bundle** gains a `registration` block: renders the Role+RoleBinding
  (namespaced, `vmalertmanagerconfigs` only) for the adapter SA, sets
  `kubernetesAccess: true` on the SignalAdapter, and puts `register` into
  the default source's config.

## Impact

- API: `SignalAdapter.spec.kubernetesAccess` (+ CRD regen); no other CRD
  changes (register rides the opaque config).
- Manager: SignalAdapter reconciler (automount + `POD_NAMESPACE`); nothing
  else — the manager never interprets `register`.
- signal-vmalertmanager 0.3.0: k8s REST client (stdlib), registration loop,
  instruction-bearing status messages.
- Chart 1.8.0 / manager 0.10.0; vm-bundle registration component.
- Live: replaces the manual VMAlertmanager repoint entirely — the pending
  0.9.0 cutover rides this change's deploy.
