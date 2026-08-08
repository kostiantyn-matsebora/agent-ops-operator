## ADDED Requirements

### Requirement: Every pull request builds, vets, and tests all four Go modules

CI SHALL run `go build ./...`, `go vet ./...`, and `go test ./...` for the
operator module and for each of `channel-telegram/`, `signal-cron/`, and
`signal-vmalertmanager/` on every pull request and every push to `master`. The
operator module's tests SHALL run with `KUBEBUILDER_ASSETS` pointing at envtest
binaries for Kubernetes 1.31.x, so the integration suite executes against a
real API server rather than being skipped. The Go toolchain version SHALL be
derived from `go.mod` rather than pinned separately in the workflow.

#### Scenario: A change breaks the operator build

- **WHEN** a pull request introduces a compile error, a vet finding, or a
  failing test in the operator module
- **THEN** the CI check fails and the failure names the operator module

#### Scenario: The envtest suite actually runs

- **WHEN** the operator test job executes
- **THEN** `KUBEBUILDER_ASSETS` is set to a provisioned envtest 1.31.x asset
  directory
- **AND** the `internal/integration` suite runs rather than skipping for
  missing assets

#### Scenario: An adapter module breaks

- **WHEN** a pull request breaks `channel-telegram`, `signal-cron`, or
  `signal-vmalertmanager`
- **THEN** that module's job fails independently of the operator job, naming
  the module

### Requirement: Every pull request renders and validates the Helm chart

CI SHALL run `helm lint` and `helm template` on `chart/` for the default
values, for `demo.enabled=true`, for `vm-bundle.enabled=true`, and for the
combination, and SHALL validate the rendered manifests against Kubernetes
schemas plus the repository's own CRDs from `chart/files/crds/`.

#### Scenario: A template change produces invalid YAML

- **WHEN** a chart template change renders malformed or schema-invalid
  manifests under any validated value permutation
- **THEN** the chart job fails and names the permutation

#### Scenario: A CR in the chart violates its own CRD schema

- **WHEN** a rendered custom resource does not conform to the CRD in
  `chart/files/crds/`
- **THEN** validation fails rather than passing for lack of a schema

### Requirement: Every pull request builds all container images without publishing

CI SHALL build all five images (`manager`, `channel-telegram`, `signal-cron`,
`signal-vmalertmanager`, `runtime-claude`) from their Dockerfiles on every pull
request with publishing disabled, using layer caching to keep feedback fast.

#### Scenario: A Dockerfile breaks

- **WHEN** a pull request breaks any Dockerfile
- **THEN** that image's build job fails

#### Scenario: No artifact escapes from a pull request

- **WHEN** CI runs for a pull request
- **THEN** no image or chart is pushed to any registry
