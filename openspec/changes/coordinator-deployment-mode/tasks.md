## 1. `AgentCapability` rendering per bundle — deployment-engineer

- [x] 1.1 `chart/charts/kubernetes/`: add the `coordinator`-mode rendering
      branch per `k8s-bundle`'s delta — the observing `AgentCapability`
      (`k8s-observe`) always, and the acting one (`k8s-operate`) only when
      `pipelines.admin.enabled`, never merged, no `Pipeline` in this mode.
      Verify with `helm template` under
      `global.agentops.wiringMode: coordinator`, flag off and on.
- [x] 1.2 `chart/charts/prometheus/`: add the `coordinator`-mode branch for
      the `alert-investigator` route per `prometheus-bundle`'s delta — one
      `AgentCapability`, no `Pipeline`. Verify with `helm template`.
- [x] 1.3 `chart/charts/home-assistant/`: add the `coordinator`-mode branch
      for BOTH routes per `ha-bundle`'s delta — two `AgentCapability`
      objects (`ha-control`, `ha-ops`), never merged, no `Pipeline`.
      Verify with `helm template` that both privilege levels stay separate.
- [x] 1.4 Every bundle's `pipelines`-mode branch is untouched — verify with
      a render diff (`helm template` before and after this change, with
      `wiringMode` unset) showing zero output difference.

## 2. Chart-level Coordinator, reaper, and wiringMode — deployment-engineer

- [x] 2.1 `chart/values.yaml`: add `global.agentops.wiringMode`, default
      `pipelines`, validated against exactly `pipelines` \| `coordinator` in
      the values schema or a render-time guard. Verify an unrecognized
      value fails the render naming both accepted values.
- [x] 2.2 `chart/templates/`: render exactly one `Coordinator` object under
      `coordinator` mode, its `signalSourceRefs` listing every enabled
      bundle's source (including the hourly `signals/cron` claim, task 2.4)
      and its `agents[]` listing every enabled bundle's `AgentCapability`
      entries from section 1. Nothing renders under `pipelines` mode.
- [x] 2.3 Add the reaper's `AgentProfile` and `AgentCapability` templates,
      gated by `coordinator` mode alone (not a separate flag) — no domain
      toolset or MCPConfig bound beyond the coordination toolset. Add the
      reaper as one more `agents[]` entry on the chart-rendered Coordinator.
- [x] 2.4 Claim `signals/cron` hourly on the chart-rendered Coordinator's
      `signalSourceRefs` (design D-E) — no per-entry trigger field, no new
      CRD field.
- [x] 2.5 Write the Coordinator's own coordinating-agent prompt (template
      under `platform/manager/internal/dispatch/templates/` or the
      chart-rendered `AgentProfile`'s prompt, per whichever the existing
      `Coordinator` inline-capability convention uses) to recognise an
      hourly cron signal and respond with `invoke(reaper, ...)`. Verify by
      reading the rendered prompt in a `helm template` dry run.
- [x] 2.6 Verify `pipelines` mode stays byte-identical: `helm template` with
      `wiringMode` unset vs. explicitly `pipelines`, diff is empty, across
      every bundle enabled.

## 3. Domain agent self-close prompt — deployment-engineer

- [x] 3.1 `platform/manager/internal/dispatch/templates/`: add the
      self-close instruction to the shared/base prompt template (or each
      bundle's profile prompt, per whichever the existing per-bundle prompt
      convention uses) — an agent may `/close` its own conversation once it
      judges the problem resolved. Verify by reading the rendered prompt.
      DONE ON THE CHART SIDE ONLY: `agentops.selfCloseInstruction` /
      `agentops.withSelfClose` in `chart/templates/_helpers.tpl`, appended
      in every bundle profile template. The shared dispatch template under
      `platform/manager/internal/dispatch/templates/` was left untouched
      (out of this role's lane) — see the hand-back report for whether a
      backend-side instruction is also needed for non-chart-authored
      profiles.

## 4. Coordinator-owner reach on `platform/mcp-aops` and the manager — backend-developer

- [x] 4.1 `platform/manager/internal/` (wherever `/coordinate/*` handlers
      live): implement the `coordinatorRef` resolution walk from design
      D-A — read the caller's own `coordinatorRef`, and if empty, walk
      `causedBy` to the uncaused root and read that root's `coordinatorRef`.
      Reuse the existing cycle-guard traversal rather than a new one. Unit
      test three cases: a plain member (no `coordinatorRef`) resolves via
      its ancestor root, a Coordinator's own root resolves via its own
      field, and a Pipeline-addressed conversation resolves to nothing.
- [x] 4.2 Implement `list_open_roots()`: list uncaused conversations whose
      `coordinatorRef` matches the caller's resolved Coordinator, excluding
      the caller's own root (its ancestor root when a member, itself when
      it is a root) (design D-B). Populate `members` with the `agents[]`
      entry names of each root's direct members. Unit test each exclusion
      scenario from `coordinator-owner-reach`'s spec: member excluded,
      closed root excluded, cross-Coordinator root excluded, own root
      excluded, and the `members` projection.
