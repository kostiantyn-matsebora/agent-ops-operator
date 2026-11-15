## Context

The manager resolves one namespace at startup (`NAMESPACE`, from the downward
API) and hands it to everything:

- the controller-runtime cache (`DefaultNamespaces: {namespace: {}}`)
- leader election
- the state store, under either backend
- the reconcilers, the router, the HTTP API, the orphan sweeper

The chart keys the manager Role, every runtime account, every claim, the
housekeeping and context-probe CronJobs and every NetworkPolicy on
`.Release.Namespace`.

That one string does two jobs: **where the control plane lives** and **where
agent code runs**. This change separates them. These facts constrain how:

| Fact | Consequence |
|---|---|
| The kubelet resolves a Secret, ConfigMap, ServiceAccount or PVC in the POD's namespace | everything a runtime pod runs as, mounts or resolves moves with it |
| A cross-namespace ownerRef is invalid, and GC deletes the dependent | an owner and its dependents share a namespace, or no ownerRef is set |
| The manager releases pod and ConfigMap explicitly under every backend | no behaviour depends on GC, so placement is free of it |
| Under the `nats` backend (the default) a conversation is a record, not a CR | the split moves no CR there, only workloads and claims |
| Under the `kube` backend the store writes `Conversation`, `ConversationInput` and `Signal` CRs | the kube store spans both namespaces, by kind |
| The manager reads no Secret and holds no `secrets` verb | it cannot copy or validate a Secret, so moving pods moves the requirement to the adopter |
| Every NetworkPolicy peer is a bare `podSelector` | each one names its own namespace only, and breaks silently across the split |
| `/signal/*`, `/channel/*`, `/work`, `/state/*` carry no namespace | no HTTP contract changes |

## Goals / Non-Goals

**Goals:**

- Agent code runs in a namespace holding no control-plane object, no transport
  credential and no manager identity.
- Namespace-keyed controls apply to agents alone: RBAC scope, NetworkPolicy,
  ResourceQuota, admission policy.
- One code path. Single-namespace is two equal names, with no `if split`
  branch.
- The split is the default from chart 20.0.0, with the migration documented
  in that release.

**Non-Goals:**

- Multi-tenancy. One conversations namespace per release, not one per Pipeline.
- Moving adapters, MCP servers or NATS.
- Copying, mirroring or reading Secrets for the adopter.
- A transition release that watches conversations in both namespaces.
- Restricting egress. The egress-proxy sidecar mediates it, unchanged.

## Decisions

### 1. The placement rule

**An object moves if it is a runtime pod or something a runtime pod runs as,
mounts or resolves, a workload sharing a runtime pod's claims, or
conversation-scoped state.** Nothing else moves.

| Object | Namespace | Rule |
|---|---|---|
| runtime pod `agentops-conv-<name>` | conversations | it runs the agent |
| MCP ConfigMap `agentops-mcp-conv-<name>` | conversations | mounted |
| `Conversation`, `ConversationInput` (kube backend) | conversations | conversation-scoped state |
| `rbac.runtime.serviceAccounts[]`, bundle route accounts | conversations | run as |
| floor account `agentops-runtime`, bound to nothing | both | runtime pods and adapters both fall back to it |
| chart claims (`agentops-context`, workspace, per-route) and manager claims `agentops-<pipeline>-<volume>` | conversations | mounted |
| housekeeping and context-probe CronJobs | conversations | they mount the claims |
| the chart's `stateReadAuth` Secret, a second render | conversations | housekeeping resolves it |
| adopter Secrets a pod resolves | conversations | resolved |
| manager, its Service, Lease, Role, nodes ClusterRole | control | control plane |
| `Signal` and every other kube-store kind | control | not conversation-scoped |
| NATS and command-bus StatefulSets and credentials | control | control plane |
| every wiring and identity CR | control | wiring |
| adapters, gateway, console, their accounts and credentials | control | transport credentials |
| MCP servers (`mcp-aops`, `agentops-mcp-k8s`, home-assistant admin) | control | they hold their own reach |

