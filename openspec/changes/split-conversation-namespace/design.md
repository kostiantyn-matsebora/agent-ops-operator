## Context

The manager resolves one namespace at startup (`NAMESPACE`, default
`agent-ops`) and hands the same string to everything: the controller-runtime
cache (`DefaultNamespaces: {namespace: {}}`), the `OpQueue`, the `Router`, the
HTTP API `Server`, and every `client.InNamespace(...)` call. Runtime pods are
created in `conv.Namespace`, which is that same namespace by construction. The
chart's manager Role, runtime ServiceAccount, runtime RBAC subjects and home PVC
all key on `.Release.Namespace`.

That single string is doing two different jobs — *where the wiring lives* and
*where agent code runs* — and this change separates them. Facts that constrain
how:

- **OwnerRef GC is namespace-local.** A cross-namespace ownerRef is invalid; the
  garbage collector treats the owner as absent and deletes the dependent. The
  Conversation must therefore live wherever its pod, its MCP ConfigMap and its
  `ConversationInput` objects live.
- **The kubelet resolves Secret and ConfigMap references in the POD's
  namespace.** `internal/runtimepod/podspec.go` mounts the repo-auth Secret,
  injects `GIT_TOKEN` via `secretKeyRef`, and mounts raw MCP
  `configMapRef`/`secretRef`; `internal/mcpcompile` emits `valueFrom` env for
  every MCPConfig header/env entry. **The manager reads no Secrets**, so it
  cannot copy or even validate them — moving the pods moves the requirement.
- **Adapters hold projected transport credentials** (`AGENTOPS_CRED_<CHANNEL>_*`
  via `envFrom`). They belong on the control side.
- **The adapter and runtime HTTP contracts are namespace-free** — channels,
  conversations and ops are named, never namespaced. Nothing in
  `docs/contracts.md` changes.
- **The operator grants adapters and runtimes no RBAC.** This change must not
  become the exception; the conversations namespace gets a ServiceAccount, and
  what it may do stays an external grant.

## Goals / Non-Goals

**Goals:**

- Agent code runs in a namespace that contains no control-plane object, no
  transport credential, and no manager identity.
- Isolation is enforceable by the mechanisms that key on namespaces: RBAC scope,
  NetworkPolicy, ResourceQuota, admission policy.
- One code path serves both layouts; single-namespace is the same code with two
  equal names.
- ownerRef GC, serial-per-conversation dispatch, and the adapter contracts are
  unchanged.

**Non-Goals:**

- Multi-tenancy — one conversations namespace, not one per pipeline or per
  tenant. The manager watches exactly two namespaces.
- Moving adapters out of the control namespace.
- Copying, mirroring, or reading Secrets on the operator's behalf. Placing
  credentials in the conversations namespace is an operator task, by design.
- Migrating conversations that already exist (see Migration Plan).

## Decisions

### 1. The split line is "conversation-scoped", and it follows ownerRefs

Moving to the conversations namespace: `Conversation`, `ConversationInput`,
`agentops-conv-<name>` pods, `agentops-mcp-conv-<name>` ConfigMaps, the
`agentops-runtime` ServiceAccount, and the home PVC.

Staying in the control namespace: the manager Deployment/Service/RBAC, adapter
workloads and their SAs, and every wiring or identity CR — `Pipeline`,
`Channel`, `SignalSource`, `ChannelAdapter`, `SignalAdapter`, `AgentProfile`,
`AgentRuntime`, `MCPToolset`, `MCPConfig`.

The rule that generates that list: **an object moves if a runtime pod's ownerRef
chain or its kubelet-resolved references reach it.** Nothing else does.

*Alternative rejected —* move only the pods. Smaller diff, but the pod can no
longer ownerRef its Conversation, so GC has to be replaced by explicit cleanup
plus an orphan sweep — trading a load-bearing invariant for the convenience of
not moving four object kinds.

### 2. `Namespaces{Control, Conversations}` replaces the single string