- [x] 4.3 Widen `close`'s bound per design D-B: caller itself, a
      conversation it directly caused, OR — when resolved to a Coordinator
      — any open uncaused root of that Coordinator other than its own
      ancestor. Unit test every scenario in `aops-mcp-server`'s delta
      (sibling-root close permitted, cross-Coordinator refused, member
      refused, Pipeline-addressed caller keeps the narrow bound).
- [x] 4.4 `platform/mcp-aops/`: expose `list_open_roots` as a new MCP tool,
      forwarding the caller's existing `coordinator:<name>:<conversation>`
      token unchanged — no new token derivation, no new context string
      (design D-D). Verify the tool is listed and callable in the server's
      existing conformance-style test, if one exists for the other verbs.
- [x] 4.5 Update the `/coordinate/*` HTTP surface's bound table in code to
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

- [x] 6.1 `.claude/rules/gotchas.md`: extend the bundle-qualification table
      with the coordinator-mode column per bundle (mirrors
      `coordinated-agents`' own rule-file updates for the primitive this
      change wires by default).
- [x] 6.2 `.claude/rules/wiring.md`: note that `global.agentops.wiringMode`
      is a chart-rendering choice only, never a CRD-level exclusivity — the
      many-to-many invariant is unchanged.

## 7. Unit tests

- [x] 7.1 Every module builds, vets and tests in the container, from the
      worktree path (`platform/manager`, `platform/mcp-aops` included).
      Verified: `go build ./... && go vet ./... && go test ./...` green in
      every module `.github/components.sh modules` lists. Two unrelated,
      pre-existing failures confirmed present on `origin/master` too (not
      caused by this change): `platform/console` (`ui.go` embeds
      `ui/dist`, which is build output nobody ran `npm run build` for in
      this environment) and `platform/context-sync`
      (`TestCheckpointReportsFailureWhenScanErrors` /
      `...StoreCheckpointErrors`, untouched by this change).
- [x] 7.2 `KUBEBUILDER_ASSETS` envtest suite green, covering the
      `coordinatorRef`-resolution walk and the widened `close` bound.
      `platform/manager/internal/integration` (envtest) is green, including
      the two new `coordinator_owner_reach_test.go` cases.
- [x] 7.3 `helm template` under `global.agentops.wiringMode: coordinator`
      and `: pipelines` (and unset), every bundle combination the existing
      permutation matrix already covers, plus the byte-identical diff from
      task 2.6. `serviceaccount-guard.py` passes on all of them.
      The deployment-engineer's hand-run `helm template` matrix (every
      bundle on/off, both modes, unset, the byte-identical diff) and
      `serviceaccount-guard.py` all pass. The five
      `charttemplate_test.go` tests that pinned the old
      demo-always-pipelines default (`TestDemoModeWiresTheObservingRoute`,
      `TestAllowMutationsPromotesTheRouteToActing`,
      `TestExplicitRouteValuesBeatTheDerivation`,
      `TestBothRoutesRenderWithoutConflict`,
      `TestWiringNamesOnlyWhatWasRendered`) are now pinned to
      `--set global.agentops.wiringMode=pipelines`, and
      `TestK8sProfileStatesTheWithheldPosture` /
      `TestK8sProfilePostureCanBeDeclined` updated for the self-close
      instruction task 3 appends. Full suite green.
- [x] 7.4 `python3 .github/scripts/publication-guard.py` and
      `retired-vocabulary-guard.py` pass — record the verdict only, never
      the matched text.
      Both clean over the whole tree: `publication-guard: clean`,
      `retired-vocabulary guard: clean (122 files)`.

