## Context

Current state, established by inspection:

| Fact | Consequence |
|---|---|
| No `.github/` directory | Greenfield — no CI to preserve or migrate |
| 12 Go modules (root + 11), all `go 1.23` | Root needs envtest assets; the eleven are dependency-free and test in seconds |
| `CLAUDE.md` says "Nine Go modules" and never mentions `platform/egress-proxy/` — not in the Map, not in the image build block | The only document listing what this repo ships is already behind it. Derive the CI and release matrices from the repo, and correct the prose so the two cannot drift apart again |
| `console/` also holds a React UI (npm, vitest) | One non-Go job: nothing runs `Text.test.tsx` / `Yaml.test.tsx` today |
| 13 Dockerfiles | 12 cross-compile via `--platform=$BUILDPLATFORM` + `TARGETOS/TARGETARCH`; `runtime-claude` is `node:22` + apt + one npm global, every layer of which is multi-arch |
| `runtime-claude` builds and RUNS on arm64 — verified: `claude --version` reports 2.1.239 on `aarch64` | There is no single-arch component. The published image is amd64-only because the hand-run command says `--platform linux/amd64`, and `CLAUDE.md`'s "upstream is single-arch" claim is wrong |
| An amd64-only `agentops-console` reached production and `ImagePullBackOff`ed weeks later on the first arm64 reschedule (2026-08-21) | Arch coverage must be ASSERTED after push, not assumed from the build command |
| Images versioned independently (manager 0.38.1, console 0.16.0, channel-telegram 0.12.0, runtime-claude 0.6.0, …); chart 5.25.0 | A single repo-wide version would be a regression |
| `CLAUDE.md`: "bump the tag on every change — never overwrite a pushed tag" | An invariant CI can enforce |
| Repo is **private**, no LICENSE | Free `ubuntu-24.04-arm` runners are public-repo only — irrelevant here, since nothing needs emulation. Packages may still be published public from a private repo |
| Adapter ServiceAccounts are created by `adapterworkload.go`, not by the chart | A chart-rendered `imagePullSecrets` cannot reach adapter pods at all — which is why private packages are rejected outright |
| Four local-path subcharts (`k8s-bundle`, `prometheus-bundle`, `ha-bundle`, `telegram-bundle`), `repository: ""` | No `helm dependency update` / no external chart repo needed in CI |
| `ha-bundle.enabled=true` alone FAILS to render — it requires a Home Assistant credential | A permutation matrix must supply each bundle's required values, or it tests the guard instead of the templates |
| Integration suite needs `KUBEBUILDER_ASSETS` (envtest 1.31.x) | CI must provision and cache it |

Confirmed decisions from the requester: OCI chart distribution to GHCR;
per-component git tags as the release trigger; cut over to GHCR only (Docker
Hub left frozen, not deleted); `runtime-claude` stays amd64-only rather than
chasing an arm64 build its upstream cannot produce; the console UI gets its own
CI job; GHCR packages are PUBLIC, so nothing needs a pull secret.

## Goals / Non-Goals

**Goals:**
- Every PR proves the twelve Go modules build/vet/test, the console UI builds
  and its tests pass, and the chart renders, before merge.
- One `git tag && git push` publishes one component, reproducibly, with its
  architecture coverage asserted rather than assumed.
- The never-overwrite-a-tag rule is enforced by machine, not memory.
- An install that pulls with no credential, from a private repository.
- No Go, CRD, or test changes — CI setup should not perturb the product.

**Non-Goals:**
- `latest` tags, cosign signing, golangci-lint, dual-registry publishing,
  auto-generated release notes, a docs site, or self-hosted runners.
- Deleting or re-pushing existing Docker Hub tags.
- Making the REPOSITORY public. Only the packages are.
- Bumping any component version as part of this change beyond the chart.

## Decisions

### D1. Three workflows: `ci.yml`, `release.yml`, and a reusable `build-image.yml`

`ci.yml` (on `pull_request` + `push: master`) runs five parallel jobs:

