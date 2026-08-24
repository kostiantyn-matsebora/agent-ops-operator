## Build / test

```sh
# EVERY module, discovered rather than listed — the same answer CI's matrix gets.
for m in $(.github/components.sh modules | jq -r '.[]'); do
  (cd $m && go build ./... && go vet ./... && go test ./...)
done
# the operator lives in platform/manager/, and everything below runs from there:
cd platform/manager
# regen after editing api/v1alpha1/ (deepcopy + CRDs). The chart is four levels
# up now, which is also why the integration suite names that path once:
go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5 object paths=./api/...
go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5 crd paths=./api/... output:crd:artifacts:config=../../chart/crds
# full tests (unit + envtest against a real API server):
KUBEBUILDER_ASSETS=$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.19 use 1.31.x --bin-dir ~/.envtest -p path) go test ./...
```

### No local Go: use a PERSISTENT container, not `docker run --rm`

**This workstation has no Go toolchain**, so every command above runs in a
container.

**Start ONE long-lived container and `docker exec` into it.** A throwaway
`docker run --rm` pays container setup per invocation and throws the build cache
away with it — warm rebuilds are ~2s through `exec` and are not through
`run --rm`.

```sh
docker volume create agentops-gomodcache; docker volume create agentops-gocache
# volumes are created ROOT-owned; chown once or every write fails as your uid
docker run --rm -u 0 -v agentops-gomodcache:/gomodcache -v agentops-gocache:/gocache \
  golang:1.23 chown -R "$(id -u):$(id -g)" /gomodcache /gocache
docker run -d --name agentops-go -u "$(id -u):$(id -g)" \
  -v "$PWD":"$PWD" -w "$PWD" \
  -v agentops-gocache:/gocache -v agentops-gomodcache:/gomodcache \
  -e GOCACHE=/gocache -e GOMODCACHE=/gomodcache \
  -e HOME=/tmp -e GOFLAGS=-buildvcs=false \
  golang:1.23 sleep infinity
# then, for every go command (-w keeps submodules working):
docker exec -i -w "$PWD" agentops-go go build ./...
```

Four details, each of which cost a debugging round:

- **The caches MUST be named volumes, never host bind mounts.** The module cache
  does heavy rename and hardlink work, and bind mounts through the Rancher
  Desktop VM corrupt it — every package fails with `zip: not a valid zip file`
  on a cache that was written seconds earlier. The repo itself is still a bind
  mount, because it must be edited from the host.
- **Run as the invoking uid.** `controller-gen` writes deepcopy and CRDs INTO
  the repo, and a root-owned generated file is a mess to undo.
- **Mount the repo at its REAL path**, not `/src`. Compiler diagnostics then
  carry paths that resolve on the host.
- **`go clean -modcache` fails** (`unlinkat //gomodcache: permission denied`) —
  it tries to remove the mount point. Remove the VOLUME instead.

**A VM-BACKED DAEMON MOUNTS YOUR HOME, NOT `/tmp`.** Rancher Desktop runs the
daemon in a VM, so `-v /tmp/whatever:/data` bind-mounts an EMPTY directory.

- **The container runs, finds nothing, writes nothing and often says nothing.**
  It reads as a broken image rather than a missing mount.
- **`docs/diagrams/export.py` builds its scratch directory BESIDE ITSELF** for
  exactly this reason. Anything else running a container over generated files
  must do the same.
- **`docker pull` from a non-interactive session fails** with
  `gpg: decryption failed`, before it ever reaches the registry — the `pass`
  credential helper needs an unlocked gpg agent.

Two traps that are not the container's fault but look like it:

- **`go build ./...` piped into `tail` reports `tail`'s exit code.** Check
  `${PIPESTATUS[0]}` or redirect to a file.
- **`openspec` needs Node**, which is likewise not installed system-wide —
  `~/.local/opt/node`, symlinked into `~/.local/bin`.

### EVERY IMAGE IS MULTI-ARCH WHEREVER IT CAN BE

**Every published manifest carries `linux/amd64` AND `linux/arm64`.**
Never `docker build --platform linux/amd64`.

**A single-arch image fails at SCHEDULE TIME, not at build, push or render** —
possibly weeks later, looking like an unrelated incident:

```
failed to pull and unpack image "...": no match for platform in manifest: not found
```

