# Design: k8s-operator-out-of-the-box

## Context

- `runtime-claude/` (image `agentops-runtime-claude`) is the reference AgentRuntime: node:22-slim + claude-code + **kubectl already installed** + `runtime.js` speaking the `/work` contract. Runtime selection: `profile.runtimeRef → CR "default" → manager env`.
- `demo.yaml` is the existing precedent for chart-shipped agent CRs: gated flag, dedicated SA (`<runtime-sa>-demo`), read-only ClusterRole/Bindings, credential env via `demo.credentialsSecret` `valueFrom` (the manager never reads it). It stays as-is (off by default, MCP-less).
- `AgentProfile.spec.mcp` supports inline stdio servers with env; `mcpcompile` renders mcp.json; allowlists gate tools (`mcp__<server>__*`).
- The chart currently ships no default-enabled CRs; `crds.enabled` ordering caveat is documented on `demo.yaml`.

## Goals / Non-Goals

**Goals:**
- `helm install` → an addressable `k8s-operator` agent with Kubernetes MCP, read-only, no assembly.
- One flag to grant full cluster rights; one flag to remove the whole bundle.
- Strict trust isolation: its own SA; rights never leak to/from the shared runtime SA or the demo SA.

**Non-Goals:**
- No changes to `runtime-claude/`, the `/work` contract, or any CRD.
- No repository/skills for the profile (project-agnostic operator; adopters fork the pattern with their own repo-backed profiles).
- No granular rights tiers between read-only and full (adopters use `rbac.runtime.*` or their own bindings for shades in between).
- Not replacing `demo.*` (kept for the zero-MCP minimal path; possible future consolidation noted, not done here).

## Decisions

### D1: Derived image, MCP baked at build time
`runtime-k8s/Dockerfile`:
```dockerfile
FROM kmatsebora/agentops-runtime-claude:<pinned-tag>
USER root
RUN npm install -g mcp-server-kubernetes@<pinned> && npm cache clean --force
USER node
```
`mcp-server-kubernetes` (npm) is the served MCP: stdio transport, resolves cluster access from the pod's ServiceAccount, supports `ALLOW_ONLY_NON_DESTRUCTIVE_TOOLS=true`. Baking (vs `npx` at start) keeps pods start-fast and egress-independent; the version is pinned so image rebuilds are deliberate. kubectl and `runtime.js` come from the base — the image contract is unchanged.

*Alternative considered:* no new image, `npx` from the profile's stdio command — rejected: runtime npm download on every pod start, egress dependency, unpinned.

### D2: Dedicated SA, rights flipped by one flag
SA `<runtime-sa>-k8s-operator` (pattern-consistent with the demo SA). RBAC:
- `fullAccess: false` (default, "demo mode"): bind `view` + a `cluster-ro` ClusterRole (nodes, namespaces, metrics — same shape as the demo's) — read-only wall.
- `fullAccess: true`: bind the built-in **`cluster-admin`** ClusterRole instead ("full rights", literally). The read-only bindings are not rendered.
The flag flips *bindings only*; the SA and everything else are identical. RBAC is the enforcement boundary; D3's MCP mode is belt-and-braces.

### D3: Profile with inline stdio MCP, mode-aware
```yaml
kind: AgentProfile            # name: k8s-operator
spec:
  agent: k8s-operator         # role adopted from prompt templates (no repo)
  runtimeRef: {name: k8s-operator}
  allowedTools: "Read,Grep,Glob,Bash,mcp__kubernetes__*"
  maxTurns: {{ .Values.k8sOperator.maxTurns }}   # default 60
  mcp:
    servers:
      kubernetes:
        type: stdio
        command: mcp-server-kubernetes          # baked into the image, on PATH
        {{- if not .Values.k8sOperator.fullAccess }}
        env: [{name: ALLOW_ONLY_NON_DESTRUCTIVE_TOOLS, value: "true"}]
        {{- end }}
```
`AgentRuntime k8s-operator`: image `k8sOperator.runtimeImage`, the dedicated SA, idle TTL from the global value, home volume when persistence is on, credential env from `k8sOperator.credentialsSecret` (`{name, key, envName}`, defaulting to the same `agentops-claude` secret shape the demo uses — one secret serves both).

### D4: Enabled by default, removable by flag
`k8sOperator.enabled: true` — the chart's first default-on CRs. Consequences handled:
- Fresh install without the credential Secret: definitions render and are valid; a runtime pod only starts when a conversation needs it, and then fails env resolution until the Secret exists — documented as the one setup step (same step the demo already documents).
- `enabled: false` renders nothing (bundle fully removable).
- Same `crds.enabled` ordering caveat as `demo.yaml` (CRs need CRDs — helm applies both in one release; the gated-template pattern is already proven there).

### D5: Values shape
```yaml
k8sOperator:
  enabled: true
  # false = demo mode: read-only RBAC + non-destructive MCP toolset
  # true  = cluster-admin binding + full MCP toolset
  fullAccess: false
  runtimeImage: kmatsebora/agentops-runtime-k8s:0.1.0
  maxTurns: 60
  credentialsSecret:
    name: agentops-claude
    key: oauthToken
    envName: CLAUDE_CODE_OAUTH_TOKEN
```

## Risks / Trade-offs

- [`fullAccess: true` = cluster-admin for an LLM agent] → explicit opt-in flag with a loud values comment; RBAC-only flip so the audit trail is one binding; the profile's format/templates already require approval flows for actions (reply lane).
- [Default-on CRs surprise existing installs on upgrade] → the bundle is additive (new names, own SA); installs that don't want it set one value; release notes call it out.
- [`mcp-server-kubernetes` upstream changes] → version pinned in the Dockerfile; bumps are deliberate rebuilds.
- [Missing credential Secret at first use] → conversation dispatches but the pod crashloops on env resolution; README documents the one-secret prerequisite up front (identical to demo).
- [Both demo and k8s-operator enabled] → disjoint names/SAs/RBAC; no interaction beyond both wanting the credential Secret (shared by default — a feature).

## Migration Plan

1. Build/push `agentops-runtime-k8s:0.1.0`; chart minor bump ships the gated template (default on).
2. `helm upgrade`: bundle CRs appear; existing conversations/profiles untouched. Opt out with `k8sOperator.enabled=false`; grant full rights later with `--set k8sOperator.fullAccess=true` (one binding swap, no pod restarts needed — RBAC is evaluated live; the MCP mode change rolls on the next runtime pod).
3. Rollback: disable the flag (CRs removed by helm), or previous chart version.

## Open Questions

- Exact `mcp-server-kubernetes` version to pin (resolve latest at implementation; verify its non-destructive env var name against the pinned version's docs).
- Whether the read-only ClusterRole should include CRD-group reads (e.g. `agentops.dev` itself) so the agent can inspect the operator's own state — leaning yes (harmless, useful for self-diagnosis).
