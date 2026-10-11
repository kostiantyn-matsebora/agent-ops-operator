## Build / test

```sh
# every module, discovered rather than listed:
for m in $(.github/components.sh modules | jq -r '.[]'); do
  (cd $m && go build ./... && go vet ./... && go test ./...)
done
cd platform/manager    # the operator
# regen after editing api/v1alpha1/ (deepcopy + CRDs):
go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5 object paths=./api/...
go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5 crd paths=./api/... output:crd:artifacts:config=../../chart/crds
# full tests, envtest against a real API server:
KUBEBUILDER_ASSETS=$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.19 use 1.31.x --bin-dir ~/.envtest -p path) go test ./...
```

**Two more tiers sit behind build tags:**

```sh
cd platform/manager
go test -tags conformance -count=1 -v ./test/conformance/   # no cluster
go test -tags system -count=1 -timeout 45m -v ./test/system/  # needs k3d
```

- **No docker/k3d? Compile inside the Go container, run outside it** —
  a static binary needs no toolchain:
  ```sh
  docker exec -i -w "$PWD" agentops-go \
    sh -c 'CGO_ENABLED=0 go test -c -tags system -o /tmp/system.test ./test/system/'
  docker cp agentops-go:/tmp/system.test /tmp/system.test
  (cd test/system && SYSTEM_REUSE=1 /tmp/system.test -test.v -test.timeout 45m)
  ```
- `docs/testing.md` owns the tier model.
- Node runtimes: `cd runtimes/claude && node --test` (same for `copilot`).

### Coverage, with the flags CI uses

| Toolchain | Command | Writes |
|---|---|---|
| Go | `go test -count=1 -coverpkg=./... -coverprofile=coverage.out ./...` | `coverage.out` |
| vitest (console UI) | `npm run test:coverage` | `coverage/lcov.info` |
| `node --test` | `node --test --experimental-test-coverage --test-reporter=lcov --test-reporter-destination=coverage.lcov` | `coverage.lcov` |
| Python scripts | `COVERAGE_PROCESS_START=... run.sh`, then `coverage combine && xml` | `.github/coverage.xml` |

- **Always pass `-coverpkg=./...`** — without it a profile only credits the
  binary's own package.

### ONE PERSISTENT CONTAINER, NEVER `docker run --rm` PER COMMAND

```sh
docker volume create agentops-gomodcache
docker volume create agentops-gocache
docker run --rm -u 0 -v agentops-gomodcache:/gomodcache -v agentops-gocache:/gocache \
  golang:1.27 chown -R "$(id -u):$(id -g)" /gomodcache /gocache
docker run -d --name agentops-go -u "$(id -u):$(id -g)" \
  -v "$PWD":"$PWD" -w "$PWD" \
  -v "$(dirname "$PWD")/agent-ops-worktrees":"$(dirname "$PWD")/agent-ops-worktrees" \
  -v agentops-gocache:/gocache -v agentops-gomodcache:/gomodcache \
  -e GOCACHE=/gocache -e GOMODCACHE=/gomodcache \
  -e HOME=/tmp -e GOFLAGS=-buildvcs=false \
  golang:1.27 sleep infinity
docker exec -i -w "$PWD" agentops-go go build ./...   # -w keeps submodules working
```

- Mount BOTH the repo and `../agent-ops-worktrees` — a worktree's `.git` is
  a file into this repo's `.git/worktrees/`.
- Named VOLUMES only, never bind mounts (corrupt under Rancher Desktop).
  `go clean -modcache` fails on the mount — remove the volume instead.
- `golang:1.27` only — 1.23/1.25 retired for fixable stdlib CVEs.

### EVERY IMAGE IS MULTI-ARCH. NO EXCEPTION

Every manifest carries `linux/amd64` AND `linux/arm64`, never only
`--platform linux/amd64` — a single-arch image fails at SCHEDULE time,
possibly weeks later. `SINGLE_ARCH` in `.github/components.sh` is EMPTY.

### PUBLISHING IS A GIT TAG — THE REPOSITORY IS PUBLIC

```sh
git tag manager-v0.44.1 && git push origin manager-v0.44.1
git tag chart-v7.0.1    && git push origin chart-v7.0.1
```

- Never read a credential to test auth — attempt the push instead.

| Failure (UI-only fix, once per package) | Symptom |
|---|---|
| no tag pushed | nothing publishes |
| Actions access not granted | `denied: write_package` |
| package still PRIVATE | `ImagePullBackOff` |

- A re-push repairs an unlinked `image.source` label. Partial-publish
  recovery is a NEW PATCH VERSION.

Local build only (no `--push`):

```sh
.github/components.sh images |
  jq -r '.[] | "\(.component) \(.context) \(.dockerfile)"' |
  while read -r component context dockerfile; do
    docker buildx build --platform linux/amd64,linux/arm64 \
      -f "$dockerfile" -t "agentops-$component:dev" "$context"
  done
```

`python3 .github/scripts/docs-generate.py --check` fails on a stale chart
version or image tag — a release isn't done until the docs print its number.

Verify with an ordinary signal (never a `/task` endpoint — there is none):

```sh
TOKEN=$(kubectl -n <ns> get secret agentops-adapter-token -o jsonpath='{.data.token}' | base64 -d)
curl -sX POST http://<manager>:8080/signal/inbound -H "Authorization: Bearer $TOKEN" \
  -d '{"source":"<src>","signals":[{"fingerprint":"smoke-1","kind":"task","payload":"..."}]}'
```
