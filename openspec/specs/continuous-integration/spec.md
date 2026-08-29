# continuous-integration Specification

## Purpose
TBD - created by archiving change sdlc-setup. Update Purpose after archive.

## Requirements

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

### Requirement: Every pull request builds the container images it could have broken

CI SHALL build images from their Dockerfiles on every pull request with
publishing disabled, using layer caching to keep feedback fast. The image set
SHALL be derived from the Dockerfiles that exist rather than enumerated in
prose.

**IT SHALL BUILD WHAT CHANGED, NOT EVERYTHING.** A component SHALL be built when
a file inside its own directory moved. Fourteen image builds and thirteen module
builds on a documentation-only commit is cost with no signal, and the wait it
adds is paid by every contributor on every push.

**THE FILTER SHALL BE DERIVED, LIKE THE MATRIX.** A list of paths maintained
beside the component list is a second thing to keep in step, and the one that
falls behind is the one nobody notices — which is the same argument that made
the matrix derived in the first place.

**FOUR KINDS OF FILE SHALL REBUILD EVERYTHING**, and they are the ones that
touch no component directory while invalidating every component: the shared
Dockerfile that is many components' recipe, the script that decides what
components exist at all, the workflow that decides how any of them is tested,
and the composite actions under `.github/actions/`, which is where the scan
every image runs through lives. A change to the scan that scans nothing has not
been tested, and reports success.

**WHERE THE COMPARISON BASE CANNOT BE ESTABLISHED, EVERYTHING SHALL BUILD.** A
branch's first push has nothing to compare against, and a shallow checkout makes
the comparison fail. Both SHALL build the world rather than build nothing: a
filter that silently matches nothing is a CI that silently tests nothing, and it
reports success while doing so.

#### Scenario: A Dockerfile breaks

- **WHEN** a pull request breaks any Dockerfile
- **THEN** that image's build job fails, naming the component

#### Scenario: A new image is covered without editing the workflow

- **WHEN** a Dockerfile is added to the repository
- **THEN** it is built by the same job without the image set being edited by
  hand

#### Scenario: One component changes

- **WHEN** a pull request touches a single component's directory
- **THEN** that component is built and every other component is skipped

#### Scenario: Only documentation changes

- **WHEN** a pull request touches no component directory
- **THEN** no module or image job runs at all

#### Scenario: The scan action changes

- **WHEN** a pull request changes anything under `.github/actions/`
- **THEN** every image is built and scanned on that pull request

#### Scenario: The shared recipe changes

- **WHEN** a pull request touches the shared Dockerfile, the component-discovery
  script, or the CI workflow itself
- **THEN** every component is built, because the change touches all of them
  without touching any of their directories

#### Scenario: There is nothing to compare against

- **WHEN** the base commit cannot be resolved, as on a branch's first push
- **THEN** everything is built rather than nothing

#### Scenario: No artifact escapes from a pull request

- **WHEN** CI runs for a pull request
- **THEN** no image or chart is pushed to any registry

### Requirement: One always-present check gates the pull request

CI SHALL expose exactly ONE check that runs on every pull request regardless of
which files changed, and that check SHALL pass only when every check relevant to
that pull request passed or was skipped for having nothing to do.

**THIS IS WHAT MAKES FILTERING COMPATIBLE WITH BRANCH PROTECTION.** A protection
rule names a check by NAME, and a job skipped for untouched paths never reports
that name — so a rule requiring a skippable check blocks the pull request
forever, waiting for a status that will not arrive. Without this, a repository
must choose between building only what changed and requiring checks at all.

**IT SHALL READ THE RESULTS EXPLICITLY** rather than relying on a helper that
treats a skipped dependency inconsistently while reading as though it checked
one. A SKIPPED job passed by having nothing to do; a CANCELLED one did not pass.
Stating that difference is what makes the gate's verdict reviewable.

#### Scenario: A filtered pull request still reports the required check

- **WHEN** a pull request touches only documentation, so the module and image
  jobs are skipped
- **THEN** the gate still runs and passes, and branch protection is satisfied

#### Scenario: A relevant check fails

- **WHEN** any job the pull request actually ran fails
- **THEN** the gate fails and names the job that failed

#### Scenario: A cancelled job is not a pass

- **WHEN** a job is cancelled rather than skipped
- **THEN** the gate fails, because nothing established that it would have passed

### Requirement: The published site builds on every pull request

CI SHALL build the documentation site on every pull request, with the same build
the hosting platform performs.

**THE PLATFORM BUILDS IT AFTER MERGE, WHICH IS TOO LATE.** A site built natively
from a branch folder is built only once the change has landed, so a broken
configuration file, an unclosed template tag or a missing include is discovered
on the published site rather than on the pull request that caused it — by a
reader, not by the author.

Checking the GENERATED CONTENT of the documentation is a different requirement
and SHALL NOT be conflated with this one: examples matching their sources says
nothing about whether the site assembles.

#### Scenario: The site fails to build

- **WHEN** a pull request breaks the site's configuration or a template
- **THEN** the site job fails on that pull request, before the site is published

#### Scenario: Building is not publishing

- **WHEN** the site job runs for a pull request
- **THEN** it builds the site and publishes nothing

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

### Requirement: The openspec artifacts are validated on every pull request

CI SHALL validate, on every pull request and every push to the default branch:

1. **every published specification**, always; and
2. **every change the pull request touches.**

It SHALL fail when any of them is invalid, naming what is invalid.