- `operator` — `actions/setup-go@v5` with `go-version-file: go.mod`, restore
  `~/.envtest` from cache, `setup-envtest use 1.31.x`, then
  `go build ./... && go vet ./... && go test ./...` with `KUBEBUILDER_ASSETS`.
- `modules` — matrix over the eleven submodules (`channel-telegram`,
  `gateway-telegram`, `signal-telegram`, `signal-cron`, `signal-alertmanager`,
  `signal-k8s-events`, `signal-ha`, `console`, `context-sync`, `housekeeping`,
  `egress-proxy`); build/vet/test each. Separate from `operator` so a module
  break is legible without reading the envtest log.
- `console-ui` — `npm ci`, `npm test` (vitest) and `npm run build` in
  `console/ui`. The only non-Go job, and it exists because the UI has tests
  nothing runs: today a broken bundle is caught by the console IMAGE build, at
  the end of a long log, and a failing `Yaml.test.tsx` is caught by nobody.
  Screenshots stay out — they need a browser, and they are regenerated
  deliberately when the UI changes, not on every PR.
- `chart` — `helm lint` + `helm template` for default, `global.demo.enabled`,
  each of the four bundles with the values it requires, `console.enabled=false`
  (the pinned one-value opt-out), and everything on at once — each piped
  through `kubeconform` in strict mode with the repo's own CRDs from
  `chart/files/crds/` supplied as schemas. Rendering *and* validating catches
  the class of bug that a lint alone misses.
  **Each bundle's permutation must carry that bundle's required values**:
  `ha-bundle.enabled=true` alone fails the render, because the log source has
  no Home Assistant credential. A matrix that omits them tests the guards and
  never reaches the templates.
- `images` — matrix over the thirteen components, `docker/build-push-action`
  with `push: false`, **amd64 only**, GitHub Actions layer cache. PR feedback
  speed matters more than arch coverage here; the arm64 build is exercised at
  release.

`release.yml` is the tag entry point. `build-image.yml` is `workflow_call`-only
with inputs `component`, `context`, `dockerfile`, `platforms`, `version`.
Thirteen components × ~10 build steps is exactly the duplication a reusable
workflow exists to remove, and it keeps the immutability guard (D5) in one
place.

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

Components: `manager`, `channel-telegram`, `gateway-telegram`,
`signal-telegram`, `signal-cron`, `signal-alertmanager`, `signal-k8s-events`,
`signal-ha`, `console`, `context-sync`, `housekeeping`, `egress-proxy`,
`runtime-claude`, `chart`. The set is derived from the Dockerfiles, and a new
module is a one-line addition — which is the property that matters, since this
inventory went from five to thirteen while the release procedure stood still.