A small value type is threaded where `Namespace string` is today
(`ConversationReconciler`, `httpapi.Server`, `chat.Router`, `chat.OpQueue`).
Each of the ~77 call sites is classified once, at review time, as control-plane
or conversation-plane. Classification rule:

| Lookup | Namespace |
| --- | --- |
| Pipeline, Channel, SignalSource, AgentProfile, AgentRuntime, MCPToolset, MCPConfig, adapters | Control |
| Conversation, ConversationInput, runtime pods, MCP ConfigMap | Conversations |

Single-namespace mode sets both fields to the same value, so there is no `if
split` branch anywhere in the reconcilers — the only place the two layouts
differ is the two strings resolved at startup.

*Alternative rejected —* keep one `Namespace` and add a `ConversationNamespace`
alongside it. Same data, but it leaves the ambiguous name in place at every call
site, which is exactly what made this change expensive to reason about.

The cache is configured with both namespaces
(`DefaultNamespaces: {control: {}, conversations: {}}`); when they are equal the
map collapses to one entry on its own. Leader election stays in the control
namespace.

### 3. RBAC stays namespace-scoped: two Roles, no ClusterRole

The manager gets a second Role + RoleBinding in the conversations namespace
carrying only what belongs there — conversations, conversationinputs, their
status subresources, pods, configmaps, events. The control-namespace Role loses
those verbs where they are no longer used there (in split mode) but keeps them
in single-namespace mode, which the template expresses by rendering the same
rule block into whichever namespace(s) apply.

*Alternative rejected —* one ClusterRole bound twice. Fewer templates, but it
grants cluster-wide read on Conversations to a manager that is deliberately
namespace-bound, and the project's posture is that a namespace-scoped operator
stays namespace-scoped.

### 4. Secrets are the migration, and the chart cannot solve it

Everything a runtime pod resolves must exist in the conversations namespace:

- `AgentProfile.spec.repo.secretRef` — SSH deploy key volume and `GIT_TOKEN`
- `AgentProfile.spec.env[].valueFrom` — secretKeyRef / configMapKeyRef
- `MCPConfig` header and env `valueFrom` entries compiled by `internal/mcpcompile`
- raw MCP `configMapRef` / `secretRef`
- image pull secrets for runtime images

The manager reads no Secrets and will not start reading them, so a missing
Secret surfaces the way it does today — the pod fails to start with the kubelet
naming the missing key — plus a preflight: the reconciler resolves
`AgentProfile` and `MCPConfig` references and reports names it expects in the
conversations namespace on the Conversation's `ToolingResolved`-style condition,
**without reading the objects**. Naming what is expected is not the same as
reading it, and it turns a mystifying `CreateContainerConfigError` into a
sentence.

The chart documents the requirement, keeps `AgentProfile` in the control
namespace (it is identity, not workload), and ships no secret-copying job — a
controller that duplicates Secrets across namespaces is a credential-spreading
machine, and this change exists to reduce that surface.

### 5. Cut over; do not migrate open conversations

After upgrade the manager watches the new conversations namespace only. Existing
`Conversation` objects in the control namespace stop being reconciled: their
pods keep running until they exit on idle TTL, and their chat threads go quiet.
Operators drain them with `kubectl delete conversation -n <control-ns> --all`
after confirming nothing is inflight; the chart's upgrade notes say so.

CRs cannot be moved between namespaces — only recreated under new names, which
changes the conversation name and therefore the runtime pod name, the MCP
ConfigMap name, console transcripts, and every thread binding that would have to
be re-patched. For an object designed to be short-lived (and, with `/close`,
explicitly endable) that risk buys nothing.

*Alternative rejected —* watch both namespaces during a transition release. It
works, but it doubles the RBAC surface and keeps a second Conversation list
alive in exactly the code paths this change is trying to make unambiguous.

### 6. NetworkPolicies ship on by default

Two policies in the conversations namespace, gated by
`networkPolicies.enabled` (default `true`):

- **default-deny ingress** — nothing dials a runtime pod; the pod always dials
  out (`/work` long-poll), which is the existing contract.
