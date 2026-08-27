## Context

See `proposal.md` — Why. What shapes the approach is the pattern being ported and
the two ways this repository differs from the one it comes from.

**The reference implementation** is `deployment-dashboard`'s
`.github/actions/trivy-scan/action.yml`: a composite action taking `image-ref`
and `category`, running `aquasecurity/trivy-action@v0.36.0` twice against one
locally-loaded image —

1. `format: sarif`, `exit-code: '0'` → `github/codeql-action/upload-sarif@v4` at
   `if: always()`
2. `format: table`, `exit-code: '1'` → the gate

both at `severity: CRITICAL,HIGH` and `ignore-unfixed: true`. Its caller builds
with `push: false`, `load: true` and a `:scan` tag, and grants
`security-events: write` on the job.

**Two differences from that repository decide the rest of this design:**

| | `deployment-dashboard` | here |
|---|---|---|
| shape | one workflow per service, category written out per service | **one matrix over fourteen components**, so the category must be derived |
| base images | uniform | **three tiers** — distroless (11 of 14), `debian:bookworm-slim` (egress-proxy, runtime-ollama), `node:22-bookworm-slim` plus an npm tree (runtime-claude) |
| what a run builds | every service | **only the components the pull request touched** (`continuous-integration`), so a pull-request scan is partial by design |

The second is why the gate's threshold is not a decision to make from taste. A
distroless image carries almost no OS package surface; the runtime image carries
Debian plus a vendor's npm dependency tree. Whatever number of findings that
produces, `ignore-unfixed` is what keeps the difference from turning into a
permanently red component.

## Goals / Non-Goals

**Goals:**

- Every image CI builds is scanned, with no per-component configuration.
- A fixable HIGH or CRITICAL cannot merge.
- Findings are visible per component, and the visibility survives a failed gate.
- The published images are covered between builds.

**Non-Goals:**

- **Designing a second scanning pattern.** This ports one that exists.
- **Scanning at release time.** D3.
- **Trivy's other scanners** — `config` (Dockerfile misconfiguration), `secret`,
  and licence scanning. Each is a defensible addition and each is its own
  decision about what may fail a build; bundling them here means the gate that
  gets disabled takes vulnerability scanning down with it.
- **Bumping this repository's other action versions.** The reference repo runs
  newer majors of `checkout`, `setup-buildx` and `build-push`; that is not this
  change's business.
- **Replacing Dependabot.** It proposes upgrades; this refuses vulnerable ones.
  They are complementary and neither implies the other.

## Decisions

### D1 — Port the composite action rather than inline the steps

`.github/actions/trivy-scan/action.yml`, taking `image-ref` and `category`,
matching the reference implementation step for step.

Inlining into `ci.yml` would work for one caller, and the scheduled workflow is
the second. Two copies of a two-run gate is how the reporting run and the gate
run come to disagree about severity — at which point the security tab and the
build are describing different things and neither says so.

**Alternative considered: one Trivy run with `exit-code: 1` and SARIF output.**
Rejected, and this is the reason the reference runs twice: a run that exits
non-zero may not produce a complete report, so the gate would suppress the
reporting it is supposed to accompany. Two runs cost little because the first
warms Trivy's vulnerability database for the second.

### D2 — The category is derived from the matrix, not written out

`trivy-${{ matrix.image.component }}`.

**Uploading under a shared category means each component's results replace the
previous component's.** The surface then shows one component's findings while
appearing to describe fourteen. Nothing fails: every upload succeeds, and the run
is green. This is the single highest-consequence detail in porting a
per-service pattern to a matrix, which is why the spec states it as a requirement
rather than leaving it to the implementation.

Deriving it also means a new component is covered without an edit, which is the
rule `components.sh` already establishes for the image set itself.

### D3 — Nothing is added to the release path

`build-image.yml` pushes and then asserts what it pushed. A scan added after the
push fails on an image that is already public, and this repository's recovery for
a bad release is a new patch version — so the failure would arrive after the only
moment it could have prevented anything.

Scanning before the push would mean building twice, or loading a multi-platform
result that `load:` cannot express.

**The pull-request scan already covers the same content**, and the gap it leaves
— a base image that changed between the pull request and the tag — is what D4
covers on a schedule.

### D4 — The scheduled scan is additive, reports only, and is separable

It goes beyond the ported pattern. It is included because it is the only
mechanism that reaches two cases:

1. **A CVE disclosed after an image was built**, where no pull request and no
   release is involved — the dominant one.
2. **A component no pull request touches.** CI builds only what changed, so a
   base-image finding in an untouched component is never seen by the
   pull-request scan, however long it stays vulnerable.

It reports and never gates: there is no change under review to reject, and a
failing scheduled run pages somebody about a fact rather than a regression.

**It is deliberately separable.** If the schedule proves noisy it can be dropped
without touching the gate, which is why it is its own workflow rather than a
second trigger on `ci.yml`.

### D5 — `ignore-unfixed` applies to the reporting run too, following the reference

The reference sets it on both runs, so an unfixable finding neither fails the
build nor appears in the security tab.

**Named because it is a real trade-off and the alternative is defensible.**
Reporting unfixed findings would make the security tab a complete inventory,
which is what an inventory is for. It would also fill it, for the three
Debian-based images, with entries nobody can act on and no way to distinguish a
new one arriving.

Following the reference keeps both repositories' surfaces meaning the same thing:
*what is actionable*. If a complete inventory is wanted later, the images already
carry an SBOM and that is the better instrument for it.

### D6 — `security-events: write` is granted on the job, not the workflow

`ci.yml` declares `permissions: contents: read` at workflow level. The scanning
job declares its own block; every other job keeps the read-only default.

**A pull request from a fork gets a read-only token regardless**, so
`upload-sarif` cannot write from one. The upload step must not turn an external
contribution red for a permission the contributor cannot be granted — the gate
run still protects the merge, which is the part that matters. Task 3.4 settles the
guard.

## Risks / Trade-offs

- **`runtime-claude` is noisy enough to make its job routinely red** → Task 1
  measures each tier before the gate is switched on, rather than discovering it
  on somebody's unrelated pull request. If the fixable count is non-trivial, the
  fixes land first and the gate is enabled last. `ignore-unfixed` is the
  structural mitigation; a seeded `.trivyignore` is the fallback, with expiries.

- **A fork's pull request fails on the SARIF upload** → D6, resolved by task 3.4.
  The failure mode to avoid is an external contributor seeing red for something
  they cannot fix or even diagnose.

- **Trivy's database download is a network dependency in every image job** →
  Up to fourteen matrix legs pulling the DB — a shared-Dockerfile edit rebuilds
  everything — is both slow and rate-limitable. Task 3.5 caches it. Worth
  stating that the gate now has an external dependency the build did not have.

- **The gate is only as current as Trivy's database** → True of any scanner, and
  the reason D4 exists: the schedule re-asks the same question against a newer
  database without anything having changed in the repository.

- **A finding is not a vulnerability in this context** → A CVE in a package
  present but unreachable still fails the gate. The exception mechanism is the
  answer, and it carries an expiry precisely so "unreachable today" is
  re-examined rather than assumed forever.

## Migration Plan

Not applicable — no runtime behaviour changes and nothing an adopter does.

**The one ordering that matters is task 1 before task 3.6.** Measure what each
tier reports, fix or except what is fixable, and enable the gate last. Enabling
first means the first red build belongs to whoever happened to open the next pull
request, on a failure that has nothing to do with their change.

**Rollback** is removing the scan step from the `images` job. The composite
action and the scheduled workflow can stay: neither gates anything on its own.
