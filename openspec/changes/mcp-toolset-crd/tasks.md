## 1. API

- [ ] 1.1 Add `api/v1alpha1/mcptoolset_types.go`: `MCPToolsetSpec{Tools []string}` (pure tool list, no status conditions), register in scheme
- [ ] 1.2 Add `ToolingBinding{Mode (Enum merge|overwrite, default merge), Refs []ObjectRef}` to `common_types.go`; add `Toolsets`/`MCPConfigs *ToolingBinding` to `PipelineSpec` and mirror both on `ConversationSpec` (doc comments: materialized state, not wiring)
- [ ] 1.3 Regenerate deepcopy and CRD YAML (`controller-gen object` + `crd` into `chart/files/crds/`); add `mcptoolsets` (lowercase plural) get/list/watch to `chart/templates/rbac.yaml`

## 2. Pipeline validation + materialization

- [ ] 2.1 `pipeline_controller.go`: `Ready` validates `toolsets.refs` (MCPToolset) and `mcpConfigs.refs` (MCPConfig) with the existing `MissingReferences` vocabulary; watch both kinds for requeue
- [ ] 2.2 Snapshot both bindings onto created Conversations at every pipeline-originated creation site: `routeSignalGroup` (`internal/httpapi/signals.go`) and both router `defaultProfile` paths (`internal/chat/router.go` — plumb the pipeline through alongside the profile name); leave `POST /task` and `/profile`-command paths binding-less
- [ ] 2.3 Integration tests: pipeline Ready flips on dangling/restored tooling refs; created conversations carry the snapshots; task-API conversations don't

## 3. Resolution — compile + dispatch

- [ ] 3.1 `internal/mcpcompile/`: add overlay entry (base MCPSpec + ordered MCPConfigSpec overlays + mode) reusing existing per-server merge and env-placeholder machinery; raw-form base + merge → typed error
- [ ] 3.2 `conversation_controller.go`: `ensureMCPConfigMap` becomes conversation-aware — no `mcpConfigs` binding → existing profile-owned `agentops-mcp-<profile>` byte-identical; with binding → conversation-owned `agentops-mcp-conv-<name>` compiled via the overlay entry, mounted by the runtime pod builder; raw-form merge error → visible conversation condition
- [ ] 3.3 Effective allowlist at dispatch: `/work` handler fetches the conversation's toolsets; `dispatch.Next` receives the resolved `allowedTools` (merge = profile ∪ toolsets dedup; overwrite = toolsets only); missing ref → visible failure, no silent fallback
- [ ] 3.4 Tests: mcpcompile overlay unit tests (merge/overwrite, key collisions, raw-form error, valueFrom preservation); envtest — byte-identical binding-less CM + WorkUnit, per-conversation CM ownership/GC, dispatch allowlist for both modes, missing-ref failure surfacing

## 4. vm-bundle + samples + docs

- [ ] 4.1 vm-bundle: `MCPToolset vm-observability` template (tools tracking enabled components), values (`toolset.name`), update the values-comment NOTE block to the Pipeline stanza; `helm template` checks (component-toggle filtering, no toolset when both mcp components off)
- [ ] 4.2 `config/samples/samples.yaml`: an MCPToolset + a Pipeline showing both tooling stanzas with modes
- [ ] 4.3 README.md (toolset/config binding concept, resolution table for both modes, bundle wiring stanza, overwrite sharp-edge note) and CLAUDE.md (terminology entry under Pipeline, map, invariants if touched)
- [ ] 4.4 Full verification: `go build ./... && go vet ./...`, CRD regen clean, envtest suite with `KUBEBUILDER_ASSETS`, `helm lint`/template smoke incl. vm-bundle matrix
