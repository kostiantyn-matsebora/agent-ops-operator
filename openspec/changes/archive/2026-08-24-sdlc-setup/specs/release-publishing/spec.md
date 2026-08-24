## ADDED Requirements

### Requirement: A per-component git tag publishes exactly one artifact

Publishing SHALL be triggered by a git tag of the form `<component>-v<semver>`
where `<component>` names one image in the repository, or `chart`. Pushing such a tag SHALL
publish that component alone at that version, leaving every other component
untouched, so components keep their independent version lines. A tag naming an
unknown component or a malformed version SHALL fail the workflow with a message
listing the valid forms, rather than skipping silently.

#### Scenario: Releasing one adapter

- **WHEN** the tag `channel-telegram-v0.12.1` is pushed
- **THEN** only `ghcr.io/kostiantyn-matsebora/agentops-channel-telegram:0.12.1`
  is published
- **AND** no other image or chart version is published

#### Scenario: A malformed tag

- **WHEN** a tag such as `telegram-1.0` or `manager-vX` is pushed
- **THEN** the workflow fails with a message naming the valid tag forms
- **AND** nothing is published

#### Scenario: Nothing publishes outside a tag

- **WHEN** a commit is pushed to `master` without a release tag
- **THEN** no image and no chart is published

### Requirement: Every image declares its architectures, and the push is verified against that declaration

Each component SHALL declare the platforms it is published for, in one place
both workflows read. Every image SHALL be published as a manifest list covering
`linux/amd64` and `linux/arm64` under one tag, without emulation, unless its
declaration says otherwise.

After pushing, the publish job SHALL inspect the pushed manifest and FAIL unless
its platforms EQUAL the declaration — an image that lost an architecture and one
that gained an undeclared one both fail. A build command is a request; only the
manifest is evidence.

A component SHALL NOT be declared single-architecture on the strength of how it
has been built before. The declaration describes what the image CAN do, and is
established by building it.

#### Scenario: Pulling on either architecture

- **WHEN** a node of either `linux/amd64` or `linux/arm64` pulls a published
  image tag
- **THEN** the pull resolves to a matching architecture from the same tag

#### Scenario: An image silently loses an architecture

- **WHEN** a component declared multi-architecture is pushed with only one
- **THEN** the publish job fails naming the missing platform, rather than the
  gap surfacing weeks later as `ImagePullBackOff` on the first reschedule onto
  the other architecture

#### Scenario: A single-architecture claim is tested, not inherited

- **WHEN** a component is believed to be single-architecture
- **THEN** the belief is settled by building and running it on the other
  architecture before the declaration records it

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
— in the parent chart's values or in any bundled subchart's — is absent from
the registry.

#### Scenario: Chart version disagrees with its tag

- **WHEN** `chart-v5.26.0` is pushed while `Chart.yaml` declares `version: 5.25.0`
- **THEN** the job fails and nothing is published

#### Scenario: Chart points at an unpublished image

- **WHEN** the chart's default values reference a first-party image tag that
  was never pushed to the registry
- **THEN** the chart job fails and names the missing image reference

#### Scenario: A bundle's image is checked too

- **WHEN** a bundled subchart's default values reference a first-party image
  tag that was never pushed
- **THEN** the chart job fails and names it, exactly as for the parent chart's
  own references

#### Scenario: Third-party images are not gated

- **WHEN** the chart references an image outside this project's namespace, such
  as a third-party MCP server
- **THEN** that reference is excluded from the existence check, by namespace
  rather than by a hardcoded list

### Requirement: Published packages are pullable with no credential

Every published package SHALL be readable anonymously, so a cluster installs
the chart and pulls every image without holding a registry credential. The
chart SHALL NOT require an `imagePullSecrets` value, and no CRD, controller or
pod-spec code SHALL carry registry credentials.

Publishing SHALL fail rather than ship an artifact nobody can pull: the chart
release SHALL resolve each first-party image reference anonymously, so a
package that is still private is caught at release time instead of at install
time.

#### Scenario: A fresh install pulls nothing but public artifacts

- **WHEN** the chart is installed into a cluster holding no registry credential
- **THEN** the manager, adapter, runtime, sidecar and job pods all pull
  successfully

#### Scenario: A package left private is caught before anyone installs it

- **WHEN** a first-party package has not been made public
- **THEN** the anonymous resolution in the chart release fails and names it

#### Scenario: No credential surface is added

- **WHEN** publishing is in place
- **THEN** the chart has no pull-secret value, and no CRD type, deepcopy or
  controller pod-spec code has changed

### Requirement: Published artifacts carry provenance and repository linkage

Each published image SHALL carry build provenance attestation, an SBOM, and
OCI source labels identifying the originating repository and commit.

#### Scenario: Inspecting a published image

- **WHEN** a published image is inspected
- **THEN** it reports the source repository and the commit it was built from
- **AND** provenance and SBOM attestations are attached
