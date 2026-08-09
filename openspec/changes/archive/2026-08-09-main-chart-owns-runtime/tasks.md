## 1. Parent chart owns the runtime

- [x] 1.1 `chart/values.yaml`: add `global.agentops.runtime.serviceAccountName`
      (default `agentops-runtime`) and `global.agentops.runtime.rbacMode`
      (default `""`), documented as the single knob for the agent's in-cluster
      power — per design D2
- [x] 1.2 `chart/values.yaml`: add the `runtime:` block — `enabled: true`,
      `name: default`, `image`, `idleTtlMinutes: ""` (D5), `nodeSelector`,
      `resources`, `homePvcRef: ""`, `credentialsSecret.{name,key,envName,token}`
- [x] 1.3 New `chart/templates/runtime.yaml`: render the `AgentRuntime` named
      `runtime.name`, `serviceAccountName` from the global, the credential env via
      `valueFrom` (never read by the manager), and the Secret when
      `credentialsSecret.token` is set
- [x] 1.4 Wire `home.pvcRef` from `persistence` (name, or `existingClaim`) with
      `runtime.homePvcRef` as the explicit override — D4; delete no parent
      `persistence` behavior
- [x] 1.5 `chart/templates/runtime-rbac.yaml`: render the mode-driven bindings —
      `readonly` = `view` + the nodes/namespaces/metrics ClusterRole (the
      bundle's rules verbatim), `full` = `cluster-admin`, `none` = nothing; keep
      `rbac.runtime.{clusterRoles,bindClusterRoles,namespaced}` additive
- [x] 1.6 Implement `""` ⇒ `readonly` under `global.demo.enabled`, `none`
      otherwise (D3), in a named helper so both the parent and the bundle's MCP
      derivation resolve the SAME effective mode
- [x] 1.7 `chart/templates/NOTES.txt`: re-point the missing-credential warning
      from `k8s-bundle.profile.runtime.*` to `runtime.*`, and gate it on
      `runtime.enabled` rather than the bundle

## 2. k8s-bundle sheds the substrate

- [x] 2.1 `charts/k8s-bundle/templates/profile.yaml`: remove the Secret, the
      ServiceAccount and the `AgentRuntime`; keep the `AgentProfile` and emit
      `runtimeRef` only when `profile.runtimeRef` is set
- [x] 2.2 Delete `charts/k8s-bundle/templates/rbac.yaml` and the `rbac:` values
      block; the events adapter's own `eventsAdapter.rbac` is untouched and must
      not be caught by the deletion
- [x] 2.3 `charts/k8s-bundle/_helpers.tpl`: drop `k8s-bundle.runtimeServiceAccount`;
      re-point every remaining reader (`mcp-server.yaml`'s equality guard) at the
      global
- [x] 2.4 `charts/k8s-bundle/values.yaml`: delete `profile.runtime.*` and
      `rbac.*`; keep `profile.{enabled,name,systemPrompt,maxTurns,runtimeRef}`
- [x] 2.5 Verify the "sharing the runtime identity is refused" guard still fires:
      `mcpServers.serviceAccountName` equal to the GLOBAL runtime SA must fail
      the render (`k8s-mcp-tooling` pins this)

## 3. MCP defaults and derivation

- [x] 3.1 `charts/k8s-bundle/values.yaml`: `mcp.enabled: true`,
      `mcpServers.enabled: true`; rewrite the comment that justified default-off
      so it explains the endpoint guard instead of a default it no longer has
- [x] 3.2 `mcpServers.readOnly: null` and `mcpServers.rbac.mode: null`, deriving
      from the effective `rbacMode` per D6's table; an explicit value wins
- [x] 3.3 Confirm `k8s-bundle.mcpAdminEnabled` still resolves correctly — with
      derivation, `rbacMode: full` must render the `k8s-admin` toolset with no
      other values set
- [x] 3.4 Keep the `mcp.yaml` endpoint `fail` untouched, and add a template test
      for the one still-broken combination: `mcp.enabled` + `mcpServers.enabled:
      false` + no `url`

## 4. Docs and release

- [x] 4.1 `docs/concepts.md`: state the rule this change establishes — the parent
      contributes the substrate (runtime, identity, credential), bundles
      contribute domain (sources, profiles, tooling, channels)
- [x] 4.2 `docs/k8s-bundle.md`: the component table loses the runtime/SA/RBAC
      rows; the MCP rows become default-on; the `rbac.mode: full` warning moves
      to the parent's `rbacMode` and keeps its teeth
- [x] 4.3 README: the `runtime:` block and the two `global.agentops.runtime.*`
      keys documented together (D2), plus the derivation table from D6
- [x] 4.4 `chart/Chart.yaml` 3.4.0 → **4.0.0**; CHANGELOG entry marked BREAKING
      carrying the migration table from design.md verbatim, and leading with the
      two upgrade-visible surprises: the runtime SA rename and the MCP server
      that now appears
- [x] 4.5 `CLAUDE.md`: chart map entry for `templates/runtime.yaml`

## 5. Downstream: _home-data-center

- [x] 5.1 `apps/ai/apps/agent-ops/helmfile.d/helmfile.yaml.gotmpl`: delete the
      `mcp:`/`mcpServers:` blocks and the `k8s-bundle.profile.runtime` block; add
      the top-level `runtime:` block carrying `appNodeSelector` and the Claude
      token; set `global.agentops.runtime.rbacMode`
- [x] 5.2 `environments/default.yaml`: delete `k8s.mcpMode` and its eight-line
      invariant comment (the chart now enforces it); move `k8s.claudeToken` to
      `runtime.claudeToken`; `k8s.rbacMode` → `rbacMode`
- [x] 5.3 `README.md` (agent-ops): the "Two identities" section keeps its point
      but renames `agentops-runtime-k8s` → `agentops-runtime`; the out-of-band
      Secret note stays deleted (it is release-managed already)
- [x] 5.4 `.github/instructions/`: check for an agent-ops instructions file and
      update the runtime/RBAC/MCP values paths if one exists

## 6. Verification

- [x] 6.1 `helm lint chart` + `helm template` matrix: defaults; `runtime.enabled=false`;
      `global.demo.enabled=true` alone; `k8s-bundle.enabled=true` alone;
      `telegram-bundle.enabled=true` alone (must now render a runtime);
      `rbacMode=full`; `rbacMode=none`
- [x] 6.2 Assert on rendered output, not just success: exactly one `AgentRuntime`
      and one runtime SA in every combination, and zero `agentops-runtime-k8s`
      objects anywhere
- [x] 6.3 `go build ./... && go vet ./...` and the envtest suite — no Go change is
      expected, so a failure here means an assumption about `RUNTIME_SA` or the
      `default` runtime fallback was wrong
- [x] 6.4 Live: `helmfile apply` the downstream install, confirm the k8s-engineer
      agent answers a cluster question through MCP writes, the console still
      mirrors, and the credential Secret came back with the release
- [x] 6.5 Confirm the old `agentops-runtime-k8s` ClusterRoleBinding is gone after
      upgrade (Helm should remove it; delete it by hand if the release adopted it)
