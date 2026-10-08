## 1. CRD schema and reconciler enforcement — backend-developer

- [ ] 1.1 Add `spec.paused` (bool, default `false`, mutable) to
      `SignalSourceSpec`, `PipelineSpec` and `CoordinatorSpec`
      (`platform/manager/api/v1alpha1/`). Add the new `MaintenanceMode` kind
      (`maintenancemode_types.go`): `spec.paused` only, `status.conditions`
      holding `Honored` and `Paused`. Regenerate deepcopy and CRD YAML
      (`controller-gen object` / `crd`, per `build-test.md`). Verify:
      `go build ./... && go vet ./...` in `platform/manager`, and the new
      `MaintenanceMode` CRD YAML lands in `chart/crds/`.
- [ ] 1.2 `signalsource_controller.go`, `pipeline_controller.go`,
      `coordinator_controller.go`: set the `Paused` condition
      (`True`/`Paused`, `False`/`NotPaused`) from `spec.paused`, as a
      sibling of the existing `Ready`/`Served`/`Wired` computation — never
      folded into it. Verify with a reconciler unit test per kind asserting
      `Paused` flips independently of `Ready`.
- [ ] 1.3 New `maintenancemode_controller.go`: honor exactly the object
      named `default` in the manager's own namespace. Set `Honored`
      (`True`/`Singleton`, `False`/`NotSingleton`, naming the honored one)
      and `Paused` (mirroring `spec.paused` only on the honored object,
      `False`/`NotHonored` on any other). Verify with a unit test covering
      one object, two objects, and zero objects.
- [ ] 1.4 `signalsource_controller.go`: add `Wired=False`/`OnlyPausedClaimants`
      alongside the existing `NoPipelineClaim`, once task 1.5's fan-out
      exclusion is in place. Verify with a test: a source claimed only by a
      paused Pipeline reports this new reason, not `NoPipelineClaim`.
- [ ] 1.5 `internal/chat/pipelines.go`: `readyPipelines`/`readyCoordinators`
      additionally exclude `Paused=True` objects, so `PipelinesForSource`
      and every caller are correct for free. Verify with a table test
      covering Ready+unpaused (included), Ready+paused (excluded),
      NotReady+unpaused (excluded, unchanged).
- [ ] 1.6 `internal/chat/router.go`: `resolveClaimant`/`HandleCommand`
      refuses a direct `/<pipeline> <task>` or `/<coordinator> <task>`
      command against a `Paused=True` object, naming it in the reply,
      before any conversation is created. `Ready` is still not checked
      here (unchanged). Verify with a router test addressing a paused
      Pipeline and asserting no conversation is created and the reply
      names the pause.
- [ ] 1.7 `internal/httpapi/signals.go`: at `/signal/inbound`, check the
      honored `MaintenanceMode` FIRST (drop every signal regardless of
      source, reason names maintenance mode), then the named
      `SignalSource.spec.paused` SECOND (reason names the source's own
      pause), before the existing `Wired` check. Both drops respond
      success-but-not-queued, matching the existing unwired-drop shape.
      Verify with a test per ordering: maintenance active beats a paused
      source's own state, and a paused source still drops once maintenance
      clears.
- [ ] 1.8 `internal/controller/conversation_controller.go`: add
      `ReasonMaintenanceMode`, checked alongside the existing storage
      breaker, holding an unadmitted conversation `Pending` via the
      existing `enterPendingFor` with no pod, no MCP ConfigMap and no
      `ensure-topic` while the honored `MaintenanceMode` is active. A
      conversation already pod-backed when maintenance mode activates is
      untouched. Verify with a controller test: a new conversation stays
      `Pending` while active, an already-running one keeps its pod, and
      `Pending` conversations are picked up by the existing FIFO promotion
      once maintenance mode clears.

## 2. The `/admin/*` surface, `aops-mcp-server` and the console backend — backend-developer

- [ ] 2.1 New `internal/httpapi/admin.go`: the six verbs from
      `operational-pause-controls`
      (`pause-signal-source`/`resume-signal-source`/`pause-pipeline`/`resume-pipeline`/`set-maintenance-mode`/`clear-maintenance-mode`),
      each a `client.Patch` (`MergeFrom`, re-read first) of exactly one
      `spec.paused` field. `400` on a malformed body, `404` on an unknown
      target or (for the two maintenance verbs) no honored singleton —
      never creating one. Verify with a handler test per verb covering the
      success and both error paths.
