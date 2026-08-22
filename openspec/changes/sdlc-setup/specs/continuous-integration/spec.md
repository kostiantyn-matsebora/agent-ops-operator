## ADDED Requirements

### Requirement: Every pull request builds, vets, and tests every Go module

CI SHALL run `go build ./...`, `go vet ./...`, and `go test ./...` for the
operator module and for EVERY submodule in the repository on every pull request
and every push to `master`. The module set SHALL be derived from the modules
that exist rather than enumerated in prose, so a module is covered by existing.

The operator module's tests SHALL run with `KUBEBUILDER_ASSETS` pointing at
envtest binaries for Kubernetes 1.31.x, so the integration suite executes
against a real API server rather than being skipped. The Go toolchain version SHALL be
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

#### Scenario: A submodule breaks

- **WHEN** a pull request breaks any submodule
- **THEN** that module's job fails independently of the operator job, naming
  the module

#### Scenario: A new module is covered without editing the workflow

- **WHEN** a module is added to the repository
- **THEN** it is built, vetted and tested by the same job, without the module
  set being edited by hand

### Requirement: The console UI is built and tested

CI SHALL install, test, and build the console's browser application on every
pull request and every push to `master`. Its tests exist and are run by nothing
today, and a broken bundle currently surfaces only inside the console image
build.

#### Scenario: A UI test fails

- **WHEN** a pull request breaks a console UI test
- **THEN** CI fails and names the UI, not the image that embeds it

#### Scenario: A UI build breaks

- **WHEN** the browser application no longer builds
- **THEN** CI fails before any image is built

### Requirement: Every pull request renders and validates the Helm chart

CI SHALL run `helm lint` and `helm template` on `chart/` for the default
values, for demo mode, for EACH bundled subchart, for the console opt-out, and
for the combination, and SHALL validate the rendered manifests against
Kubernetes schemas plus the repository's own CRDs from `chart/files/crds/`.

Each permutation SHALL supply the values its subject REQUIRES. A subchart that
refuses to render without a credential or an endpoint SHALL be given one, so
the permutation exercises the templates rather than the guard.

#### Scenario: A template change produces invalid YAML

- **WHEN** a chart template change renders malformed or schema-invalid
  manifests under any validated value permutation
- **THEN** the chart job fails and names the permutation

#### Scenario: A CR in the chart violates its own CRD schema

- **WHEN** a rendered custom resource does not conform to the CRD in
  `chart/files/crds/`
- **THEN** validation fails rather than passing for lack of a schema

#### Scenario: A bundle that needs values gets them

- **WHEN** a subchart fails to render without a required credential or endpoint
- **THEN** its permutation supplies one and the templates are exercised, rather
  than the permutation passing on the guard's error being expected

### Requirement: Every pull request builds all container images without publishing

CI SHALL build EVERY image in the repository from its Dockerfile on every pull
request with publishing disabled, using layer caching to keep feedback fast.
The image set SHALL be derived from the Dockerfiles that exist rather than
enumerated in prose.

#### Scenario: A Dockerfile breaks

- **WHEN** a pull request breaks any Dockerfile
- **THEN** that image's build job fails, naming the component

#### Scenario: A new image is covered without editing the workflow

- **WHEN** a Dockerfile is added to the repository
- **THEN** it is built by the same job without the image set being edited by
  hand

#### Scenario: No artifact escapes from a pull request

- **WHEN** CI runs for a pull request
- **THEN** no image or chart is pushed to any registry
