## 1. Placeholder CRD type — backend-developer

- [x] 1.1 Add a new, explicitly placeholder-named kind (e.g.
  `ControlPlaneProbe`) under `platform/manager/api/v1alpha1/`, spec-only
  (name + key/value pair, no status subresource — mirror
  `mcptoolset_types.go`'s shape). The type's doc comment states "disposable
  scaffolding, to be replaced, not extended" in its first sentence. Verify:
  `go build ./...` in `platform/manager`.
- [x] 1.2 Regenerate deepcopy and CRD YAML:
  `go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5 object
  paths=./api/...` then the matching `crd` invocation targeting
  `../../chart/crds`. Verify: `chart/crds/` contains the new kind's YAML and
  `git diff` shows only generated files plus the new type.

## 2. Control-plane HTTP surface — backend-developer

- [x] 2.1 Add `platform/manager/internal/control-plane/` with the
  constant-time bearer-token middleware, reading `CONTROL_PLANE_TOKEN` once
  at process start, following the shape of `adapterAuth`/`coordinate`'s
  token checks in `internal/httpapi/server.go`. An empty/unset token refuses
  every request with 503. Verify: a unit test for empty-token-503,
  wrong-token-401, correct-token-pass.
- [x] 2.2 Add the placeholder's CRUD handlers (create-or-update-by-name,
  read-by-name, 404 on unknown name) in the same package, operating on the
  CRD from 1.1 through the manager's existing controller-runtime client.
  Verify: a unit test for write-then-read, update-overwrites, read-unknown
  404, and a `PUT` whose body `name` differs from the route's `{name}`
  (refused, nothing stored).
- [x] 2.3 Register `/control-plane/*` routes on the mux in
  `internal/httpapi/server.go` (or wherever `cmd/manager/main.go` wires the
  server), guarded by the new middleware, beside the existing
  `/signal/*`/`/channel/*`/`/work`/`/coordinate/*` groups. Verify: a unit
  test asserting `ADAPTER_TOKEN` and the coordinator token both get 401 on
  `/control-plane/*`, and `CONTROL_PLANE_TOKEN` gets 401 on the other route
  groups. Also a unit test that a valid `CONTROL_PLANE_TOKEN` requesting a
  kind with no handler gets 404, and no other kind's object is created, read
  or modified.
- [x] 2.4 Add `CONTROL_PLANE_TOKEN` to the manager's env wiring in
  `cmd/manager/main.go` (or the server's config struct), alongside the
  existing `os.Getenv("ADAPTER_TOKEN")` reads. Verify: `go build ./...`.

## 3. Chart and RBAC — deployment-engineer

- [x] 3.1 Add the new placeholder kind to the manager's existing `Role` in
  `chart/templates/rbac.yaml`, same full verb set
  (`get`/`list`/`watch`/`create`/`update`/`patch`/`delete`) every other
  manager-owned kind already has there. No new ServiceAccount, Role, or
  binding. Verify: `helm template` (or the chart's render tests) shows the
  new resource name in the existing manager `Role`, and no new RBAC object.
- [x] 3.2 Add a `controlPlaneAuth` values block (`existingSecret` /
  `secretName`, key `token`) mirroring `adapterAuth`'s shape, and wire
  `CONTROL_PLANE_TOKEN` into `chart/templates/deployment.yaml`'s manager
  container via `secretKeyRef`, next to the existing `ADAPTER_TOKEN` entry.
  Verify: `helm template --set controlPlaneAuth.secretName=test` renders the
  env var and a Secret, same as the `adapterAuth` case does today.
- [x] 3.3 Render the new CRD under `chart/crds/` (carried from 1.2) and
  confirm `helm template` does not error on an unknown kind. Verify:
  `helm template --dry-run=server` against a cluster with the CRD applied,
  per `chart.md`'s CRD-install-order note.

## 4. Unit tests

- [x] 4.1 `go build ./... && go vet ./... && go test ./...` passes in
  `platform/manager`, covering the new middleware (2.1), handlers (2.2),
  route isolation (2.3), and CRD deepcopy (1.1/1.2).
- [x] 4.2 Chart render tests cover the new `Role` entry (3.1) and the new
  env/Secret wiring (3.2).

## 5. System tests

- [x] 5.1 Add a lane to `platform/manager/test/system/` that, against a real
  envtest/cluster API server, exercises the full round trip through
  `/control-plane/*`: correct token writes a placeholder object, a read
  returns it, an unconfigured/wrong token is refused, a request for a kind
  with no handler returns 404 and touches no other kind, and the manager's
  Role (not a new one) is what RBAC actually grants — this is owed because
  RBAC enforcement and the CRD's establishment are both cluster-decided
  facts. Verify: `go test -tags system -count=1 -timeout 45m -v
  ./test/system/` passes against THIS worktree's tree, not master's.

## 6. Documentation

### Reference docs

- [x] 6.1 `docs/concepts.md`: document the placeholder CRD, its disposable
  nature stated first, and the RBAC pattern this surface establishes
  (manager Role = full CRUD, every other component's Role = read-only or
  nothing) as the rule future residents follow.
- [x] 6.2 `docs/contracts.md`: document the `/control-plane/*` HTTP surface
  — its routes, its dedicated token auth, and the isolation from
  `ADAPTER_TOKEN`/the coordinator token.
- [x] 6.3 `docs/CHANGELOG.md`: the new CRD, the new `controlPlaneAuth` chart
  value, and the `kubectl apply -f chart/crds/` upgrade step (Helm never
  upgrades an existing CRD).
- [x] 6.3a `docs/configuration.md`: document the `controlPlaneAuth` chart
  value, its generated token Secret, and the 503 an unset token yields.
- [x] 6.4 Run `python3 .github/scripts/docs-generate.py --check` (this
  change adds a CRD field and a chart value) and fix any reported drift.

### Adopter site

- [x] 6.5 Confirm no adopter-facing page (landing page, Getting started,
  Installation, guides) describes manager-internal HTTP surfaces or
  scaffolding CRDs today, so none needs a change for this proposal — state
  this explicitly rather than silently skipping it, per `documentation.md`.
