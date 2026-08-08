# k8s-mcp-tooling — design

## Context

`k8s-bundle` (archived 2026-08-08) ships three components: the cluster Events lane, the `k8s-engineer` profile with its runtime and ServiceAccount, and that SA's RBAC. The profile is specified as having "no repository/MCP" — the agent reads the cluster with `Bash` + `kubectl`, authenticating as `agentops-runtime-k8s` because the runtime pod automounts its SA token and `runtime-claude` bakes in kubectl (pinned v1.34.3).

`vm-bundle` solved the same problem differently because it had to: VictoriaLogs and VictoriaMetrics have no CLI in the runtime image and speak LogsQL/PromQL over HTTP, so it ships `MCPConfig` CRs (fixed server keys `victorialogs`/`victoriametrics`), an optional `mcpServers` component deploying the upstream server workloads, and — since `mcp-toolset-crd` — a `vm-observability` `MCPToolset`. Attaching all of it is one Pipeline stanza.

Nothing about that pattern is VictoriaMetrics-specific. The asymmetry exists only because kubectl was already in the image when the bundle was designed.

## Goals / Non-Goals

**Goals:**

- Structured Kubernetes access as MCP tools, wired the same way vm-bundle wires its tools, so the pattern is learned once.
- The bundle self-wires: with the component on, its own Pipeline binds the config and toolset, and no operator step remains.
- The MCP server's permissions are a separate, reviewable identity from the runtime SA's.
- Nothing regresses for operators who keep using kubectl.

**Non-Goals:**

- Not removing `kubectl` from `runtime-claude`, and not changing the profile's `allowedTools`. MCP becomes the capable path; kubectl stays the fallback. Removal is `runtime-drop-kubectl`.
- Not requiring `builtin-toolsets`. That change makes withholding `Bash` a Pipeline decision, which is what turns MCP into a real boundary — but this change stands alone without it.
- No write/apply MCP tools enabled by default, whatever the chosen server supports.
- No second MCP server for metrics — `vm-bundle` already covers that lane.

## Decisions

### D1: Mirror vm-bundle's component shape exactly

```yaml
mcp:
  enabled: true
  name: k8s-api                 # MCPConfig CR name
  url: ""                       # required when enabled unless mcpServers deploys it
  headers: []                   # valueFrom passthrough; the manager reads no Secrets
  toolset:
    name: k8s-observability
mcpServers:
  enabled: false                # deploy the server workload itself
  image: {repository: <TBD>, tag: <TBD>}
  port: 8080
  serviceAccountName: agentops-mcp-k8s
  rbac:
    create: true
    mode: readonly              # what the SERVER may reach, distinct from the agent's RBAC
  env: []
  resources: {}
```

Same names, same gating, same "empty url defaults onto the deployed Service" behavior as `vm-bundle.mcp` / `vm-bundle.mcpServers`, including failing the render with a message naming the required value when a component is enabled with no URL and no deployed server. Deliberate mimicry: two bundles that solve the same problem differently is a worse outcome than either shape on its own.

The server key `kubernetes` is FIXED and not values-configurable, for the reason vm-bundle fixes its own: the key IS the `mcp__kubernetes__*` tool namespace that toolsets and allowlists name, so making it configurable would let a values edit silently strip an agent's tools.

### D2: The MCP server gets its own identity

The server pod runs as `agentops-mcp-k8s`, not the runtime SA. This is the substantive difference from the kubectl path, and the reason the component is worth having beyond structure:

- With kubectl, the agent's reach is exactly the runtime SA's RBAC, and `Bash` gives it everything that SA can do.
- With MCP, reach is the intersection of (what the server SA may do) and (which `mcp__kubernetes__*` tools the allowlist grants). Two independent walls, each reviewable on its own.

`rbac.mode: readonly` binds `view` plus the node/namespace/metrics reads — the same shape the bundle's runtime RBAC uses, so the vocabulary is consistent. It defaults on because a server with no grants is a server that answers nothing.

An operator can therefore run the agent's own SA with no RBAC at all (`rbac.enabled: false` on the bundle) and let ALL cluster reads flow through MCP, which is the posture `runtime-drop-kubectl` eventually assumes.

### D3: The bundle wires itself

When `mcp.enabled`, the events Pipeline template adds:

```yaml
  mcpConfigs: {refs: [{name: <mcp.name>}]}
  toolsets:   {refs: [{name: <mcp.toolset.name>}]}
```

Default `merge` mode, so the profile's own tools survive and the MCP tools are added. This is the piece `vm-bundle` cannot do — it ships no profile and no Pipeline, so its README documents a manual stanza. `k8s-bundle` renders both, so it can close the loop and a default install has zero manual wiring.

Conversations reaching the profile by other paths (`POST /task`, `/<profile>` commands) do NOT get the MCP tools, since they carry no bindings. That is the documented consequence of wiring-level tooling, not a defect — and `builtin-toolsets`, by giving AgentProfile its own `toolsets`, is what would let an operator grant them everywhere if they want that.

### D4: Choosing the server image is in scope, and gates the defaults

The values above leave `image` as `<TBD>` on purpose. Selection criteria, to be settled in task 1.1 before any template hardcodes a default:

- Serves MCP over SSE or streamable HTTP (stdio-only servers cannot be a Service the runtime pod dials).
- Authenticates as its pod's ServiceAccount in-cluster, so D2's second identity is real rather than a config file holding a kubeconfig.
- Has a read-only mode or clearly separable read tools, so `rbac.mode: readonly` is not the only thing standing between an LLM and a mutating call.
- Maintained, and tag-pinnable.

If no server satisfies these, the honest outcome is to ship the `MCPConfig` + `MCPToolset` halves (useful for operators running their own server) and leave `mcpServers.enabled` unimplemented rather than defaulting to something unsuitable. Recorded so that outcome is a decision, not a silent descope.

## Risks / Trade-offs

- [The chosen server becomes a supply-chain dependency of the bundle] → `mcpServers` stays OFF by default, exactly as vm-bundle's does; the `MCPConfig` half works against an operator's own deployment. The image is tag-pinned and named in values.
- [Two RBAC identities to reason about] → That is the feature (D2), but it is genuinely more surface. The README documents both explicitly, and the readonly default keeps the server's grants equal to the agent's previous ones, so the starting posture is no wider than today's.
- [MCP tools and kubectl overlap, so the agent has two ways to do everything] → Accepted for this change: kubectl is the fallback by design. The overlap is exactly what `runtime-drop-kubectl` later removes, and keeping both first is what makes that removal evidence-based.
- [`mcp__kubernetes__*` is coarse] → It is the same granularity vm-bundle's namespaces have, and the toolset is values-overridable for operators who want to enumerate individual tool names once the server's tool list is known.
- [A default install now runs one more workload] → No: `mcpServers.enabled` defaults to false, so a default install renders only the `MCPConfig` and `MCPToolset` — and the `MCPConfig` points nowhere until the operator supplies a URL, which the render-time guard makes loud.

## Migration Plan

Purely additive: new values blocks default to a state that renders two inert CRs and no workload. Existing installs see no behavior change until they set `mcp.url` or enable `mcpServers`. Rollback = disable the component; the CRs are release-managed and removed.

## Open Questions

- Which server image (D4) — blocking for the values defaults, not for the rest of the design.
- Whether `mcp.enabled` should default to `true` while `mcp.url` is empty. Leaning yes with the render-time guard only firing when a URL is genuinely required, so the toolset and config are discoverable in a default install; the alternative hides the pattern until an operator goes looking.