- **egress allow-list** — DNS (kube-dns, UDP/TCP 53) and the manager Service in
  the control namespace on 8080. Everything else is denied unless
  `networkPolicies.extraEgress` adds it.

Agent internet access (package installs, provider APIs) is an explicit
`extraEgress` entry rather than a default, because the default should be the
posture an operator would have to justify weakening — not the one they would
have to remember to tighten. MCP servers reachable from agents (e.g. the
k8s-bundle MCP server) need their own entry; the bundle ships one for itself.

A CNI without policy support ignores these objects silently, so the chart's
NOTES call that out — a policy that is present but not enforced is worse than a
known absence.

*Alternative rejected —* ship disabled. Nothing would ever turn it on, and the
network boundary is most of what the namespace split buys.

### 7. Console reads across both namespaces

The console is a channel adapter with `kubernetesAccess: true` (identity only;
its read-only Role is a chart grant). It gains a second read-only Role in the
conversations namespace for conversations, conversationinputs and pods, and its
`POD_NAMESPACE`-derived single namespace becomes an explicit pair passed as env
(`CONTROL_NAMESPACE`, `CONVERSATION_NAMESPACE`), defaulting to `POD_NAMESPACE`
for both so a hand-deployed console keeps working.

## Risks / Trade-offs

- **A missing Secret in the new namespace breaks every conversation at once, and
  the failure is a kubelet error on a pod.** → The preflight condition names the
  expected Secrets on the Conversation; upgrade notes list exactly which
  references have to be re-created; `helm upgrade` does not delete the old ones,
  so a rollback restores service.
- **77 call sites reclassified by hand — a single miswired lookup yields a
  NotFound that looks like a missing CR.** → The `Namespaces` type makes each
  site name its plane explicitly; integration tests run the split layout so a
  control-plane lookup pointed at the conversations namespace fails loudly in
  CI, not in a cluster.
- **Default-on NetworkPolicy can break agent workloads that reached something
  undeclared.** → Enumerated allowances, `extraEgress` escape hatch, and a
  single values flag to disable; NOTES warn when the CNI may not enforce.
- **Open conversations are abandoned on upgrade.** → Accepted deliberately;
  drain first, and the `/close` command from the capacity change makes draining
  a chat action rather than a kubectl one.
- **Two namespaces make `kubectl get` answers less obvious** ("where is my
  conversation?"). → README and NOTES print both namespaces; console shows both
  without the operator choosing.
- **ResourceQuota on the conversations namespace can silently block pod
  creation.** → Out of scope to configure, but the reconciler already surfaces
  pod-creation errors on the Conversation, and the capacity cap makes the pod
  count predictable.

## Migration Plan

1. Upgrade with `conversationNamespace` set to the release namespace first if a
   staged move is wanted — this is a no-op release that ships the two-namespace
   code path in single-namespace configuration.
2. Create the Secrets the runtime needs in the conversations namespace (repo
   auth, profile env, MCP config env, image pull secrets).
3. Drain open conversations (`/close`, or `kubectl delete conversation --all` in
   the control namespace once nothing is inflight).
4. Upgrade with `conversationNamespace: agent-ops-conversations`. New
   conversations land in the new namespace; the runtime SA, PVC and
   NetworkPolicies are created there.
5. Verify with a live task against the stub runtime
   (`POST /task {"pipeline":"…","task":"…"}`) and confirm the pod appears in the
   conversations namespace.
6. Rollback is `conversationNamespace` back to the release namespace plus a
   chart rollback; objects created in the new namespace are inert, and the PVC
   carries `helm.sh/resource-policy: keep` so session history survives either
   direction.

## Open Questions

- Should the conversations namespace be created by the chart at all, or always
  pre-created by the operator? Defaulting to `createConversationNamespace: true`
  is convenient but makes `helm uninstall` deletion semantics for a namespace
  containing user Secrets worth pinning down (proposed: label it, never delete
  it on uninstall).
- Does the preflight Secret-name condition belong on the Conversation or on the
  AgentProfile? The profile is where the reference is declared; the conversation
  is where the failure is felt.
