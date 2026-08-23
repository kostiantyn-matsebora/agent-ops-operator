## ADDED Requirements

### Requirement: Pipeline accepts inline definitions of the resources it wires

The `Pipeline` CRD SHALL accept five optional inline stanzas alongside its
reference fields: `spec.channels[]`, `spec.signalSources[]`, `spec.profile`,
`spec.toolsets.inline[]`, and `spec.mcpConfigs.inline[]`. Each entry SHALL be a
`name` plus exactly the spec schema of the corresponding CR, so a field added
to `Channel`, `SignalSource`, `AgentProfile`, `MCPToolset`, or `MCPConfig` is
inlinable without a second schema being maintained. Inline and reference forms
SHALL be usable together in one Pipeline, except that exactly one of
`spec.profileRef` or `spec.profile` SHALL be present. Inline blocks SHALL carry
secret *references* (`credentialsSecretRef`, `valueFrom`, `secretRef`) only,
never secret values, and the Pipeline reconciler SHALL read no Secrets.

#### Scenario: A complete flow in one manifest

- **WHEN** a Pipeline inlines a channel, a signal source, a profile, and a
  toolset, and is applied to an empty namespace
- **THEN** the flow works end to end with no other CR applied

#### Scenario: Inline mixed with references

- **WHEN** a Pipeline inlines one channel and also lists a `channelRefs` entry
  for a shared channel
- **THEN** conversations of that pipeline are bound to both channels

#### Scenario: Existing manifests keep validating

- **WHEN** a Pipeline written before this change, using only reference fields,
  is applied
- **THEN** it validates and reports `Ready` exactly as before

#### Scenario: Profile is given exactly once

- **WHEN** a Pipeline sets both `profileRef` and `profile`, or neither
- **THEN** the API server rejects it with a message naming the exclusivity rule

#### Scenario: The manager still reads no Secrets

- **WHEN** a Pipeline with inline blocks naming `credentialsSecretRef` and
  `valueFrom` sources is reconciled
- **THEN** the manager performs no Secret read, and the secret references are
  copied into the materialized CRs untouched

### Requirement: Inline entries materialize into owned child CRs

For each inline entry the reconciler SHALL create and keep current a real CR of
the corresponding kind, named `<pipeline>-<entry name>`, in the Pipeline's
namespace, carrying an `ownerReference` to the Pipeline. Materialized CRs SHALL
be indistinguishable to every other component from hand-written ones: adapters
read them over the adapter contracts, `Served`/`ConfigValid` conditions are
written to them, credential projection uses them, and conversations reference
them. Materialized entries SHALL join the Pipeline's effective reference set
before wiring validation, conflict detection, and routing run.

#### Scenario: An inline channel is a real Channel

- **WHEN** a Pipeline named `ops` inlines a channel entry named `home`
- **THEN** `kubectl get channel ops-home` returns a Channel owned by the
  Pipeline, and its serving adapter receives it in `GET /channel/channels`

#### Scenario: Editing an inline block updates the child

- **WHEN** an inline entry's config is edited on the Pipeline
- **THEN** the materialized CR's spec is updated in place, and conversations
  already running under it observe the new content without a pod restart

#### Scenario: Inline sources are claimed like referenced ones

- **WHEN** a Pipeline inlines a signal source and a second Pipeline references
  that materialized source by name
- **THEN** the newer Pipeline reports `SourceConflict=True` and the older claim
  keeps routing

#### Scenario: A child edited by hand is corrected

- **WHEN** someone edits a materialized child directly
- **THEN** the reconciler restores it to match the inline block and emits a
  Kubernetes Event recording the correction

### Requirement: Existing objects are never adopted

The reconciler SHALL NOT modify, adopt, or delete any object of a target name
that it does not already own. On collision it SHALL report `Ready=False` with
reason `NameConflict`, naming the object and its owner, and SHALL leave the
existing object untouched.

#### Scenario: Inline name collides with a hand-made CR

- **WHEN** a Pipeline `ops` inlines a channel `home` while a hand-written
  Channel `ops-home` already exists
- **THEN** the Pipeline reports `Ready=False` with reason `NameConflict` and the
  existing Channel is neither modified nor deleted

#### Scenario: Entry names are bounded so child names are legal

- **WHEN** an inline entry name would make `<pipeline>-<entry>` exceed the
  object-name limit, or is not DNS-1123 compliant
- **THEN** the API server rejects the Pipeline

### Requirement: Removing an inline block removes its child

The reconciler SHALL delete children it owns that are no longer named in the
Pipeline's spec, without waiting for owner-reference garbage collection.
Deleting the Pipeline SHALL delete all of its materialized children. Ownership
SHALL be determined from `ownerReferences`, never from a label alone.

#### Scenario: A block is removed

- **WHEN** an inline channel entry is deleted from a Pipeline's spec
- **THEN** the corresponding Channel is deleted while the Pipeline continues to
  reconcile its remaining children

#### Scenario: The Pipeline is deleted

- **WHEN** a Pipeline with materialized children is deleted
- **THEN** every child it owned is garbage-collected

#### Scenario: A forged label does not grant deletion

- **WHEN** an unrelated object carries the management label but no owner
  reference to the Pipeline
- **THEN** the reconciler does not delete or modify it

### Requirement: A materialized child can graduate to a standalone CR

Annotating a materialized child `agentops.dev/graduate: "true"` SHALL make the
reconciler remove its owner reference and management label, drop it from
`status.materialized`, and stop managing it. While the corresponding inline
block is still present, the Pipeline SHALL report `Ready=False` with reason
`GraduationPending` and a message naming the exact spec edit that completes the
swap, and SHALL neither recreate nor further modify the graduated object. The
condition SHALL clear once the inline block is replaced by a reference.

#### Scenario: Graduating a channel out of all-in-one

- **WHEN** a materialized Channel is annotated for graduation
- **THEN** its owner reference is removed, it survives deletion of the Pipeline,
  and the Pipeline reports `GraduationPending` with the remedy

#### Scenario: Completing the swap clears the condition

- **WHEN** the inline block is removed and a `channelRefs` entry naming the
  graduated object is added
- **THEN** the Pipeline returns to `Ready=True` and the object is not deleted

#### Scenario: No duplicate is created during the handshake

- **WHEN** a child has graduated but the inline block is still present
- **THEN** the reconciler does not create a second object of that name

### Requirement: Pipeline status reports what it owns

`Pipeline.status` SHALL list every materialized child as `{kind, name}` and
SHALL carry a `Materialized` condition reflecting whether all inline entries
currently have healthy children.

#### Scenario: Blast radius is visible before deletion

- **WHEN** an operator inspects a Pipeline with inline blocks
- **THEN** `status.materialized` lists every object that deleting the Pipeline
  would remove

#### Scenario: A failed materialization is visible

- **WHEN** a child cannot be created
- **THEN** `Materialized` is `False` with a reason naming the failing entry
