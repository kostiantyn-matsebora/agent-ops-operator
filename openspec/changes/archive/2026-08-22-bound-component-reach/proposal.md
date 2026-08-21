## Why

Component-to-component communication has no trust boundary, and the one caller
we do restrict is restricted only by its own cooperation. Both halves were
verified live on 2026-08-21 from an unauthenticated pod in another namespace:
`agentops-mcp-k8s` completed an MCP handshake and listed `pods_exec` and
`resources_delete` under a **cluster-admin** ServiceAccount, the manager's work
contract answered without a token, the console served its full API below its
authenticating proxy, and both telegram adapters accepted `POST /updates`.

The agent is inside that boundary too. Its reach is meant to be the toolsets its
`Pipeline` binds, but `--allowedTools` is enforced by the CLI **inside the
runtime pod** and the MCP server has never heard of an `MCPToolset`. Any agent
holding a shell — bound on ordinary pipelines — reaches the server directly and
calls whatever it registers. The documented two walls are one.

Decided in `docs/adr/0001-bound-component-reach.md`.

## What Changes

**Network isolation for every component (ADR D1).**

- The chart gains NetworkPolicy templates covering the manager, the adapters, the
  console, the runtime pods and the bundle-shipped MCP servers — default-deny
  ingress plus one allow per wired flow.
- Opt-in, default off. Enabling it is an install decision, not a silent one.
- `NOTES.txt` states plainly that **an unenforcing CNI applies these objects and
  protects nothing**, and what remains exposed in that case. The chart cannot
  detect enforcement, so it must not imply it.

**Toolset enforcement outside the agent's control (ADR D2).**

- A new module: a per-conversation transparent egress proxy, running as a native
  sidecar in the runtime pod, that the agent's traffic cannot route around.
- It enforces the conversation's bound toolset on MCP traffic and forwards
  everything else untouched — no TLS interception.
- The allowlist is injected at pod creation by the manager, which already
  resolves it. The proxy holds no Kubernetes access and no RBAC.
- Opt-in per `AgentRuntime`, default off, absent meaning today's pod exactly —
  the same contract `spec.contextSync` already has.
- The runtime pod's non-root identity becomes load-bearing rather than
  incidental, and is pinned by test.

**Out of scope, and named so in the ADR**: authentication for the surfaces that
lack it (the manager's work contract, the console's auth model, adapter inbound
push), and replacing the MCP server's cluster-admin binding. Network isolation
narrows all four. It resolves none, and each is its own change.

## Capabilities

### New Capabilities

- `runtime-egress-mediation`: the per-conversation egress proxy — what it
  intercepts, what it enforces, what it passes through, how it is opted into,
  and the pod properties its soundness depends on.
- `component-network-isolation`: the chart's network restriction of agent-ops
  components — which flows are allowed, how it is opted into, and the
  unverifiable-enforcement warning that must accompany it.

### Modified Capabilities

- `mcp-toolset-model`: the effective allowlist gains an enforcement point that
  does not depend on the agent honouring it. Today the spec describes resolution
  and passes the result to the CLI as the sole authority.
- `k8s-mcp-tooling`: its two-walls claim is qualified. The toolset wall was
  inside the runtime pod, so it bound only a cooperating agent.

## Impact

- **New module** `egress-proxy/` — dependency-free Go, its own image, multi-arch.
- `internal/runtimepod/` — sidecar and interception wiring, container security
  context, opt-in resolution from `AgentRuntime`.
- `api/v1alpha1/` — the `AgentRuntime` opt-in field, plus generated deepcopy and
  CRDs.
- `chart/` — NetworkPolicy templates and values, `NOTES.txt` warning, the new
  image reference.
- `chart/charts/k8s-bundle/`, `chart/charts/ha-bundle/` — policies for the MCP
  server workloads they own.
- **Docs**: `docs/concepts.md` (capability resolution is no longer CLI-only),
  `docs/installation.md` (the isolation decision), `docs/k8s-bundle.md` and
  `docs/ha-bundle.md` (the two-walls wording), `CHANGELOG.md`.
- **Runtime pods gain a privileged init container** when the feature is enabled.
  Adopters with pod-security admission on `restricted` must know before enabling.
