## 1. Build hygiene (no CI yet)

- [ ] 1.1 Add `.dockerignore` excluding `.git/`, `openspec/`, `chart/`, `docs/`,
      `config/`, `runtime-claude/`, `channel-telegram/`, `signal-*/`, `*.md` —
      the root manager build copies only `go.mod`, `go.sum`, `api/`, `cmd/`,
      `internal/`; verify `docker build .` still succeeds afterwards
- [ ] 1.2 Fix `runtime-claude/Dockerfile` for arm64: declare `ARG TARGETARCH`
      and change the kubectl URL to `.../bin/linux/${TARGETARCH}/kubectl`
- [ ] 1.3 Add a `RUN kubectl version --client` step to `runtime-claude` so a
      wrong-architecture binary fails the build, not the pod
- [ ] 1.4 Verify locally: `docker buildx build --platform linux/amd64,linux/arm64 ./runtime-claude/`
      completes and both arches report a working kubectl

## 2. CI workflow

- [ ] 2.1 Create `.github/workflows/ci.yml` triggered on `pull_request` and
      `push: master`, with `permissions: {contents: read}` and concurrency
      cancel-in-progress per ref
- [ ] 2.2 Job `operator`: `actions/setup-go@v5` with `go-version-file: go.mod`,
      Go build cache, `actions/cache` on `~/.envtest`, `setup-envtest use 1.31.x`,
      then `go build ./... && go vet ./...` and `go test ./...` with
      `KUBEBUILDER_ASSETS` — confirm the `internal/integration` suite runs
      rather than skipping
- [ ] 2.3 Job `adapters`: matrix over `channel-telegram`, `signal-cron`,
      `signal-vmalertmanager` running build + vet + test in each module dir
- [ ] 2.4 Job `chart`: `helm lint`, then `helm template` for default,
      `demo.enabled=true`, `vm-bundle.enabled=true`, and both together
- [ ] 2.5 Extend the `chart` job with `kubeconform` in strict mode over each
      rendered permutation, passing `chart/files/crds/` as additional schemas so
      CRs are validated against the repo's own CRDs
- [ ] 2.6 Job `images`: matrix over the five components,
      `docker/build-push-action` with `push: false`, `platforms: linux/amd64`,
      and GHA layer cache
- [ ] 2.7 Open a scratch PR and confirm all jobs run and pass; deliberately
      break one module and confirm the failure is attributed to it

## 3. Reusable image publish workflow

- [ ] 3.1 Create `.github/workflows/build-image.yml` as `workflow_call` with
      inputs `component`, `context`, `dockerfile`, `platforms`, `version`,
      `qemu` (bool) and `permissions: {contents: read, packages: write,
      id-token: write, attestations: write}`
- [ ] 3.2 Preflight immutability guard: query
      `ghcr.io/kostiantyn-matsebora/agentops-<component>:<version>` and fail
      before building if it resolves, with a message saying to cut a new patch
      version rather than re-push
- [ ] 3.3 Steps: `docker/setup-qemu-action` (only when `qemu`),
      `docker/setup-buildx-action`, `docker/login-action` against `ghcr.io`
      with `GITHUB_TOKEN`
- [ ] 3.4 `docker/metadata-action` producing the exact version tag only (no
      `latest`) plus OCI labels including
      `org.opencontainers.image.source` so the package links to the repo
- [ ] 3.5 `docker/build-push-action` with the input platforms, `push: true`,
      `provenance: mode=max`, `sbom: true`, GHA layer cache, and
      `timeout-minutes: 60` on the job for the emulated case
- [ ] 3.6 Post-push verification: `docker buildx imagetools inspect` asserts the
      manifest list contains both `linux/amd64` and `linux/arm64`

## 4. Release entry point

- [ ] 4.1 Create `.github/workflows/release.yml` on `push: tags: ['*-v*']`
- [ ] 4.2 Job `determine`: split the ref at the last `-v`, validate the
      component against the known set and the version against a semver regex,
      emit `component`/`version` outputs, and fail with the valid tag forms
      listed when either is invalid (never skip silently)
