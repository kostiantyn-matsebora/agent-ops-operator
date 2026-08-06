# Tasks: ha-bundle

## 1. Subchart

- [ ] 1.1 Scaffold `chart/charts/ha-bundle/` (Chart.yaml 0.1.0, values.yaml per design D2 with documented defaults + prerequisites comment incl. the LF-only SSH-key gotcha)
- [ ] 1.2 Templates: `mcpconfig.yaml` (per-server guards), `agentprofile.yaml` (optional repository/HA-server/env blocks, configRef to the bundle MCPConfig), `signalsource.yaml` (type/config passthrough, optional channelRef, grouping)
- [ ] 1.3 Parent `chart/Chart.yaml`: first `dependencies` entry (`file://charts/ha-bundle`, `condition: ha-bundle.enabled`); parent values `ha-bundle.enabled: false`; chart minor bump

## 2. Verification

- [ ] 2.1 `helm lint` + `helm template` matrix: default (nothing rendered), enabled-full (all three objects, zero Secret kinds), enabled-partial (repo-less, single MCP server — valid CRs), custom signal type passthrough
- [ ] 2.2 Envtest-independent sanity: apply the enabled-full rendered output to envtest CRDs via `kubectl --dry-run=server` equivalent (or apply in a scratch namespace on the live cluster with throwaway names, then delete) to prove CR validity against real schemas

## 3. Docs

- [ ] 3.1 README bundle section (enable recipe, values table pointer, prerequisites, adoption guidance for hand-applied installs per design D4) + CLAUDE.md map line for `chart/charts/ha-bundle/`