Image name: `ghcr.io/kostiantyn-matsebora/agentops-<component>` — preserving
the existing `agentops-*` names so only the registry prefix changes. Published
tags per release: the exact version, and nothing else (no `latest`, no
floating minor — the project's stated posture is pinned tags).

Alternative rejected: `v<semver>` repo-wide tags with path-filtered builds.
It collapses fourteen independently-versioned artifacts into one version line
and would force a manager release every time an adapter changes.

### D3. Every image is multi-arch. There is no exception.

All thirteen images publish `linux/amd64` and `linux/arm64` as one manifest
list. The twelve Go Dockerfiles cross-compile with `--platform=$BUILDPLATFORM`
and `GOARCH=${TARGETARCH}`, so both arches build on an amd64 runner at native
speed. `runtime-claude` is `node:22-bookworm-slim` plus apt packages and
`npm install -g @anthropic-ai/claude-code` — every layer of that is already
multi-arch. **No QEMU anywhere.**

*What this replaces, and why it matters beyond this change.* Two earlier
versions of this design carved out `runtime-claude`: first by parameterising a
hardcoded amd64 `kubectl` URL (that CLI was removed in image 0.5.0), then by
declaring its upstream single-arch and building it amd64-only. The second claim
came from `CLAUDE.md`, and building the image settled it:

    docker buildx build --platform linux/arm64 ./runtime-claude/   # succeeds
    docker run --platform linux/arm64 … claude --version
    arch: aarch64 / v22.23.2 / 2.1.239 (Claude Code)

So the constraint was never upstream's. It was the hand-run build command in
`CLAUDE.md` — `docker build --platform linux/amd64` — and the `nodeSelector`
the chart ships for runtime pods is compensating for that flag rather than for
anything about the vendor. Correcting both is in scope here (task 8.6): a
release process derived from the repo is worth little while the prose beside it
asserts a constraint that does not exist.

### D4. Architecture coverage is ASSERTED after the push, per component

Every component declares its platforms in `.github/components.sh`, and the
publish job verifies the pushed manifest against that declaration with
`docker buildx imagetools inspect` — EQUALITY, not containment, so an image
that gains or loses an architecture fails either way.

Today every component declares both arches and the declaration mechanism has no
exceptions in it. It stays anyway, for two reasons. A future vendor image may
genuinely be single-arch, and — the reason that is not hypothetical — an
amd64-only `agentops-console` ran in production for weeks purely because every
reschedule happened to land on an amd64 node, and the first one that did not
left the console in `ImagePullBackOff`. Nothing in the chart or the CR was
wrong. Nothing at build, push or render time had said a word.

A build command is a request. Only the manifest is evidence.

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
resolves — anonymously, since the packages are public — every FIRST-PARTY
image reference the chart renders by default — six
in `chart/values.yaml` (manager, console, housekeeping, the runtime, the
context sidecar, the egress proxy) and six across the bundles (telegram
router/channel/signal, alertmanager, k8s-events, home-assistant) — and fails if
any tag is absent from GHCR. Shipping a chart
that points at images which were never pushed is the single most likely
release-day mistake in a multi-artifact repo, and it is cheap to prevent.

Third-party images are excluded from the existence check — they are not ours to
guarantee. Today that is `ghcr.io/containers/kubernetes-mcp-server`,
`ghcr.io/pab1it0/prometheus-mcp-server` and `ghcr.io/homeassistant-ai/ha-mcp`;
the rule is "not under our namespace", not a hardcoded list, because that list
has already turned over once.

### D7. Public packages, so nothing needs a pull secret

Every GHCR package is published PUBLIC. Pulling is anonymous, the chart gains no
value, and no pod spec, ServiceAccount or controller learns about credentials.

The alternative was private packages with `imagePullSecrets`, and it fails on a
fact about this codebase rather than on taste. Pull secrets must reach three
classes of pod: the manager Deployment, the runtime pods built by
`internal/runtimepod/podspec.go`, and the adapter Deployments built by
`internal/controller/adapterworkload.go`. **The adapter ServiceAccount is
created by that reconciler, not by the chart** — one per adapter CR, named
after it, owner-referenced to it. The chart never sees those names, so a
chart-rendered pull secret covers the manager and the runtime and leaves every
adapter pod in `ImagePullBackOff`. Six of the thirteen images run under those
SAs.

Closing that gap needs the manager to carry the secret names and stamp them on
the SAs it creates — a Go change to satisfy a CI decision, which is the trade
this design set out to avoid.

What public packages cost instead: **one manual flip per package**. A GHCR
package is created private by its first push and `GITHUB_TOKEN` cannot change
its visibility, so each of the thirteen images and the chart is flipped once,
by hand, after its first release. It is a checklist item with a loud failure
mode — a package left private is an install that cannot pull — and it is paid
once per package rather than in every cluster.

The consequence that decides it: **the LICENSE stops being optional.** Public
packages publish the built binaries, so the repository settles its license
before the first publish. That is recorded as a prerequisite in the tasks, not
as an open question.

### D8. Add `.dockerignore`; do not restructure build contexts

The root build context is the whole repository — `chart/`, `openspec/`,
`runtimes/claude/`, `.git/` all get uploaded to the daemon for a build that
copies only `go.mod`, `go.sum`, `api/`, `cmd/`, `internal/`. A `.dockerignore`
fixes that in one file. Restructuring contexts or introducing a Makefile is a
larger change with no CI-correctness payoff.

### D9. Dependabot for actions and Go modules; nothing else

`.github/dependabot.yml` covering `github-actions` (weekly), the twelve `gomod`
directories (weekly) and `console/ui`'s npm manifest (weekly). Action pinning
drift, Go CVEs and a lockfile nothing else updates are the maintenance costs
this change creates; Dependabot is the proportionate answer. Docker base-image
updates are deliberately excluded — `golang:1.23`, `node:22-alpine` and
`distroless/static` float already, and PR noise on thirteen Dockerfiles would
outweigh the benefit.

## Risks / Trade-offs

- [Publishing `runtime-claude` multi-arch changes what a cluster may schedule]
  → For the better, and only after the image exists on both arches: the chart's
  runtime `nodeSelector` was compensating for a single-arch image, so it becomes
  optional rather than load-bearing. Relaxing it is the operator's call and is
  NOT done as part of this change — the image ships first, the constraint is
  dropped afterwards, in that order.
- [A package is left private after its first push, so installs cannot pull it]
  → The visibility flip is a task per component, and the chart's image-existence
  gate (D6) queries anonymously — a private package fails that gate at release
  time, before anyone installs it. The failure is loud either way
  (`ImagePullBackOff`), never silent.
- [The registry cut-over strands an install that cannot reach GHCR] → Docker
  Hub tags stay pullable and unchanged, and the migration note gives the
  `--set image.repository=` override to stay on Docker Hub.
- [Public packages publish the built binaries from a private repository] →
  Accepted deliberately, and it is what makes the LICENSE a prerequisite rather
  than a footnote. The source stays private; the artifacts do not.
- [Enforcing tag immutability blocks a legitimate re-run after a partial
  failure] → Accepted deliberately: the correct recovery is a new patch
  version, which is exactly what the rule intends. The guard message says so.
- [envtest asset download flakes make CI red for unrelated reasons] → Cached on
  `~/.envtest` keyed by the envtest version; a cache miss re-downloads, and the
  step is isolated so the failure names itself.
- [CI runs thirteen image builds on every PR, slowing feedback] → amd64-only,
  no push, layer-cached, and matrixed in parallel. Full multi-arch is
  release-only. If the matrix becomes the slowest job, the fallback is
  path-filtering each component's build — deliberately NOT done first, because
  a skipped job reads as a passed one.
- [The console UI job adds a Node toolchain to CI for one module] → Accepted:
  it is the only module with a build step of its own, and its tests exist and
  run nowhere. Scoped to `console/ui` and cached on the lockfile.

## Migration Plan

Rollout is additive and reversible:

1. Merge CI-only workflows first (`ci.yml`, `.dockerignore`, Dependabot) — no
   publishing, no chart change. Confirms the test matrix is green on real PRs.
2. Settle the LICENSE, before anything is published. No chart or Dockerfile
   change is needed in this step.
3. Add `release.yml` + `build-image.yml`, then cut one **patch** release per
   component to prove the path end to end (e.g. `manager-v0.38.2`) and verify
   each manifest list matches its declared platforms.
4. Repoint chart defaults to GHCR, bump `Chart.yaml`, tag `chart-v<next>`, and
   verify a clean `helm install` from `oci://…` into a scratch namespace.
5. Record the `CHANGELOG.md` migration entry and update `docs/installation.md`,
   `README.md` and `CLAUDE.md`.

Rollback: delete the workflow files (Docker Hub images remain; the hand-run
`docker buildx` commands in `CLAUDE.md` still work), and revert the chart's
image repositories. Published GHCR tags can be left in place; nothing depends
on them once the chart points elsewhere.

## Open Questions

- **Which LICENSE.** That the repository needs one is settled — public packages
  publish the built binaries. Which one is the owner's call, and it blocks the
  first publish rather than the workflows.
- Whether an arm64-capable runtime image is worth building from a different
  vendor, which would remove the last single-arch artifact in the release. Out
  of scope here: it is a runtime decision, not a CI one.
