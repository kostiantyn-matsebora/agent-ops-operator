## 1. Signal adapter instance label — backend-developer

- [ ] 1.1 In `platform/manager/internal/controller/signaladapter_controller.go`, add `"app.kubernetes.io/instance": adapter.Name` to the `labels` map built before `ensureAdapterWorkload`/`ensureAdapterService` (around line 104), leaving `app.kubernetes.io/name` and `signalAdapterLabelKey` untouched. Verify by reading the updated reconciler and confirming the map carries both keys.

## 2. Channel adapter instance label — backend-developer

- [ ] 2.1 In `platform/manager/internal/controller/channeladapter_controller.go`, add `"app.kubernetes.io/instance": adapter.Name` to the `labels` map built before `ensureAdapterWorkload`/`ensureAdapterService` (around line 105), leaving `app.kubernetes.io/name` and `channelAdapterLabelKey` untouched. Verify by reading the updated reconciler and confirming the map carries both keys.

## 3. Unit tests

- [ ] 3.1 Add/extend a test in `signaladapter_controller_test.go` asserting that two differently-named `SignalAdapter`s render Deployments (and a Service, for the port-declaring case) carrying `app.kubernetes.io/name: agentops-signal-adapter` plus distinct `app.kubernetes.io/instance` values equal to each CR's name. Verify with `go test ./...` in `platform/manager`.
- [ ] 3.2 Add/extend a test in `channeladapter_controller_test.go` asserting the same for two differently-named `ChannelAdapter`s (`app.kubernetes.io/name: agentops-adapter`, distinct `app.kubernetes.io/instance`). Verify with `go test ./...` in `platform/manager`.
- [ ] 3.3 Run `go vet ./...` and `go test -coverpkg=./... -coverprofile=coverage.out ./...` in `platform/manager` against this worktree's tree and confirm no regression in `internal/controller`.

## 4. System tests

- [ ] 4.1 Not applicable — no cluster-decided behavior changes. The label is observational metadata read by no selector, RBAC rule, kubelet decision, or informer in this repo. `networkpolicy.yaml` and every `spec.selector` are explicitly left untouched (see design.md — Decisions).

## 5. Documentation

### Reference docs

- [ ] 5.1 Add a `### Added` entry to `docs/CHANGELOG.md` under the next unreleased manager version, noting `app.kubernetes.io/instance` now distinguishes SignalAdapter/ChannelAdapter instances in Kubernetes tooling (Hubble, Grafana, `kubectl -l`).

### Adopter site

- [ ] 5.2 Not applicable — no landing, getting-started, installation, or guide page states or depends on this label (see proposal.md — Impact).