- [ ] 4.3 Five image jobs calling `build-image.yml`, each gated on
      `needs.determine.outputs.component`, with the right context/dockerfile and
      `qemu: true` only for `runtime-claude`
- [ ] 4.4 End-to-end proof: cut `manager-v<next-patch>`, confirm the image
      publishes, both arches are present, and a re-push of the same tag is
      rejected by the guard

## 5. Chart publishing

- [ ] 5.1 Add the `chart` job to `release.yml`: assert `Chart.yaml` `version`
      equals the tag version, failing with both values on mismatch
- [ ] 5.2 Add the image-existence gate: resolve every first-party image the
      chart renders by default (`image.*`, `telegramAdapter.image.*`,
      `vm-bundle` adapter image, `demo.runtimeImage`) against GHCR and fail
      naming any missing reference; exclude third-party
      `ghcr.io/victoriametrics/*` references
- [ ] 5.3 Immutability guard for the chart tag, then `helm package chart/` and
      `helm push` to `oci://ghcr.io/kostiantyn-matsebora/charts`
- [ ] 5.4 Verify: `helm show chart oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator --version <v>`
      resolves, and a `helm install` into a scratch namespace succeeds

## 6. Private-registry pull support

- [ ] 6.1 Add `global.imagePullSecrets: []` to `chart/values.yaml` with a
      comment explaining it is required for private GHCR packages and that the
      Secret is created out-of-band (the chart renders references, never
      Secrets)
- [ ] 6.2 Render `imagePullSecrets` onto the manager ServiceAccount, the runtime
      ServiceAccount (`agentops-runtime`), and the adapter ServiceAccount when
      the value is non-empty; omit the field entirely when empty
- [ ] 6.3 Confirm no Go, CRD, or deepcopy change was needed — `git status` shows
      no `api/` or `internal/` modifications from this group
- [ ] 6.4 Verify in a cluster: with a private package and a named pull secret,
      manager, adapter, and runtime pods all pull; then confirm the default
      (empty) rendering is byte-identical to today's ServiceAccounts

## 7. Registry cut-over

- [ ] 7.1 Repoint chart image defaults to
      `ghcr.io/kostiantyn-matsebora/agentops-*`: `image.repository`,
      `telegramAdapter.image.repository`, `demo.runtimeImage`, and
      `chart/charts/vm-bundle/values.yaml` `alertmanager.image.repository` —
      keep existing tags, change only the registry prefix
- [ ] 7.2 Publish the current versions of all five images to GHCR
      (`manager-v0.13.1`-equivalent onward) so the repointed defaults resolve;
      leave existing Docker Hub tags untouched
- [ ] 7.3 Bump `chart/Chart.yaml` version and cut `chart-v<new>`; verify the
      image-existence gate passes on the new defaults
- [ ] 7.4 Grep for remaining `kmatsebora/` references across `chart/`,
      `config/samples/`, `README.md`, `CLAUDE.md`, and the Dockerfile header
      comments; repoint or annotate each

## 8. Docs and maintenance

- [ ] 8.1 Update the README install section: the OCI `helm install` command, the
      pull-secret prerequisite for private packages, and the GHCR image names
- [ ] 8.2 Write the registry cut-over migration note (Docker Hub → GHCR, the
      pull-secret recipe, and the `--set image.repository=` escape hatch) into
      `CHANGELOG.md` if `organize-docs` has landed, otherwise as a README
      migration section
- [ ] 8.3 Replace the manual `docker build` block in `CLAUDE.md` with the
      tag-driven release flow (tag grammar, what each tag publishes, the
      immutability guard) and note that CI enforces the never-overwrite rule
- [ ] 8.4 Add `.github/dependabot.yml` for `github-actions` (weekly) and the
      four `gomod` directories (weekly)
- [ ] 8.5 Record the two owner decisions left open in the README or CLAUDE.md as
      appropriate: GHCR package visibility per package (private + pull secret is
      the shipped default) and the unresolved LICENSE, which must be settled
      before any package is made public
