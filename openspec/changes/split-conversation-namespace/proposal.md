## Why

Everything runs in one namespace today: the manager, the wiring CRs, the
adapters holding transport credentials — and the runtime pods that execute
agent code with shell and edit tools. A namespace is the unit Kubernetes
actually enforces things at (RBAC scope, NetworkPolicy scope, ResourceQuota,
`default` ServiceAccount, admission policy), so as long as agent workloads share
one with the control plane, "the operator grants agents nothing" is enforced
only by our own care in writing RBAC, not by a boundary. Splitting the two gives
the isolation a name.

## What Changes

- Conversation-scoped objects move to a second namespace, `agent-ops-conversations`
  by default: `Conversation` and `ConversationInput` CRs, the runtime pods
  (`agentops-conv-<name>`), their compiled MCP ConfigMaps
  (`agentops-mcp-conv-<name>`), the `agentops-runtime` ServiceAccount, and the
  home PVC. Owner and dependents stay co-located, so ownerRef GC is untouched.
- The control namespace keeps the manager, all wiring and identity CRs
  (`Pipeline`, `Channel`, `SignalSource`, `AgentProfile`, `AgentRuntime`,
  `MCPToolset`, `MCPConfig`), and the adapter workloads — adapters hold
  projected transport credentials and must not share a namespace with agent
  code.
- The manager becomes two-namespace aware: a `Namespaces{Control, Conversations}`
  value replaces the single `Namespace` field threaded through the reconcilers,
  the router, the op queue and the HTTP API; the controller-runtime cache and
  RBAC cover both.
- **Opt-in with the split as the default**: `conversationNamespace` defaults to
  `agent-ops-conversations`; setting it to the release namespace restores
  today's single-namespace behavior through the same code path.
- Chart creates and labels the conversations namespace, places the runtime SA,
  runtime RBAC subjects and PVC there, and binds a manager Role in it.
- **NetworkPolicies for the conversations namespace**, enabled by default:
  default-deny ingress, egress limited to DNS and the manager Service, with
  broader agent egress an explicit opt-in.
- **BREAKING**: every Secret and ConfigMap a runtime pod resolves —
  `AgentProfile` repo auth (SSH deploy key / `GIT_TOKEN`), `AgentProfile.env`
  `valueFrom`, `MCPConfig` env `valueFrom`, raw MCP `configMapRef`/`secretRef` —
  must now exist in the conversations namespace, because the kubelet resolves
  them where the pod runs and the manager reads no Secrets.
- **BREAKING**: conversations already open in the control namespace stop being
  reconciled after upgrade; they are drained by hand.

## Capabilities

### New Capabilities
- `conversation-namespace-isolation`: which objects live in which namespace,
  how the manager operates across both, what the runtime pod's namespace implies
  for secret resolution, and the network posture of the conversations namespace.

### Modified Capabilities
- `k8s-bundle`: the bundle's runtime ServiceAccount and its RBAC subjects follow
  the runtime into the conversations namespace, while its profile, pipeline and
  MCP CRs stay in the control namespace.

## Impact

- **Manager**: `cmd/manager/main.go` (namespace pair, cache config for two
  namespaces), `internal/controller/conversation_controller.go`,
  `internal/httpapi/server.go`, `internal/httpapi/signals.go`,
  `internal/chat/router.go`, `internal/chat/ops.go`, `internal/chat/pipelines.go`
  — 77 namespace references across 12 files, each of which has to be classified
  as control-plane or conversation-plane.
- **Pod spec**: `internal/runtimepod/podspec.go` — pods are built for the
  conversations namespace; `CONTROL_URL` already carries the manager's namespace
  in its FQDN and needs no change.
- **Console**: `console/kube.go` / `console/main.go` — reads Conversations from
  the conversations namespace and everything else from the control namespace;
  its read-only Role is bound in both.
- **Chart**: new namespace template, `values.yaml`
  (`conversationNamespace`, `createConversationNamespace`, `networkPolicies.*`),
  `rbac.yaml`, `runtime-rbac.yaml`, `pvc.yaml`, `console.yaml`,
  `charts/k8s-bundle/`.
- **Docs**: `README.md`, `docs/concepts.md`, `docs/contracts.md` (adapter
  contracts are namespace-free and stay that way), `CLAUDE.md`.
- **Tests**: `internal/integration/` — envtest suite creates both namespaces and
  asserts placement; single-namespace mode keeps its own coverage.
