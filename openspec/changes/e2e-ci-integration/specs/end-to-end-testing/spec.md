## ADDED Requirements

### Requirement: The full tier drives the console through a real browser

The full tier SHALL run `platform/console/ui/e2e-live/` (the real-browser,
real-backend Playwright suite) against the cluster and the console
Deployment the same pack run already stood up, after the pack's own
assertions pass.

This tier SHALL NOT gate a pull request, on the same footing as the
real-runtime lane: it runs only where `end-to-end-testing`'s existing
nightly-when-moved and on-demand rule already admits the full tier.

The pack SHALL supply this suite's required fixtures — a reachable console
URL, a `kubectl` context, and the namespace the pack deployed into — rather
than the suite arranging its own cluster, which stays true to
`global-setup.ts`'s existing contract of pointing at an already-running
install.

#### Scenario: The live browser suite runs inside the nightly full pack

- **WHEN** the full tier runs and the pack's own cluster-based assertions
  pass
- **THEN** `e2e-live` runs against that same cluster's console, and the
  tier fails if any spec fails

#### Scenario: A pull request never provisions this tier

- **WHEN** a pull request runs, whatever files it touches
- **THEN** no job runs `e2e-live`, and no cluster is provisioned for it

#### Scenario: The pack's own failure skips the browser pass

- **WHEN** the pack fails before the console Deployment becomes reachable
- **THEN** `e2e-live` does not run, and the failure is reported as the
  pack's, not as a browser-suite failure
