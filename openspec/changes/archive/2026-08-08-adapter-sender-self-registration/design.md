# Design: adapter-sender-self-registration

## D1 — `kubernetesAccess` is an implementation property; permissions stay external

Whether an implementation talks to the Kubernetes API is a property of the
implementation (like `port`), so it lives on `SignalAdapter.spec`. What it
may DO there is deployment configuration, so it lives outside the operator:
a Role/RoleBinding bound to the deterministic SA name
(`agentops-signal-<name>`) by the bundle or the user. The reconciler only
flips `automountServiceAccountToken` and injects `POD_NAMESPACE` (downward
API). No RBAC objects are ever created by the operator — no escalate/bind
verbs needed, no privilege-escalation path through adapter CRs. ChannelAdapter
can adopt the same property later if a channel implementation needs it;
this change keeps it signal-side.

## D2 — `register` rides the opaque source config

`SignalSource.spec.config.register` is interpreted only by
signal-vmalertmanager (the manager stays type-blind):

```yaml
config:
  register:
    vmalertmanager: {name: vmks, namespace: victoria-metrics}  # required
    matchers: ['severity=~"warning|critical"']                 # optional
    sendResolved: false                                        # optional
```

Sources without `register` behave exactly as today (webhook path named in
the Ready message). A webhook inbox can still have N senders — `register`
automates one known VMAlertmanager; others register manually.

## D3 — self-registration via stdlib REST; deterministic object identity

The adapter ensures `VMAlertmanagerConfig` `agentops-<source>` in the target
namespace: webhook receiver `url:
http://agentops-signal-<adapter>.<POD_NAMESPACE>.svc:<port>/webhook/<source>`
plus a route with `continue: true` (never steals alerts from existing
receivers) and the configured matchers. In-cluster client from the mounted
SA (`/var/run/secrets/kubernetes.io/serviceaccount/`), raw HTTPS + JSON —
the module stays dependency-free. Reconciled on every registry poll (15s):
GET → create or update on drift. Deleting the SignalSource orphans the
object deliberately (the adapter has no watch; documented, and listed in the
manual-cleanup note of the Ready message when a source disappears is NOT
attempted — keep it simple).

## D4 — degradation carries instructions, and self-heals

Registration failure NEVER unserves the source (the inbox keeps working).
The Ready condition stays True with `reason: RegistrationManual` and a
message containing the cause and the exact manual action — the webhook URL
and a one-line `VMAlertmanagerConfig`/`webhook_configs` pointer. Success
reports `reason: AdapterReady` with `registered in VMAlertmanager
<ns>/<name>`. Because the loop retries every poll, granting RBAC or creating
the CRD later heals the condition without restarts. Caveat documented (not
solvable adapter-side): vm-operator adds a namespace matcher to
VMAlertmanagerConfig routes unless `VMAlertmanager.spec.disableNamespaceMatcher`
— cluster-wide alert routing needs that flag on the user's stack.

## D5 — vm-bundle owns the RBAC + wiring rendering

`alertmanager.registration: {enabled: false, vmalertmanager: {name,
namespace}, matchers: [], sendResolved: false}`. When enabled: SignalAdapter
gets `kubernetesAccess: true`; a Role (get/list/create/update/patch on
`vmalertmanagerconfigs.operator.victoriametrics.com`) + RoleBinding for the
adapter SA render into the VMAlertmanager's namespace; the default source's
config gains the `register` block. NOTES.txt states whether registration is
automatic or prints the manual URL.

## D6 — live cutover rides this change

Deploying 0.10.0 + registration replaces the manual repoint: the adapter
registers itself, VMAlertmanager starts pushing to the webhook, and only
then is the old built-in source retired (the old receiver entry in the
helm-owned config secret is the user's to remove — we provide the exact
edit in the cutover notes).
