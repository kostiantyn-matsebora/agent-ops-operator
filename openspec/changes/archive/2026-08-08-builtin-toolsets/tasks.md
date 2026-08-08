> **Scope narrowed mid-implementation.** An `AgentProfile.spec.toolsets` field and
> a two-layer resolution fold were built, then reverted on user direction: tool and
> MCP access belong to the Pipeline, not the profile. The per-route boundary never
> needed the field — a Pipeline binding `overwrite` already withholds tools the
> profile grants. See the proposal's "No profile-side API" bullet.

## 1. Chart — the built-in toolset catalog

- [x] 1.1 `chart/templates/builtin-toolsets.yaml`: three `MCPToolset` CRs (observe / shell / edit), values-overridable names, values-extendable tool lists, gated on `global.builtinToolsets.enabled` (default true)
- [x] 1.2 `chart/values.yaml`: the `global.builtinToolsets` block — under `global.` so subcharts can reference the same names, with a comment showing the `overwrite` stanza that drops shell for one route

## 2. Task-lane binding propagation

- [x] 2.1 `internal/httpapi/server.go` `handleTask`: when the request names a pipeline, copy `Toolsets`/`MCPConfigs` alongside `ChannelRefs`; requests without `pipeline` stay binding-less
- [x] 2.2 Integration test: `POST /task` with a pipeline carries its bindings; without one it does not

## 3. Verification, samples, docs

- [x] 3.1 `helm template` checks: toolsets render by default, vanish when disabled, and a rename propagates to every reference
- [x] 3.2 `config/samples/samples.yaml`: the built-in toolsets, and a Pipeline binding observe-only in `overwrite` mode against a profile that grants shell
- [x] 3.3 README: the built-in catalog and the `overwrite` idiom; note the `POST /task` propagation behavior change
- [x] 3.4 CLAUDE.md: `MCPToolset` terminology — bound from `Pipeline.spec.toolsets` ONLY, chart ships the catalog under `global.builtinToolsets`
- [x] 3.5 Full verification: `go build ./... && go vet ./...`, CRD regen clean, envtest suite, `helm lint` + template matrix
- [x] 3.6 Record the `all-in-one-crd` interaction (it inlines `Pipeline.spec.toolsets`; no profile binding exists to reconcile after the revert)
