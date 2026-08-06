# Proposal: k8s-operator-out-of-the-box

## Why

Today a working Kubernetes agent requires assembling the pieces by hand (profile, runtime, SA, RBAC, MCP), and the closest built-in — the `demo.enabled` advisor — is off by default, MCP-less, and read-only only. The operator should ship a ready-made **k8s-operator** agent out of the box: a runtime image derived from the claude runtime with a Kubernetes MCP server baked in, a matching AgentProfile, read-only by default, full cluster rights behind an explicit flag — installed by default and removable with one value.

## What Changes

- New **`runtime-k8s/` image** (`agentops-runtime-k8s`): `FROM` the claude runtime image (kubectl is already there), adding a pinned Kubernetes MCP server baked in at build time (no runtime npm downloads in a possibly egress-restricted pod).
- Chart ships, **enabled by default** (`k8sOperator.enabled: true`):
  - `AgentRuntime k8s-operator` — the derived image, its **own dedicated ServiceAccount** (trust isolation per the AgentRuntime SA rule), claude credential env from a secretRef (same values shape as `demo.credentialsSecret`);
  - `AgentProfile k8s-operator` — repo-less, Kubernetes MCP (inline stdio server from the baked-in binary), `mcp__kubernetes__*` + shell tooling allowlist, `runtimeRef: k8s-operator`;
  - RBAC for the dedicated SA: **read-only by default** (demo-mode rights: `view` + nodes/namespaces/metrics), **full rights** (`cluster-admin` binding) when `k8sOperator.fullAccess: true`. The MCP server's non-destructive mode is additionally enabled whenever full access is off (defense in depth; RBAC remains the wall).
- `k8sOperator.enabled: false` removes every definition; the existing `demo.*` advisor is untouched and orthogonal.
- Addressable immediately: `POST /task {"profile":"k8s-operator", …}` or `/k8s-operator <task>` on any channel.

## Capabilities

### New Capabilities

- `k8s-agent-runtime`: the derived runtime image — base, baked-in Kubernetes MCP server (pinned), non-destructive-mode support, image contract unchanged from the claude runtime.
- `k8s-operator-bundle`: the chart-shipped bundle — AgentRuntime + AgentProfile + dedicated SA/RBAC, the `enabled` (default true) and `fullAccess` (default false) flags, credential wiring, demo-mode vs full-rights semantics.

### Modified Capabilities

<!-- none — no existing spec covers the demo bundle or runtime images -->

## Impact

- New `runtime-k8s/` directory (Dockerfile; no code — the claude runtime's `runtime.js` is inherited).
- `chart/`: new gated template `k8s-operator.yaml`, `k8sOperator.*` values, chart minor bump. First default-enabled CRs in the chart (same CRD-dependency caveat as `demo.yaml`); pods start only once the claude credential Secret exists (documented — definitions render regardless).
- `config/samples/` unaffected (the bundle IS the sample); README gains the out-of-the-box section; CLAUDE.md map/build lines.
- No API, manager, contract, or adapter changes.
