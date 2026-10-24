<!--
Three questions, and they are the review. Delete nothing — an empty section is
an answer ("none of those"), a missing one is a round trip.

New here? CONTRIBUTING.md states the workflow, which is unusual enough that it
cannot be inferred from the tree.
-->

## What changed

<!-- In the terms the commit convention uses: what it does, and why that
mattered. Not which files were touched — the diff says that. -->

Refs #

<!-- A CHANGE'S TRACKING ISSUE CLOSES AT ARCHIVE, NOT AT MERGE. `Closes #` here
     hands it to GitHub to close the moment this pull request lands — so a
     PROPOSAL closes the issue that was meant to follow the change through
     applying, review and archiving. Write `Refs #<n>`.

     `Closes #<n>` is right in exactly two cases: this pull request ARCHIVES the
     change (the issue's life really does end with it), or the issue is an
     ordinary filed one that this work finishes and no change tracks. -->

## What it affects

<!-- Tick what this touches. Anything ticked in the first three rows usually
wants an openspec change behind it; say which, or say why not. -->

- [ ] **What a CRD means** — a field's semantics, a condition, how something resolves
- [ ] **A contract** — the work contract, an adapter contract, an HTTP endpoint
- [ ] **What the chart grants** — RBAC, a default that widens reach, a new workload
- [ ] **Behaviour an adopter would notice**, without any of the above
- [ ] **None of those** — a fix, a test, a refactor, a typo

**Breaking?**

- [ ] No.
- [ ] Yes — and `docs/CHANGELOG.md` carries the upgrade entry, newest first, and the commit subject has `!` before the colon.

<!-- THE OPENSPEC CHANGE IS NOT ASKED FOR. It is read from the branch, which is
named `change/<name>` — anything derivable is derived, and a field repeating the
branch is a field that can disagree with it. A pull request from a branch with no
`change/` prefix is work with no openspec change behind it, which the table above
already covers. -->

## Documentation

<!-- THIS IS NOT A FOLLOW-UP. A change is finished when every document it made
untrue is updated in this pull request. Both halves are skipped independently,
so both are listed. -->

- [ ] **The reference docs** — `docs/concepts.md`, `docs/contracts.md`, a bundle page, `docs/CHANGELOG.md`
- [ ] **The adopter site** — the landing page, Introduction, Getting started, Installation, `docs/guides/`
- [ ] **Generated output re-run**, if a CRD field, an api doc comment or a chart value a guide shows changed: `python3 .github/scripts/docs-generate.py`
- [ ] **Console assets re-run**, if the UI changed: `npm run screenshots` and `npm run demo` in `platform/console/ui`
- [ ] **Nothing this change touched is documented anywhere.** <!-- rare; say which reader is unaffected and why -->

## Verification

<!-- What you ran, and what you looked at. A green CI run is necessary and not
sufficient: a rendered chart is not a running one. If the change is visible in
the console, say that you looked at it. -->

- [ ] `go build ./... && go vet ./... && go test ./...` in every module it touches
- [ ] The envtest suite in `platform/manager`, if manager behaviour changed
- [ ] `helm lint` / `helm template` for the permutations it affects
- [ ] `python3 .github/scripts/publication-guard.py` is clean <!-- record the VERDICT, never the text it matched -->
- [ ] Exercised against a real cluster <!-- say what you saw -->
- [ ] **Verified against THIS branch's tree**, not master's <!-- the build container and the GitOps deploy both resolve a fixed path by default: a run without the override tests master and reports success -->