*Rejected:* move the pods alone. A pod cannot run as an account or mount a
claim in another namespace, so the "small" version does not run.

### 2. A namespace pair replaces the single string

`Namespaces{Control, Conversations}` is threaded wherever `Namespace string`
is today. Each of the roughly 150 uses is classified once by the table in
decision 1.

- Single-namespace sets both to one value. The only difference between
  layouts is two strings resolved at startup.
- The cache covers both namespaces, each kind only where its Role grants
  it: control by default, `conversations`, `conversationinputs` and PVCs in
  conversations, pods and configmaps in both. A kind cached in a namespace
  that its Role does not grant never syncs. Equal values collapse every
  entry to one.
- Leader election stays in control.
- `CONVERSATION_NAMESPACE` is read at startup, defaulting to `NAMESPACE`.

*Rejected:* keep `Namespace` and add `ConversationNamespace` beside it. The
ambiguous name would survive at every call site, which is what makes the
classification expensive.

### 3. The kube store spans two namespaces, by kind

`kube.OpenStores` takes the pair. Conversations, runs and input blobs
(`ConversationInput`) open in Conversations. Signals, cooldowns, receipts and
adapter state open in Control.

- Under kube, the synthetic ownerRef from pod and ConfigMap to the
  `Conversation` CR stays valid, because they share a namespace.
- Under nats nothing changes: `NoOwnerReference` holds as today.
- The `OrphanSweeper` lists the conversations namespace.
- The one-time kube-to-nats import reads conversation kinds from Conversations
  and the rest from Control.

### 4. RBAC: two manager Roles and a console Role, no new ClusterRole

| Role | Namespace | Rules |
|---|---|---|
| manager control Role | control | every wiring kind, `signals`, adapter Deployments, serviceaccounts `get/list/watch/create/update/patch` (no `delete`), Services, configmaps, leases, events, pods `get/list/watch` |
| manager conversations Role | conversations | pods, configmaps, `conversations`, `conversationinputs` and status, PVC `get/list/watch/create`, events |

| console read-only Role | conversations | pods `get/list/watch` |

The control Role's pod read reaches every pod in control: RBAC cannot scope it to the manager's own replicas.

- In single-namespace mode both rule sets render into one Role, identical to
  today's.
- The nodes ClusterRole is unchanged. **No ClusterRole is added.**
- Neither Role carries a `secrets` verb or a PVC `delete`.
- Runtime grants stay cluster-wide ClusterRoleBindings. Only their subject
  namespace changes.

*Rejected:* one ClusterRole bound twice. It grants cluster-wide Conversation
reach to a manager that is namespace-bound by design.

### 5. Secrets are the adopter's, and the manager names them

Every reference a runtime pod resolves must exist in the conversations
namespace:

- `AgentProfile.spec.repository.auth.secretRef` (SSH volume, `GIT_TOKEN`)
- `AgentProfile.spec.env[].valueFrom`
- the runtime's model credential
- `MCPConfig` header and env `valueFrom`, raw MCP `configMapRef`/`secretRef`
- image pull secrets

The conversation reconciler sets a condition naming each expected reference,
derived from the CRs alone. **Naming a reference is not reading it.** A
`CreateContainerConfigError` becomes a sentence on the conversation.

The chart's own generated `stateReadAuth` token renders into both namespaces
from one lookup. That is the chart rendering its own credential, not a copy
job. `docs/security.md` records that `allowPodExecution` now guards it too.

*Rejected:* a Secret-mirroring controller. It spreads credentials into
exactly the namespace this change exists to fence.

### 6. NetworkPolicy peers gain a namespace

The gate stays `global.agentops.networkPolicy.enabled`, off by default.

- Every peer naming runtime pods or the manager across the split adds a
  `namespaceSelector` on `kubernetes.io/metadata.name`. That covers the
  manager's ingress, `mcp-aops`, the k8s MCP server and the home-assistant MCP
  server.
