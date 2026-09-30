## 1. `AgentCapability` rendering per bundle — deployment-engineer

- [ ] 1.1 `chart/charts/kubernetes/`: add the `coordinator`-mode rendering
      branch for the `k8s-engineer` route per `k8s-bundle`'s delta —
      `AgentCapability` at the observing privilege level, no `Pipeline` in
      this mode. Verify with `helm template` under
      `global.agentops.wiringMode: coordinator`.
- [ ] 1.2 `chart/charts/prometheus/`: add the `coordinator`-mode branch for
      the `alert-investigator` route per `prometheus-bundle`'s delta — one
      `AgentCapability`, no `Pipeline`. Verify with `helm template`.
- [ ] 1.3 `chart/charts/home-assistant/`: add the `coordinator`-mode branch
      for BOTH routes per `ha-bundle`'s delta — two `AgentCapability`
      objects (`ha-user`, `ha-operator`), never merged, no `Pipeline`.
      Verify with `helm template` that both privilege levels stay separate.
- [ ] 1.4 Every bundle's `pipelines`-mode branch is untouched — verify with
      a render diff (`helm template` before and after this change, with
      `wiringMode` unset) showing zero output difference.

## 2. Chart-level Coordinator, reaper, and wiringMode — deployment-engineer

- [ ] 2.1 `chart/values.yaml`: add `global.agentops.wiringMode`, default
      `pipelines`, validated against exactly `pipelines` \| `coordinator` in
      the values schema or a render-time guard. Verify an unrecognized
      value fails the render naming both accepted values.
- [ ] 2.2 `chart/templates/`: render exactly one `Coordinator` object under
      `coordinator` mode, its `signalSourceRefs` listing every enabled
      bundle's source (including the hourly `signals/cron` claim, task 2.4)
      and its `agents[]` listing every enabled bundle's `AgentCapability`
      entries from section 1. Nothing renders under `pipelines` mode.
- [ ] 2.3 Add the reaper's `AgentProfile` and `AgentCapability` templates,
      gated by `coordinator` mode alone (not a separate flag) — no domain
      toolset or MCPConfig bound beyond the coordination toolset. Add the
      reaper as one more `agents[]` entry on the chart-rendered Coordinator.
- [ ] 2.4 Claim `signals/cron` hourly on the chart-rendered Coordinator's
      `signalSourceRefs` (design D-E) — no per-entry trigger field, no new
      CRD field.
- [ ] 2.5 Write the Coordinator's own coordinating-agent prompt (template
      under `platform/manager/internal/dispatch/templates/` or the
      chart-rendered `AgentProfile`'s prompt, per whichever the existing
      `Coordinator` inline-capability convention uses) to recognise an
      hourly cron signal and respond with `invoke(reaper, ...)`. Verify by
      reading the rendered prompt in a `helm template` dry run.
- [ ] 2.6 Verify `pipelines` mode stays byte-identical: `helm template` with
      `wiringMode` unset vs. explicitly `pipelines`, diff is empty, across
      every bundle enabled.

## 3. Domain agent self-close prompt — deployment-engineer

- [ ] 3.1 `platform/manager/internal/dispatch/templates/`: add the
      self-close instruction to the shared/base prompt template (or each
      bundle's profile prompt, per whichever the existing per-bundle prompt
      convention uses) — an agent may `/close` its own conversation once it
      judges the problem resolved. Verify by reading the rendered prompt.

## 4. Coordinator-owner reach on `platform/mcp-aops` and the manager — backend-developer

- [ ] 4.1 `platform/manager/internal/` (wherever `/coordinate/*` handlers
      live): implement the `coordinatorRef` resolution walk from design
      D-A — read the caller's own `coordinatorRef`, and if empty, walk
      `causedBy` to the uncaused root and read that root's `coordinatorRef`.
      Reuse the existing cycle-guard traversal rather than a new one. Unit
      test three cases: a plain member (no `coordinatorRef`) resolves via
      its ancestor root, a Coordinator's own root resolves via its own
      field, and a Pipeline-addressed conversation resolves to nothing.
- [ ] 4.2 Implement `list_open_roots()`: list uncaused conversations whose
      `coordinatorRef` matches the caller's resolved Coordinator, excluding
      the caller's own ancestor root (design D-B). Unit test each exclusion
      scenario from `coordinator-owner-reach`'s spec: member excluded,
      closed root excluded, cross-Coordinator root excluded, own-ancestor
      root excluded.
