> **Blocked on one decision.** Design D1 — capability-only Pipelines vs. resolving
> the profile's oldest routing Pipeline vs. accepting no capabilities — decides
> whether a bare `POST /task` still works, which is the README's five-minute demo.
> Get sign-off before task 1.1; everything below assumes D1(a).
>
> **Removing a CRD field deletes that data on the next write.** The migration in
> section 5 is a PRE-upgrade step, not a post-upgrade one.

## 1. API

- [ ] 1.1 Remove `AllowedTools` and `MCP` from `AgentProfileSpec`; update the type's doc comment to say identity, not capability
- [ ] 1.2 Remove `Mode` from `ToolingBinding` and the `Merges()` helper; drop the `ToolingMerge`/`ToolingOverwrite` constants
- [ ] 1.3 Add `ConfigMapRef`/`SecretRef` to `MCPConfigSpec` as alternatives to `servers` (D4), with a doc comment stating a raw config is exclusive
- [ ] 1.4 Regenerate deepcopy + CRD YAML; confirm the removed fields are gone from `chart/files/crds/agentops.dev_agentprofiles.yaml` and `_pipelines.yaml`

## 2. Compile and dispatch collapse

- [ ] 2.1 `internal/mcpcompile`: replace `Compile`/`CompileOverlaid` with one `Compile([]MCPConfigSpec)`; delete `RawMergeError` and the raw-form-merge rule; add the raw-config exclusivity error; keep the env-placeholder machinery untouched (the manager still reads no Secrets)
- [ ] 2.2 `internal/dispatch`: `EffectiveAllowedTools` becomes a concatenation of the bound toolsets — no base, no mode
- [ ] 2.3 `internal/controller/conversation_controller.go`: `ensureMCPConfigMap` loses its profile branch and the profile-owned ConfigMap; a conversation with no `mcpConfigs` binding mounts an empty `mcp.json`; drop the `IncompatibleMCPForm` condition reason
- [ ] 2.4 `internal/httpapi/server.go`: `effectiveAllowedTools` stops reading `profile.Spec.AllowedTools`

## 3. Capability-only Pipeline resolution (D1)

- [ ] 3.1 `internal/chat/pipelines.go`: add `CapabilityPipelineForProfile` — the Ready Pipeline naming this profile with no sources and no channels; return none when two match, so ambiguity never resolves silently
- [ ] 3.2 `pipeline_controller.go`: surface the duplicate-baseline conflict as a condition on both Pipelines, mirroring the source-conflict vocabulary
- [ ] 3.3 Apply it at the pipeline-less creation sites: `handleTask` (no `pipeline` field) and the router's `/<profile>` command path — snapshot the baseline's bindings the same way routing pipelines are snapshotted
- [ ] 3.4 Integration tests: bare `POST /task` gets the baseline; a routing pipeline overrides it; `/<profile>` on a channel wired to a different profile gets the named profile's baseline; no baseline means no capabilities; two baselines mean neither applies and both report

## 4. Chart, bundles, samples

- [ ] 4.1 `k8s-bundle`: profile drops `allowedTools`; render a capability-only Pipeline carrying the built-in toolsets so the demo's `POST /task` still works; the events Pipeline keeps its own bindings
- [ ] 4.2 `vm-bundle`: remove the documented "edit your profile (`mcp.configRefs` + allowlist)" alternative — Pipeline stanzas are the only path now
- [ ] 4.3 `config/samples/samples.yaml`: profiles carry no capabilities; show a capability-only Pipeline alongside the routing ones
- [ ] 4.4 `helm template` matrix: demo mode renders a capability Pipeline for the bundle profile; no rendered AgentProfile carries `allowedTools` or `mcp`; validate rendered CRs against the CRD structural schemas (pruning hides typos)

## 5. Migration, docs, verification

- [ ] 5.1 Write the pre-upgrade audit recipe: a `kubectl` one-liner listing every AgentProfile carrying `allowedTools` or `mcp`, so an operator sees exactly what must move BEFORE the fields are pruned
- [ ] 5.2 `chart/templates/NOTES.txt`: post-install check that warns when profiles were upgraded without migrating
- [ ] 5.3 README: shrink the resolution table (no modes, no profile layer), document capability-only Pipelines as a profile's baseline, and add the migration section with the audit recipe
- [ ] 5.4 CLAUDE.md: terminology under `AgentProfile` (identity only), `Pipeline` (sole capability source; sourceless+channelless = baseline), `MCPToolset`; update the `mcpcompile/` and `dispatch/` map entries
- [ ] 5.5 Delete the tests describing behavior that no longer exists — raw-form merge, byte-identical profile ConfigMap, mode-dependent resolution — and replace them with the assertions above, deliberately per the repo rule on dispatch/compile fixtures
- [ ] 5.6 Full verification: `go build ./... && go vet ./...` in all modules, CRD regen clean, envtest suite with `KUBEBUILDER_ASSETS`, `helm lint` + template matrix
- [ ] 5.7 Reconcile the active changes named in the proposal: `ha-bundle` (profile carries `configRefs` + `allowedTools`), `all-in-one-crd` (assumes `ToolingBinding.mode`)
