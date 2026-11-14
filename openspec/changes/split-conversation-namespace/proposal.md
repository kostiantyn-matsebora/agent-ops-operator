## Why

Agent code runs in the same namespace as the control plane. That one namespace
holds the manager, the wiring CRs, the state store, the adapters and their
transport credentials, and the runtime pods that run shell and edit tools.

A namespace is the unit Kubernetes enforces at: RBAC scope, NetworkPolicy scope,
ResourceQuota, admission policy. While agents share one with the control plane,
"the operator grants agents nothing" rests on careful RBAC alone. **A second
namespace gives the isolation a boundary Kubernetes itself enforces.**

## What Changes

- **A conversations namespace, `agent-ops-conversations` by default**, holds
  everything a runtime pod runs as, mounts or resolves:
  - runtime pods `agentops-conv-<name>` and MCP ConfigMaps `agentops-mcp-conv-<name>`
  - `Conversation` and `ConversationInput` CRs, under the kube state backend
  - the floor account `agentops-runtime`, every `rbac.runtime.serviceAccounts`
    entry, and every bundle route account, with their binding subjects
  - every persistence claim, chart-rendered or manager-created
  - the housekeeping and context-probe CronJobs, which mount those claims
- **The control namespace keeps the rest**: the manager, every wiring and
  identity CR, `Signal` CRs and the rest of the state store, NATS, the
  adapters, the gateway, the console, and every MCP server.
- **The manager carries a namespace pair** in place of one string. Every
  lookup names its plane. Single-namespace is the same code with two equal
  names.
- **The manager gets a second namespaced Role** in the conversations namespace.
  No new ClusterRole. Still no `secrets` verb, still no PVC delete.
- **Every NetworkPolicy peer naming runtime pods or the manager gains a
  `namespaceSelector`.** The existing `global.agentops.networkPolicy` gate and
  its off default are unchanged.
- **Signal adapters receive `CONVERSATION_NAMESPACE`**, and `k8s-events` drops
  every event in that namespace by a fourth, non-configurable self-exclusion
  mechanism. Signal-adapter contract 2.5.0.
- **BREAKING, on by default.** Chart major 20.0.0.
  - Every Secret a runtime pod resolves must exist in the conversations
    namespace.
  - Claims that exist before the upgrade are left in the release namespace.
    The release's upgrade notes give the PV rebind that carries context
    across.
  - Under the kube backend, open `Conversation` CRs in the release namespace
    stop being reconciled.

## Capabilities

### New Capabilities

- `conversation-namespace-isolation`: what lives in which namespace, how the
  manager works across both, what the pod's namespace means for Secret
  resolution, the network policy peers, and the release-tied migration.

### Modified Capabilities

- `signal-self-exclusion`: a fourth mechanism drops every event in the
  conversations namespace.
- `signal-adapter-lifecycle`: the reconciler injects `CONVERSATION_NAMESPACE`.
- `k8s-bundle`: route accounts and their binding subjects follow the split,
  everything else the bundle renders stays in the release namespace.
- `console-deployment`: the console's read-only grant covers pods in the
  conversations namespace too.

## Impact

### Code

| Component | Touches |
|---|---|
| `platform/manager` | `cmd/manager/main.go` and `store.go` (pair, cache, kube store namespace), `internal/controller` (conversation, pipeline, signal-adapter reconcilers, `orphansweep.go`), `internal/chat`, `internal/httpapi`, `internal/signalroute`, `internal/state/kube`. About 150 `Namespace` uses to classify |
| `platform/console` | `config.go`, `kube.go`: pods from both namespaces |
| `signals/k8s-events` | `selfexclude.go`: mechanism 4 |
| `chart/` | namespace template, `values.yaml`, `rbac.yaml`, `runtime-rbac.yaml`, `pvc.yaml`, `housekeeping.yaml`, `context-probe.yaml`, `state-read-auth`, `networkpolicy.yaml`, `coordination.yaml`, `console.yaml`, `deployment.yaml`, `NOTES.txt`, `Chart.yaml` |
| `chart/charts/*` | `kubernetes` (route accounts, MCP server policy), `home-assistant` (MCP admin policy), `nats`/`commandbus` policies checked |

### Reference docs

- `docs/concepts.md`: the two-namespace layout and the placement rule
- `docs/contracts.md`: no manager surface carries a namespace
- `docs/contracts/signal-adapter.md`: `CONVERSATION_NAMESPACE`, `ADP-10`, 2.5.0
- `docs/configuration.md`: `conversationNamespace`, `createConversationNamespace`
- `docs/security.md`: the moved trust boundary, `threat-model.py` re-run
- `docs/integrations/kubernetes.md`: route accounts live in the conversations namespace
- `docs/console.md`: runtime pods read from the conversations namespace
- `docs/CHANGELOG.md`: `[20.0.0]` with its Upgrade section, the migration of record

### Adopter site

- `docs/installation.md`: both namespaces, Secret placement, the 19 to 20
  upgrade
- `docs/getting-started.md`: where the first conversation's pod appears
- `docs/index.md` and `README.md`: the isolation claim
- `docs/guides/agent-profile.md`, `agent-runtime.md`, `toolsets.md`: Secrets
  belong in the conversations namespace
- `docs/guides/pipeline.md`: claims are created in the conversations namespace
