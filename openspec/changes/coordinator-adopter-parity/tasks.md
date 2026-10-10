## 1. Coordinator guide, changelog and security catch-up

- [x] 1.1 Rewrite `docs/guides/coordinate-agents.md`'s worked examples
      (`log-analyzer`, `remediator`, `home-desk`) off trigger-style
      descriptions onto the purpose shape: what the agent IS, what it CAN
      do, what it CANNOT, what to HAND it. Verify by reading the result
      against PR #301's actual shipped bundle descriptions
      (`chart/charts/*/values.yaml`) for the same shape.
      DONE: all three worked examples (`log-analyzer`, `remediator`,
      `home-desk`) rewritten to the IS/CAN/CANNOT/HAND shape, matching the
      shipped `k8s-observe`/`k8s-operate`/`ha-control`/`ha-ops`/
      `alert-investigator` descriptions in `chart/charts/*/values.yaml`.
- [x] 1.2 Correct the guide's surrounding prose (currently: "a description
      the coordinating agent reads to decide when to use it", "decide
      which member answers which task") from dispatcher framing to the
      orchestrator framing the shipped prompt
      (`chart/templates/coordinator.yaml`) actually uses. Verify by
      re-reading the whole guide section for any remaining "matches what
      arrived" / "decide when to use" phrasing.
      DONE: both phrases reworded in "The overall shape". The
      `agents[]` bullet now states the IS/CAN/CANNOT/HAND shape and names
      the coordinating agent's tools AS the `agents[]` list. The no-
      description-refused note now says "match" an instruction or signal
      rather than "decide which member answers". Re-read confirms no
      remaining "matches what arrived" / "decide when to use" phrasing.
- [x] 1.3 Add the missing `docs/CHANGELOG.md` entry for #301 (orchestrator
      prompt reframe, description `maxLength` 512→2048, three bundles'
      rewritten capability descriptions) to the next chart version
      heading. Verify with `python3 .github/scripts/docs-generate.py
      --check` naming no stale version number.
      DONE: added to `## [Unreleased]`'s `### Changed` section (no new
      version is cut by this docs-only change, so the entry waits under
      Unreleased for the next chart release). `docs-generate.py --check` reports "52
      generated file(s) up to date".
