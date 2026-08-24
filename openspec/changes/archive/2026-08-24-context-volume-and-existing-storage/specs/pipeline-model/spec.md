## ADDED Requirements

### Requirement: Persistence is wiring, declared on the Pipeline
A `Pipeline` SHALL declare where the conversations it originates keep their
state, in `spec.persistence`, with an independent binding for the CONTEXT volume
and for the WORKSPACE volume.

An `AgentRuntime` SHALL NOT declare either volume. A runtime is an ENGINE — an
image and its pod-level defaults — and WHERE a route's conversations persist is
a property of the ROUTE, decided beside the tools it grants, the channels it
delivers to, the runtime it selects and the identity it executes under. Those
four are already the Pipeline's.

The runtime SHALL keep `spec.contextStorage`, which is the one question only it
can answer: whether its BACKEND writes context to a disk at all. A runtime
keeping context at a vendor API needs no volume and SHALL NOT be given one.
The split is BACKEND SHAPE against PLACEMENT.

Two Pipelines sharing one runtime SHALL be able to keep their conversations on
different volumes without cloning that runtime. Requiring a clone is the same
failure that once required one to express a second trust level.

#### Scenario: A route declares its own storage
- **WHEN** a Pipeline declares a context volume binding
- **THEN** conversations it originates keep their accumulated context there, whatever runtime executes them

#### Scenario: One runtime, two routes, two volumes
- **WHEN** two Pipelines name the same runtime and different context volumes
- **THEN** each route's conversations use its own volume, and neither Pipeline needs a runtime of its own

#### Scenario: The runtime still declares its backend's shape
- **WHEN** a runtime declares that its context lives outside the operator's storage
- **THEN** no volume is required for continuity, whatever any Pipeline binds

### Requirement: A binding names a claim or a volume, and that decides who creates what
A persistence binding SHALL accept either the name of a `PersistentVolumeClaim`
that already exists, or the name of a `PersistentVolume`. A pod can mount only a
claim, so naming a VOLUME requires that something render the claim on it, and
which thing SHALL follow from where the binding was declared:

| Declared on | Renders the claim |
|---|---|
| a Pipeline | the MANAGER |
| the chart | the CHART |

This is the ONE place the operator creates storage, and it SHALL be stated
rather than implied: elsewhere in this system naming a resource never creates
it. The manager already creates Pods and ConfigMaps, so the category is not new
— what is new is that this object OUTLIVES the conversation.

A claim the manager creates SHALL NOT carry an ownerRef on the Pipeline.
Deleting a Pipeline SHALL NEVER delete the accumulated context of the
conversations it started. Storage is the one thing here whose loss cannot be
repaired by reconciling again.

#### Scenario: A Pipeline names a claim
- **WHEN** a Pipeline binds a claim that already exists
- **THEN** nothing is created and its conversations mount that claim

#### Scenario: A Pipeline names a volume
- **WHEN** a Pipeline binds a `PersistentVolume` by name
- **THEN** the manager creates a claim bound to that volume and its conversations mount it

#### Scenario: Deleting the wiring never deletes the context
- **WHEN** a Pipeline whose binding caused a claim to be created is deleted
- **THEN** the claim and its volume survive

### Requirement: Absent wiring falls back to the release default, then to ephemeral
Where a Pipeline binds no volume, its conversations SHALL use the release-wide
one the chart configures. Where the chart configures none — persistence turned
off entirely — they SHALL use ephemeral storage and the conversation SHALL
record from the outset that it cannot be continued.

The chain SHALL be exactly:

```
pipeline.spec.persistence.<volume>  ->  the chart's release default  ->  ephemeral
```

The runtime SHALL NOT appear in it. A Pipeline binding its own volume SHALL keep
it even where release-wide persistence is off, because that operator has said
where this route's state goes.

#### Scenario: No binding takes the release default
- **WHEN** a Pipeline binds no context volume and the release configures one
- **THEN** its conversations use the release's volume with nothing restated

#### Scenario: Persistence off means ephemeral
- **WHEN** release-wide persistence is disabled and a Pipeline binds nothing
- **THEN** its conversations run on ephemeral storage and say they cannot be continued

#### Scenario: A route keeps its own volume regardless
- **WHEN** release-wide persistence is disabled and a Pipeline binds its own volume
- **THEN** that route's conversations still persist there

### Requirement: The resolved volume is snapshotted into the conversation
The claim a conversation runs against SHALL be resolved ONCE, at creation, and
recorded on the `Conversation` — exactly as the execution identity is.

Nothing SHALL resolve a volume through `spec.pipelineRef` at dispatch time.
Without the snapshot, editing a Pipeline changes which volume an INFLIGHT
conversation's next pod mounts, which is a storage change applied to work
already in progress — the sharper form of the privilege change the identity
snapshot exists to prevent, because the work has already written to the old one.

#### Scenario: Re-wiring does not move work in progress
- **WHEN** a Pipeline's persistence binding is edited while one of its conversations is running
- **THEN** that conversation keeps the volume it was created against, and only later conversations use the new one
