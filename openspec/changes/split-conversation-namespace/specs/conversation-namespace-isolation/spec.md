## ADDED Requirements

### Requirement: Conversation-scoped objects live in the conversations namespace
A release SHALL resolve two namespaces, a **control namespace** (the release
namespace) and a **conversations namespace**. An object SHALL live in the
conversations namespace if and only if one of these holds:

- it is a runtime pod, or a runtime pod runs as it, mounts it or resolves it
- it is a workload mounting a claim a runtime pod mounts, or such a workload
  resolves it
- it is conversation-scoped state

The conversations namespace SHALL hold:

- runtime pods `agentops-conv-<name>` and MCP ConfigMaps `agentops-mcp-conv-<name>`
- `Conversation` and `ConversationInput` CRs, under the `kube` state backend
- every `rbac.runtime.serviceAccounts` entry and every bundle route account
- every persistence claim, chart-rendered or manager-created
- the housekeeping and context-probe CronJobs

The control namespace SHALL hold everything else, including the manager,
every wiring and identity CR, `Signal` CRs and the rest of the state store,
NATS, adapters, the gateway, the console and every MCP server.

The floor account `agentops-runtime` SHALL render in both namespaces, bound
to nothing in either. Runtime pods and adapters both fall back to it, and each
can only use an account in its own namespace.

#### Scenario: A dispatched conversation's workload lands in the conversations namespace
- **WHEN** a conversation is dispatched in a split release
- **THEN** its runtime pod and its MCP ConfigMap are in the conversations namespace, and the pod runs as an account in that namespace

#### Scenario: Wiring is read from the control namespace
- **WHEN** the manager resolves a conversation's Pipeline, profile, runtime, channels, toolsets and MCP configs
- **THEN** every one of them is read from the control namespace

#### Scenario: Signals stay in the control namespace under kube
- **WHEN** the `kube` state backend stores a Signal and a Conversation it opened
- **THEN** the `Signal` CR is in the control namespace and the `Conversation` CR and its `ConversationInput`s are in the conversations namespace

#### Scenario: Transport credentials never share a namespace with agent code
- **WHEN** the chart renders a channel adapter, a signal adapter, the gateway or the console
- **THEN** the workload, its account and its credentials are in the control namespace

### Requirement: One code path serves both layouts
The manager SHALL carry a control and conversations namespace pair in place of
a single namespace:

- the pair SHALL be read from `NAMESPACE` and `CONVERSATION_NAMESPACE`, the
  latter defaulting to the former
- every lookup SHALL name the plane it uses
- equal values SHALL reproduce the single-namespace deployment with no branch
  on layout

The cache SHALL cover exactly the two namespaces, each kind only in the
namespace whose Role grants it, and leader election SHALL stay in the control
namespace.

#### Scenario: Equal namespaces reproduce today's layout
- **WHEN** `CONVERSATION_NAMESPACE` equals `NAMESPACE`
- **THEN** every object renders and is created in that one namespace, as before the split

#### Scenario: The cache is two namespaces, never the cluster
- **WHEN** the manager starts with two distinct namespaces
- **THEN** it watches those two namespaces and no other

#### Scenario: Every kind is cached only where its Role grants it
- **WHEN** the manager starts split under the chart's own Roles
- **THEN** every informer syncs: wiring kinds watched in control alone, `conversations` and `conversationinputs` and PVCs in conversations alone, pods and configmaps in both, serviceaccounts in control alone

#### Scenario: The lease stays in control
- **WHEN** the manager runs split
- **THEN** its leader-election Lease is in the control namespace

### Requirement: A conversation's workload is released explicitly in its own namespace
The manager SHALL release a conversation's pod and MCP ConfigMap explicitly,
in the conversations namespace, under every state backend. It SHALL NOT emit a
cross-namespace owner reference. The orphan sweeper SHALL list the
conversations namespace.

#### Scenario: Closing releases the workload in the conversations namespace
- **WHEN** a conversation in a split release is closed
- **THEN** its pod and its MCP ConfigMap are deleted from the conversations namespace