**`openspec/specs/` is the published answer to "is this behaviour intended", and
nothing checked that it parses.** A specification is trusted precisely because it
is not code — no compiler reads it, no test exercises it, and a malformed one is
indistinguishable from a correct one until somebody relies on it. That is the
whole argument for validating it mechanically, and unconditionally.

**A CHANGE IN FLIGHT IS JUDGED ONLY BY THE PULL REQUEST THAT TOUCHES IT.** A
dozen changes are open at any time, and a delta that is incomplete today is
incomplete correctly — the change is not finished. A check that judged all of
them would fail every pull request for work it was not about, and would be
switched off within a day. The scope is what makes the gate survivable, and
therefore what makes it a gate at all.

The check SHALL report through the always-present gate, so that requiring it
needs no change to branch protection.

#### Scenario: A change's artifacts are malformed

- **WHEN** a pull request carries a change whose artifacts do not validate
- **THEN** the check fails, names the change and states what is wrong

#### Scenario: A published specification is broken

- **WHEN** a pull request leaves a specification under `openspec/specs/` invalid
- **THEN** the check fails, whether or not the pull request is the thing that
  broke it

#### Scenario: An unrelated change is mid-flight and incomplete

- **WHEN** a pull request touches no openspec change, and another change in
  flight has an incomplete delta
- **THEN** the check passes, because that change is not what this pull request
  is about

### Requirement: A change's documentation task is verified on every pull request

CI SHALL fail a pull request that FINISHES an openspec change whose task list
does not end in a completed documentation section covering both the reference
docs and the adopter site.

**A pull request finishes a change two ways, and no others:** it ARCHIVES the
change — which is the claim of completion itself — or every task outside the
documentation section is complete. A change a pull request merely TOUCHES is not
judged on its documentation: a proposal's documentation tasks are unticked
because nothing has been written yet, and failing that pull request asks for the
documentation of work nobody has done.

**The check SHALL judge an archived change at its archived location.** Archiving
MOVES the change, so the live task list is absent from the tree the check reads —
and a check scoped to what still exists reports that the pull request touches no
change at all, at the one moment the rule exists to protect.

**Whether the last section IS a documentation section SHALL be judged whatever
the change's phase.** That is a property of the file rather than of the work, and
a proposal is the cheapest moment to catch a task list that never had one.

**The rule already existed and was enforced in one contributor's local
tooling.** A gate that lives in a harness is absent for every other
contributor, for every session whose tooling is not installed, and for
automation — and this repository is public, so "every other contributor" is now
a real population rather than a hypothetical one.

The check SHALL state both halves when it fails, because they are skipped
independently: a change feels finished once the reference docs are right, and
the adopter never reads the reference docs.

Local enforcement SHALL be retained rather than replaced. The local gate fails
open on anything it cannot read, which is what stops it being disabled; the
check is what makes failing open safe, by asserting the same decision where it
cannot be skipped.

#### Scenario: The documentation task is unticked

- **WHEN** a pull request finishes a change whose documentation tasks are not all
  complete
- **THEN** the check fails and names the outstanding tasks

#### Scenario: A change is proposed

- **WHEN** a pull request adds a change whose implementation tasks are all
  outstanding
- **THEN** the check reports it as pending and does not fail

#### Scenario: A change is archived

- **WHEN** a pull request archives a change
- **THEN** its documentation section is judged at the archived location

#### Scenario: The documentation section is missing entirely

- **WHEN** a change's task list does not end in a documentation section
- **THEN** the check fails, naming both halves such a section must cover

#### Scenario: The two enforcement points disagree

- **WHEN** the local gate and the check are given the same task list
- **THEN** they reach the same verdict, and a divergence between them fails a
  test rather than a pull request

### Requirement: Contract conformance reports through the always-present check, and the cluster jobs live elsewhere
Contract conformance — every adapter's built binary against a fake manager —
SHALL be a job of the `ci` workflow and SHALL be listed in `ci-green`'s
`needs:`. It SHALL NOT be a separately required status check: branch
protection names one check, and adding a gate is a line in that job's
`needs:`. The `ci` workflow SHALL provision no cluster: the k3s smoke belongs
to the release workflow and to on-demand runs, both calling `e2e.yml`.

The job SHALL run only when the diff touches what a running install is built
from — a component group, the chart, the test doubles under `test/`, or the
e2e workflow itself — and a job that did not run reports through `ci-green` as
skipped, exactly as every other path-filtered job does.

The boundary is stated on every side: `continuous-integration` owns per-module
build, vet and unit test, the envtest suite, chart lint and render, the image
builds and scans, the site, the guards, and the job that RUNS the conformance
suite on a pull request; `contract-conformance-suite` owns what that suite
asserts; `end-to-end-testing` owns the cluster-based jobs only. No capability
restates another's jobs, so "what CI runs" has exactly one definition per tier.

#### Scenario: Conformance gates through ci-green
- **WHEN** a pull request changes a component, the chart or the test doubles
- **THEN** the `conformance` job runs, and `ci-green` fails if it failed

#### Scenario: A docs-only pull request runs nothing of it
- **WHEN** a pull request changes only pages under `docs/`
- **THEN** the `conformance` job is skipped and `ci-green` treats it as skipped, not failed

#### Scenario: A per-module job is not duplicated in the e2e workflow
- **WHEN** a change adds a per-module build, vet or lint step
- **THEN** it is specified and wired under `continuous-integration`, and `e2e.yml` carries no copy of it