- **An amd64-only `agentops-console` did exactly that on 2026-08-21.** It had
  run for weeks only because every reschedule landed on an amd64 node; the first
  that did not left the console in `ImagePullBackOff`.
- **Nothing in the chart or the CR was wrong.** The image had no arm64 half.
- **Every adapter here is dependency-free Go** and cross-compiles for free, so
  there is no reason to ship one arch.

**THERE IS NO EXCEPTION.** This page used to name one — "a runtime whose
UPSTREAM is single-arch, `runtime-claude` is the case" — and it was wrong.
Building the image settles it:

```
docker buildx build --platform linux/arm64 ./runtimes/claude/     # succeeds
docker run --platform linux/arm64 … claude --version
arch: aarch64 / v22.23.2 / 2.1.239 (Claude Code)
```

`node:22-bookworm-slim` plus apt plus one npm global is multi-arch throughout.

**The constraint was the `--platform linux/amd64` in the hand-run build
command.** The runtime `nodeSelector` the chart ships compensates for a build
flag rather than for the vendor, and can be relaxed once a multi-arch runtime
image is published.

**A component may still be single-arch one day.** Establish that by BUILDING it
on the other architecture and running the binary, never by inheriting a claim
from prose — including this prose. The declaration lives in the `SINGLE_ARCH`
map in `.github/components.sh`, which is EMPTY, and the release asserts what it
pushed against it as EQUALITY — an image that lost an arch and one that gained
an undeclared one both fail.

### PUBLISHING IS A GIT TAG — WHILE CI RUNS. ON A PRIVATE REPO IT DOES NOT

**A tag pushed against a private repo publishes NOTHING, and says nothing.**
There is no workflow run, so the absence is indistinguishable from a build still
queued.

- **Check the REGISTRY, never the tag**, before believing an image shipped.
- **The hand build is the buildx push below**, and it stays MULTI-ARCH — the
  cluster is mixed, and a single-arch image fails at SCHEDULE time.
- **A credential is NEVER read to test whether auth works.**
  `docker-credential-* get` prints the secret. Attempt the push and read the
  error. See `gotchas.md`.

Everything below is the CI path, and it is what holds once the repo is public.

**`<component>-v<semver>` publishes exactly that component**, and `chart-v<semver>`
publishes the chart. The component name is the one `.github/components.sh`
derives from the directory, so a tag names a path.

```sh
git tag manager-v0.44.1 && git push origin manager-v0.44.1
git tag chart-v7.0.1    && git push origin chart-v7.0.1
```

- **`.github/workflows/release.yml` splits the ref at the last `-v`**, validates
  the component against the derived list and the version against semver, and
  FAILS naming the valid forms when either is wrong. A typo'd tag never
  silently publishes nothing.
- **`build-image.yml` refuses a tag already in the registry**, before building.
  This repo's "never overwrite a pushed tag" rule is a gate now, not a note, and
  the recovery from a partial failure is a NEW PATCH VERSION.
- **Images go to `ghcr.io/kostiantyn-matsebora/agentops-<component>`**, the
  chart to `oci://ghcr.io/kostiantyn-matsebora/charts`. Docker Hub
  (`kmatsebora/*`) is frozen, not deleted.

**PUBLISHING A NEW IMAGE IS FOUR THINGS, AND EACH FAILS AT A DIFFERENT LAYER:**

| Thing | Skipped, it fails | Where |
|---|---|---|
| the **tag** `<component>-v<semver>` | nothing publishes | no workflow run at all |
| the package's **Actions access** for this repo | `denied: permission_denied: write_package` | in the release job, at the push |
| the **package visibility flip** to public | `ImagePullBackOff` for whoever installs | in a cluster, later |
| the **platform declaration** in `.github/components.sh` | the post-push equality assert | in the release job |

**ACTIONS ACCESS IS NOT THE REPOSITORY LINK, AND THE ERROR BLAMES THE TOKEN.**
`LABEL org.opencontainers.image.source` connects a package to a repository for
DISPLAY. Whether a workflow may WRITE it is a separate list on the package, and
a package first published by a hand `docker push` starts with that list empty.

- **The failure reads as a credentials problem and is not one.** On 2026-08-24
  every release failed at the push while the job log said `Packages: write` and
  `docker/login-action` said `Login Succeeded!`. Authentication was fine;
  authorization ON THE PACKAGE was not.
