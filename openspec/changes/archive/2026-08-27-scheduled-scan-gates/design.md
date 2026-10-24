## Context

`.github/actions/trivy-scan/action.yml` already runs two scans per image: a
SARIF run that never fails and is uploaded `if: always()`, then a gate run
that exits 1 on a fixable CRITICAL/HIGH. `image-scan.yml` calls it with
`gate: "false"`, which is the whole of why its status is always green. Design
D4 of `trivy-image-scanning` chose that on the argument that a scheduled run
has no change to reject and a red one "pages somebody about a fact rather than
a regression". The first scheduled run showed the other half of that argument:
a fact nobody is paged about is a fact nobody acts on.

## Goals / Non-Goals

**Goals:**

- A scheduled run is red exactly when a published image carries a fixable
  CRITICAL/HIGH finding, and the security tab is current either way.
- A README badge on that workflow reads as a claim about today's images, and
  is true.

**Non-Goals:**

- Changing the threshold, the exceptions file or the pull-request gate.
- Auto-remediation. A red run names the image; rebuilding it is the release
  procedure in `build-test.md`, and the three-tags-per-push rule applies.
- Notifications beyond the workflow's own failure e-mail and the badge.

## Decisions

**Drop `gate: "false"` rather than add a third mode.** The action already
orders upload before gate, so the scheduled run gets the same pair as a pull
request: report, then fail. A separate "fail after report" input would be the
same behaviour under a second name. The input itself stays — a fork's pull
request still needs `upload: false` with the gate on, and `gate: false` is
kept for a caller that has a reason, with its description no longer naming
the scheduled scan as that caller.

**`fail-fast: false` stays.** Every image is scanned even after one fails,
so a red run's log names every affected image in one place.

**Badges are shields.io, three lines.** The chart version reads the newest
`chart-v*` tag with shields' `filter` parameter, so it needs no file to
update. The two workflow badges use GitHub's own `actions/workflows/<file>/badge.svg`
on `master`, so they need no token and no third party. The scan badge is
labelled `published images` so it reads as what it measures, not as the
pull-request gate.

**The spec's requirement is rewritten, not appended.** "SHALL NOT gate" is
replaced, and the scenario "no build is failed by it" becomes "the run fails
naming the image". Keeping both would be a spec that contradicts itself.

## Risks / Trade-offs

- **A disclosure now turns the badge red until someone re-releases.** That is
  the point, and the cost is a red badge on the forge for the interval. The
  interval is a tag push per affected component, three at a time.
- **An unfixable finding cannot redden it** — `ignore-unfixed: true` is on
  both runs, unchanged. The badge means "nothing actionable", which is the
  same meaning the pull-request gate already has.
- **A rate-limited database pull would fail the run for a reason unrelated to
  the images.** Unchanged risk: the run already restores the day's cached
  database and tolerates yesterday's.
