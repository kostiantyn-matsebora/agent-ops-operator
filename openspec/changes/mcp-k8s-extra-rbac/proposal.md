## Why

`agentops-mcp-k8s`'s RBAC is a fixed, hardcoded rule set
(`agentops.runtimeReadRules`/`runtimeWriteRules` in `chart/templates/_helpers.tpl`).
It covers only core/apps/batch/networking/storage-style resources, so a read on
any third-party CRD — VictoriaMetrics' `vmrules.operator.victoriametrics.com`
hit this first — is Forbidden with no chart-driven way to grant it.

The only fix on hand today is a hand-applied RBAC object against the live
cluster. That is exactly the posture this repo's invariants forbid: every
RBAC grant here is chart-rendered from values, never hand-authored YAML.

## What Changes

- Add a values-driven, additive, read-only RBAC extension point on
  `k8s-bundle`'s MCP server ServiceAccount — a list of extra
  `{apiGroups, resources, verbs}` rules composed onto the existing fixed read
  rule set.
- The render fails, naming the offending entry, on any of five shapes:
  - a verb outside `get`/`list`/`watch`
  - `resources: ["secrets"]` under `apiGroups: [""]`
  - `apiGroups` or `resources` equal to `"*"`
  - `apiGroups: ["rbac.authorization.k8s.io"]`
  - `apiGroups: ["agentops.dev"]`

  So this knob cannot become a second way to grant mutation, read secrets, or
  escalate RBAC/CRD access.
- Default: empty. No install's RBAC changes unless it opts in.
- No new values path for the write-side rule set — mutations stay confined to
  the existing `mcpServers.readOnly`/toolset split, unchanged.
- **BREAKING**: none. Additive and default-off.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `k8s-mcp-tooling`: the "optional server workload runs under its own
  reviewable identity" requirement gains the extra-rules extension point —
  still values-gated, still reviewable, still distinct from the "no built-in
  role" posture other RBAC in this repo already follows.

## Impact

**Code**:
- `chart/charts/kubernetes/templates/` (the MCP server's `ClusterRole`) —
  compose the new values list onto the rendered rule set, with the five
  refusal checks.
- `chart/charts/kubernetes/values.yaml` — new `mcpServers.rbac.extraReadRules`
  key, default `[]`.
- `chart/charts/kubernetes/` render tests — new cases for the extension, each
  of the five refusal shapes, and the empty-default no-op.

**Reference docs** — made untrue by this change:
- `docs/configuration.md` — new row(s) for
  `mcpServers.rbac.extraReadRules`, grouped with the bundle's other reversible
  RBAC values.

**Adopter site** — made untrue by this change:
- `docs/integrations/kubernetes.md` — the real decision this adds (granting
  the MCP server read access to a specific third-party CRD, with the
  VictoriaMetrics `vmrules` case as the worked example), stated in prose, no
  exhaustive values table.
