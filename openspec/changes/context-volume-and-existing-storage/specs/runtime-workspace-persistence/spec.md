## MODIFIED Requirements

### Requirement: Agent session persistence is the shipped default
The chart SHALL provision the runtime context volume by default, so that an
agent's accumulated context survives a runtime pod restart out of the box. An
operator SHALL be able to opt out explicitly for clusters without a suitable
provisioner, and the opt-out SHALL be documented alongside the symptom it
avoids.

The volume SHALL be named for what it holds. It holds a conversation's
accumulated context — the thing `runtimeContextId` is a handle into and
`contextStorage` promises continuity on — and SHALL therefore be called the
CONTEXT volume everywhere it is named, never the home volume. That it happens to
be the runtime process's `$HOME` is a property of one runtime image, not what
the volume is for.

The MOUNT PATH SHALL NOT move. It is the process home directory, and the
reference runtime keys its stored context off it, so relocating it would break
continuity for every existing conversation.

Turning persistence off SHALL NOT be silent at the moment it costs something. A
run whose context is gone SHALL FAIL with an explicit reason rather than
answering without context, and that reason SHALL reach both the thread, for the
person waiting on it, AND the `Conversation`, for anyone asking later why the
agent does not remember. A warning that scrolls past in a chat surface is not a
record, and the runtime pod that emitted it has usually exited by the time the
question is asked.

#### Scenario: Fresh install persists sessions
- **WHEN** the chart is installed with no persistence values supplied
- **THEN** a claim is provisioned and the rendered `AgentRuntime` carries a context volume reference naming it

#### Scenario: Cluster without a suitable provisioner
- **WHEN** an operator disables context persistence
- **THEN** the install succeeds, runtime pods use ephemeral storage, and the documentation states that context dies with each pod

#### Scenario: Lost session is explained, not hidden
- **WHEN** a run is asked to continue a context whose stored state no longer exists
- **THEN** the run fails with an explicit reason instead of answering without context, the thread is told what happened and that a new conversation is the remedy, and the conversation records that it can no longer be continued

#### Scenario: The mount path is unchanged by the rename
- **WHEN** a runtime pod is built after the rename
- **THEN** the context volume is mounted at the same path it was mounted at before, and a conversation created before the rename resumes its context

### Requirement: AgentRuntime can declare a persistent workspace volume
`AgentRuntime.spec` SHALL accept a `workspace` volume declaration alongside the
context volume, referencing a PersistentVolumeClaim. When declared, the runtime
pod SHALL mount the repository checkout path from that claim; when absent, the
checkout path SHALL remain ephemeral.

The checkout path SHALL NOT move — claude-code sessions are keyed by working
directory, and relocating it breaks session resume.

#### Scenario: Workspace claim is mounted
- **WHEN** an `AgentRuntime` declares a workspace claim
- **THEN** runtime pods built from it mount the checkout path from that claim at the unchanged path

#### Scenario: Absent declaration stays ephemeral
- **WHEN** an `AgentRuntime` declares no workspace volume
- **THEN** runtime pods use ephemeral storage for the checkout, as before

### Requirement: Workspace persistence is opt-in and provisioned by the chart
The chart SHALL expose a workspace persistence block that is disabled by
default and, when enabled, provisions a claim and wires it into the rendered
`AgentRuntime` without the operator restating the claim name. The default SHALL
be off: a fresh checkout is cheap and always correct, whereas a stale shared
checkout is neither.

The chart SHALL support pointing at an existing claim instead of provisioning
one.

#### Scenario: Disabled by default
- **WHEN** the chart is installed with no workspace values supplied
- **THEN** no workspace claim is rendered and the `AgentRuntime` declares no workspace volume

#### Scenario: Enabling needs one value, not two
- **WHEN** an operator enables workspace persistence
- **THEN** the claim is provisioned and the rendered `AgentRuntime` references it with no runtime-side claim name set

#### Scenario: Existing claim is honored
- **WHEN** an operator names an existing claim for the workspace
- **THEN** the chart provisions nothing and the `AgentRuntime` references the named claim

## ADDED Requirements

### Requirement: Either volume can bind to storage the chart did not create
An operator SHALL be able to run either volume — context or workspace — against
storage provisioned outside the release, and SHALL be able to do so by values
alone.

Three forms SHALL be supported, and ALL THREE SHALL be available on BOTH
volumes. A capability offered on one volume and not the other is the
two-spellings-of-one-fact problem, not a smaller feature.

