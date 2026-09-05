## Context

See proposal.md — Why. What shapes the approach:

- **The gate is `agentops`, already provisioned and default** by
  `sonar-provision.sh`'s `--gate` stage (`coverage-across-packages`, #139). It
  carries the built-in `Sonar way`'s new-code conditions plus overall coverage
  ≥ 80%. Extending it is a `wanted=` list edit in the same script, not a new
  mechanism.
- **The organisation is on the Clean Code taxonomy (MQR mode)**: an issue
  carries `impacts[]`, each `{softwareQuality, severity}`, and
  `severity ∈ {BLOCKER, HIGH, MEDIUM, LOW, INFO}` per quality — reliability,
  security, maintainability. `issues/search` filters on this with
  `impactSeverities=`. The RETIRED five-level `severity` field
  (BLOCKER/CRITICAL/MAJOR/MINOR/INFO) is the legacy taxonomy this is not; a
  provisioning or enumeration call built against the wrong one returns either
  nothing or the wrong set, silently.
- **The overall rating metrics are `reliability_rating`,
  `security_rating`, `sqale_rating`** (maintainability keeps its historical
  SQALE key), each 1.0–5.0 where 1=A … 5=E. `coverage LT 80` is the
  precedent for the condition shape: `<metric> GT 2` fails a project rated
  worse than B, mirroring the coverage condition's own `LT` on the numeric
  scale.
- **`sonar-issues.py` reads only a pull request's OWN issues**
  (`issues/search?componentKeys=...&pullRequest=<n>`), because it feeds the
  autofix loop's work list for a diff already under review. Nothing today
  reads a component's BRANCH-WIDE backlog — the set this change has to fix.
- **The finding count per component is unknown until it is read.** `SONAR_TOKEN`
  is a write-only repository secret; no session holds it, exactly as
  `coverage-across-packages`' task 1.1 could not read the dashboard's coverage
  numbers. This shapes the task breakdown below rather than being an aside.

## Goals / Non-Goals

**Goals:**

- Every component's project reaches at least a B overall reliability,
  security and maintainability rating, in code — not by relaxing a threshold.
- "Passes the gate" grows the same three ratings as conditions, provisioned
  the same repeatable, idempotent way coverage's condition was.
- The before-and-after is recorded as counts per component per rating, same
  discipline as coverage-across-packages.

**Non-Goals:**

- Medium/Low/Info findings. B is the target the ask states; reaching A closes
  every finding regardless of severity, which is a larger and separately
  justified change.
- New-code rating conditions. The built-in `Sonar way` already holds those;
  this only adds OVERALL conditions.
- Automating the fix via the existing autofix loop. That loop reacts to a
  pull request's own diff (`review-dispatch.yml`'s `collect` step scopes
  `sonar-issues.py` to the PR); the backlog here predates every open pull
  request, so nothing triggers it. A one-time sweep, not a standing mechanism.

## Decisions

### D1. The enumeration is a new, narrow script, not a `sonar-issues.py` mode

`sonar-issues.py`'s whole contract is "one pull request's issues, for the
autofix loop." Adding a branch-wide mode would give it two return shapes keyed
on whether `--pr` is passed, and the loop's own callers would need to prove
they never hit the new one. A second script,
`.github/scripts/sonar-findings-baseline.py`, shares nothing but the `sq`-style
fetch helper and reads `issues/search` with `impactSeverities=BLOCKER,HIGH`
and no `pullRequest`, across every component — read-only, exactly as
`sonar-issues.py` is, and run once by hand, not by CI.

- Alternative considered: extend `sonar-issues.py` with `--branch`. Rejected —
  the autofix loop's own tests would need a case proving the new flag is never
  reachable from `land-dispatch.py`, for a script whose only caller today is a
  workflow that always passes `--pr`.

### D2. The gate's rating conditions are added AFTER the fixes land, not before

`coverage-across-packages` shipped its condition first, deliberately red,
because the follow-up work was explicitly out of scope for that change (D3
there: "the change that requires it comes after the coverage changes that make
it green"). This change IS the fix — there is no follow-up to hand a
permanently-red gate to. Landing the condition first would fail every other
component's next unrelated pull request on a backlog this change is already
committed to clearing in the same branch.

- The condition still lands in THIS change, in the same commit shape
  coverage's did — it is sequenced last in the task list, not deferred to
  another proposal.
- Once landed, a rating that regresses below B on a later pull request is
  caught going forward, which is the mechanism's whole point.

### D3. The task list has an enumeration task, then a fix task PER COMPONENT WITH FINDINGS — sized once the enumeration exists

Task 1.1 in `coverage-across-packages` recorded a table it could not fully
populate without the org's token, and stated so rather than inventing numbers.
This change cannot even name which components need fixing yet. Tasks are
therefore split:

1. Run `sonar-findings-baseline.py` (OPEN, needs the token — the org owner's
   step, same as coverage's 1.1 and 2.2).
2. Its output — counts per component per rating, never the finding text or
   the org identifier (`publication.md`) — is pasted into this file's task
   1.1, exactly as coverage's table was.
3. A fix task is added per component the output names, during `/opsx:apply` or
   a follow-up `/opsx:update` once the list exists — this design does not
   invent them now, because a task written against a guess would be rewritten
   the moment the real list arrives, which is worse than not writing it.

This is not an Open Question deferred past this design: it is the reason the
design is a two-phase task list instead of a flat one, stated here rather than
smuggled into task 1.1's prose alone.

## Risks / Trade-offs

- [The finding count is unknown, so the size of this change is unknown] →
  the two-phase task list above; a component with many findings may warrant
  splitting into its own change rather than growing this one, decided once the
  count is read.
- [Assuming Clean Code taxonomy metric keys and `impactSeverities` when the
  org might still be on the legacy severity scale] → the enumeration script's
  first call is `GET issues/search` for one component with both filters tried;
  a 0-result `impactSeverities` query against a project known to carry issues
  fails naming the taxonomy mismatch rather than reporting a clean backlog.
- [A rating condition failing on a component this change does not reach —
  e.g. one with zero Blocker/High findings today but a borderline Medium
  backlog that trips B on a later push] → out of scope per the Goals above;
  accepted on the SAME grounds D2 already accepts for coverage: the
  condition can start a component red the moment it is provisioned, and
  that component's own next pull request is what turns it green, one at a
  time.

  CORRECTED after archiving, on a review finding against PR #169: this
  entry originally said the gate's verdict "stays informational... visible
  rather than blocking" — false. One project has ONE gate; `sonar-scan`'s
  `sonar.qualitygate.wait=true` fails the step (and so `ci-green`) on that
  gate's combined ERROR verdict, whichever condition failed it, ratings
  included. The mitigation this risk actually rests on is D2's "deliberately
  red is the expected state, not a regression" — not non-blocking.
- [Fixing many findings across many components in one branch is a large,
  hard-to-review diff] → components are committed and, where the pull request
  grows unwieldy, split by component into separate follow-up pull requests
  against this same change's branch, decided once the enumeration is read.

## Open Questions

None beyond what D3 already scopes into the task list.
