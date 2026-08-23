## 1. Build hygiene (no CI yet)

- [x] 1.1 Add `.dockerignore` excluding `.git/`, `openspec/`, `chart/`, `docs/`,
      `config/`, and every submodule directory — the root manager build copies
      only `go.mod`, `go.sum`, `api/`, `cmd/`, `internal/`; verify
      `docker build .` still succeeds afterwards
- [x] 1.2 Write down the per-component PLATFORM DECLARATION the workflows read:
      `linux/amd64,linux/arm64` for ALL THIRTEEN. No exceptions today — the
      mechanism stays for a future vendor image that genuinely has one. No
      Dockerfile change: the twelve Go images cross-compile, and
      `runtime-claude` was verified to build and run on arm64
- [x] 1.3 Verify locally that the declaration is achievable: one multi-arch
      `docker buildx build --platform linux/amd64,linux/arm64` per Go module
      completes, and `runtime-claude` builds amd64
- [x] 1.4 Collapse the NINE byte-identical Dockerfiles into
      `.github/docker/go-module.Dockerfile`, reached with `-f` and each
      component's OWN directory as the context. The tell was an edit to all
      thirteen headers that had to be SCRIPTED — nine copies of one file is nine
      places for a base-image bump to be applied in eight of. `components.sh`
      discovery becomes the UNION of Dockerfile-bearing and `go.mod`-bearing
      directories, an own Dockerfile winning; verify all thirteen still resolve
      and all nine still build

## 2. CI workflow

- [x] 2.1 Create `.github/workflows/ci.yml` triggered on `pull_request` and
      `push: master`, with `permissions: {contents: read}` and concurrency
      cancel-in-progress per ref
- [x] 2.2 Job `operator`: `actions/setup-go@v5` with `go-version-file: go.mod`,
      Go build cache, `actions/cache` on `~/.envtest`, `setup-envtest use 1.31.x`,
      then `go build ./... && go vet ./...` and `go test ./...` with
      `KUBEBUILDER_ASSETS` — confirm the `internal/integration` suite runs
      rather than skipping
- [x] 2.3 Job `modules`: matrix over every submodule with a `go.mod`
      (`channel-telegram`, `gateway-telegram`, `signal-telegram`, `signal-cron`,
      `signal-alertmanager`, `signal-k8s-events`, `signal-ha`, `console`,
      `context-sync`, `housekeeping`, `egress-proxy`) running build + vet + test
      in each module dir. Derive the matrix from the `go.mod` files present, so
      module number twelve needs no workflow edit
- [x] 2.4 Job `console-ui`: `npm ci`, `npm test` and `npm run build` in
      `console/ui`, cached on the lockfile. Screenshots are out of scope — they
      need a browser and are regenerated when the UI changes, not per PR
- [x] 2.5 Job `chart`: `helm lint`, then `helm template` for default,
      `global.demo.enabled=true`, each of `k8s-bundle`, `prometheus-bundle`,
      `ha-bundle` and `telegram-bundle` **with the values that bundle requires**
      (`ha-bundle` alone fails to render without a Home Assistant credential —
      confirm each permutation reaches the templates rather than a guard),
      `console.enabled=false`, and everything on at once
- [x] 2.6 Extend the `chart` job with `kubeconform` in strict mode over each
      rendered permutation, passing `chart/files/crds/` as additional schemas so
      CRs are validated against the repo's own CRDs
- [x] 2.7 Job `images`: matrix over the thirteen Dockerfiles,
      `docker/build-push-action` with `push: false`, `platforms: linux/amd64`,
      and GHA layer cache. Derive the matrix from the Dockerfiles present
- [ ] 2.8 Open a scratch PR and confirm all jobs run and pass; deliberately
      break one module and one UI test and confirm each failure is attributed to
      the thing that broke — STILL OWED, because it needs a runner. Every job's
      COMMANDS were run by hand first (operator incl. 343 envtest cases, eleven
      submodules, the console UI, seven chart permutations under kubeconform,
      the docs drift check, thirteen image builds) and that found a real defect:
      the chart job failed on eleven CRDs in every permutation, since
      kubeconform treats a missing schema as an error and its strict set has
      none for CustomResourceDefinition

## 3. Reusable image publish workflow

- [x] 3.1 Create `.github/workflows/build-image.yml` as `workflow_call` with
      inputs `component`, `context`, `dockerfile`, `platforms`, `version` and
      `permissions: {contents: read, packages: write, id-token: write,
      attestations: write}` — declared on the CALLING job too, since a called
      workflow can only downgrade the token and `contents: read` alone makes
      every push 403. QEMU is registered unconditionally: the twelve Go images
      cross-compile and never touch it, and `runtime-claude` runs `apt` and
      `npm` AS arm64, so that half of its build is emulated