| Form | The operator supplies | The chart |
|---|---|---|
| existing claim | a claim they already created | renders no claim and references theirs |
| volume by name | the `PersistentVolume`'s name | renders a claim bound to it |
| volume by label | a selector matching one | renders a claim carrying that selector |

Binding to a pre-created volume SHALL be expressible. A claim that names or
selects a volume SHALL be able to decline dynamic provisioning, which requires
an EXPLICIT empty storage class in the rendered claim rather than an absent
field — an absent field is filled in by the cluster's default StorageClass,
which provisions a second volume and leaves the operator's untouched.

The storage class value SHALL follow the convention the wider Helm ecosystem
already uses, so that an operator who has configured any other chart's
persistence already knows this one:

| Value | Renders |
|---|---|
| undefined or empty | no field — the cluster's default provisioner |
| `-` | `storageClassName: ""` — no class, bind to a pre-created volume |
| a name | that class |

This SHALL be additive. The empty value SHALL keep the meaning it has today, so
that no existing install changes behaviour on upgrade.

The chart SHALL NOT render a `PersistentVolume`. A pre-created volume is by
definition not the release's to create, and a release that created one would own
the lifecycle of storage holding agent context it did not put there.

#### Scenario: A pre-created volume is actually bound
- **WHEN** an operator names a pre-created `PersistentVolume` and sets the storage class to the disabling value
- **THEN** the rendered claim carries that volume name and an explicit empty storage class, and it binds to that volume rather than provisioning a new one

#### Scenario: Empty is not the disabling value
- **WHEN** an operator leaves the storage class empty or unset
- **THEN** the rendered claim omits the field and the cluster's default StorageClass applies, exactly as before this change

#### Scenario: A volume is matched by label
- **WHEN** an operator supplies a label selector instead of a volume name
- **THEN** the rendered claim carries that selector and binds to a matching pre-created volume

#### Scenario: Both volumes offer the same forms
- **WHEN** either the context volume or the workspace volume is configured against pre-created storage
- **THEN** the same values are accepted with the same meaning, and every consumer of that volume follows the resulting claim name

#### Scenario: No PersistentVolume is rendered
- **WHEN** any combination of these values is rendered
- **THEN** the release contains no `PersistentVolume` object

### Requirement: The rename does not strand an installed runtime or its stored context
The context volume's rename SHALL NOT be able to silently separate an install
from the context it already has.

The retired `AgentRuntime` field SHALL be DUAL-READ for one release: a runtime
declaring the old field SHALL keep working, and the new field SHALL win where
both are present. A rename that merely moved the field would strand every
installed runtime at the moment of upgrade.

The DEFAULT CLAIM NAME changes with the rename, and nothing copies a volume. An
upgrade that adopted the new name unremarked would provision a second, empty
claim, and every conversation in that install would answer without its context
while every signal reported success. The chart SHALL therefore FAIL THE RENDER
where it can detect that outcome, naming the one value that avoids it, rather
than proceeding on a guess. Failing an upgrade is recoverable in a way that a
silently emptied install is not.

The migration SHALL move no data. Naming the existing claim SHALL be sufficient
to keep it, and the existing claim SHALL survive leaving the rendered manifest.

The VALUES that configure the volume move with it, and a values file supplying
the retired ones SHALL FAIL the render naming where each went. Helm reports no
unread values key, so the alternative is silence — and the quiet case is the
costly one: an operator with no ReadWriteMany provisioner who declined the
volume gets it provisioned anyway, sitting `Pending`, with no runtime pod ever
scheduling behind it. This check SHALL NOT depend on reading the cluster, so
unlike the claim guard it also protects a GitOps install.

#### Scenario: An old runtime keeps running
- **WHEN** an `AgentRuntime` declaring only the retired field is reconciled after the upgrade
- **THEN** its volume is mounted as before and its conversations keep their context

#### Scenario: Both fields present
- **WHEN** an `AgentRuntime` declares both the retired field and its replacement
- **THEN** the replacement is used

#### Scenario: An upgrade cannot silently move the volume
- **WHEN** an install holding a claim under the retired name is upgraded without stating which claim its context lives on
- **THEN** the render fails naming the value to set, and no new claim is created

#### Scenario: A retired values key is refused, not ignored
- **WHEN** a values file supplies one of the keys that moved
- **THEN** the render fails naming that key and where it moved to, whether or not the renderer can see a cluster

#### Scenario: Keeping the existing volume is one value
- **WHEN** the operator names their existing claim
- **THEN** the release references it, renders no claim of its own, the existing claim is not deleted, and no data is copied
