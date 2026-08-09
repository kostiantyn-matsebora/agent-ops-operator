# main-chart-owns-runtime

## Why

`AgentRuntime` is created in exactly one place in the chart —
`charts/k8s-bundle/templates/profile.yaml` — and nothing about it is
Kubernetes-shaped. Image, Claude credential, idle TTL, `nodeSelector` and home
volume describe *how an agent executes*, which is the same for a VictoriaMetrics
lane, a Home Assistant lane, or a chat-only install. The placement is not a
design decision: `chart 2.x — demo values move into k8s-bundle` relocated the
parent's `demo.*` block wholesale, and the runtime rode along because demo mode
means "this bundle with its defaults".

The consequences are all visible in the values today:

- **No runtime without the k8s bundle.** `telegram-bundle` and `vm-bundle` ship
  no runtime, and the parent has no `AgentRuntime` template, so an install of
  either alone renders nothing that can execute a conversation. The bundle is a
  shared prerequisite wearing a bundle's clothes.
- **Two runtime ServiceAccounts exist.** The parent already creates
  `serviceAccounts.runtime` (`agentops-runtime`, granted nothing) and the
  manager already defaults runtime pods onto it (`RUNTIME_SA`); the bundle
  creates a second one, `agentops-runtime-k8s`, and grants that one everything.
- **`homePvcRef` is a documented workaround** — "the parent chart's
  `persistence` block is not visible to a subchart, so name its claim here" —
  for a volume the parent creates.
- **Idle TTL is configured twice**, as the manager's `runtimeIdleTtlMinutes`
  (default 1) and the bundle's `profile.runtime.idleTtlMinutes` (default 10).

The same redundancy shows up one layer out, in the k8s bundle's MCP values. The
bundle's own comments concede the invariant: `mcpServers.readOnly` and
`mcpServers.rbac.mode` have to agree with `rbac.mode`, or a `full` agent pushes
every write back onto kubectl — the single-wall path MCP exists to replace. That
agreement is currently maintained by hand in every install's values, and the
MCP component defaults to off only because it cannot self-supply an endpoint
while the server workload also defaults to off.

## What Changes

- **The main chart gains a `runtime:` component**, enabled by default: the
  `AgentRuntime` named `default`, its credential Secret when a token is
  supplied, and `home.pvcRef` wired automatically from the parent's own
  `persistence` block. It runs as the parent's existing
  `serviceAccounts.runtime` — the second SA disappears.
- **The agent's in-cluster power becomes one knob**,
  `global.agentops.runtime.rbacMode` (`readonly` | `full` | `none`), readable by
  the parent and by every subchart. The parent renders the canned bindings from
  it; `rbac.runtime.{clusterRoles,bindClusterRoles,namespaced}` keep working for
  targeted additions.
- **BREAKING — `k8s-bundle` stops shipping or configuring any runtime.** The
  `profile.runtime.*` block and the bundle's `rbac.*` block are removed, along
  with the runtime ServiceAccount and the credential Secret. The bundle keeps
  the profile's identity (`name`, `systemPrompt`, `maxTurns`) and
  `profile.runtimeRef` for pointing at a non-default runtime.
- **MCP tooling defaults on**: `mcp.enabled` and `mcpServers.enabled` both
  become `true`. Flipping them together satisfies the endpoint guard that
  justified the old default — the config URL defaults onto the deployed
  Service.
- **The MCP server's posture derives from the one RBAC knob**:
  `mcpServers.readOnly` and `mcpServers.rbac.mode` become `null`-by-default and
  follow `rbacMode` (`full` ⇒ a write-capable server under a `full` SA;
  anything else ⇒ read-only under a readonly SA). An explicit value still wins,
  so "kubectl writes, MCP reads only" stays reachable — it stops being the
  shape you get by accident.
- **Chart 4.0.0** with a migration table; `runtime.enabled: false` is the hold
  position for anyone managing `AgentRuntime` CRs themselves.

## Capabilities

### Added Capabilities

- `agent-runtime-ownership`: the main chart owns the default execution
  substrate — one `AgentRuntime`, one runtime ServiceAccount, one credential,
  one RBAC mode — and bundles contribute profiles, sources, channels and
  tooling that reference it.

### Modified Capabilities

- `k8s-bundle`: the profile component ships identity only; the runtime, its SA,
  its credential and its RBAC move to the parent. A default install now renders
  an `AgentRuntime` (from the parent) and still no bundle objects.
- `k8s-mcp-tooling`: the MCP component and its server workload default on, and
  the server's read-only mode and ServiceAccount RBAC derive from the single
  runtime RBAC mode instead of being set independently in every install.

## Impact

- **Chart (parent)**: new `templates/runtime.yaml`; `values.yaml` gains
  `runtime:` and `global.agentops.runtime.*`; `runtime-rbac.yaml` gains the
  mode-driven bindings; `NOTES.txt` credential warning re-points from
  `k8s-bundle.profile.runtime.*` to `runtime.*`; version 3.4.0 → **4.0.0**.
- **Chart (k8s-bundle)**: `templates/profile.yaml` loses the Secret, the
  ServiceAccount and the `AgentRuntime`; `templates/rbac.yaml` removed;
  `_helpers.tpl` loses `runtimeServiceAccount`; `values.yaml` loses
  `profile.runtime` and `rbac`, and changes four MCP defaults.
- **Upgrade-visible, beyond the values moves**: the runtime SA changes from
  `agentops-runtime-k8s` to `agentops-runtime`, so bindings are replaced; and
  an install that enabled the bundle without touching MCP now gets an MCP
  server workload it did not have. Both are called out in the migration note.
- **Docs**: `docs/k8s-bundle.md`, `docs/k8s-mcp-tooling` content in
  `docs/k8s-bundle.md`, `docs/concepts.md` (runtime is not a bundle concern),
  README migration table, CHANGELOG.
- **Downstream (`_gitops`)**: `apps/agent-ops` — the
  `mcpMode` knob and the whole `mcp:`/`mcpServers:` values block are deleted,
  `claudeToken` and `appNodeSelector` move to the top-level `runtime:` block,
  and `rbacMode` becomes the global. Tracked in this change because the
  redundancy there is what surfaced the design fault.
- **Composes with** `runtime-drop-kubectl`: that change argues the runtime image
  must carry no domain tooling; this one argues the runtime *object* must not
  live in a domain bundle. Neither depends on the other, but landing this first
  makes that one a single-file edit.
- **Unblocks** `ha-bundle`: it can ship a profile and reference the parent's
  runtime rather than duplicating the runtime block a third time.
