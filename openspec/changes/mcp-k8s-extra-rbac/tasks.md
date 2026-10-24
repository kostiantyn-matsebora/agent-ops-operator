## 1. Chart: extra read-rule extension — deployment-engineer

- [x] 1.1 Add `mcpServers.rbac.extraReadRules` to
      `chart/charts/kubernetes/values.yaml`, default `[]`, documented inline
      with the entry shape (`apiGroups`/`resources`/`verbs`).
- [x] 1.2 Add a `kubernetes.mcpServerExtraReadRules` helper in
      `chart/charts/kubernetes/templates/_helpers.tpl` (or inline in
      `mcp-server.yaml` if the chart's existing convention favors that for a
      single-use block) that renders the list and `fail`s per entry on any
      of these shapes:
      - a verb outside `get`/`list`/`watch`
      - `apiGroups: [""]` with `resources` containing `secrets`
      - `apiGroups` or `resources` containing `"*"`
      - `apiGroups: ["rbac.authorization.k8s.io"]`
      - `apiGroups: ["agentops.dev"]`

      Verify by rendering each rejected shape with `helm template` and
      confirming the failure names the entry.
- [x] 1.3 Call the helper in both the `readonly` and `full` branches of the
      `agentops-mcp-k8s` ClusterRole in
      `chart/charts/kubernetes/templates/mcp-server.yaml`, immediately after
      `agentops.runtimeReadRules`. Verify with `helm template` that an
      extra rule appears in the rendered ClusterRole under both
      `mcpServers.rbac.mode` settings.

## 2. Unit tests

- [x] 2.1 Extend
      `platform/manager/internal/integration/charttemplate_test.go` with
      cases for: default (no extra rule rendered), one valid extra rule
      rendered under `readonly`, the same rule rendered under `full`, and
      each of the five rejected shapes failing with a message naming the
      entry. Verify with
      `go test ./internal/integration/... -run TestChartTemplate` (adjust
      to the file's actual test function names) from
      `platform/manager/`.
- [x] 2.2 Run the full chart render suite and `go vet`/`go build` for
      `platform/manager` to confirm no other template broke. Verify with
      `go build ./... && go vet ./... && go test ./...` from
      `platform/manager/`.

## 3. System tests

- [x] 3.1 Nothing a cluster decides changes here: the extension renders a
      ClusterRole whose grant a server's API calls already exercise, same
      as every other RBAC rule this chart renders, and the system pack's
      existing MCP-server lane already asserts rendered RBAC takes effect
      against a real API server. No new system-test lane is needed. Ticked
      as the stated not-applicable this section requires.

## 4. Documentation

### Reference docs
- [x] 4.1 Add `mcpServers.rbac.extraReadRules` to `docs/configuration.md`,
      grouped with the bundle's other reversible RBAC values, naming the
      entry shape and the five rejected shapes.

### Adopter site
- [x] 4.2 Add a short, prose-only decision to `docs/integrations/kubernetes.md`:
      granting the MCP server read access to a specific third-party CRD, with
      the VictoriaMetrics `vmrules` case as the one worked example. No
      exhaustive values table, no renders table — matches the page's
      existing shape.
- [x] 4.3 Run the writing lint over both touched docs pages and fix any
      finding. Verify with
      `python3 .claude/scripts/rules_compliance.py docs/configuration.md docs/integrations/kubernetes.md`.