- The runtime ingress-deny policy renders in the conversations namespace.
- No egress policy is added. The egress-proxy sidecar mediates egress.
- `TestEveryWiredFlowIsAllowed` runs in both layouts.

### 7. Signal adapters learn the conversations namespace

The SignalAdapter reconciler injects `CONVERSATION_NAMESPACE` beside
`POD_NAMESPACE`, as a literal value. Channel adapters are unchanged.

`k8s-events` adds **mechanism 4**: every event in the conversations namespace
is dropped, unconfigurably, whenever it differs from `POD_NAMESPACE`. That
namespace holds only agent-ops machinery, and claims such as `agentops-context`
match no excluded prefix.

The signal-adapter contract moves to **2.5.0**. `ADP-4` already forbade the
signal, so `ADP-10` names how to honour it rather than adding a duty.

### 8. The console reads pods in both namespaces

Conversations and Signals already come through `/state/*`. Only its pod watch
widens, to both namespaces, so the Infrastructure view keeps runtime pods. The
chart passes `CONVERSATION_NAMESPACE`, defaulting to `POD_NAMESPACE`.

### 9. The chart owns the namespace and keeps it

`conversationNamespace` defaults to `agent-ops-conversations`.
`createConversationNamespace` defaults to true. A namespace the chart creates
carries the release labels and `helm.sh/resource-policy: keep`, so uninstall
leaves it, its Secrets and its claims standing.

### 10. Migration is release documentation, not code

No code moves state. Chart 20.0.0's `CHANGELOG.md` Upgrade section is the
migration of record, mirrored in `installation.md`'s upgrade section.

| Backend | Open conversation after upgrade |
|---|---|
| nats | the record survives. Closed before upgrade, it reopens into the new namespace |
| kube | its CR stays in the release namespace, unreconciled. The notes give the delete |

Context survives only where the claim is carried. The notes give the PV
rebind. A conversation promised context it no longer has fails its run, as
`promised-and-lost` already requires.

## Risks / Trade-offs

- **A missing Secret breaks every conversation at once.** → The expected-
  references condition names it. `helm upgrade` deletes nothing in the old
  namespace, so rolling `conversationNamespace` back restores service.
- **A misclassified lookup surfaces as NotFound.** → The pair forces each
  call site to name its plane. Envtest runs split, so a wrong plane fails in
  CI.
- **An uncarried claim loses context.** → The rebind procedure, and runs that
  fail loudly rather than answer fresh.
- **The state-read token now sits beside agent code.** → It grants read on
  `/state/*` only, and reading a Secret needs pod execution, which stays off by
  default.
- **An old `k8s-events` image misses unprefixed objects in the new
  namespace.** → The chart pins the matching image tag in the same release.
- **"Where is my pod?"** → NOTES and `installation.md` print both namespaces.

## Migration Plan

Written into the 20.0.0 Upgrade section, steps in order:

1. Close every open conversation, so no pod or claim is in use.
2. Create the conversations namespace, or let the chart create it.
3. Create in it every Secret a runtime pod resolves (decision 5).
4. Carry each claim across:
   1. Set the bound PV's reclaim policy to `Retain`.
   2. Delete the old claim, then clear the PV's `claimRef`.
   3. Pre-create the claim in the new namespace, `volumeName` set to that PV.
   4. Point the chart at it with `persistence.<volume>.existingClaim`. A
      manager-created claim needs no setting, because the manager is
      idempotent by existence.
5. Apply the CRDs, then `helm upgrade`.
6. Under kube, delete the unreconciled `Conversation` CRs left in the release
   namespace.
7. Verify with an ordinary signal and confirm the pod appears in the
   conversations namespace.

**Rollback** is `conversationNamespace` set to the release namespace. Objects
in the new namespace go inert. The claims carry `resource-policy: keep` either
way.

## Open Questions

None. Both earlier ones are settled: the chart creates and keeps the namespace
(decision 9), and the expected-references condition sits on the conversation
(decision 5).
