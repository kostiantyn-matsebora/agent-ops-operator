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

### Requirement: CI refuses content that identifies a person or a private deployment

The repository is to be published, so every file AND every commit message SHALL
be readable by strangers without disclosing who runs the author's cluster, what
it is called, or how to reach it.

CI SHALL enforce this with an ALLOWLIST of permitted shapes, never a list of
forbidden strings. A denylist has to spell out the thing it protects, which
publishes it in the guard — and a denylist only catches what someone already
thought of.

The guard SHALL cover the whole repository, `openspec/` included, and the commit
messages of the range under review.

#### Scenario: A hostname outside the documented example space

- **WHEN** a change adds a hostname that is not a reserved example domain, a
  cluster-internal service name, or a loopback name
- **THEN** CI fails, naming the file and line

#### Scenario: A chat identifier that is not the documented placeholder

- **WHEN** a change adds a chat, group or thread identifier other than the one
  the documentation carries as its example
- **THEN** CI fails

#### Scenario: A repository URL that is not this project's

- **WHEN** a change adds a clone or remote URL naming a repository other than
  this one
- **THEN** CI fails

#### Scenario: An address literal outside the documented example set

- **WHEN** a change adds a private-range address literal that is not one of the
  documented examples
- **THEN** CI fails

#### Scenario: A commit message carries what the tree does not

- **WHEN** a pull request's commit messages contain a violation its tree does
  not
- **THEN** CI fails on the message

#### Scenario: The report does not republish what it caught

- **WHEN** the guard fails in CI
- **THEN** it prints the file, the line number and the rule that was violated,
  and NOT the text that matched — a public repository has public build logs, so
  a guard that quotes its findings leaks them to the same audience it protects
  the tree from
- **AND** a local invocation MAY print the matched text, because that is where
  the fixing happens

#### Scenario: The guard names nothing it forbids

- **WHEN** the guard and its allowlist are read
- **THEN** they contain no personal name, no private hostname and no real
  identifier — only the shapes that are permitted
