## MODIFIED Requirements

### Requirement: The system pack runs against a real single-node cluster
A system pack SHALL exist that provisions a real single-node Kubernetes
cluster (k3s), installs the chart built from the working tree, and waits for
the manager and every enabled adapter to become Ready.

It SHALL assert against the live cluster. It SHALL live in the manager's Go
module (`platform/manager/`) so it inherits the existing Kubernetes client
dependency, and SHALL NOT introduce a new Go module.

The pack's subject is the **substrate** — the layer the existing envtest
suite structurally cannot reach, because envtest runs no kubelet, no
scheduler and no CSI.

A test whose assertion holds under envtest SHALL NOT be duplicated here. The
pack exists for what envtest cannot decide, and every test in it SHALL be
justifiable by naming the component (kubelet, scheduler, informer, CNI, image
puller) whose participation makes the assertion possible.

#### Scenario: The chart under test is the working tree, not a published release
- **WHEN** the pack runs
- **THEN** it installs the chart from `chart/` in the working tree with images built from the same commit, so a template change and a code change are verified together

#### Scenario: A test that envtest could make is rejected from the pack
- **WHEN** a candidate test asserts only on CR fields written by a reconciler, with no kubelet, scheduler, informer or image-pull participation
- **THEN** it belongs in `internal/integration/`, not in the system pack

### Requirement: The pack's scope boundary with continuous integration is stated
The system pack SHALL own the definition of the cluster-based jobs only.
Per-module build, vet and unit test, the envtest suite, and chart lint/render
remain owned by the `continuous-integration` capability.

Neither capability SHALL restate the other's jobs, so that "what CI runs"
has exactly one definition per tier.

#### Scenario: A per-module job is not duplicated here
- **WHEN** a change adds a per-module build or lint step
- **THEN** it is specified under `continuous-integration`, not under system testing
