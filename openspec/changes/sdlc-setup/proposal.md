# SDLC setup: GitHub Actions CI + multi-arch GHCR publishing for images and chart

## Why

The repository has **no `.github/` directory at all**. Every artifact this
project ships — five container images and a Helm chart — is built by hand from
the `docker buildx` command lines pasted in `CLAUDE.md`, pushed to a personal
Docker Hub namespace (`kmatsebora/*`), and versioned by remembering to bump a
tag. Nothing verifies that `go build`, `go vet`, `go test` (including the
envtest suite) or `helm lint` pass before a change lands, and nothing enforces
the project's own "never overwrite a pushed tag" rule. A release today is a
sequence of five manual commands that must not be typed wrong.

## What Changes

- **CI on every PR and push to `master`** (`.github/workflows/ci.yml`):
  build + vet + test the operator module against a real API server (envtest,
  Kubernetes 1.31.x per `CLAUDE.md`), build + vet + test each of the three
  dependency-free adapter modules, `helm lint`/`helm template` the chart across
  its meaningful value permutations (default, `demo.enabled`,
  `vm-bundle.enabled`) with schema validation of the rendered manifests, and
  build all five images without pushing so a broken Dockerfile fails the PR.
- **Tag-driven publishing to GHCR** (`.github/workflows/release.yml` + a
  reusable `build-image.yml`): a tag of the form `<component>-v<semver>`
  (`manager-v0.13.2`, `channel-telegram-v0.4.1`, `runtime-claude-v0.1.2`,
  `signal-cron-v…`, `signal-alertmanager-v…`) publishes exactly that one
  component. Preserves today's independent per-component versioning.
- **Multi-arch images (`linux/amd64` + `linux/arm64`)** published as a single
  manifest list per tag. The four Go images already cross-compile
  (`BUILDPLATFORM` + `TARGETARCH`); **`runtime-claude` is fixed to be
  buildable at all on arm64** — it currently hardcodes an amd64 kubectl
  download URL — and is built for both arches via QEMU emulation.
- **Helm chart published as an OCI artifact** on a `chart-v<semver>` tag:
  `oci://ghcr.io/kostiantyn-matsebora/charts`, installable with
  `helm install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator --version <v>`.
- **Tag immutability is enforced in CI**: a publish job that finds its tag
  already present in the registry fails before building. The project rule
  becomes a gate instead of a note in `CLAUDE.md`.
- **Registry cut-over to GHCR — BREAKING for new installs.** Chart defaults
  repoint from `kmatsebora/*` to `ghcr.io/kostiantyn-matsebora/agentops-*`
  (manager, telegram adapter, vm-alertmanager adapter, demo runtime). Existing
  Docker Hub tags stay pullable, so running installs are unaffected until they
  upgrade.
- **`imagePullSecrets` support in the chart — required, not optional.** The
  repo is private, so GHCR packages are private by default and **nothing in the
  cluster can pull them today**: there is no `imagePullSecrets` handling in the
  chart, in `runtimepod/podspec.go`, or in `adapterworkload.go`. The chart gains
  a `global.imagePullSecrets` value attached to the three ServiceAccounts
  (`agentops-manager`, `agentops-runtime`, the adapter SA), which covers manager
  pods, runtime pods, and adapter pods **without any Go or CRD change**.
- **Release provenance**: build provenance attestations and SBOMs attached to
  each published image, and `org.opencontainers.image.source` set so GHCR
  packages link back to the repo.
- **Docs**: install instructions gain the OCI path and the pull-secret
  prerequisite; the registry cut-over is recorded as a migration entry;
  `CLAUDE.md`'s manual `docker build` block is replaced by the tag-driven flow.

Not in scope: adding a linter beyond `go vet`, signing images with cosign,
publishing a `latest` tag, dual-publishing to Docker Hub, choosing a LICENSE
(the repo has none — see Impact), or a release-notes generator.

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
- **`runtime-claude/Dockerfile`**: kubectl URL becomes `TARGETARCH`-aware; this
  is the only image whose Dockerfile is not already multi-arch-capable.
- **`chart/`**: `values.yaml` image repositories repoint to GHCR;
  `global.imagePullSecrets` value plus ServiceAccount template changes;
  `Chart.yaml` version bump. No template logic beyond the SA additions.
- **Docs**: `README.md` install section; `CLAUDE.md` build/test and image
  sections. The registry cut-over is a migration note — it belongs in
  `CHANGELOG.md` if the in-flight `organize-docs` change has landed, otherwise
  in the README's migration sections.
- **No Go code, CRD, or API change**, and no test change — the SA-level pull
  secret is deliberately chosen to keep it that way.
- **Repo settings (manual, outside the diff)**: GHCR package visibility per
  package. Default here is private-to-match-the-repo with pull secrets; making
  a package public removes the pull-secret requirement but publishes the built
  binaries — that call stays with the owner.
- **Flag, not blocking**: the repo has no LICENSE and `README.md` says "License
  TBD". That is fine while packages are private; it should be settled before
  any package is made public.