- [x] 3.2 Preflight immutability guard: query
      `ghcr.io/kostiantyn-matsebora/agentops-<component>:<version>` and fail
      before building if it resolves, with a message saying to cut a new patch
      version rather than re-push
- [x] 3.3 Steps: `docker/setup-buildx-action`, `docker/login-action` against
      `ghcr.io` with `GITHUB_TOKEN`
- [x] 3.4 `docker/metadata-action` producing the exact version tag only (no
      `latest`) plus OCI labels including
      `org.opencontainers.image.source` so the package links to the repo
- [x] 3.5 `docker/build-push-action` with the input platforms, `push: true`,
      `provenance: mode=max`, `sbom: true`, and GHA layer cache
- [x] 3.6 Post-push verification: `docker buildx imagetools inspect` asserts the
      pushed manifest's platforms EQUAL the component's declaration. Equality,
      not containment — an image that lost an arch and one that gained an
      undeclared one both fail. This is the check that would have caught the
      amd64-only console before it reached a cluster
- [x] 3.7 Prove the assertion fails: build one component for a single arch on
      purpose and confirm the job rejects it — done against REAL pushed
      manifests, both directions: an image that LOST an arch and one that GAINED
      an undeclared one are each rejected, so the check is equality rather than
      containment. The immutability preflight was proven in the same pass, on a
      live tag. Run by hand rather than in the job, because the account's
      Actions minutes were unavailable

## 4. Release entry point

- [x] 4.1 Create `.github/workflows/release.yml` on `push: tags: ['*-v*']`
- [x] 4.2 Job `determine`: split the ref at the last `-v`, validate the
      component against the known set and the version against a semver regex,
      emit `component`/`version` outputs, and fail with the valid tag forms
      listed when either is invalid (never skip silently)
- [x] 4.3 One image job per component calling `build-image.yml`, each gated on
      `needs.determine.outputs.component`, with the right context/dockerfile and
      the component's platform declaration from 1.2
- [ ] 4.4 End-to-end proof: cut `manager-v<next-patch>`, confirm the image
      publishes, both arches are present, and a re-push of the same tag is
      rejected by the guard
- [ ] 4.5 Second proof on the image that was single-arch by accident: cut
      `runtime-claude-v<next-patch>`, confirm it publishes BOTH arches, and pull
      it on an arm64 node

## 5. Chart publishing

- [x] 5.1 Add the `chart` job to `release.yml`: assert `Chart.yaml` `version`
      equals the tag version, failing with both values on mismatch
- [x] 5.2 Add the image-existence gate: resolve every first-party image the
      chart renders by default — six in `chart/values.yaml` (manager, console,
      housekeeping, `runtime.image`, `contextSync`, `egressMediation`) and six
      across the bundles (telegram router/channel/signal, alertmanager,
      k8s-events, home-assistant) — against GHCR and fail naming any missing
      reference. Exclude anything outside this project's namespace, by
      namespace rather than by a hardcoded vendor list
- [x] 5.3 Immutability guard for the chart tag, then `helm package chart/` and
      `helm push` to `oci://ghcr.io/kostiantyn-matsebora/charts`
- [ ] 5.4 Verify: `helm show chart oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator --version <v>`
      resolves, and a `helm install` into a scratch namespace succeeds

## 6. Public packages

- [x] 6.1 Settle the LICENSE and replace "License TBD" in `README.md`. This
      GATES the first publish: public packages publish the built binaries
- [x] 6.2 After each component's first release, flip its GHCR package to public
      (thirteen images plus the chart). `GITHUB_TOKEN` cannot do this — it is a
      manual per-package step, recorded as a checklist beside the tag grammar
- [x] 6.3 Confirm the chart needs NO pull-secret value and no ServiceAccount
      change: `git status` shows no `api/`, `internal/` or ServiceAccount
      template modifications from this group
- [x] 6.4 Verify anonymously: `docker pull` each published image and
      `helm show chart oci://…` with no credential configured, then install into
      a scratch cluster holding no pull secret and confirm manager, adapter,
      runtime, sidecar and job pods all pull

## 7. Registry cut-over

- [x] 7.1 Repoint every first-party image default to
      `ghcr.io/kostiantyn-matsebora/agentops-*` — the six in
      `chart/values.yaml` and the six across `chart/charts/*/values.yaml` —
      keeping existing tags and changing only the registry prefix
