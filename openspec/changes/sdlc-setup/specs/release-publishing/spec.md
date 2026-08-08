## ADDED Requirements

### Requirement: A per-component git tag publishes exactly one artifact

Publishing SHALL be triggered by a git tag of the form `<component>-v<semver>`
where `<component>` is one of `manager`, `channel-telegram`, `signal-cron`,
`signal-vmalertmanager`, `runtime-claude`, or `chart`. Pushing such a tag SHALL
publish that component alone at that version, leaving every other component
untouched, so components keep their independent version lines. A tag naming an
unknown component or a malformed version SHALL fail the workflow with a message
listing the valid forms, rather than skipping silently.

#### Scenario: Releasing one adapter

- **WHEN** the tag `channel-telegram-v0.4.1` is pushed
- **THEN** only `ghcr.io/kostiantyn-matsebora/agentops-channel-telegram:0.4.1`
  is published
- **AND** no other image or chart version is published

#### Scenario: A malformed tag

- **WHEN** a tag such as `telegram-1.0` or `manager-vX` is pushed
- **THEN** the workflow fails with a message naming the valid tag forms
- **AND** nothing is published

#### Scenario: Nothing publishes outside a tag

- **WHEN** a commit is pushed to `master` without a release tag
- **THEN** no image and no chart is published

### Requirement: Images are published as multi-architecture manifest lists

Each published image SHALL be a single manifest list covering `linux/amd64` and
`linux/arm64` under one tag. Images whose Dockerfile cross-compiles SHALL be
built without emulation; emulation SHALL be enabled only for the component that
requires executing target-architecture instructions during the build.

#### Scenario: Pulling on either architecture

- **WHEN** a node of either `linux/amd64` or `linux/arm64` pulls a published
  image tag
- **THEN** the pull resolves to a matching architecture from the same tag

#### Scenario: The runtime image carries an architecture-matched kubectl

- **WHEN** the `runtime-claude` image is built for `linux/arm64`
- **THEN** the bundled `kubectl` is the arm64 binary
- **AND** a wrong-architecture binary fails the build rather than the running pod

### Requirement: Published tags are immutable

Before building, a publish job SHALL check whether its target tag already
exists in the registry and SHALL fail without building if it does. This applies
to images and to the chart.

#### Scenario: Re-pushing an existing version

- **WHEN** a tag is pushed for a version already present in the registry
- **THEN** the job fails before the build starts
- **AND** the existing published artifact is left unchanged

### Requirement: The Helm chart is published as an OCI artifact

A `chart-v<semver>` tag SHALL package `chart/` and push it to
`oci://ghcr.io/kostiantyn-matsebora/charts`, installable by version with
`helm install ... oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator --version <semver>`.
The job SHALL fail unless `Chart.yaml`'s `version` equals the tag's version,
and SHALL fail if any first-party image reference the chart renders by default
is absent from the registry.

#### Scenario: Chart version disagrees with its tag

- **WHEN** `chart-v1.15.0` is pushed while `Chart.yaml` declares `version: 1.14.0`
- **THEN** the job fails and nothing is published

#### Scenario: Chart points at an unpublished image

- **WHEN** the chart's default values reference a first-party image tag that
  was never pushed to the registry
- **THEN** the chart job fails and names the missing image reference

#### Scenario: Third-party images are not gated

- **WHEN** the chart references a third-party image such as the VictoriaMetrics
  MCP server images
- **THEN** that reference is excluded from the existence check

### Requirement: Cluster workloads can pull from a private registry

The chart SHALL support a `global.imagePullSecrets` value, empty by default,
rendered onto the manager, runtime, and adapter ServiceAccounts so that manager
pods, runtime pods, and adapter pods created by the controllers all inherit the
pull secret. The chart SHALL NOT create the Secret itself, and no CRD or
controller change SHALL be required to carry the pull secret.

#### Scenario: Private GHCR install

- **WHEN** `global.imagePullSecrets` names an existing Secret and the release is
  installed against a private registry
- **THEN** manager, adapter, and runtime pods all pull successfully

#### Scenario: Default remains unchanged

- **WHEN** `global.imagePullSecrets` is empty
- **THEN** the rendered ServiceAccounts carry no `imagePullSecrets` field

#### Scenario: No API surface added

- **WHEN** pull-secret support is in place
- **THEN** no CRD type, deepcopy, or controller pod-spec code has changed

### Requirement: Published artifacts carry provenance and repository linkage

Each published image SHALL carry build provenance attestation, an SBOM, and
OCI source labels identifying the originating repository and commit.

#### Scenario: Inspecting a published image

- **WHEN** a published image is inspected
- **THEN** it reports the source repository and the commit it was built from
- **AND** provenance and SBOM attestations are attached
