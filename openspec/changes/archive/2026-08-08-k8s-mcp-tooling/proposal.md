# k8s-mcp-tooling

## Why

`k8s-bundle` ships an agent that reaches the cluster exclusively through `Bash` + `kubectl`, with Kubernetes RBAC as the only wall. That works, but it is the odd one out: `vm-bundle` already ships MCP servers, `MCPConfig` CRs, and a `vm-observability` toolset, so an operator who learns the tooling pattern there finds nothing analogous for the cluster itself. It also leaves structured cluster access unavailable — kubectl output has to be parsed out of shell text — and pins the runtime image to a kubectl binary it must rebuild to update.

## What Changes

- `k8s-bundle` gains an **`mcp` component** mirroring vm-bundle's shape: an `MCPConfig` CR (default `k8s-api`) with a fixed server key `kubernetes` (the key IS the `mcp__kubernetes__*` tool namespace, so it is deliberately not values-configurable), pointing at a Kubernetes MCP server endpoint.
- An **`mcpServers` sub-component** (off by default, like vm-bundle's) optionally deploys the MCP server workload itself with its own ServiceAccount, so the bundle is a complete appliance rather than a set of references. The server's SA is a **second, separately-reviewable identity** — the RBAC granted to it is what MCP tools can reach, independent of the runtime SA's grants.
- A **`k8s-observability` `MCPToolset`** granting `mcp__kubernetes__*`, so attaching the tools stays a one-stanza Pipeline edit.
- Every Pipeline the bundle renders — the events lane and the addressable one — binds both halves (`mcpConfigs` + `toolsets`) when the component is active, so turning it on needs no operator wiring step at all. (The addressable Pipeline post-dates this proposal; excluding it would leave `POST /task` with a strictly weaker agent than an event reaches, which nobody asked for.)
- The component is **off by default**, alone among the bundle's components: an `MCPConfig` needs an endpoint, and with the server sub-component off there is none to default onto, so a default-on component would fail its own "missing endpoint" guard on every render — demo mode included.
- **Choosing the server image is part of this change**, not an assumption: it needs an SSE/HTTP-mode Kubernetes MCP server, evaluated for read-only capability and for whether it authenticates as its pod's ServiceAccount. Settled in design D4: `ghcr.io/containers/kubernetes-mcp-server`, pinned, run with `--read-only`.
- `kubectl` stays exactly as it is — MCP becomes the capable path, kubectl remains the fallback wherever no MCP server is deployed. Removing it is a separate, later change.

## Capabilities

### New Capabilities

- `k8s-mcp-tooling`: the bundle's Kubernetes MCP half — the MCPConfig with its fixed server key, the optional server workload and its distinct RBAC identity, the observability toolset, and the self-wiring of the bundle's Pipeline.

### Modified Capabilities

- `k8s-bundle`: the component set gains `mcp` (and its `mcpServers` sub-component); the events component's Pipeline binds the bundle's tooling when it is active; the "no repository/MCP" property of the shipped profile is replaced by "no repository; MCP via wiring, not on the profile".

## Impact

- **Chart**: new templates in `chart/charts/k8s-bundle/templates/` (MCPConfig, MCPToolset, optional Deployment + Service + SA + RBAC for the server); the events Pipeline template gains the two tooling stanzas; new `mcp`/`mcpServers` values blocks.
- **No API, controller, or module changes**: `MCPConfig`, `MCPToolset` and the Pipeline bindings all exist already (`mcp-toolset-crd`, archived 2026-08-08).
- **Docs**: README's k8s bundle section (component table, the MCP-vs-kubectl relationship, the second RBAC identity), `config/samples/samples.yaml`.
- **Dependency**: none blocking. Composes with `builtin-toolsets` (which makes withholding `Bash` a Pipeline decision) but does not require it — this change is additive on its own.
- **Follow-up this unblocks**: `runtime-drop-kubectl`, which needs a capable MCP path to exist first.
