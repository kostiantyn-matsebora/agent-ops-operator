# SDLC setup: GitHub Actions CI + multi-arch GHCR publishing for images and chart

## Why

The repository has **no `.github/` directory at all**. Every artifact this
project ships — **thirteen container images** and a Helm chart — is built by
hand from the `docker buildx` command lines pasted in `CLAUDE.md`, pushed to a
personal Docker Hub namespace (`kmatsebora/*`), and versioned by remembering to
bump a tag. The inventory has tripled since this was written and the release
procedure has not changed: it is still a sequence of hand-typed commands, one
per image, each of which must not be typed wrong. Nothing verifies that `go build`, `go vet`, `go test` (including the envtest
suite) or `helm lint` pass before a change lands, nothing runs the console UI's
own tests at all, and nothing enforces the project's own "never overwrite a
pushed tag" rule.

## What Changes

- **CI on every PR and push to `master`** (`.github/workflows/ci.yml`):
  build + vet + test the operator module against a real API server (envtest,
  Kubernetes 1.31.x per `CLAUDE.md`), build + vet + test each of the **eleven**
  dependency-free submodules, run the **console UI's** npm build and vitest
  suite, `helm lint`/`helm template` the chart across its meaningful value
  permutations (default, `global.demo.enabled`, each of the four bundles, and
  `console.enabled=false`) with schema validation of the rendered manifests,
  and build all thirteen images without pushing so a broken Dockerfile fails
  the PR.
- **Tag-driven publishing to GHCR** (`.github/workflows/release.yml` + a
  reusable `build-image.yml`): a tag of the form `<component>-v<semver>`
  (`manager-v0.38.2`, `console-v0.16.1`, `channel-telegram-v0.12.1`,
  `runtime-claude-v0.6.1`, …) publishes exactly that one component. Preserves
  today's independent per-component versioning, which is now thirteen version
  lines plus the chart.
- **Multi-arch images (`linux/amd64` + `linux/arm64`)** for ALL THIRTEEN,
  published as a single manifest list per tag, with **no emulation and no
  exception**. The twelve Go images cross-compile; `runtime-claude` was verified
  to build AND run on arm64 (`claude --version` reports 2.1.239 on `aarch64`),
  which retires this repo's belief that its upstream is single-arch. That image
  is amd64-only today because the hand-run build command says so.
- **The release workflow asserts what it pushed** against a per-component
  platform declaration, as equality. A silently single-arch image is a failure
  this project has already had in production (`agentops-console`, 2026-08-21),
  and nothing at build, push or render time reported it.
- **A publication hygiene guard in CI**, because this repository is going
  public. It is an ALLOWLIST of permitted shapes — reserved example domains,
  cluster-internal names, this repo's own clone URL, the documented placeholder
  identifiers — covering the whole tree AND the commit messages of the range
  under review. Never a denylist: a list of forbidden strings has to spell out
  what it protects, publishing it in the guard itself, and catches only what
  someone already thought of. It reports positions and rules, never the matched
  text, because a public repository has public build logs.
  **It lands before any cleanup change is written**, so that a change naming
  what it removes fails the build rather than being archived and republished.
- **Helm chart published as an OCI artifact** on a `chart-v<semver>` tag:
  `oci://ghcr.io/kostiantyn-matsebora/charts`, installable with
  `helm install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator --version <v>`.
- **Tag immutability is enforced in CI**: a publish job that finds its tag
  already present in the registry fails before building. The project rule
  becomes a gate instead of a note in `CLAUDE.md`.
- **Registry cut-over to GHCR — BREAKING for new installs.** All twelve
  first-party image references the chart renders repoint from `kmatsebora/*` to
  `ghcr.io/kostiantyn-matsebora/agentops-*` — in `chart/values.yaml` (manager,
  console, housekeeping, runtime, context-sync, egress-proxy) and in the four
  bundles' own values (telegram ×3, alertmanager, k8s-events, ha). Existing
  Docker Hub tags stay pullable, so running installs are unaffected until they
  upgrade.