- [x] 1.4 Disclose in `docs/security.md`'s "Agent-invoked agents" section
      that a coordinated member's ask-before-acting consent boundary is
      currently prompt-only (`agentops.memberScopeInstruction`,
      `chart/templates/_helpers.tpl`), not a mechanical gate at
      `/coordinate/invoke` — naming the #296 incident history as why this
      residual risk is stated rather than assumed solid. Verify by
      confirming `platform/manager/internal/httpapi/coordinate.go` and
      `platform/manager/internal/chat/coordinate.go` still hold no
      consent-check code, so the disclosure stays true as written.
      DONE: added a row to the "Residual risk" table (the section this
      kind of disclosure already lives in, alongside "Depth in a
      coordination"). Confirmed `coordinate.go`'s handlers check only the
      `agents[]` list, budgets and the cycle guard — no task-authorization
      check exists anywhere in `httpapi/coordinate.go` or
      `chat/coordinate.go`, so the disclosure holds.

## 2. Elevate Coordinator in the adopter narrative — frontend-developer

- [x] 2.1 Add the one clause to README.md's "How it works" step 3 naming
      `Coordinator` as the other route, and align the "Compose agents,
      another seam" bullet's wording with the orchestrator framing from
      task 1.2. Verify `wc -l README.md` stays within the file's stated
      budget (`.claude/rules/documentation.md`, README section).
      DONE: `wc -l README.md` → 211 (budget 215). The diagram's `alt`
      text also rewritten, since the diagram itself now shows both
      routes.
- [x] 2.2 Redraw `docs/diagrams/readme-flow.py` to branch into Pipeline
      and Coordinator after "you declare it", PORTING the composition from
      `prototypes/readme-diagram-mockup.html` — the branch shape, the
      member fan-out, the result-returns loop, and escalate as its own
      conditional path — per design.md's "What the prototype settles, and
      what it does not". Run the script by hand and commit the
      regenerated `assets/img/readme-flow-{light,dark}.svg`. Verify by
      opening both SVGs and confirming no connector stops short of its
      box (the exact defect class the prototype was debugged against).
      DONE: second full-width row added below the (verbatim) Pipeline
      row — hub-and-spoke fan-out to three member chips, a dashed
      "a result returns first" loop, and a dashed escalate path rising
      to the channel card from underneath. Opened both regenerated SVGs
      (light/dark): every arrowhead touches its target box edge. Also
      caught and fixed a caption left over from before the redraw —
      the Pipeline card still said "the only place wiring lives",
      which the new row directly underneath it would have contradicted
      — now "the wiring, for one agent".
- [x] 2.3 Rework `index.md`'s `.ao-presentation` list and
      `assets/js/presentation.js` to carry Coordinator as a second story,
      PORTING the composition from
      `prototypes/landing-presentation-mockup.html`. Carry forward: the
      tab order (Orchestrator first, Pipeline second), the hub-and-branch
      shape, the per-kind shapes mirrored from
      `platform/console/ui/src/graph/shapes.tsx`, and the diamond chosen
      deliberately for the Coordinator hub. Verify by building the site
      per `docs/CLAUDE.md`'s "Build it and LOOK" step and confirming the
      Pipeline tab still renders unchanged.
      DONE: built with the native Jekyll gem (no docker in this
      environment), screenshotted both tabs in light and dark with
      Playwright. Pipeline tab verified BEHAVIORALLY unchanged (same
      geometry, same story) rather than byte-identical — the shape
      system (rect/circle/diamond/cylinder/hexagon) now applies to
      BOTH stories uniformly, per design.md's own requirement that the
      drawing be "built from the site's own tokens and the console's
      per-kind shapes". Applying it to Coordinator alone would have
      left two inconsistent visual vocabularies on one page. Confirmed
      no horizontal overflow in either theme, reduced-motion renders
      paused with the existing "press to play" contract, and
      `node --check` passes on `presentation.js`.

## 3. Review introduction.md and getting-started.md

- [x] 3.1 Read both pages end to end for the same Pipeline-only framing
      found in README.md and index.md. Verify by stating, in this task's
      own completion note, either what was found and fixed or that
      nothing needed changing and why.
      FOUND AND FIXED in `introduction.md`: the Pipeline concept card
      claimed "the wiring, and the only object that carries any" and "there
      is nowhere else to look" — stale since `Coordinator` shipped as the
      second wiring kind, and contradicting the page's own "Two wiring
      kinds share one capability shape" paragraph a few lines below it.
      Reworded to "the wiring for one agent" with a pointer to Coordinator.
      `getting-started.md`: NOTHING NEEDED CHANGING. It already states
      `wiringMode=pipelines` explicitly with a callout explaining why (demo
      mode's new `coordinator` default would leave the console unanswered),
      and links to `coordinate-agents.md` to try `coordinator` mode instead
      — this was already corrected by `coordinator-deployment-mode`'s own
      task 9b.1.

## 4. Close coordinator-deployment-mode's remaining verification — testing-specialist

- [ ] 4.1 Run task 5.1 from
      `openspec/changes/coordinator-deployment-mode/tasks.md` (smoke test
      against the live install, from this working copy's chart, not
      master's) and tick it there once the verdict is recorded.
      NOT PERFORMED HERE: a hand-exploratory smoke against a live install
      (hand-trigger the hourly cron path, watch a self-close prompt
      resolve in-turn) needs a live cluster and a browser
      (`visual-check.md`, `remote-session.md`), which this remote session
      does not have and cannot dispatch as a CI run — unlike 4.2, this one
      is not an automated test. Left open for whoever runs the
      workstation-only verification.
- [ ] 4.2 Run task 8.1 from the same file (the already-written e2e
      coordinator lane, `go test -tags e2e ./test/e2e/` from this working
      copy's `platform/manager/`, against a cluster built from this
      working copy's chart) and tick it there once the verdict is
      recorded.
      DISPATCHED, VERDICT PENDING: this remote session has no docker
      daemon and no k3d, so the pack cannot run directly here. First
      dispatched `e2e-smoke.yml` (run 38072406832) — the coordinator
      tests (`TestCoordinatorModeInvokeAndMemberResultRouting` and
      siblings) all logged "full tier only (E2E_TIER=full)" and were
      SKIPPED under the smoke tier, correcting the assumption they used
      the stub runtime and ran under smoke. That run also failed on two
      unrelated, known-flaky lanes
      (`TestConsolePlainConversationBulkCloseAndDelete`,
      `TestReplicasThreeDeliversEveryConsoleThreadInParallel` — the
      second has a documented flakiness history in `gotchas.md`). Not
      this change's doing, since it touches no code — master's last
      scheduled full run (38042912111, same day) passed both.
      Dispatched `e2e-full.yml` (run 38074304800) to actually exercise
      the coordinator lane. Per `implement-issue.md` step 6, not waited
      on here. Both runs are linked in the pull request.
- [x] 4.3 Resolve task 9b.5 from the same file: confirm whether the
      conditional screenshot/demo re-check is actually needed (expected
      no-op, since that change touched no console code) and tick it with
      the finding recorded, rather than left open or silently skipped.
      DONE: confirmed genuinely no-op, not merely unverified — `git show
      --stat --name-only` on every commit `coordinator-deployment-mode`
      and its two follow-up fixes shipped under (#291, #296, #301, and the
      `signal-cron` pin fix) touches zero files under `platform/console/`.
      Ticked 9b.5 there with this finding.

## 5. Unit tests

- [x] 5.1 Run `python3 .claude/scripts/rules_compliance.py` over every
      page this change touched (README.md, docs/guides/coordinate-agents.md,
      docs/CHANGELOG.md, docs/security.md, index.md, introduction.md,
      getting-started.md) and confirm silent output.
      DONE: every finding on every touched file is pre-existing, confirmed
      by diffing against `origin/master`'s own copy of each file (same
      line content, shifted line numbers). No new finding from this
      change's own prose.
- [x] 5.2 Run `python3 .github/scripts/publication-guard.py` and `python3
      .github/scripts/retired-vocabulary-guard.py` over the full tree and
      confirm both pass — the prototypes under `prototypes/` included.
      DONE: `publication-guard: clean`, `retired-vocabulary guard: clean
      (127 files)`.
- [x] 5.3 Run `python3 .github/scripts/docs-generate.py --check` and
      confirm clean (no CRD, chart value, or doc-comment change in this
      change, so this should already be a no-op, confirmed rather than
      assumed).
      DONE: "52 generated file(s) up to date" — confirmed no-op, as
      expected.

## 6. E2E tests

- [x] 6.1 Not applicable. Nothing in this change is decided by a
      cluster beyond the coordinator-deployment-mode verification tracked
      in section 4 (4.2 is still open, its verdict pending), which belongs
      to that change's own e2e task (8.1) and is run, not re-specified,
      here.

## 7. Documentation

### Reference docs

- [x] 7.1 Re-read `docs/guides/coordinate-agents.md`,
      `docs/CHANGELOG.md`, and `docs/security.md` as finished pages (not
      diffs) and confirm each reads correctly on its own, now that every
      other task in this change has landed.
      DONE: all three read correctly as finished pages, each self-
      contained with no reference to this change's own process.

### Adopter site

- [x] 7.2 Build the docs site per `docs/CLAUDE.md`'s pre-flight steps
      (Jekyll build, serve, look at README.md's GitHub-rendered form and
      every page this change touched in both themes) and fix anything
      the look catches that an earlier task did not.
      DONE: built with the native Jekyll gem, served, screenshotted
      `index`, `introduction`, `getting-started` and
      `guides/coordinate-agents` in light and dark — no horizontal
      overflow on any page (`scrollWidth > clientWidth` false
      throughout), no visual regression, both presentation tabs
      (Orchestrator and Pipeline) render cleanly with consistent
      styling. No additional defect found beyond the stale caption
      already caught and fixed under task 2.2.
- [x] 7.3 Confirm no other adopter-facing page (console-guide.md,
      installation.md, any integration page) makes a claim this change
      contradicts — e.g. still describing Pipeline as the only way to
      wire an agent. Record the finding, even if it is "none found".
      FOUND AND FIXED, in a reference page beyond the proposal's own
      list: `docs/concepts.md`'s `### Pipeline` section called itself
      "the only place either is declared" for capabilities/execution, and
      separately claimed "Wiring lives ONLY here" for source-claiming —
      both false since `Coordinator` ships its own inline capability and
      execution fields and claims sources identically (confirmed against
      `CoordinatorSpec` in `coordinator_types.go`). Both corrected, with a
      pointer to the `### Coordinator` section below. No exclusivity claim
      found in `console-guide.md`, `installation.md`, any
      `integrations/*.md` or `runtimes/*.md` page (grepped for "only" /
      "exclusively" near "pipeline" — none).
