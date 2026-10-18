## MODIFIED Requirements

### Requirement: Reconciler owns the workload and names its identity

The SignalAdapter reconciler SHALL render and own (ownerRef) a Deployment per
adapter. **IT SHALL CREATE NO ServiceAccount.** It SHALL name the account
resolved from `spec.serviceAccountName`, falling back to the release's FLOOR
account, and SHALL mount that account's token.

The Deployment SHALL carry:

- `MANAGER_URL`
- **`ADAPTER_NAME`** (the CR's name — the same env name channel adapters receive, replacing `SOURCE_TYPE`)
- `POD_NAMESPACE`
- the per-adapter derived token

It SHALL run `replicas 1` with `Recreate` when singleton, and project each
served SignalSource's `credentialsSecretRef` as `envFrom` under
`AGENTOPS_CRED_<SOURCE>_`. Source changes roll the pod.

When `spec.port` is set the reconciler SHALL also own a Service
`agentops-signal-<name>` targeting that port and inject `LISTEN_ADDR=:<port>`.
Deleting the SignalAdapter SHALL remove workload and Service. SignalSources
select the adapter by `spec.adapter` equalling the CR's name.

**EVERY SIGNAL ADAPTER SHARES ONE `app.kubernetes.io/name`, SO IT CANNOT TELL
INSTANCES APART.** `cron`, `alertmanager`, `k8s-events` and `ha` all render
the same value today, collapsing every instance into one node in
label-grouping tooling — a service map, a dashboard, `kubectl get pods -l`.

The Deployment, its Pod template, and its Service (when rendered) SHALL
additionally carry `app.kubernetes.io/instance: <CR name>`.

`app.kubernetes.io/name` is UNCHANGED. `chart/templates/networkpolicy.yaml`
selects ingress to the manager by that exact value for every signal adapter,
so repurposing it would break that policy. `app.kubernetes.io/instance` is
additive only — no existing Deployment, Service, or NetworkPolicy selector
reads it.

**NO RECONCILER CREATES AN IDENTITY, AND THIS WAS THE ONLY ONE THAT DID.**
Every other identity in this API is a reference the chart creates.

The exception cost what an exception costs. The manager is forbidden from
binding RBAC to an adapter — correctly, since a `SignalAdapter` is an
ordinary namespaced object, and a reconciler that could grant one would make
CR-edit rights a privilege escalation.

So it created accounts it was not allowed to grant anything to. On the
reference install six existed and one was bound.

**THE GRANT AND THE IDENTITY ARE NOW IN ONE FILE.** The bundle that knows an
adapter reads Events writes the ClusterRole, the account and the CR's
reference together.

A reviewer answers "what can this adapter do" by reading one object rather
than a chart template plus a Go reconciler.

#### Scenario: Deployment shape matches the channel-adapter posture
- **WHEN** a singleton SignalAdapter's Deployment is rendered for two credentialed sources
- **THEN** it runs replicas 1/Recreate under the account it names, carries both projected credential prefixes, and its `ADAPTER_TOKEN` is the signal-context derived token

#### Scenario: Port-declaring adapter gets its Service
- **WHEN** a SignalAdapter with `port: 8080` is reconciled
- **THEN** a Service `agentops-signal-<name>` targeting 8080 exists, owned by the adapter, and the pod runs with `LISTEN_ADDR=:8080`

#### Scenario: Portless adapter gets no Service
- **WHEN** a SignalAdapter without `port` is reconciled (e.g. cron)
- **THEN** no Service is rendered

#### Scenario: The reconciler creates no ServiceAccount
- **WHEN** any SignalAdapter is reconciled
- **THEN** no ServiceAccount object is created by the operator, whatever the CR declares

#### Scenario: Contract env carries the adapter's identity
- **WHEN** a SignalAdapter named `cron` is reconciled
- **THEN** its pod runs with `ADAPTER_NAME=cron` and no `SOURCE_TYPE`

#### Scenario: Distinct instances carry distinct instance labels
- **WHEN** `SignalAdapter`s named `cron` and `alertmanager` are both reconciled
- **THEN** both Deployments carry `app.kubernetes.io/name: agentops-signal-adapter`, and their Pod templates carry `app.kubernetes.io/instance: cron` and `app.kubernetes.io/instance: alertmanager` respectively