- [x] 7.2 Publish the current version of every image to GHCR (manager 0.38.1,
      console 0.16.0, and each module's current tag) so the repointed defaults
      resolve; leave existing Docker Hub tags untouched
- [ ] 7.3 Bump `chart/Chart.yaml` version and cut `chart-v<new>`; verify the
      image-existence gate passes on the new defaults
- [x] 7.4 Grep for remaining `kmatsebora/` references across `chart/`,
      `config/samples/`, `README.md`, `CLAUDE.md`, `docs/`, and every
      Dockerfile header comment; repoint or annotate each

## 8. Docs and maintenance

- [x] 8.1 Update `docs/installation.md` (the OCI `helm install` command and the
      GHCR image names — no credential step, the packages are public) and keep
      `README.md` to the commands alone, within its 150-line budget
- [x] 8.2 Write the registry cut-over migration entry (Docker Hub → GHCR and
      the `--set image.repository=` escape hatch for staying on Docker Hub) into
      `CHANGELOG.md`, newest first
- [x] 8.3 Replace the manual `docker build` block in `CLAUDE.md` with the
      tag-driven release flow (tag grammar, what each tag publishes, the
      immutability guard) and note that CI enforces the never-overwrite rule
- [x] 8.4 Add `.github/dependabot.yml` for `github-actions` (weekly), the twelve
      `gomod` directories (weekly), and `console/ui`'s npm manifest (weekly)
- [x] 8.5 Correct `CLAUDE.md`'s module and image inventory in the same pass:
      the opening line still says "Nine Go modules" and lists nine, and
      **`platform/egress-proxy/` appears nowhere in that file** — not in the count, not
      in the Map, and not in the image build block this change replaces, so
      anyone following it today ships twelve of the thirteen images. Add it
      beside `platform/context-sync/` and `platform/housekeeping/`, and state the count as what it
      is
- [x] 8.6 Correct `CLAUDE.md`'s multi-arch section: "The exception is a runtime
      whose UPSTREAM is single-arch. `runtime-claude` is the case" is FALSE —
      that image builds and runs on arm64. Remove the exception, and note that
      the chart's runtime `nodeSelector` was compensating for a single-arch
      image rather than for the vendor, so it becomes optional once the
      multi-arch image ships (relaxing it is the operator's call, and comes
      after)
- [x] 8.7 Record in `CLAUDE.md` that publishing a NEW image is three things,
      not one: the tag, the package visibility flip, and the platform
      declaration the release asserts — a component that skips any of them
      fails at a different layer

## 9. Publication hygiene guard

This section lands BEFORE any cleanup change is written, and that ordering is
the whole point. A change that removes identifying content has to NAME what it
removes; archived, that naming is republished by the very repository it was
cleaning. With the guard already in CI, an artifact that names a literal fails
the build instead of reaching the archive.

- [x] 9.1 Write the guard as an ALLOWLIST of permitted shapes — reserved example
      domains, cluster-internal service names, loopback, this repository's own
      clone URL, the documented placeholder identifiers, and a documented set of
      private-range address literals. Never a list of forbidden strings: a
      denylist publishes what it protects and catches only what someone already
      thought of
- [x] 9.2 Scope it to the WHOLE repository — `openspec/`, `docs/`, `chart/`,
      every module, and the agent context under `.claude/` — with binary and
      lockfile exclusions declared in the allowlist rather than hardcoded
- [x] 9.3 Extend it over the commit MESSAGES in the range under review. Messages
      live outside the tree, so a tree-only guard leaves the one hole that
      cannot be fixed by editing a file later
- [x] 9.4 Report shape: file, line and the rule violated — never the matched
      text. A public repository has public build logs, so a guard that quotes
      its findings leaks them to the audience it exists to protect the tree
      from. A `--show` flag prints matches for local fixing
- [x] 9.5 Wire it into `ci.yml` as its own job, required on every pull request
      and on pushes to `master`
- [x] 9.6 Prove it fails, and prove the report is clean: add a scratch commit
      carrying an out-of-allowlist hostname in a file and another in a commit
      message, confirm both fail, and confirm the CI log names positions and
      rules without reproducing either value
- [x] 9.7 Run it over the repository as it stands and record ONLY the count of
      violations per rule. The list of actual findings is a local artifact and
      is never committed — it is the input to the cleanup change, not a document
- [x] 9.8 Record in `.claude/rules/` the one rule this creates for every future
      change: verification is recorded as "the guard passes", never as the text
      it matched. Pasting a grep result into a ticked task is the most natural
      way to reintroduce exactly what was removed
