## 1. Chart values and template — deployment-engineer

- [x] 1.1 Add `coordinator.agents: []` to `chart/values.yaml`, with a
      comment stating the empty/set behavior (proposal.md - What Changes),
      and verify `helm lint chart/` passes.
- [x] 1.2 In `chart/templates/coordinator.yaml`, right after the `$contrib`
      read, add the render-time shape guard: for each `coordinator.agents`
      entry, `fail` naming the entry if it carries both `capability` and
      `coordinator`, or neither (design.md - Decisions, "Validate shape at
      render time"). Verify with `helm template chart/ --set
      'coordinator.agents[0].name=x'` (missing both refs) failing with that
      entry's name in the error.
- [x] 1.3 In the same file, reassign `$agents` to `$c.agents` when it is
      non-empty, before the reaper fold-in block (design.md - Decisions,
      "Override point sits above the reaper fold-in"). Verify
      `helm template chart/ --set global.agentops.wiringMode=coordinator`
      with `coordinator.agents` unset still renders byte-identical output
      to the pre-change template (diff against a snapshot taken before
      1.1–1.4).
- [x] 1.4 Update the final `agents:` render block to emit `coordinatorRef`
      when an entry carries `.coordinator`, and `capabilityRef` otherwise
      (design.md - Decisions, the switch snippet). Verify
      `helm template chart/ --set global.agentops.wiringMode=coordinator
      --set coordinator.agents[0].name=x --set
      coordinator.agents[0].description=y --set
      coordinator.agents[0].coordinator=z` renders a `coordinatorRef: {name:
      z}` entry and no `capabilityRef`.

## 2. Unit tests

- [x] 2.1 In `platform/manager/internal/integration/charttemplate_test.go`,
      add a render test asserting `coordinator.agents` empty (the default)
      renders the `Coordinator`'s `agents:` block byte-identical to the
      current contributions-derived output. `go test ./...` in
      `platform/manager` passes.
- [x] 2.2 Add a render test asserting a set `coordinator.agents` list
      replaces the derived roster entirely, with one `capability` entry
      mapped to `capabilityRef` and one `coordinator` entry mapped to
      `coordinatorRef`. Same module, `go test ./...` passes.
- [x] 2.3 Add a render test asserting the reaper fold-in still appends its
      `agents[]` entry and `signalSourceRefs` claim on top of a set
      `coordinator.agents` override, with `reaper.enabled=true`. Same
      module, `go test ./...` passes.
- [x] 2.4 Add a render test asserting a `coordinator.agents` entry naming
      both `capability` and `coordinator`, and one naming neither, each
      fail `helm template` naming that entry. Same module, `go test
      ./...` passes.

## 3. System tests

- [x] 3.1 Not applicable — this change is confined to `helm template`
      rendering of chart values into a `Coordinator` manifest. No kubelet,
      RBAC, informer or pod lifecycle decision is touched (docs/testing.md).

## 4. Documentation

### Reference docs

- [x] 4.1 Add `coordinator.agents` to `docs/configuration.md`'s "Wiring
      posture: pipelines or coordinator" table (reversible chart value,
      per the root `CLAUDE.md` documentation-ownership table), stating the
      empty/set behavior and the replace-not-merge rule.
- [x] 4.2 Run `python3 .github/scripts/docs-generate.py --check` and commit
      any regenerated output (the chart-value change requires this run even
      though no CRD field changed).

### Adopter site

- [x] 4.3 Confirm `docs/getting-started.md`'s coordinator-mode walkthrough
      needs no change — it demonstrates the derived roster, which stays
      the default — and record that check in the PR description rather
      than editing the page.
