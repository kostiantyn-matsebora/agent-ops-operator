## 1. Signal adapter instance label — backend-developer

- [x] 1.1 In `platform/manager/internal/controller/signaladapter_controller.go`, add `"app.kubernetes.io/instance": adapter.Name` to the `labels` map built before `ensureAdapterWorkload`/`ensureAdapterService` (around line 104), leaving `app.kubernetes.io/name` and `signalAdapterLabelKey` untouched. Verify by reading the updated reconciler and confirming the map carries both keys.

## 2. Channel adapter instance label — backend-developer

- [x] 2.1 In `platform/manager/internal/controller/channeladapter_controller.go`, add `"app.kubernetes.io/instance": adapter.Name` to the `labels` map built before `ensureAdapterWorkload`/`ensureAdapterService` (around line 105), leaving `app.kubernetes.io/name` and `channelAdapterLabelKey` untouched. Verify by reading the updated reconciler and confirming the map carries both keys.

## 3. Unit tests

- [x] 3.1 Add `TestSignalAdapterInstanceLabel` to `internal/integration/signaladapter_test.go` (the reconcilers are envtest-covered there, not in a `*_controller_test.go`) asserting that two differently-named `SignalAdapter`s render Deployments (and a Service, for the port-declaring one) carrying `app.kubernetes.io/name: agentops-signal-adapter` plus distinct `app.kubernetes.io/instance` values equal to each CR's name. Verified with `KUBEBUILDER_ASSETS=... go test ./internal/integration/... -run TestSignalAdapterInstanceLabel -v` — PASS.
- [x] 3.2 Add `TestChannelAdapterInstanceLabel` to `internal/integration/channeladapter_test.go` asserting the same for two differently-named `ChannelAdapter`s (`app.kubernetes.io/name: agentops-adapter`, distinct `app.kubernetes.io/instance`). Verified with `KUBEBUILDER_ASSETS=... go test ./internal/integration/... -run TestChannelAdapterInstanceLabel -v` — PASS.
- [x] 3.3 Ran `go vet ./...` (clean) and `KUBEBUILDER_ASSETS=... go test -coverpkg=./... -coverprofile=coverage.out ./...` in `platform/manager` against this worktree's tree — all packages `ok`, no regression in `internal/controller` or `internal/integration`.

## 4. System tests

- [x] 4.1 Not applicable — no cluster-decided behavior changes. The label is observational metadata read by no selector, RBAC rule, kubelet decision, or informer in this repo. `networkpolicy.yaml` and every `spec.selector` are explicitly left untouched (see design.md — Decisions).

## 5. Documentation

### Reference docs

- [x] 5.1 Added a `### Added` entry under a new `## [Unreleased]` heading in `docs/CHANGELOG.md`, noting `app.kubernetes.io/instance` now distinguishes SignalAdapter/ChannelAdapter instances in Kubernetes tooling (Hubble, Grafana, `kubectl -l`).

### Adopter site

- [x] 5.2 Not applicable — no landing, getting-started, installation, or guide page states or depends on this label (see proposal.md — Impact).
