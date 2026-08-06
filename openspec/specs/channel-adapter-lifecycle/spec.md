# channel-adapter-lifecycle

## Purpose

The ChannelAdapter CRD and its reconciler: declaring a channel-type implementation as a CR whose adapter workload the reconciler owns, with per-adapter derived auth, kubelet-only credential projection from Channels, and served/not-served visibility on both sides.

## Requirements

### Requirement: ChannelAdapter CRD declares an implementation, never credentials
The `ChannelAdapter` CRD SHALL declare a channel-type implementation: required immutable `spec.type` (the routing key it serves), required `spec.image`, and non-secret tuning (`env`, `resources`, `singleton` defaulting true). It SHALL NOT carry credentials: the reconciler MUST reject `env` entries that reference Secrets. At most one ChannelAdapter per type SHALL be active; a conflicting newer one gets a `TypeConflict` condition and is not deployed.

#### Scenario: Plug a new channel type with CRs only
- **WHEN** a `ChannelAdapter` naming an image for `type: slack` is applied, followed by a `Channel` with `spec.type: slack`
- **THEN** the adapter workload is created and serves the channel without any operator, chart, or helm change

#### Scenario: Secret-shaped env rejected
- **WHEN** a `ChannelAdapter` is applied with `env` containing a `secretKeyRef`
- **THEN** it is not deployed and a condition explains that credentials belong on `Channel.credentialsSecretRef`

#### Scenario: Duplicate type claim refused
- **WHEN** a second `ChannelAdapter` claims an already-served type
- **THEN** the newer one reports `TypeConflict` and no second workload is created

### Requirement: Reconciler owns the adapter workload
The ChannelAdapter reconciler SHALL create and own (ownerRef, GC) a Deployment per ChannelAdapter: injecting `MANAGER_URL` and the per-adapter derived auth token; running under a dedicated ServiceAccount with no RBAC and `automountServiceAccountToken: false`; applying `replicas: 1` + `strategy: Recreate` when `singleton` is true. Deleting the ChannelAdapter SHALL remove the workload.

#### Scenario: Singleton discipline enforced
- **WHEN** a singleton ChannelAdapter's Deployment is rendered
- **THEN** it has replicas 1 and Recreate strategy, so no rollout ever runs two instances side by side

#### Scenario: Zero ambient authority
- **WHEN** the adapter pod starts
- **THEN** it has no ServiceAccount token mounted and its SA has no RBAC bindings

### Requirement: Channel credentials are projected into the adapter pod
For every `Channel` of the adapter's type with `credentialsSecretRef`, the reconciler SHALL project the secret into the adapter pod spec as `envFrom` under the deterministic prefix `AGENTOPS_CRED_<CHANNEL>_` (every key `K` becomes env `<prefix>K`), resolved by the kubelet — no component reads secret values or key names through the API. Channel additions, removals, or credential changes SHALL roll the adapter pod. Sanitized-name collisions SHALL be reported as a condition, never silently overwritten.

#### Scenario: Multi-bot through one adapter
- **WHEN** two `type: telegram` Channels reference different bot-token Secrets
- **THEN** the single adapter pod carries both projected credentials and can serve both bots

#### Scenario: Credential change rolls the pod
- **WHEN** a Channel's `credentialsSecretRef` is changed to another Secret
- **THEN** the adapter Deployment's pod template changes and the pod restarts with the new projection

### Requirement: Unserved channel types are visible
A `Channel` whose `spec.type` has neither an in-process provider nor a Ready `ChannelAdapter` SHALL carry a `Served=False` condition; it SHALL flip to True when an adapter becomes ready. `ChannelAdapter.status` SHALL report deployment state and the adapter's contract-reported readiness.

#### Scenario: Typo'd type is diagnosable
- **WHEN** a Channel is created with `type: slak` and nothing serves that type
- **THEN** the Channel shows `Served=False` instead of ops queueing silently forever