## 8. E2E tests

- [ ] 8.1 Add or extend a coordination lane in
      `platform/manager/test/e2e/` covering: coordinator mode end to end
      (source claim, invoke, member-result routing), the reaper's hourly
      survey against a live cluster (kubelet-backed conversation lifecycle,
      not just unit-level routing), and the ancestor-root exclusion against
      real conversation objects — the one behavior a real API server's
      `causedBy` chain must be walked against, not a fake. Run the pack and
      record the verdict.
      `test/e2e/coordinator.go` + `test/e2e/coordinator_test.go` added:
      `TestCoordinatorModeInvokeAndMemberResultRouting` (source claim,
      invoke, member-result routing, over hand-authored
      Coordinator/AgentCapability CRs — legitimate per `wiring-mode`'s own
      "an operator may hand-write a Coordinator" scenario, since rendering
      one from the chart is already pinned at the envtest tier) and
      `TestCoordinatorSelfHealSurveyExcludesAncestorRoot` (real cron-adapter
      pod fires an admitted signal every minute standing in for the hourly
      schedule — the same substitution `TestCronLane` already makes —
      reaper invoked as a real member, `list_open_roots` asserted against
      REAL conversation objects over the LIVE deployed manager, the reaper's
      own re-check `invoke` on the domain capability, ancestor-root
      exclusion asserted, widened `close` asserted both ways). Both compile
      under `-tags e2e` (`go test -tags e2e -c -o /dev/null ./test/e2e/`),
      `go vet ./...` and the untagged `go test ./...` stay green.
      A gap surfaced while writing the second lane and was fixed, not
      merely noted: `coordinator-self-heal` requires the reaper (a plain
      `causedBy` member) to `invoke` a domain capability to re-check it, but
      `handleCoordinateInvoke`'s `callerConversation`
      (`platform/manager/internal/httpapi/coordinate.go`) required the
      caller to carry `spec.coordinatorRef` DIRECTLY — unlike
      `handleCoordinateClose` / `handleCoordinateOpenRoots`, which already
      resolved through `callerActingForCoordinator`
      (`ResolveActingCoordinator`, design D-A). `handleCoordinateInvoke` now
      calls `callerActingForCoordinator` too, and `chat.Router.InvokeMember`
      resolves the Coordinator it searches `agents[]` on via
      `ResolveActingCoordinator` instead of reading
      `caller.Spec.CoordinatorRef` directly — the same walk `close` and
      `list_open_roots` already use. `docs/contracts.md`'s `invoke` bound
      row and refusal table updated to match.
      NOT RUN against a live cluster: this session has no docker daemon and
      no `k3d` — workstation/CI-only per `remote-session.md`. Left
      UNTICKED for that reason alone — the lane is written, compiles, and
      (per the fix above) should pass end to end once actually run against
      a cluster, but "should pass" is not a verdict this session can claim.

## 9. Documentation — THE LAST TASK, and it is not optional

### 9a. Reference docs

- [x] 9a.1 `docs/concepts.md`: `global.agentops.wiringMode`, the reaper's
      role, the `coordinatorRef`-resolution walk, the widened `close`
      bound, the new `list_open_roots` verb.
      DONE: a bullet on `### Coordinator` plus three new subsections at the
      end of `## Coordinated agents` — Deployment posture, The self-heal
      reaper, Coordinator-owner reach — linking to `contracts.md` for the
      full verb/bound table rather than restating it.
- [x] 9a.2 `docs/contracts.md`: the Coordinator-owner reach class added to
      the `/coordinate/*` verb table, alongside the existing coordinator
      and channel-reader classes.
      VERIFIED, no change needed: the backend engineer's commit (3287c00)
      already carries this in full — the three reach classes, the widened
      `close` bound, `list_open_roots` in the tool list and the verb
      section. Checked against the landed code in
      `platform/manager/internal/chat/coordinate.go` and
      `platform/mcp-aops/tools.go` — matches.
- [x] 9a.3 `docs/configuration.md`: `global.agentops.wiringMode` value
      documented, both accepted values, default.
      DONE: new `### Wiring posture: pipelines or coordinator` section
      after `### The runtime`, as a REFERENCE-page table (no front matter,
      no component markup) — value, default, the Coordination side effect,
      reversibility, and a link to the guide.