- [ ] 4.3 Widen `close`'s bound per design D-B: caller itself, a
      conversation it directly caused, OR — when resolved to a Coordinator
      — any open uncaused root of that Coordinator other than its own
      ancestor. Unit test every scenario in `aops-mcp-server`'s delta
      (sibling-root close permitted, cross-Coordinator refused, member
      refused, Pipeline-addressed caller keeps the narrow bound).
- [ ] 4.4 `platform/mcp-aops/`: expose `list_open_roots` as a new MCP tool,
      forwarding the caller's existing `coordinator:<name>:<conversation>`
      token unchanged — no new token derivation, no new context string
      (design D-D). Verify the tool is listed and callable in the server's
      existing conformance-style test, if one exists for the other verbs.
- [ ] 4.5 Update the `/coordinate/*` HTTP surface's bound table in code to
      match `aops-mcp-server`'s delta spec exactly.

## 5. Smoke against a live install — testing-specialist

- [ ] 5.1 Smoke from the worktree chart: coordinator mode with one bundle
      enabled, a signal opens a root, the coordinating agent invokes the
      bundle's domain capability, a self-close prompt resolves an incident
      in-turn, and the hourly cron path (triggered by hand rather than
      waiting an hour) invokes the reaper, which lists open roots, re-checks
      one via `invoke`, and closes it. Record the verdict, not the
      transcript.

## 6. Rules and vocabulary

- [ ] 6.1 `.claude/rules/gotchas.md`: extend the bundle-qualification table
      with the coordinator-mode column per bundle (mirrors
      `coordinated-agents`' own rule-file updates for the primitive this
      change wires by default).
- [ ] 6.2 `.claude/rules/wiring.md`: note that `global.agentops.wiringMode`
      is a chart-rendering choice only, never a CRD-level exclusivity — the
      many-to-many invariant is unchanged.

## 7. Unit tests

- [ ] 7.1 Every module builds, vets and tests in the container, from the
      worktree path (`platform/manager`, `platform/mcp-aops` included).
- [ ] 7.2 `KUBEBUILDER_ASSETS` envtest suite green, covering the
      `coordinatorRef`-resolution walk and the widened `close` bound.
- [ ] 7.3 `helm template` under `global.agentops.wiringMode: coordinator`
      and `: pipelines` (and unset), every bundle combination the existing
      permutation matrix already covers, plus the byte-identical diff from
      task 2.6. `serviceaccount-guard.py` passes on all of them.
- [ ] 7.4 `python3 .github/scripts/publication-guard.py` and
      `retired-vocabulary-guard.py` pass — record the verdict only, never
      the matched text.

## 8. E2E tests

- [ ] 8.1 Add or extend a coordination lane in
      `platform/manager/test/e2e/` covering: coordinator mode end to end
      (source claim, invoke, member-result routing), the reaper's hourly
      survey against a live cluster (kubelet-backed conversation lifecycle,
      not just unit-level routing), and the ancestor-root exclusion against
      real conversation objects — the one behavior a real API server's
      `causedBy` chain must be walked against, not a fake. Run the pack and
      record the verdict.

## 9. Documentation — THE LAST TASK, and it is not optional

### 9a. Reference docs

- [ ] 9a.1 `docs/concepts.md`: `global.agentops.wiringMode`, the reaper's
      role, the `coordinatorRef`-resolution walk, the widened `close`
      bound, the new `list_open_roots` verb.
- [ ] 9a.2 `docs/contracts.md`: the Coordinator-owner reach class added to
      the `/coordinate/*` verb table, alongside the existing coordinator
      and channel-reader classes.
- [ ] 9a.3 `docs/configuration.md`: `global.agentops.wiringMode` value
      documented, both accepted values, default.
- [ ] 9a.4 `.github/scripts/docs-generate.py` re-run, then commit every
      regenerated block and `docs/cr-reference.md`.

### 9b. Adopter site

- [ ] 9b.1 `docs/getting-started.md`: if demo mode's default posture
      changes to `coordinator`, update the walkthrough's description of
      what gets deployed.
- [ ] 9b.2 `docs/installation.md`: the reaper's cron
      requirement (uses the existing `signals/cron` component — no new
      component to install).
- [ ] 9b.3 `docs/guides/coordinate-agents.md` (from `coordinated-agents`):
      add the deployment-mode and self-heal sections — regenerate any CR
      blocks the guide's generated markers cover.
- [ ] 9b.4 `README.md`: update the "what you write" tab if the demo's
      default shape changes, staying within its line budget.
- [ ] 9b.5 `platform/console/ui`: re-run BOTH `npm run screenshots` and
      `npm run demo` if the reaper's conversations need distinct treatment
      in the tree view, then commit the assets.