- **CHECK THE JOB LOG'S `GITHUB_TOKEN Permissions` GROUP FIRST.** It states the
  token's actual scopes, so it settles in one line whether the workflow or the
  package is at fault — reading the workflow YAML does not.
- **UI ONLY, once per package, forever:** Package settings → *Manage Actions
  access* → add the repository with **Write**. The Packages REST API lists,
  gets, deletes and restores; it manages no access, and neither does GraphQL.
- **A package FIRST published by the workflow inherits the repository's access**,
  so this bites only packages that already existed because somebody pushed them
  by hand. Thirteen did here.

- **A GHCR package is created PRIVATE by its first push**, and `GITHUB_TOKEN`
  cannot change that. Flipping it is a manual step, once per package, and the
  chart's image-existence gate is what catches a forgotten one — it resolves
  every first-party reference ANONYMOUSLY at chart-release time.
- **THERE IS NO API FOR VISIBILITY.** The Packages REST API lists, gets, deletes
  and restores; it does not set visibility, and neither does GraphQL. The UI is
  the only path, deliberately — it is the action that makes something
  world-readable.
- **THE REPOSITORY LINK IS A `LABEL` IN THE DOCKERFILE**, and GitHub's own
  instruction is literal about where it goes:

  ```dockerfile
  LABEL org.opencontainers.image.source=https://github.com/OWNER/REPO
  ```

  - **Passing it as `--label` on the build command is NOT the same thing**, and
    that is what cost thirteen packages their link. The value really does land
    in the image config either way — verified on the pushed image — so the
    mistake looks correct from every angle except the one that matters.
  - **A MULTI-ARCH PUSH IS AN INDEX, and a label reaches the per-architecture
    CONFIGS, not the index.** `build-image.yml` therefore also sets
    `annotations: index:org.opencontainers.image.source=…`. Both halves, because
    only one of them was ever going to be the reason.
  - **A RE-PUSH DOES REPAIR AN UNLINKED PACKAGE**, so connecting one by hand is
    a fallback rather than the fix. Pushing the same version again, carrying the
    annotation, linked a package that had been sitting unlinked — no deletion,
    no new version number.
    - **Established by testing it on a package NOBODY HAD TOUCHED.** The first
      attempt proved nothing: that package had already been connected by hand,
      so it would have shown linked either way.
    - **Thirteen were repaired this way in one pass**, which is the difference
      between thirteen clicks and none.
- **Visibility and the link are ONCE PER PACKAGE, EVER.** Both live on the
  package, not the version, so every later tag inherits them.
- **A component that skips any of the three looks fine at the layer it was
  skipped in.** That is why they are listed together rather than in three
  places.

**Building locally is still ordinary**, and it does not publish:

```sh
# no --push: buildx cannot export a multi-platform result to the local daemon,
# so this verifies the build rather than shipping it.
#
# -f IS NOT OPTIONAL. Nine components have no Dockerfile of their own — they are
# built by the shared recipe at .github/docker/go-module.Dockerfile, with their
# OWN directory as the context. components.sh answers both, per component.
.github/components.sh images |
  jq -r '.[] | "\(.component) \(.context) \(.dockerfile)"' |
  while read -r component context dockerfile; do
    docker buildx build --platform linux/amd64,linux/arm64 \
      -f "$dockerfile" -t "agentops-$component:dev" "$context"
  done
```

- **`docker build ./signals/cron` on its own NO LONGER WORKS**, and that is the
  price of the collapse. The context is unchanged, the recipe moved.
- **The shared image's entrypoint is `/app`**, not `/signal-cron`. Exec-form
  `ENTRYPOINT` cannot expand a build argument and distroless has no shell, so a
  per-component path was never available.

After a release, to move an install onto it:

1. **Update the image refs** — chart values for the manager, `AgentRuntime` CRs
   for runtimes.
2. **`helm upgrade`.**
3. **Verify with a live task.** A task is an ordinary signal to a source a Ready
   Pipeline claims. There is no `/task` endpoint.

```sh
TOKEN=$(kubectl -n <ns> get secret agentops-adapter-token \
  -o jsonpath='{.data.token}' | base64 -d)
curl -sX POST http://<manager>:8080/signal/inbound -H "Authorization: Bearer $TOKEN" \
  -d '{"source":"<src>","signals":[{"fingerprint":"smoke-1","kind":"task","payload":"..."}]}'
```

Point the claiming Pipeline at a stub runtime and it costs no LLM.