- [x] 9a.4 `.github/scripts/docs-generate.py` re-run, then commit every
      regenerated block and `docs/cr-reference.md`.
      DONE. `--check` first caught a REAL regression this change
      introduced: preset `tier1` (`global.demo.enabled=true`) now renders
      coordinator mode by default, so `guides/pipeline.md`'s
      `Pipeline/k8s-observe` worked example no longer existed to generate
      from. Fixed by pinning `global.agentops.wiringMode: pipelines` on
      that preset (it is a guide about the Pipeline kind, not about demo
      mode's own default posture) and adding the literal to
      `DOCUMENTED_PLACEHOLDERS`. Regenerating then picked up one real
      change already landed but never regenerated:
      `guides/agent-profile.md`'s rendered `k8s-engineer` prompt now carries
      the self-close instruction (task 3). `docs/cr-reference.md` is
      unchanged — the `Coordinator`/`AgentCapability` CRDs are untouched by
      this change. `--check` is clean.

### 9b. Adopter site

- [x] 9b.1 `docs/getting-started.md`: demo mode now selects `coordinator`
      posture, so update the walkthrough's description of
      what gets deployed.
      DONE, honestly: the install command now pins
      `--set global.agentops.wiringMode=pipelines` explicitly, with a
      callout stating PLAINLY that demo mode alone now defaults to
      `coordinator` posture, that nothing yet auto-claims the console's
      source and channel for a chart-rendered `Coordinator` the way
      `pipelines` mode does for a `Pipeline`, and that leaving the new
      default in place would leave this walkthrough's console with no
      route answering it. Chose to keep this page's own promise — ending at
      something working — over silently landing on a broken default, and
      linked `guides/coordinate-agents.md` for trying `coordinator` mode
      instead.
- [x] 9b.2 `docs/installation.md`: the reaper's cron
      requirement (uses the existing `signals/cron` component — no new
      component to install).
      DONE: one paragraph under "Enable a bundle" — `coordinator` mode also
      renders Coordination regardless of `coordination.enabled`, and the
      reaper's hourly trigger is the chart's own `signals/cron`
      `SignalAdapter`, deployed automatically — no bundle to enable, no new
      component.
- [x] 9b.3 `docs/guides/coordinate-agents.md` (from `coordinated-agents`):
      add the deployment-mode and self-heal sections — regenerate any CR
      blocks the guide's generated markers cover.
      DONE: two new sections before "What comes next" — "Let the chart wire
      this for you" (wiringMode, what each mode renders, the hand-written
      `Pipeline`/`Coordinator` coexistence, and a callout stating the
      console auto-wiring gap plainly, including that even a hand-claimed
      source would not reach the console as a thread since a Coordinator's
      `channelRefs` are escalation-only) and "Self-heal: the hourly reaper"
      (the survey/re-check/close flow, the self-close instruction, and the
      Coordinator-owner reach class). No generated marker in this guide
      needed a field-list change — `docs-generate.py --check` is clean.
- [x] 9b.4 `README.md`: update the "what you write" tab if the demo's
      default shape changes, staying within its line budget.
      VERIFIED, no change made: "What you write" shows a generic hand-wired
      `Pipeline` example unrelated to demo mode's own default shape, and
      "Try it" / "What agent-ops is" make no claim this change breaks — the
      Kubernetes-events half of the demo (no console involved) still works
      under `coordinator` mode, since the chart-rendered Coordinator claims
      that source too. `wc -l README.md`: 210 before and after.
- [x] 9b.5 `platform/console/ui`: re-run BOTH `npm run screenshots` and
      `npm run demo` if the reaper's conversations need distinct treatment
      in the tree view, then commit the assets.
      CONFIRMED NO-OP (coordinator-adopter-parity task 4.3): the condition
      never triggers. `git show --stat --name-only` on every commit this
      change and its two follow-up fixes shipped under (#291, #296, #301,
      plus the `signal-cron` pin fix) touches zero files under
      `platform/console/`. The reaper's conversations are ordinary
      Coordinator members with no console-visible field of their own, so
      there is nothing for a screenshot or demo re-run to pick up — not
      merely unverified for lack of a live cluster, but genuinely nothing
      to run.
