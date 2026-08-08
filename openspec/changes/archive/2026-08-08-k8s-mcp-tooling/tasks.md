## 1. Choose the server (gates the defaults)

- [x] 1.1 Evaluate Kubernetes MCP server images against design D4: serves SSE or streamable HTTP (not stdio-only), authenticates as its pod ServiceAccount in-cluster, has a read-only mode or separable read tools, maintained and tag-pinnable. Record the choice and the rejected candidates in `design.md`
- [x] 1.2 If no candidate qualifies: ship the config + toolset halves only, mark `mcpServers` unimplemented in values with a comment naming the blocker, and update the proposal — an explicit descope, never a silent one
- [x] 1.3 Capture the chosen server's tool names, so the toolset can be narrowed from `mcp__kubernetes__*` to an enumerated read-only list if the server mixes read and write tools

## 2. Chart — config and toolset halves

- [x] 2.1 `chart/charts/k8s-bundle/values.yaml`: add the `mcp` block (`enabled`, `name`, `url`, `headers`, `toolset.name`) mirroring `vm-bundle.mcp`, with comments explaining why the server key is fixed
- [x] 2.2 New `templates/mcp.yaml`: the `MCPConfig` with fixed server key `kubernetes` + values URL + `headers` passthrough, and the `MCPToolset`; render-time `fail` naming `k8s-bundle.mcp.url` when required and empty
- [x] 2.3 Events template: when `mcp.enabled`, add `mcpConfigs` + `toolsets` stanzas in `merge` mode to the rendered Pipeline; when the events source is off, render the CRs and no Pipeline

## 3. Chart — optional server workload

- [x] 3.1 `mcpServers` values block (off by default): image, port, `serviceAccountName` (default `agentops-mcp-k8s`), `rbac.{create,mode}`, env, resources
- [x] 3.2 Templates: Deployment + Service + ServiceAccount, and the server's OWN RBAC (readonly = `view` + node/namespace/metrics reads) bound to that SA — distinct from the runtime SA, which is the point of the component
- [x] 3.3 Empty `mcp.url` defaults onto the deployed Service, matching vm-bundle's behavior

## 4. Verification, samples, docs

- [x] 4.1 `helm template` matrix: defaults render the config + toolset and NO workload; `mcpServers.enabled=true` renders workload/Service/SA/RBAC and defaults the URL onto the Service; `mcp.enabled=false` renders neither and leaves the Pipeline without tooling stanzas; MCP on + events off renders CRs but no Pipeline; empty required URL fails naming the value; the server SA is never the runtime SA
- [x] 4.2 Validate every rendered `agentops.dev` object against the CRD structural schemas — CRDs prune unknown fields silently, so a typo would vanish rather than fail
- [x] 4.3 `config/samples/samples.yaml`: a Pipeline binding the k8s MCP config + toolset to an operator-owned profile
- [x] 4.4 README: k8s bundle component table gains `mcp`/`mcpServers`; document the two-identity model (server SA vs runtime SA) and that kubectl remains the fallback; note that MCP tools reach only pipeline-originated conversations
- [x] 4.5 CLAUDE.md: k8s-bundle map entry gains the MCP component and the second SA
- [x] 4.6 `helm lint` + full template smoke including co-installation with `vm-bundle` and with demo mode
