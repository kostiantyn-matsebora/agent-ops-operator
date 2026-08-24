# signal-adapter-lifecycle

## Purpose

The SignalAdapter CRD and its reconciler: declaring a signal-type implementation as a CR whose adapter workload the reconciler owns with the channel-adapter security posture, per-type conflict guarding, credential projection from served sources, and Served visibility on SignalSources.

## Requirements
### Requirement: SignalAdapter CRD declares an implementation, never credentials
The `SignalAdapter` CRD SHALL mirror `ChannelAdapter` as pure implementation: required `spec.image`, workload-run properties (`singleton` defaulting true, `resources`), an optional `spec.port` declaring the port the image's own HTTP surface listens on (webhook-receiving implementations), and an optional `spec.kubernetesAccess` (default false) declaring that the implementation talks to the Kubernetes API. It SHALL carry no `env`, no `type`, and no credentials. **The CR name IS the routing key**: SignalSources whose `spec.adapter` equals the adapter's name are served by it; one adapter per implementation holds by construction.

#### Scenario: Plug a new signal kind with CRs only
- **WHEN** a `SignalAdapter` named `cron` naming an image is applied, followed by a `SignalSource` with `spec.adapter: cron`
- **THEN** the adapter workload is created and the source is served without any operator, chart, or helm change

#### Scenario: Duplicate implementation impossible by construction
- **WHEN** a second `SignalAdapter` with the same name is applied
- **THEN** the API server rejects it as an ordinary name conflict — no TypeConflict condition exists

### Requirement: Reconciler owns the workload and names its identity

The SignalAdapter reconciler SHALL render and own (ownerRef) a Deployment per
adapter. **IT SHALL CREATE NO ServiceAccount.** It SHALL name the account
resolved from `spec.serviceAccountName`, falling back to the release's FLOOR
account, and SHALL mount that account's token.

The Deployment SHALL carry `MANAGER_URL`, **`ADAPTER_NAME` (the CR's name — the
same env name channel adapters receive, replacing `SOURCE_TYPE`)**,
`POD_NAMESPACE`, and the per-adapter derived token; `replicas 1 + Recreate` when
singleton; each served SignalSource's `credentialsSecretRef` projected as
`envFrom` under `AGENTOPS_CRED_<SOURCE>_`; source changes roll the pod. When
`spec.port` is set the reconciler SHALL also own a Service
`agentops-signal-<name>` targeting that port and inject `LISTEN_ADDR=:<port>`.
Deleting the SignalAdapter SHALL remove workload and Service. SignalSources
select the adapter by `spec.adapter` equalling the CR's name.

**NO RECONCILER CREATES AN IDENTITY, AND THIS WAS THE ONLY ONE THAT DID.** Every
other identity in this API is a reference the chart creates. The exception cost
what an exception costs: the manager is forbidden from binding RBAC to an
adapter — correctly, since a `SignalAdapter` is an ordinary namespaced object and
a reconciler that could grant one would make CR-edit rights a privilege
escalation — so it created accounts it was not allowed to grant anything to. On
the reference install six existed and one was bound.

**THE GRANT AND THE IDENTITY ARE NOW IN ONE FILE.** The bundle that knows an
adapter reads Events writes the ClusterRole, the account and the CR's reference
together, and a reviewer answers "what can this adapter do" by reading one
object rather than a chart template plus a Go reconciler.

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

### Requirement: An adapter's identity is a reference, and absent means the floor

`SignalAdapter.spec.serviceAccountName` SHALL name the ServiceAccount its
workload runs as. It is a REFERENCE: the operator SHALL NOT create it, SHALL NOT
validate that it exists, and SHALL NOT bind anything to it. The chart that
grants an adapter its permissions SHALL render the account beside that grant.

**NAMING AN ACCOUNT SHALL MEAN MOUNTING ITS TOKEN.** `spec.kubernetesAccess` is
DELETED with no alias — it was the same decision wearing a second name. Naming
an account whose token is never mounted grants nothing, because the pod never
presents that identity; mounting a token without naming an account mounts the
namespace default's. The combinations that were not the decision were all
meaningless.

`POD_NAMESPACE` SHALL be injected unconditionally. It is a downward-API field
naming the pod's own namespace, which is not a permission and never was.

**WHERE THE FIELD IS ABSENT the workload SHALL run as the release's FLOOR
account** — created always by the chart, bound to nothing, refused as a binding
target — with its token mounted.

- **Not the namespace `default` account.** That carries whatever the cluster
  gave it, so an adapter's reach would depend on something outside this release.
- **Not "no account and no token".** A pod holding no token cannot be told apart
  from one whose grant was forgotten, and an authenticated identity denied
  everything is a better report than an anonymous one.
- **One floor serves both lanes.** The property that matters is bound-to-nothing
  and refused-as-a-binding-target; a second empty account is a second thing to
  keep empty.

An adapter SHALL NOT share the runtime identity of any route. What an agent may
do is model output's reach; what an adapter may do is this project's own code's
reach, and they are opposite grants — an adapter reading Events cluster-wide is
a permission no agent should hold, and a route's workload writes are permissions
no adapter should hold.

#### Scenario: An adapter naming an account runs as it
- **WHEN** a SignalAdapter names a ServiceAccount the chart rendered
- **THEN** its pod runs as that account with the token mounted, and the operator creates and binds nothing

#### Scenario: An adapter naming nothing runs as the floor
- **WHEN** a SignalAdapter declares no `serviceAccountName`
- **THEN** its pod runs as the release's floor account, with the token mounted and every verb denied

#### Scenario: Naming an account that does not exist
- **WHEN** a SignalAdapter names an account nothing created
- **THEN** the operator reports nothing and creates nothing, and the pod fails at admission naming the account

#### Scenario: An adapter CR cannot escalate
- **WHEN** anyone able to create a SignalAdapter names any account at all
- **THEN** no RBAC object is created or bound by the operator, so the CR grants exactly what somebody already granted that account

### Requirement: Unserved signal types are visible
A `SignalSource` whose `spec.adapter` is not claimed by a Ready `SignalAdapter` (nor adapter-reported Ready) SHALL carry `Served=False`; it SHALL flip True when a serving adapter appears — same reason vocabulary as Channel's Served condition. There are no built-in signal types: every type needs an adapter. `kubectl get signalsources` SHALL surface useful state (type, received count) without dead columns.

#### Scenario: Typo'd type is diagnosable
- **WHEN** a SignalSource is created with `adapter: pagerdutty` and nothing serves it
- **THEN** the source shows `Served=False` instead of silently never producing conversations

#### Scenario: No type is served without an adapter
- **WHEN** a SignalSource names `adapter: alertmanagerWebhook` after the built-in removal and no `SignalAdapter` of that name exists
- **THEN** it shows `Served=False` — the manager itself serves nothing
