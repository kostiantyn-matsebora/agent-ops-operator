# channel-adapter-lifecycle — delta

## MODIFIED Requirements

### Requirement: ChannelAdapter CRD declares an implementation, never credentials
The `ChannelAdapter` CRD SHALL declare a channel-type implementation as pure implementation: required `spec.image` plus workload-run properties (`singleton` defaulting true, `resources`). It SHALL carry **no configuration surface**: there is no `env` field and no credentials — per-surface settings live on the served Channels (`config`, `credentialsSecretRef`). **The CR name IS the routing key**: Channels whose `spec.type` equals the adapter's name are served by it; one adapter per implementation holds by construction (Kubernetes name uniqueness), with no conflict machinery.

#### Scenario: Plug a new channel type with CRs only
- **WHEN** a `ChannelAdapter` named `slack` naming an image is applied, followed by a `Channel` with `spec.type: slack`
- **THEN** the adapter workload is created and serves the channel without any operator, chart, or helm change

#### Scenario: No configuration on the implementation
- **WHEN** a `ChannelAdapter` manifest carries an `env` or `type` field against the new CRD
- **THEN** the fields are rejected/pruned — configuration belongs on Channels, and the name already identifies what the adapter serves

#### Scenario: Duplicate implementation impossible by construction
- **WHEN** a second `ChannelAdapter` with the same name is applied
- **THEN** the API server rejects it as an ordinary name conflict — no TypeConflict condition exists

### Requirement: Reconciler owns the adapter workload
The ChannelAdapter reconciler SHALL create and own (ownerRef, GC) a Deployment per ChannelAdapter: injecting `MANAGER_URL`, `CHANNEL_TYPE` (= the CR name), and the per-adapter derived auth token; running under a dedicated ServiceAccount with no RBAC and `automountServiceAccountToken: false`; applying `replicas: 1` + `strategy: Recreate` when `singleton` is true. Deleting the ChannelAdapter SHALL remove the workload. All injected environment is appliance-owned by the reconciler — none of it is user-specified.

#### Scenario: Singleton discipline enforced
- **WHEN** a singleton ChannelAdapter's Deployment is rendered
- **THEN** it has replicas 1 and Recreate strategy, so no rollout ever runs two instances side by side

#### Scenario: Zero ambient authority
- **WHEN** the adapter pod starts
- **THEN** it has no ServiceAccount token mounted and its SA has no RBAC bindings
