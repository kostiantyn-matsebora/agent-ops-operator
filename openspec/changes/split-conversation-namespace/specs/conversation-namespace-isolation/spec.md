## ADDED Requirements

### Requirement: Conversation-scoped objects live in a dedicated namespace
The deployment SHALL resolve two namespaces: a **control namespace** holding the
manager, adapter workloads, and every wiring or identity CR (`Pipeline`,
`Channel`, `SignalSource`, `ChannelAdapter`, `SignalAdapter`, `AgentProfile`,
`AgentRuntime`, `MCPToolset`, `MCPConfig`); and a **conversations namespace**
holding `Conversation` and `ConversationInput` CRs, runtime pods
(`agentops-conv-<name>`), compiled MCP ConfigMaps (`agentops-mcp-conv-<name>`),
the runtime ServiceAccount, and the home PVC. An object SHALL belong to the
conversations namespace if and only if a runtime pod's owner chain or its
kubelet-resolved references reach it. Adapter workloads SHALL remain in the
control namespace because they carry projected transport credentials.

#### Scenario: Runtime pod is created in the conversations namespace
- **WHEN** a conversation is dispatched with the split configured
- **THEN** its runtime pod, its MCP ConfigMap, and its `ConversationInput` objects are all in the conversations namespace

#### Scenario: Wiring stays in the control namespace
- **WHEN** the manager resolves a conversation's pipeline, profile, channels, toolsets and MCP configs
- **THEN** every one of those objects is read from the control namespace

#### Scenario: Adapters are not co-located with agent code
- **WHEN** the chart renders a channel or signal adapter workload
- **THEN** the workload and its ServiceAccount are in the control namespace

### Requirement: Owner references stay within one namespace
Every object the manager creates on a conversation's behalf SHALL be created in
the same namespace as its owning `Conversation`, so ownerRef garbage collection
continues to remove runtime pods, MCP ConfigMaps and input objects when the
conversation is deleted. The manager SHALL NOT create a cross-namespace owner
reference.

#### Scenario: Deleting a conversation collects its dependents
- **WHEN** a `Conversation` in the conversations namespace is deleted
- **THEN** its runtime pod and its `agentops-mcp-conv-<name>` ConfigMap are garbage collected

#### Scenario: No cross-namespace ownership is emitted
- **WHEN** any object is created for a conversation
- **THEN** its owner reference names an object in its own namespace

### Requirement: A single code path serves split and single-namespace layouts
The manager SHALL carry a control/conversations namespace pair rather than a
single namespace value, and every lookup SHALL name which of the two it uses.
When the two resolve to the same namespace the behavior SHALL be identical to
the previous single-namespace deployment, with no conditional branching between
layouts. The controller-runtime cache SHALL be scoped to both namespaces, and
leader election SHALL remain in the control namespace.

#### Scenario: Single-namespace mode is unchanged
- **WHEN** the conversations namespace is configured equal to the control namespace
- **THEN** all objects are created in that one namespace and behavior matches the pre-split deployment

#### Scenario: Cache covers both namespaces
- **WHEN** the manager starts with two distinct namespaces
- **THEN** it watches conversations in the conversations namespace and wiring CRs in the control namespace without cluster-wide watches

#### Scenario: Leader election is unaffected by the split
- **WHEN** the manager starts in split mode
- **THEN** its lease is held in the control namespace

### Requirement: Manager permissions stay namespace-scoped in both namespaces
The chart SHALL grant the manager a Role and RoleBinding in each namespace
carrying only the permissions used there — conversations, conversationinputs,
their status subresources, pods, configmaps and events in the conversations
namespace; wiring CRs, adapter workloads, services, serviceaccounts, configmaps
and leases in the control namespace. No ClusterRole SHALL be introduced for the
manager, and no reconciler SHALL create RBAC objects.

#### Scenario: Two namespace-scoped Roles render
- **WHEN** the chart is rendered in split mode
- **THEN** a manager Role and RoleBinding exist in each namespace and no manager ClusterRole is created

#### Scenario: Manager cannot read conversations cluster-wide
- **WHEN** the manager lists conversations in an unrelated namespace
- **THEN** the request is forbidden

### Requirement: Runtime references resolve in the conversations namespace
Because the kubelet resolves Secret and ConfigMap references in the pod's
namespace, every reference a runtime pod consumes — `AgentProfile` repo auth
(deploy key volume and `GIT_TOKEN`), `AgentProfile.spec.env[].valueFrom`,
`MCPConfig` header and env `valueFrom` entries, raw MCP `configMapRef` /
`secretRef`, and image pull secrets — SHALL exist in the conversations
namespace. The manager SHALL NOT read, copy or mirror any Secret. The manager
SHALL report the names it expects in the conversations namespace as a condition
on the Conversation, derived from the referencing CRs alone and without reading
the referenced objects.

