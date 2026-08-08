## Context

Current state, established by inspection:

| Fact | Consequence |
|---|---|
| No `.github/` directory | Greenfield — no CI to preserve or migrate |
| 4 Go modules (root + 3 adapters), all `go 1.23.0` | Root needs envtest assets; adapters are dependency-free and test in seconds |
| 5 Dockerfiles | 4 already use `--platform=$BUILDPLATFORM` + `TARGETOS/TARGETARCH` cross-compile; `runtime-claude` does not |
| `runtime-claude/Dockerfile` hardcodes `.../bin/linux/amd64/kubectl` | Would produce a broken arm64 image, not a failed build — silent breakage |
| Images tagged independently (manager 0.13.1, telegram 0.4.0, vm-alertmanager 0.6.0, runtime 0.1.1); chart 1.14.0 | A single repo-wide version would be a regression |
| `CLAUDE.md`: "bump the tag on every change — never overwrite a pushed tag" | An invariant CI can enforce |
| Repo is **private**, no LICENSE | GHCR packages default to private; free `ubuntu-24.04-arm` runners are public-repo only |
| No `imagePullSecrets` anywhere: chart, `runtimepod/podspec.go`, `adapterworkload.go` | Private GHCR would make the whole product unpullable |
| `chart/charts/vm-bundle` is a local-path dependency (`repository: ""`) | No `helm dependency update` / no external chart repo needed in CI |
| Integration suite needs `KUBEBUILDER_ASSETS` (envtest 1.31.x) | CI must provision and cache it |

Confirmed decisions from the requester: OCI chart distribution to GHCR;
per-component git tags as the release trigger; cut over to GHCR only (Docker
Hub left frozen, not deleted); QEMU emulation for `runtime-claude` arm64.

## Goals / Non-Goals

**Goals:**
- Every PR proves the four modules build/vet/test and the chart renders, before
  merge.
- One `git tag && git push` publishes one component, multi-arch, reproducibly.
- The never-overwrite-a-tag rule is enforced by machine, not memory.
- A private-repo default that actually works end to end in a cluster.
- No Go, CRD, or test changes — CI setup should not perturb the product.

**Non-Goals:**
- `latest` tags, cosign signing, golangci-lint, dual-registry publishing,
  auto-generated release notes, a docs site, or self-hosted runners.
- Deleting or re-pushing existing Docker Hub tags.
- Deciding LICENSE or package visibility (owner's call; both flagged).
- Bumping any component version as part of this change beyond the chart.

## Decisions

### D1. Three workflows: `ci.yml`, `release.yml`, and a reusable `build-image.yml`

`ci.yml` (on `pull_request` + `push: master`) runs four parallel jobs:

- `operator` — `actions/setup-go@v5` with `go-version-file: go.mod`, restore
  `~/.envtest` from cache, `setup-envtest use 1.31.x`, then
  `go build ./... && go vet ./... && go test ./...` with `KUBEBUILDER_ASSETS`.
- `adapters` — matrix over `channel-telegram`, `signal-cron`,
  `signal-vmalertmanager`; build/vet/test each. Separate from `operator` so an
  adapter break is legible without reading the envtest log.
- `chart` — `helm lint` + `helm template` for default, `demo.enabled=true`, and
  `vm-bundle.enabled=true` (plus the two together), each piped through
  `kubeconform` in strict mode with the repo's own CRDs from
  `chart/files/crds/` supplied as schemas. Rendering *and* validating catches
  the class of bug that a lint alone misses.
- `images` — matrix over the five components, `docker/build-push-action` with
  `push: false`, **amd64 only**, GitHub Actions layer cache. PR feedback speed
  matters more than arch coverage here; the arm64 build is exercised at release
  and, for `runtime-claude`, by D4's dedicated check.

`release.yml` is the tag entry point. `build-image.yml` is `workflow_call`-only
with inputs `component`, `context`, `dockerfile`, `platforms`, `version`. Five
components × ~10 build steps is exactly the duplication a reusable workflow
exists to remove, and it keeps the immutability guard (D5) in one place.

Alternative rejected: one monolithic workflow with `if:` gates on every job. It
reads as a single file but the tag-parse conditionals multiply across every
step, and a failure surfaces as "release.yml failed" rather than
"build-image (runtime-claude) failed".

### D2. Tag grammar `<component>-v<semver>`, parsed by a `determine` job

`release.yml` triggers on `push: tags: ['*-v*']`. A small `determine` job
splits the ref at the last `-v`, validates the component against the known set
and the version against a semver regex, and emits both as outputs. Unknown
component or malformed version fails immediately with a message naming the
valid forms — a typo'd tag must not silently publish nothing (a `paths`- or
`if`-filtered job that simply skips is indistinguishable from success).

