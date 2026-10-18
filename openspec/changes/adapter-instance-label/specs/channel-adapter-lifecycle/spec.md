## MODIFIED Requirements

### Requirement: Reconciler owns the adapter workload

The ChannelAdapter reconciler SHALL render and own (ownerRef) a Deployment per
adapter. **IT SHALL CREATE NO ServiceAccount.**

It SHALL name the account resolved from `spec.serviceAccountName` — a
REFERENCE, with the same semantics as `SignalAdapter.spec.serviceAccountName`
— falling back to the release's FLOOR account, and SHALL mount that
account's token.

The Deployment SHALL carry:

- `MANAGER_URL`
- **`ADAPTER_NAME`** (the CR's name — one env name across both adapter surfaces, replacing `CHANNEL_TYPE`)
- `POD_NAMESPACE`
- the per-adapter derived token

It SHALL run `replicas 1` with `Recreate` when singleton, and project each
served Channel's `credentialsSecretRef` as `envFrom` under
`AGENTOPS_CRED_<CHANNEL>_` (collisions reported, never silently overwritten).
Channel add/remove/credential changes roll the pod. Deleting the
ChannelAdapter SHALL remove the workload. Channels select the adapter by
`spec.adapter` equalling the CR's name.

The operator SHALL create and bind no RBAC. What the named account may do
remains an external grant against the account the chart rendered — for the
console, the read-only Role the chart binds to `agentops-adapter-<name>`.
That is the channel-adapter workload name. `agentops-channel-<name>` does not
exist.

**EVERY CHANNEL ADAPTER SHARES ONE `app.kubernetes.io/name`, SO IT CANNOT
TELL INSTANCES APART.** `console` and `telegram` both render
`agentops-adapter` today, collapsing both into one node in label-grouping
tooling — a service map, a dashboard, `kubectl get pods -l`.

This holds even for the console, which is itself an ordinary
`ChannelAdapter` CR and gets no special treatment.

The Deployment, its Pod template, and its Service (when rendered) SHALL
additionally carry `app.kubernetes.io/instance: <CR name>`.

`app.kubernetes.io/name` is UNCHANGED. `chart/templates/networkpolicy.yaml`
selects ingress to the manager by that exact value for every channel
adapter, so repurposing it would break that policy. `app.kubernetes.io/instance`
is additive only — no existing Deployment, Service, or NetworkPolicy
selector reads it.

#### Scenario: Singleton discipline enforced
- **WHEN** a singleton ChannelAdapter's Deployment is rendered
- **THEN** it has replicas 1 and Recreate strategy, so no rollout ever runs two instances side by side

#### Scenario: Zero ambient authority
- **WHEN** a ChannelAdapter naming no account starts
- **THEN** it runs as the floor account — every verb denied, and nothing the operator created or bound

#### Scenario: The reconciler creates no ServiceAccount
- **WHEN** any ChannelAdapter is reconciled
- **THEN** no ServiceAccount object is created by the operator, whatever the CR declares

#### Scenario: A channel adapter that reads the API names its account
- **WHEN** the console's ChannelAdapter names the account the chart granted its read-only Role
- **THEN** its pod runs as that account with the token mounted, and the grant and the identity are read from one file

#### Scenario: Contract env carries the adapter's identity
- **WHEN** a ChannelAdapter named `telegram` is reconciled
- **THEN** its pod runs with `ADAPTER_NAME=telegram` and no `CHANNEL_TYPE`

#### Scenario: Distinct instances carry distinct instance labels
- **WHEN** ChannelAdapters named `console` and `telegram` are both reconciled
- **THEN** both Deployments carry `app.kubernetes.io/name: agentops-adapter`, and their Pod templates carry `app.kubernetes.io/instance: console` and `app.kubernetes.io/instance: telegram` respectively