#### Scenario: No owner reference crosses a namespace
- **WHEN** any object is created for a conversation
- **THEN** it carries no owner reference, or one naming an object in its own namespace

#### Scenario: An orphan in the conversations namespace is swept
- **WHEN** a runtime pod in the conversations namespace has no conversation record
- **THEN** the orphan sweeper deletes it

### Requirement: The manager's permissions are namespaced in both namespaces
The chart SHALL grant the manager one namespaced Role per namespace, each with
only what is used there:

- the control-namespace Role SHALL cover every wiring and identity kind,
  `signals`, adapter Deployments, Services, configmaps,
  leases, events, serviceaccounts with `get`/`list`/`watch`/`create`/`update`/`patch`
  and no `delete` (a grant carried over unchanged from the single-namespace Role), and
  a read (`get`/`list`/`watch`) on pods
  - that pod read reaches EVERY pod in the control namespace
  - the manager reads it to resolve the Lease holder to a pod IP for the
    leader proxy, and to refuse the state store's one-time import beside an
    older replica
  - RBAC cannot scope a verb to the manager's own replicas
- the conversations-namespace Role SHALL cover pods, configmaps,
  `conversations`, `conversationinputs` and their status, events, and PVC
  `get`/`list`/`watch`/`create`

In a single-namespace release both rule sets SHALL render as one Role.

No ClusterRole SHALL be added for the manager. Neither Role SHALL carry a
`secrets` verb or a PVC `delete`, `update` or `patch`.

#### Scenario: Two Roles render, no new ClusterRole
- **WHEN** the chart renders split
- **THEN** a manager Role and RoleBinding exist in each namespace, and the only manager ClusterRole is the existing nodes grant

#### Scenario: The control-namespace Role reads pods
- **WHEN** the chart renders split
- **THEN** the control-namespace Role carries `get`, `list` and `watch` on `pods` and no pod write verb

#### Scenario: The control-namespace Role cannot delete serviceaccounts
- **WHEN** the chart renders split
- **THEN** the control-namespace Role carries `get`, `list`, `watch`, `create`, `update` and `patch` on `serviceaccounts` and no `delete`

#### Scenario: The manager cannot reach a third namespace
- **WHEN** the manager lists pods in an unrelated namespace
- **THEN** the request is forbidden

#### Scenario: The storage invariant holds in the new namespace
- **WHEN** the manager's access to PVCs in the conversations namespace is checked
- **THEN** `create` is allowed and `delete`, `update` and `patch` are denied

### Requirement: References a runtime pod resolves are the adopter's to place, and the manager names them
Every Secret and ConfigMap a runtime pod resolves SHALL exist in the
conversations namespace. That covers profile repo auth, profile env
`valueFrom`, the runtime's model credential, MCP config `valueFrom` and raw
references, and image pull secrets.

The manager SHALL NOT read, copy or mirror a Secret. It SHALL set a condition
on the conversation naming each expected reference, derived from the
referencing CRs alone.

#### Scenario: Expected references are named without a read
- **WHEN** a conversation is admitted whose profile names a repo auth Secret and whose MCP config names a `valueFrom` Secret
- **THEN** a condition on the conversation names both as expected in the conversations namespace, and no Secret is read through the API

#### Scenario: A missing Secret fails at the pod, with a pointer
- **WHEN** a referenced Secret is absent from the conversations namespace
- **THEN** the pod fails with the kubelet's reason and the condition names the expected reference

#### Scenario: No Secret is copied
- **WHEN** Secrets exist only in the control namespace
- **THEN** nothing creates a copy in the conversations namespace

### Requirement: The chart splits by default and keeps the namespace it creates
The chart SHALL expose `global.agentops.conversationNamespace`, defaulting
to `agent-ops-conversations`, and `global.agentops.createConversationNamespace`,
defaulting to true. Setting `global.agentops.conversationNamespace` to the
release namespace SHALL render the single-namespace layout.

