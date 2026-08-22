## Build / test

```sh
go build ./... && go vet ./...
for m in channel-telegram telegram-router signal-telegram signal-cron \
         signal-alertmanager signal-k8s-events signal-ha; do
  (cd $m && go build ./... && go vet ./... && go test ./...)
done
# regen after editing api/v1alpha1/ (deepcopy + CRDs):
go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5 object paths=./api/...
go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5 crd paths=./api/... output:crd:artifacts:config=chart/files/crds
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

**Use `buildx --platform linux/amd64,linux/arm64 --push`.**
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
docker buildx build --platform linux/arm64 ./runtime-claude/     # succeeds
docker run --platform linux/arm64 … claude --version
arch: aarch64 / v22.23.2 / 2.1.239 (Claude Code)
```

`node:22-bookworm-slim` plus apt plus one npm global is multi-arch throughout.

**The constraint was the `--platform linux/amd64` in the build command below.**
The runtime `nodeSelector` the chart ships compensates for a build flag rather
than for the vendor, and can be relaxed once a multi-arch runtime image is
published.

**A component may still be single-arch one day.** Establish that by BUILDING it
on the other architecture and running the binary, never by inheriting a claim
from prose — including this prose.

**Bump the tag on every change. Never overwrite a pushed tag.**

```sh
# MULTI-ARCH, and --push in the same command: buildx cannot export a
# multi-platform result to the local daemon, so a separate `docker push` would
# silently ship whichever single arch got loaded.
BX="docker buildx build --platform linux/amd64,linux/arm64 --push"
$BX -t <registry>/agentops-manager:<tag> .
$BX -t <registry>/agentops-channel-telegram:<tag> ./channel-telegram/
$BX -t <registry>/agentops-telegram-router:<tag> ./telegram-router/
$BX -t <registry>/agentops-signal-telegram:<tag> ./signal-telegram/
$BX -t <registry>/agentops-signal-cron:<tag> ./signal-cron/
$BX -t <registry>/agentops-signal-alertmanager:<tag> ./signal-alertmanager/
$BX -t <registry>/agentops-signal-k8s-events:<tag> ./signal-k8s-events/
$BX -t <registry>/agentops-signal-ha:<tag> ./signal-ha/
$BX -t <registry>/agentops-console:<tag> ./console/
$BX -t <registry>/agentops-context-sync:<tag> ./context-sync/
$BX -t <registry>/agentops-egress-proxy:<tag> ./egress-proxy/
$BX -t <registry>/agentops-housekeeping:<tag> ./housekeeping/
$BX -t <registry>/agentops-runtime-claude:<tag> ./runtime-claude/

# VERIFY before believing it — the failure mode is invisible until it schedules:
docker manifest inspect <registry>/agentops-console:<tag> \
  | jq -r '.manifests[].platform | "\(.os)/\(.architecture)"'
```

Then:

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