- [ ] 2.2 Auth on `/admin/*`: accept the existing `anyAdapterAuth` class
      unconditionally (`internal/httpapi/server.go`), and a
      coordinator-rooted conversation token
      (`coordinator:<name>:<conversation>`, same derivation
      `/coordinate/*` already uses in `coordinate.go`) gated by a
      server-side check that the calling Conversation's OWN materialized
      `spec.toolsets`/`spec.mcpConfigs` — resolved the same way
      `dispatch.ResolveCapability`/`EffectiveAllowedTools` already compose
      a Pipeline's bound tools — actually contains the matching
      `mcp__aops__<verb>` pattern. `401` with neither identity, `403`
      naming the missing tool for an authenticated-but-unbound coordinator
      caller. Verify with a test proving a coordinator token whose bound
      toolset lacks the tool is refused even when its OWN claimed
      `--allowedTools` says otherwise (the client self-report must never
      be trusted).
- [ ] 2.3 `platform/mcp-aops/tools.go` + `client.go`: add
      `pause_signal_source`, `resume_signal_source`, `pause_pipeline`,
      `resume_pipeline`, `set_maintenance_mode`, `clear_maintenance_mode`,
      each forwarding the caller's `AOPS_MCP_TOKEN` to the matching
      `/admin/*` verb verbatim — same shape as every existing tool in that
      file. Verify with the existing `tools_coverage_test.go` pattern
      extended to cover all six.
- [ ] 2.4 `platform/console/manager.go`: add `Manager` client methods for
      the six verbs, calling the manager's `/admin/*` over the console's
      existing adapter-authenticated path (same shape as `Reopen`/`Delete`).
      `platform/console/api.go`: six new `POST /api/admin/*` routes, each
      wrapped in the existing `a.write(...)` gate (`writesAllowed()`), so
      an install with `writeEnabled: false` refuses these exactly as it
      refuses every other console write. Verify with an `api_test.go` case
      per verb, covering both `writeEnabled: true` and `false`.

## 3. Chart: `MaintenanceMode` instance, RBAC, values — deployment-engineer

- [ ] 3.1 `chart/templates/`: render exactly one `MaintenanceMode` object
      named `default`, `spec.paused` from a new
      `global.agentops.maintenanceMode.paused` value, default `false`.
      Verify with `helm template` showing exactly one object of this kind
      at every values permutation.
- [ ] 3.2 `chart/templates/rbac.yaml`: add `maintenancemodes` (and
      `maintenancemodes/status`) to the manager's existing resource lists —
      `signalsources`/`pipelines`/`coordinators` already carry full CRUD on
      spec, so no verb change is needed for `spec.paused` on those three.
      Verify by diffing the rendered Role before and after this task:
      `maintenancemodes` is the only resource addition.
- [ ] 3.3 `chart/crds/`: confirm the three existing CRDs' generated YAML
      (task 1.1's regen) renders `paused` in their OpenAPI schema and the
      new `maintenancemode.yaml` is present under `chart/crds/`. Verify
      with the chart's existing CRD-presence test (`structure.md`'s
      pattern) extended to the new kind.

## 4. Console UI: pause/resume and maintenance controls — frontend-developer