A namespace the chart creates SHALL carry the release labels and
`helm.sh/resource-policy: keep`. The chart SHALL render its generated
state-read token into both namespaces from one value: the control namespace
for the manager, the conversations namespace for housekeeping.

#### Scenario: Default install is split
- **WHEN** the chart is installed with no namespace values
- **THEN** conversation-scoped objects target `agent-ops-conversations`, which the chart creates and labels

#### Scenario: Single namespace is one value away
- **WHEN** `global.agentops.conversationNamespace` equals the release namespace
- **THEN** no namespace is created and every object renders in the release namespace

#### Scenario: Uninstall leaves the conversations namespace standing
- **WHEN** the release is uninstalled
- **THEN** the conversations namespace, its Secrets and its claims still exist

#### Scenario: Every runtime account follows the pods
- **WHEN** the chart renders runtime RBAC split
- **THEN** every runtime account is in the conversations namespace, and every binding subject names it there

#### Scenario: The floor exists on both sides, bound to nothing
- **WHEN** the chart renders split
- **THEN** `agentops-runtime` exists in both namespaces, no binding names either, and an adapter with no `spec.serviceAccountName` runs as the control-namespace one

### Requirement: NetworkPolicy peers name their namespace
Under the existing `global.agentops.networkPolicy.enabled` gate, off by
default, every peer selecting runtime pods or the manager across the split
SHALL add a `namespaceSelector`. The runtime ingress-deny policy SHALL render
in the conversations namespace. No egress policy SHALL be added.

#### Scenario: Every wired flow survives the split
- **WHEN** policies are enabled in a split release
- **THEN** runtime pods reach the manager, `mcp-aops` and every bundle MCP server, and the manager's replicas reach each other

#### Scenario: Runtime pods accept no ingress
- **WHEN** policies are enabled in a split release
- **THEN** a policy in the conversations namespace admits no ingress to runtime pods

#### Scenario: The gate's default is unchanged
- **WHEN** the chart renders with no NetworkPolicy values
- **THEN** no NetworkPolicy renders

### Requirement: The manager's HTTP surfaces stay namespace-free
`/work`, `/channel/*`, `/signal/*` and `/state/*` SHALL carry no namespace on
any request or response. No adapter SHALL need adopter-set configuration for
the split. `CONTROL_URL` SHALL keep addressing the manager Service by its
fully qualified name in the control namespace.

#### Scenario: A runtime pod reaches the manager across namespaces
- **WHEN** a runtime pod in the conversations namespace long-polls `/work`
- **THEN** it reaches the manager Service in the control namespace by its FQDN

#### Scenario: Adapters need no new values
- **WHEN** a release is split
- **THEN** every channel and signal adapter runs with no adopter-set namespace configuration

### Requirement: The migration is documented in the release that ships the split
The chart release that makes the split the default SHALL carry the migration
in its `CHANGELOG.md` Upgrade section, mirrored in `installation.md`. No code
SHALL move state between namespaces. The section SHALL give, in order:

1. closing open conversations
2. placing Secrets in the conversations namespace
3. carrying each claim across by PV rebind
4. upgrading
5. deleting `kube`-backend `Conversation` CRs left behind

#### Scenario: A nats-backed conversation reopens into the new namespace
- **WHEN** a conversation closed before the upgrade is reopened after it, under the `nats` backend
- **THEN** its pod is created in the conversations namespace and its record is unchanged

#### Scenario: A kube-backed conversation left behind is inert
- **WHEN** the manager is upgraded split with `Conversation` CRs still in the release namespace
- **THEN** those CRs receive no reconciliation and produce no error loop

#### Scenario: An uncarried claim fails loudly
- **WHEN** a conversation promised context reopens and its claim was not carried
- **THEN** the run fails as promised-and-lost rather than answering fresh

#### Scenario: The release notes give the procedure
- **WHEN** an adopter reads the release's Upgrade section
- **THEN** it lists the closing, Secret, PV rebind, upgrade and cleanup steps in order, with commands
