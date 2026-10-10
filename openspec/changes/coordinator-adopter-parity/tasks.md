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
      DONE: added to `## [14.0.0]`'s `### Changed` section (the version
      #301 actually shipped under, confirmed via `git log` — no new
      version is cut by this docs-only change, so there is no later
      heading to add it to). `docs-generate.py --check` reports "52
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

- [ ] 2.1 Add the one clause to README.md's "How it works" step 3 naming
      `Coordinator` as the other route, and align the "Compose agents,
      another seam" bullet's wording with the orchestrator framing from
      task 1.2. Verify `wc -l README.md` stays within the file's stated
      budget (`.claude/rules/documentation.md`, README section).
- [ ] 2.2 Redraw `docs/diagrams/readme-flow.py` to branch into Pipeline
      and Coordinator after "you declare it", PORTING the composition from
      `prototypes/readme-diagram-mockup.html` — the branch shape, the
      member fan-out, the result-returns loop, and escalate as its own
      conditional path — per design.md's "What the prototype settles, and
      what it does not". Run the script by hand and commit the
      regenerated `assets/img/readme-flow-{light,dark}.svg`. Verify by
      opening both SVGs and confirming no connector stops short of its
      box (the exact defect class the prototype was debugged against).
- [ ] 2.3 Rework `index.md`'s `.ao-presentation` list and
      `assets/js/presentation.js` to carry Coordinator as a second story,
      PORTING the composition from
      `prototypes/landing-presentation-mockup.html`. Carry forward: the
      tab order (Orchestrator first, Pipeline second), the hub-and-branch
      shape, the per-kind shapes mirrored from
      `platform/console/ui/src/graph/shapes.tsx`, and the diamond chosen
      deliberately for the Coordinator hub. Verify by building the site
      per `docs/CLAUDE.md`'s "Build it and LOOK" step and confirming the
      Pipeline tab still renders unchanged.

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
- [ ] 4.2 Run task 8.1 from the same file (the already-written e2e
      coordinator lane, `go test -tags e2e ./test/e2e/` from this working
      copy's `platform/manager/`, against a cluster built from this
      working copy's chart) and tick it there once the verdict is
      recorded.
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

- [ ] 5.1 Run `python3 .claude/scripts/rules_compliance.py` over every
      page this change touched (README.md, docs/guides/coordinate-agents.md,
      docs/CHANGELOG.md, docs/security.md, index.md, introduction.md,
      getting-started.md) and confirm silent output.
- [ ] 5.2 Run `python3 .github/scripts/publication-guard.py` and `python3
      .github/scripts/retired-vocabulary-guard.py` over the full tree and
      confirm both pass — the prototypes under `prototypes/` included.
- [ ] 5.3 Run `python3 .github/scripts/docs-generate.py --check` and
      confirm clean (no CRD, chart value, or doc-comment change in this
      change, so this should already be a no-op, confirmed rather than
      assumed).

## 6. E2E tests

- [ ] 6.1 Not applicable. Nothing in this change is decided by a
      cluster beyond the coordinator-deployment-mode verification already
      covered in section 4, which belongs to that change's own e2e task
      (8.1) and is run, not re-specified, here.

## 7. Documentation

### Reference docs

- [ ] 7.1 Re-read `docs/guides/coordinate-agents.md`,
      `docs/CHANGELOG.md`, and `docs/security.md` as finished pages (not
      diffs) and confirm each reads correctly on its own, now that every
      other task in this change has landed.

### Adopter site

- [ ] 7.2 Build the docs site per `docs/CLAUDE.md`'s pre-flight steps
      (Jekyll build, serve, look at README.md's GitHub-rendered form and
      every page this change touched in both themes) and fix anything
      the look catches that an earlier task did not.
- [ ] 7.3 Confirm no other adopter-facing page (console-guide.md,
      installation.md, any integration page) makes a claim this change
      contradicts — e.g. still describing Pipeline as the only way to
      wire an agent. Record the finding, even if it is "none found".
