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
  image: {repository: ghcr.io/containers/kubernetes-mcp-server, tag: v0.0.66}
  port: 8080
  readOnly: true                # --read-only; filters at REGISTRATION (see D4)
  toolsets: [core, config]      # upstream default adds `helm`; dropped here
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

**Resolved (2026-08-08): `ghcr.io/containers/kubernetes-mcp-server:v0.0.66`.**

Against the four criteria:

- **Transport** — `--port <n>` puts it in HTTP mode serving streamable HTTP at `/mcp` and SSE at `/sse`; `--bind-address` defaults to `0.0.0.0`. The bundle defaults to `type: http` on `/mcp` (streamable HTTP is the current MCP transport; SSE is the legacy one), with a `mcp.transport` value for operators whose own server only speaks SSE.
- **Pod identity** — it talks to the API directly (no kubectl shell-out) and resolves in-cluster config when `--kubeconfig` is absent, so the pod's ServiceAccount IS the credential. D2's second identity is real, not a mounted kubeconfig.
- **Read-only** — `--read-only` drops every tool whose `ReadOnlyHint` is not true, and `--disable-destructive` drops `DestructiveHint` tools. Critically, the filter (`Configuration.isToolApplicable`) runs in `collectApplicableTools`, which decides what gets **registered** with the MCP server — not what a `tools/list` response shows. An unregistered tool is uncallable, which is the distinction the rejected Node server got wrong (below).
- **Maintenance / pinning** — v0.0.66 released 2026-07-31, multi-arch tags published per release under `ghcr.io/containers/kubernetes-mcp-server` (`v0.0.63`…`v0.0.66` all present). No `latest` in the values default.

Upstream's default toolsets are `core, config, helm`. The bundle defaults to `core, config`: `--read-only` would already drop `helm_install`/`helm_uninstall`, but a Kubernetes *observability* toolset has no reason to carry a Helm client at all, and dropping the toolset is one less thing depending on the annotation filter being right.

**Rejected:**

- `Flux159/mcp-server-kubernetes` (Node) — requires `kubectl`/`helm` binaries in the image, so the pod SA reaches the cluster through a shell-out rather than a client; SSE transport is gated behind an env var upstream names *unsafe*; and CVE-2026-46519 (fixed v3.6.0) was exactly the failure this criterion exists to catch — its restriction env vars, read-only among them, were enforced at the discovery layer but not at execution, so a client that already knew a tool name could call it anyway.
- `Azure/mcp-kubernetes` — closest runner-up: three transports, and `--access-level readonly` is its default with filtering at registration. Rejected for shelling out to `kubectl` (plus optional `helm`/`cilium`/`hubble`), which reintroduces inside the MCP server exactly the binary dependency `runtime-drop-kubectl` is trying to remove from the runtime, and for being an Azure-flavored distribution of a cluster-generic job.
- `openshift/openshift-mcp-server` — the same codebase downstream with OpenShift additions; `containers/` is the vendor-neutral upstream.
- ToolHive / Stacklok's Kubernetes MCP guide — a platform for *running* MCP servers, not a server. Adopting it means a second operator in the cluster to solve a problem one Deployment solves.

### D5: The toolset's tool list, and what read-only actually means here

Task 1.3's question — narrow `mcp__kubernetes__*` to enumerated names? — resolves to **no, by default**, with the enumeration recorded so narrowing is a values edit (`mcp.toolset.tools`).

With `--read-only` the server registers only these, so the wildcard and the enumeration grant the same thing:

| toolset | tools surviving `--read-only` |
| --- | --- |
| `core` | `pods_list`, `pods_list_in_namespace`, `pods_get`, `pods_log`, `pods_top`, `resources_list`, `resources_get`, `events_list`, `namespaces_list`, `projects_list`, `nodes_log`, `nodes_stats_summary`, `nodes_top` |
| `config` | `configuration_contexts_list`, `configuration_view`, `targets_list` |

Dropped by the filter: `pods_delete`, `pods_exec`, `pods_run`, `resources_create_or_update`, `resources_delete`, `resources_scale` (and the whole `helm` toolset, unshipped).

The wildcard is the default because the allowlist is the *second* wall, not the only one: the first is the server's own registration filter, the third is the server SA's RBAC. It matters most when an operator points `mcp.url` at a server this chart did not deploy — there the wildcard grants whatever that server exposes, which is why `mcp.toolset.tools` is values-overridable and the table above exists.

Note on RBAC coverage: `nodes_log` and `nodes_stats_summary` read through `nodes/proxy`, which `rbac.mode: readonly` deliberately does NOT grant — `view` plus node/namespace/metrics reads is the same grant shape the bundle's runtime RBAC uses, and `nodes/proxy` is a large privilege to hand a server by default. Those two tools are registered and will fail with a Forbidden the agent can read; widening is a deliberate operator grant.

## Risks / Trade-offs

- [The chosen server becomes a supply-chain dependency of the bundle] → `mcpServers` stays OFF by default, exactly as vm-bundle's does; the `MCPConfig` half works against an operator's own deployment. The image is tag-pinned and named in values.
- [Two RBAC identities to reason about] → That is the feature (D2), but it is genuinely more surface. The README documents both explicitly, and the readonly default keeps the server's grants equal to the agent's previous ones, so the starting posture is no wider than today's.
- [MCP tools and kubectl overlap, so the agent has two ways to do everything] → Accepted for this change: kubectl is the fallback by design. The overlap is exactly what `runtime-drop-kubectl` later removes, and keeping both first is what makes that removal evidence-based.
- [`mcp__kubernetes__*` is coarse] → It is the same granularity vm-bundle's namespaces have, and the toolset is values-overridable for operators who want to enumerate individual tool names once the server's tool list is known.
- [A default install now runs one more workload] → No: `mcpServers.enabled` defaults to false, so a default install renders only the `MCPConfig` and `MCPToolset` — and the `MCPConfig` points nowhere until the operator supplies a URL, which the render-time guard makes loud.

## Migration Plan

Purely additive: new values blocks default to a state that renders two inert CRs and no workload. Existing installs see no behavior change until they set `mcp.url` or enable `mcpServers`. Rollback = disable the component; the CRs are release-managed and removed.

## Open Questions

- ~~Which server image (D4)~~ — settled in D4: `ghcr.io/containers/kubernetes-mcp-server:v0.0.66`.
- ~~Whether `mcp.enabled` should default to `true` while `mcp.url` is empty.~~ — settled **no**. The leaning recorded here ("yes, with the guard only firing when a URL is genuinely required") does not survive contact with the guard: with `mcpServers` off by default there is no Service to default the URL onto, so `mcp.enabled: true` + empty `url` is precisely the case the guard must fail. A component that fails every default render is not "discoverable", it is broken. So `mcp.enabled` defaults to **false**, and discoverability is carried by the values comments and the README instead. Turning the component on is one flag plus the URL it genuinely needs — or one flag plus `mcpServers.enabled`, which supplies the URL itself.