Components: `manager`, `channel-telegram`, `signal-cron`,
`signal-vmalertmanager`, `runtime-claude`, `chart`.

Image name: `ghcr.io/kostiantyn-matsebora/agentops-<component>` — preserving
the existing `agentops-*` names so only the registry prefix changes. Published
tags per release: the exact version, and nothing else (no `latest`, no
floating minor — the project's stated posture is pinned tags).

Alternative rejected: `v<semver>` repo-wide tags with path-filtered builds.
It collapses four independently-versioned artifacts into one version line and
would force a manager release every time an adapter changes.

### D3. Cross-compile the Go images; QEMU only where it is unavoidable

The four Go Dockerfiles already build with `--platform=$BUILDPLATFORM` and
`GOARCH=${TARGETARCH}`, so `buildx --platform linux/amd64,linux/arm64` produces
both arches on an amd64 runner at native speed with no emulation. Only
`runtime-claude` runs `apt-get`/`npm install -g` in the target image, which
must execute as arm64 — so `docker/setup-qemu-action` is enabled **for that
component only** (an input on the reusable workflow), keeping the other four
builds free of a slow, avoidable emulation layer.

Cost accepted: the `runtime-claude` arm64 layers run emulated (order 10–20
minutes on a cold cache). That image changes rarely — its version is still
0.1.1 while the manager is at 0.13.1 — so the cost lands a handful of times a
year. Native arm64 runners would fix it, but on a private repo they are paid.

### D4. Fix `runtime-claude`'s kubectl URL, and assert the fix

`.../bin/linux/amd64/kubectl` becomes `.../bin/linux/${TARGETARCH}/kubectl`
with `ARG TARGETARCH` declared. Without this, the arm64 build *succeeds* and
ships an amd64 kubectl binary into an arm64 image — the cluster-apply lane then
fails at runtime with `exec format error`, far from the cause. The build
therefore also runs `kubectl version --client` in a final `RUN`, so a wrong-arch
binary fails the build under emulation instead of in production.

### D5. Tag immutability is a preflight registry query, not a post-hoc check

Before any build, `build-image.yml` queries the registry for the target tag
(`docker manifest inspect`, or `gh api /user/packages/.../versions`) and fails
if it resolves. Same for the chart. This encodes `CLAUDE.md`'s rule at the only
point where it can actually be violated. It runs *before* the build so a
mistaken re-tag costs seconds, not a full emulated build.

### D6. The chart release asserts version agreement and image existence

The `chart` component job fails unless `Chart.yaml`'s `version` equals the
tag's version — otherwise the OCI artifact's tag and its embedded metadata
disagree, which is invisible until someone debugs a deployment. It then
resolves every image reference the chart renders by default
(`image.*`, `telegramAdapter.image.*`, `vm-bundle`'s adapter image,
`demo.runtimeImage`) and fails if any tag is absent from GHCR. Shipping a chart
that points at images which were never pushed is the single most likely
release-day mistake in a multi-artifact repo, and it is cheap to prevent.

Third-party images in `vm-bundle` (`ghcr.io/victoriametrics/mcp-*`) are
excluded from the existence check — they are not ours to guarantee, and they
default to `latest`.

### D7. `imagePullSecrets` on ServiceAccounts, not on pod specs

A private GHCR needs a pull secret on every pod: the manager Deployment, the
adapter Deployments built by `internal/controller/adapterworkload.go`, and the
runtime pods built by `internal/runtimepod/podspec.go`. Two of those three are
constructed in Go, so a `podSpec.imagePullSecrets` approach would mean new CRD
fields on `AgentRuntime`/`ChannelAdapter`/`SignalAdapter`, deepcopy regen, CRD
regen, and controller changes — a product API change caused by a CI decision.

Instead the chart renders `imagePullSecrets` on the three **ServiceAccounts**
(`agentops-manager`, `agentops-runtime`, and the adapter SA). The kubelet
merges SA-level pull secrets into every pod using that SA, so all three pod
classes are covered with zero Go changes and zero new API surface. The value is
`global.imagePullSecrets` (a list of `{name}`), empty by default so public-
package users and Docker Hub holdouts see no change.

The Secret itself is created by the operator of the cluster, not the chart —
consistent with the manager-reads-no-secrets posture and with how every other
credential in this project is handled (the chart renders references, never
Secret manifests).

### D8. Add `.dockerignore`; do not restructure build contexts

The root build context is the whole repository — `chart/`, `openspec/`,
`runtime-claude/`, `.git/` all get uploaded to the daemon for a build that
copies only `go.mod`, `go.sum`, `api/`, `cmd/`, `internal/`. A `.dockerignore`
fixes that in one file. Restructuring contexts or introducing a Makefile is a
larger change with no CI-correctness payoff.

### D9. Dependabot for actions and Go modules; nothing else

`.github/dependabot.yml` covering `github-actions` (weekly) and the four
`gomod` directories (weekly). Action pinning drift and Go CVEs are the two
maintenance costs this change creates; Dependabot is the proportionate answer.
Docker base-image updates are deliberately excluded — `golang:1.23` and
`distroless/static` float already, and PR noise on five Dockerfiles would
outweigh the benefit.

## Risks / Trade-offs

- [`runtime-claude` arm64 build under QEMU is slow enough to time out] → Its
  job gets an explicit `timeout-minutes: 60` and GHA layer caching; if it
  proves painful the escape hatch is a public repo with free native arm64
  runners, or dropping arm64 for that one image. Recorded, not pre-solved.
- [GHCR packages are private by default, and a fresh install silently
  `ImagePullBackOff`s] → D7 ships the pull-secret path in the same change, and
  the README install section states the prerequisite before the `helm install`
  line. The failure is loud (`ImagePullBackOff`), not silent.
- [The registry cut-over strands users who upgrade the chart without a pull
  secret] → Docker Hub tags stay pullable and unchanged; the migration note
  gives both the pull-secret recipe and the `--set image.repository=` override
  to stay on Docker Hub.
- [`GITHUB_TOKEN` can push packages but cannot change package visibility, and
  the first push of a new package is what creates it] → Documented as a
  one-time manual step per package after its first release; the
  `org.opencontainers.image.source` label (set by `docker/metadata-action`)
  auto-links the package to the repo so it inherits repo permissions.
- [Enforcing tag immutability blocks a legitimate re-run after a partial
  failure] → Accepted deliberately: the correct recovery is a new patch
  version, which is exactly what the rule intends. The guard message says so.
- [envtest asset download flakes make CI red for unrelated reasons] → Cached on
  `~/.envtest` keyed by the envtest version; a cache miss re-downloads, and the
  step is isolated so the failure names itself.
- [CI runs five image builds on every PR, slowing feedback] → amd64-only, no
  push, layer-cached, and matrixed in parallel. Full multi-arch is release-only.
- [Two in-flight doc changes (`organize-docs`) move the file this change must
  write its migration note into] → The note's destination is stated
  conditionally in the tasks; whichever lands first, the other adapts. Ordering
  is not enforced.

## Migration Plan

Rollout is additive and reversible:

1. Merge CI-only workflows first (`ci.yml`, `.dockerignore`, Dependabot) — no
   publishing, no chart change. Confirms the test matrix is green on real PRs.
2. Merge the `runtime-claude` Dockerfile fix and the chart's
   `global.imagePullSecrets` support — both inert until used.
3. Add `release.yml` + `build-image.yml`, then cut one **patch** release per
   component to prove the path end to end (e.g. `manager-v0.13.2`) and verify
   the manifest list reports both arches.
4. Repoint chart defaults to GHCR, bump `Chart.yaml`, tag `chart-v<next>`, and
   verify a clean `helm install` from `oci://…` into a scratch namespace.
5. Record the migration note and update README/CLAUDE.md.

Rollback: delete the workflow files (Docker Hub images remain; the hand-run
`docker buildx` commands in `CLAUDE.md` still work), and revert the chart's
image repositories. Published GHCR tags can be left in place; nothing depends
on them once the chart points elsewhere.

## Open Questions

- **GHCR package visibility per package** — private (default here, needs pull
  secrets) or public (no secrets, publishes the built binaries). Owner's call,
  post-first-release, and reversible. Unblocks nothing.
- **LICENSE** — absent, `README.md` says "License TBD". Irrelevant while
  packages are private; must be settled before making one public.
- Whether `runtime-claude` should eventually drop the bundled `kubectl` in
  favour of the runtime SA calling the API directly — would remove the only
  arch-specific artifact in that image. Out of scope, noted because D4 is a
  workaround for it.
