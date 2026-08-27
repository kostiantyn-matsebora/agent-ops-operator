## 1. The scheduled scan gates

- [x] 1.1 `.github/workflows/image-scan.yml`: drop `gate: "false"`, and rewrite
      the header — it reports AND fails on a fixable CRITICAL/HIGH, after the
      upload, and its status is what the README badge shows.
- [x] 1.2 `.github/actions/trivy-scan/action.yml`: the `gate` input's
      description stops naming the scheduled scan as the caller passing false.
- [ ] 1.3 Verify from the WORKTREE's tree: `gh workflow run image-scan.yml
      --ref change/scheduled-scan-gates` cannot run (the workflow file differs
      from master's), so verify by reading: the action's step order is upload
      `if: always()` then gate, and the caller passes no `gate`. After merge,
      dispatch it once on master and confirm the run is green with every image
      scanned.

## 2. The badges

- [x] 2.1 `README.md`: three badge lines after the existing three — chart
      version (`img.shields.io/github/v/tag/...?filter=chart-v*&label=chart`,
      linking the changelog), `ci` workflow status, `image-scan` workflow
      status labelled `published images` (linking the workflow's runs).
- [x] 2.2 `wc -l README.md` stays within the 215-line budget.
- [x] 2.3 Check the rendered badges on the forge once the branch is pushed
      (a shields filter that matches nothing renders "no tags").

## 3. Documentation

### 3.1 Reference docs

- [x] 3.1.1 `CONTRIBUTING.md`: the paragraph on the weekly scan — it fails on a
      fixable finding after reporting, and the fix is a re-release.
- [x] 3.1.2 `openspec validate scheduled-scan-gates --strict`.

### 3.2 Adopter site

- [x] 3.2.1 `docs/security.md`: "Only the pull-request scan blocks" is
      rewritten — both scans fail on a fixable finding; one blocks a merge,
      the other turns the badge red until the image is re-released.
- [x] 3.2.2 The landing page, introduction, getting started, installation and
      the guides: confirmed they do not describe the scan (grep), nothing owed.