- [ ] 4.1 `platform/console/ui/src/pages/Config.tsx` (and its detail
      sub-view): add a pause/resume control on a `SignalSource`,
      `Pipeline` and `Coordinator`'s detail view, visible only when
      `writeEnabled` is true (the existing flag the page already reads for
      every other write affordance), calling the new `/api/admin/*`
      routes from task 2.4. State explicitly, in a code comment at the
      control, that this is a deliberate exception to the page's own
      stated read-only position (`Config.tsx`'s existing header comment)
      — pausing is operational state, not wiring.
- [ ] 4.2 Add an install-wide maintenance-mode switch (a new small
      component, or a header affordance visible console-wide), reading
      the honored `MaintenanceMode` singleton's `Paused` condition and
      calling `set-maintenance-mode`/`clear-maintenance-mode`. Verify by
      running the dev server against a port-forwarded console
      (`visual-check.md`) and screenshotting: the switch's on/off states,
      a pause/resume click on each of the three kinds succeeding, and the
      control hidden when `writeEnabled` is false.

## 5. Rules and vocabulary

- [ ] 5.1 `.claude/rules/invariants.md`: add the `/admin/*` line beside
      the existing "the only two writes... are POST /channel/inbound and
      POST /signal/inbound" — this change makes it three, with the same
      reasoning the proposal states (operational toggle, not a Kubernetes
      write, console-authenticated).
  - Verify: the sentence names all three surfaces, and `console-topology`'s
    delta spec is cross-referenced rather than restated.

## 6. Unit tests

- [ ] 6.1 `go build ./... && go vet ./... && go test ./...` green in every
      module `.github/components.sh modules` lists, from the worktree
      path — `platform/manager` and `platform/mcp-aops` at minimum touch
      this change, `platform/console` for the new routes.
- [ ] 6.2 `KUBEBUILDER_ASSETS` envtest suite
      (`platform/manager/internal/integration`) green, covering the new
      `MaintenanceMode` reconciler, the `Paused`-aware fan-out and the
      admission hold against a real API server.
- [ ] 6.3 `helm template` passes for every bundle permutation this repo's
      existing matrix already covers, with the new `MaintenanceMode`
      object and RBAC addition present in every one. The chart's render
      tests (`platform/manager/internal/integration/charttemplate_test.go`)
      extended with a case asserting exactly one `MaintenanceMode` renders.
- [ ] 6.4 `platform/console/ui`: `npm test` (or the project's vitest
      command) green, including new coverage for the pause/resume control
      and the maintenance switch's two states.
- [ ] 6.5 `python3 .github/scripts/publication-guard.py` and
      `retired-vocabulary-guard.py` pass over the whole tree — record the
      verdict only, never matched text.

## 7. E2E tests

- [ ] 7.1 Add a lane in `platform/manager/test/e2e/` covering: a
      `SignalSource` paused by `kubectl patch` drops a live signal (no
      conversation created), a `Pipeline` paused the same way is excluded
      from a shared source's fan-out while a sibling claimant still opens
      a conversation, and the install-wide `MaintenanceMode` singleton
      paused holds a new conversation `Pending` with no pod while an
      already-running conversation (started before the toggle) keeps
      running to completion — the one behavior only a real kubelet-backed
      pod lifecycle decides. Run the pack and record the verdict, not the
      transcript.
- [ ] 7.2 Extend the pack with one `/admin/*` case over the live deployed
      manager: the console's adapter token succeeds unconditionally, and a
      coordinator-rooted conversation's token is refused `403` when the
      tool is not in its bound toolset and succeeds when it is — proving
      the server-side cross-check against real dispatch-time binding, not
      a fake.

## 8. Documentation — THE LAST TASK, and it is not optional

### 8a. Reference docs

- [ ] 8a.1 `docs/concepts.md`: `spec.paused` on the three existing kinds'
      sections, the new `Paused` condition, and a new `### MaintenanceMode`
      section in "The kinds" — the singleton convention, the chart default,
      what it holds during ingest and admission.
- [ ] 8a.2 `docs/contracts.md`: a new `/admin/*` subsection (verb table,
      both identity classes, the bound-tool cross-check) alongside "The
      aops MCP server contract," and the six new tools in its tool list.
      Add the `/signal/inbound` drop-order note (maintenance, then source
      pause, then `Wired`) to the signal adapter contract section.
- [ ] 8a.3 `docs/console.md`: the new `/api/admin/*` routes, gated by the
      same `writeEnabled` flag as every other console write, and the RBAC
      addition for `MaintenanceMode` (still read-only for the console — it
      reaches `/admin/*` through the manager, never the Kubernetes API
      directly).
- [ ] 8a.4 `docs/cr-reference.md`: regenerate —
      `python3 .github/scripts/docs-generate.py` — for the new field on
      three kinds and the new `MaintenanceMode` kind. `--check` must pass.
- [ ] 8a.5 `docs/configuration.md`: `global.agentops.maintenanceMode.paused`
      value entry (reversible by `helm upgrade`, so configuration rather
      than installation).
- [ ] 8a.6 `CHANGELOG.md`: new entry, newest first.
- [ ] 8a.7 `.claude/rules/invariants.md`'s own edit (task 5.1) counted
      here too, since it documents a changed invariant.

### 8b. Adopter site

- [ ] 8b.1 `docs/console-guide.md`: what the pause/resume controls and the
      maintenance switch do, who can reach them (a logged-in,
      write-enabled console), and what an operator should expect while
      maintenance mode is active (queued work, nothing evicted).
- [ ] 8b.2 Re-run `npm run screenshots` and `npm run demo` in
      `platform/console/ui` — the UI changed, so both build outputs must
      be regenerated to match (`documentation.md`'s rule for any console
      UI change).
- [ ] 8b.3 Check `docs/guides/` for any generated-marker guide walking
      SignalSource, Pipeline or Coordinator creation — if one shows the
      full field list, regenerate it rather than hand-editing.