- **GHCR packages are PUBLIC; no pull secrets anywhere.** A GHCR package is
  created private on first push and is flipped to public by hand, once per
  package. That flip is a release prerequisite, not a nicety: pulling stays
  anonymous, the chart needs no `imagePullSecrets` value, and nothing in
  `runtimepod/podspec.go` or `adapterworkload.go` has to learn about
  credentials.
  The alternative — private packages — was rejected on a fact about this
  codebase: **adapter ServiceAccounts are created by the manager's reconciler,
  not by the chart** (`adapterworkload.go`), so a chart-rendered pull secret
  cannot reach adapter pods at all. Six of the thirteen images run under those
  SAs. The repository itself stays private; only the packages are public.
- **Release provenance**: build provenance attestations and SBOMs attached to
  each published image, and `org.opencontainers.image.source` set so GHCR
  packages link back to the repo.
- **A LICENSE, before the first publish.** Public packages publish the built
  binaries, and `README.md` still says "License TBD". This stops being a
  footnote and becomes a prerequisite of the first release.
- **Docs**: `docs/installation.md` gains the OCI path (it owns the parent
  chart's install), `README.md` keeps only the commands within its 150-line
  budget, the registry cut-over is a `CHANGELOG.md` migration entry, and
  `CLAUDE.md`'s manual `docker build` block is replaced by the tag-driven flow.

Not in scope: adding a linter beyond `go vet`, signing images with cosign,
publishing a `latest` tag, dual-publishing to Docker Hub, making the REPOSITORY
public, or a release-notes generator.

## Capabilities

### New Capabilities
- `continuous-integration`: what must pass before a change lands — per-module
  Go build/vet/test including the envtest suite, chart lint/render/validate
  across value permutations, and a no-push build of every image.
- `release-publishing`: how artifacts leave the repo — per-component tag
  triggers, multi-arch manifest lists on GHCR, the OCI Helm chart, tag
  immutability, version/tag agreement, and provenance metadata.

### Modified Capabilities
<!-- none: no spec in openspec/specs/ references a registry, image name, chart
     version, or build process -->

## Impact

- **New**: `.github/workflows/ci.yml`, `.github/workflows/release.yml`,
  `.github/workflows/build-image.yml` (reusable), `.github/dependabot.yml`,
  `.dockerignore` (absent today — the manager build context currently uploads
  the whole tree including `chart/` and `openspec/`).
- **No Dockerfile change.** All thirteen build multi-arch as they stand. The
  earlier plan to make `runtime-claude` `TARGETARCH`-aware described a `kubectl`
  download that image no longer performs (dropped in 0.5.0), and the plan after
  that declared it single-arch on a premise the build disproved.
- **`CLAUDE.md` correction**: its "the exception is a runtime whose UPSTREAM is
  single-arch" note is wrong and is removed. The chart's runtime `nodeSelector`
  becomes optional once the multi-arch image ships — relaxing it is the
  operator's call, after, not part of this change.
- **`chart/`**: image repositories repoint to GHCR in `values.yaml` and in all
  four bundles' `values.yaml`; `Chart.yaml` version bump. **No other chart
  change** — public packages mean no pull-secret value and no ServiceAccount
  edits.
- **Docs**: `docs/installation.md` (the parent chart's install and values),
  `README.md` (commands only), `CLAUDE.md` build/test and image sections, and a
  `CHANGELOG.md` migration entry for the registry cut-over.
  `CLAUDE.md`'s inventory is corrected as part of that: it says "Nine Go
  modules" and **omits `platform/egress-proxy/` entirely**, including from the image
  build block — so the one document that lists what this repo ships is missing
  a shipped image. A release process derived from the repo rather than from
  prose is the fix; correcting the prose is what stops the two disagreeing
  again.
- **No Go code, CRD, or API change**, and no test change.
- **Repo settings (manual, outside the diff)**: each GHCR package is created
  private by its first push and must be flipped to public once. `GITHUB_TOKEN`
  cannot do it, so it is a documented per-package step — thirteen images plus
  the chart — and a package left private is an `ImagePullBackOff` for whoever
  installs next.
- **BLOCKING**: the repo has no LICENSE and `README.md` says "License TBD".
  Public packages publish the built binaries, so this is settled before the
  first publish, not after.