#### Scenario: Expected references are named without being read
- **WHEN** a conversation is admitted whose profile declares a repo auth secret and whose MCP configs declare valueFrom env
- **THEN** a condition on the conversation names those Secret names as expected in the conversations namespace, and no Secret is read through the API

#### Scenario: Missing secret fails at the pod, with a pointer
- **WHEN** a referenced Secret does not exist in the conversations namespace
- **THEN** the runtime pod fails to start with the kubelet's error and the conversation's condition names the reference that is expected

#### Scenario: No secret copying is performed
- **WHEN** the split is enabled and Secrets exist only in the control namespace
- **THEN** the manager does not create copies in the conversations namespace

### Requirement: The chart configures the split with the split as the default
The chart SHALL expose `conversationNamespace`, defaulting to
`agent-ops-conversations`, and `createConversationNamespace`, defaulting to
true. Setting `conversationNamespace` to the release namespace SHALL produce the
single-namespace layout. A namespace the chart creates SHALL be labelled as
belonging to the release and SHALL NOT be deleted on uninstall, because it holds
operator-managed Secrets and the home PVC. The runtime ServiceAccount, the home
PVC, and the subjects of runtime RBAC bindings SHALL be placed in the
conversations namespace.

#### Scenario: Default install is split
- **WHEN** the chart is installed with no namespace values set
- **THEN** conversation objects target `agent-ops-conversations` and the namespace is created and labelled

#### Scenario: Single namespace is one value away
- **WHEN** `conversationNamespace` equals the release namespace
- **THEN** no second namespace is created and every object renders in the release namespace

#### Scenario: Uninstall leaves the conversations namespace standing
- **WHEN** the release is uninstalled
- **THEN** the conversations namespace and the home PVC still exist

#### Scenario: Runtime SA follows the pods
- **WHEN** the chart renders runtime RBAC in split mode
- **THEN** the runtime ServiceAccount is created in the conversations namespace and every binding names it there

### Requirement: The conversations namespace is network-isolated by default
The chart SHALL ship NetworkPolicies for the conversations namespace gated by
`networkPolicies.enabled`, defaulting to **true**: a default-deny ingress policy,
and an egress policy allowing only DNS and the manager Service in the control
namespace. Additional egress SHALL require explicit `networkPolicies.extraEgress`
entries, including any agent access to the public internet and any MCP server a
bundle ships. The chart NOTES SHALL warn that a CNI without policy support
ignores these objects.

#### Scenario: Runtime pods accept no ingress
- **WHEN** the policies are enabled
- **THEN** no ingress to runtime pods is permitted, and runtime pods still reach the manager's `/work` endpoint

#### Scenario: Undeclared egress is denied
- **WHEN** an agent attempts to reach a host not covered by DNS, the manager Service, or `extraEgress`
- **THEN** the connection is denied

#### Scenario: Policies can be disabled wholesale
- **WHEN** `networkPolicies.enabled=false`
- **THEN** no NetworkPolicy objects render and traffic behavior is unchanged

### Requirement: Adapter and runtime contracts remain namespace-free
The `/channel/*`, `/signal/*` and `/work` contracts SHALL continue to identify
channels, conversations and operations by name only, carrying no namespace.
Adapters SHALL require no knowledge of the split, and `CONTROL_URL` SHALL
continue to address the manager Service by its fully qualified name in the
control namespace.

#### Scenario: Adapters are unaffected by the split
- **WHEN** the split is enabled
- **THEN** existing channel and signal adapter images work unchanged with no new configuration

#### Scenario: Runtime reaches the manager across namespaces
- **WHEN** a runtime pod in the conversations namespace long-polls `/work`
- **THEN** it reaches the manager Service in the control namespace via its FQDN

### Requirement: Existing conversations are not migrated
Upgrading into the split SHALL NOT move or recreate `Conversation` objects that
exist in the control namespace; the manager stops reconciling them. The upgrade
documentation SHALL state that open conversations must be drained before
upgrade and SHALL give the command to remove the stragglers.

#### Scenario: Old conversations stop being reconciled
- **WHEN** the manager is upgraded into split mode with conversations still present in the control namespace
- **THEN** those conversations receive no further reconciliation and no error loop is produced

#### Scenario: Upgrade notes state the drain step
- **WHEN** an operator reads the upgrade notes
- **THEN** the notes instruct draining open conversations first and name the cleanup command
