> Also fixes a live regression in `master` (`81688e2`): both bundles' routing
> Pipelines declare no capabilities, so every signal-driven conversation
> dispatches an empty allowlist. Task 3.1 is the assertion whose absence let it
> ship.
>
> Capabilities are declared, never inferred. A Pipeline that grants nothing is a
> valid configuration — no fallback, and no warning about it.

## 1. Remove the baseline

- [x] 1.1 `internal/chat/pipelines.go`: delete `CapabilityPipelineForProfile` and `IsCapabilityPipeline`
- [x] 1.2 `internal/controller/pipeline_controller.go`: delete `ConditionBaselineConflict`, `baselineConflicts`, and their condition wiring
- [x] 1.3 `internal/chat/router.go`: the `/<profile>` command path stops resolving a baseline (it passes no origin, so its conversations carry the Pipeline's capabilities or none)
- [x] 1.4 Delete the baseline tests in `internal/integration/capability_test.go` that describe the removed concept, deliberately per the repo rule on dispatch/compile fixtures

## 2. Task API addresses a Pipeline

- [x] 2.1 `internal/httpapi/server.go`: `handleTask` takes `{"pipeline","task","agent"?,"title"?}`; drop the `profile` field; resolve profile, channelRefs, toolsets, and mcpConfigs from the named Pipeline; 400 when `pipeline` is missing, 404 when unknown
- [x] 2.2 Update the package doc comment on `internal/httpapi` describing `POST /task`
- [x] 2.3 Integration tests: a task against a Pipeline carries its profile, channels, and capabilities; a missing `pipeline` is 400; an unknown one is 404

## 3. Bundles declare what they grant

- [x] 3.1 The test that would have caught the regression: an event routed through the k8s-bundle's own Pipeline shape (as the chart emits it) dispatches a NON-empty allowlist
- [x] 3.2 k8s-bundle: replace the baseline template with an addressable Pipeline for the shipped agent (values-gated, demo-on), and make the `cluster-events` Pipeline declare the built-in toolsets; drop `profile.baseline.*` values
- [x] 3.3 vm-bundle: the default-source Pipeline declares the bundle's toolset + MCPConfigs when those components are active
- [x] 3.4 `helm template` matrix: every rendered Pipeline declares capabilities; the addressable Pipeline renders in demo mode and disappears behind its flag; no baseline object remains

## 4. Docs and verification

- [x] 4.1 README: the demo curl names a Pipeline; the capabilities section drops the baseline and states that capabilities are declared per route and never inferred
- [x] 4.2 `chart/templates/NOTES.txt`: the ask-an-agent line names a Pipeline; drop the baseline hint
- [x] 4.3 CLAUDE.md: Pipeline terminology — addressable, sole capability source, no default; remove the baseline entry
- [x] 4.4 `config/samples/samples.yaml`: replace the baseline Pipeline with an addressable one
- [x] 4.5 Reconcile `chat-signal-origination`: it notes capability-only Pipelines as an interaction — record that they are gone and that chat addressing is its to define
- [x] 4.6 Full verification: `go build ./... && go vet ./...`, envtest suite with `KUBEBUILDER_ASSETS`, `helm lint` + template matrix, CRD-schema validation of rendered CRs
