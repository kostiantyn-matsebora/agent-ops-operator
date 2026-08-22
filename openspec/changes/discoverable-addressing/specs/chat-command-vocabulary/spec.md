## Purpose

A surface can only offer what it can see, and a channel adapter is granted no
Kubernetes access — it cannot read a Pipeline and never will. This capability
covers the vocabulary the manager publishes so any surface can tell a person
what may be typed: the built-in commands and the addressable Pipelines, each
with the position it is valid in, plus the revision that tells a surface its
copy went stale.

## ADDED Requirements

### Requirement: The manager publishes what may be typed
The manager SHALL publish a single vocabulary naming everything a person may
address on a chat surface. It SHALL cover both halves of that set: the built-in
commands the manager itself intercepts, and every addressable Pipeline.

Each entry SHALL carry the name to be typed, a one-line description, and a kind
distinguishing a built-in from a Pipeline. A Pipeline entry SHALL additionally
name the profile answering for it.

The vocabulary SHALL be available to any authenticated adapter without
Kubernetes access of its own.

#### Scenario: Both halves are published
- **WHEN** a surface reads the vocabulary
- **THEN** it receives the built-in commands and every addressable Pipeline in
  one list, each with a name, a description and its kind

#### Scenario: An adapter with no cluster access can read it
- **WHEN** an adapter that holds no ServiceAccount token reads the vocabulary
- **THEN** it is served, authenticated by its adapter token alone

### Requirement: A Pipeline's description needs no new CRD field
A Pipeline entry's description SHALL be derived from what the Pipeline already
declares — the profile answering for it. No CRD field SHALL be added to carry
prose for this purpose.

#### Scenario: Description comes from existing wiring
- **WHEN** a Pipeline appears in the vocabulary
- **THEN** its description is derived from its declared profile, and the
  Pipeline CRD gains no field

### Requirement: Only addressable Pipelines appear
A Pipeline SHALL appear in the vocabulary only while it is Ready. An unready
Pipeline names wiring that does not resolve, and offering it would invite a
request nothing can serve.

The vocabulary SHALL be namespace-wide rather than per-channel, because
addressing is: a command naming a Pipeline is resolved by name, without a claim
check, and the originating surface is folded into the resulting conversation's
bindings. A Pipeline is therefore reachable from every wired surface, and a
per-channel vocabulary would understate what a person may type.

#### Scenario: Unready pipelines are absent
- **WHEN** a Pipeline's wiring does not resolve
- **THEN** it is absent from the vocabulary, matching what the Pipeline-listing
  command reports

#### Scenario: One vocabulary serves every surface
- **WHEN** two adapters of different types read the vocabulary
- **THEN** both receive the same Pipeline entries, because any Ready Pipeline is
  addressable from any wired surface

### Requirement: Every entry states where it is valid
Each entry SHALL declare the POSITION it is valid in: the general surface a
conversation starts from, or inside an existing conversation's thread.

The two positions take disjoint sets. Addressing a Pipeline SHALL be valid only
on a general surface — inside a thread the same text is input for the agent.
The commands that end or release a conversation SHALL be valid only inside a
thread — on a general surface there is no conversation to act on.

A surface that can express the distinction SHALL offer only the entries valid
where the person is typing. A surface whose transport cannot express it SHALL
NOT be required to, and the manager's existing usage replies remain the
correction for a command used in the wrong position.

#### Scenario: Position is stated per entry
- **WHEN** a surface reads the vocabulary
- **THEN** each entry declares whether it is valid on a general surface or
  inside a thread

#### Scenario: A capable surface offers only what applies
- **WHEN** a person types in a composer attached to an existing conversation
- **THEN** the surface offers the thread-position entries and not the
  Pipeline entries

#### Scenario: An incapable surface is still conformant
- **WHEN** a transport cannot vary its offered commands by position
- **THEN** offering the union is conformant, and a command used in the wrong
  position is answered with the manager's existing usage reply

### Requirement: The manager states meaning and never filters by transport
The vocabulary SHALL be published unfiltered. The manager SHALL NOT omit,
rewrite or re-spell an entry to satisfy any transport's constraints on command
names, list length, or presentation.

Which entries a surface can express SHALL be decided by the adapter serving it,
from knowledge of its own transport — the same division that already places
escaping, length limits and thread naming with the adapter.

#### Scenario: A transport-illegal name is still published
- **WHEN** a Pipeline's name cannot be registered as a command on some transport
- **THEN** the manager publishes it unchanged, and that adapter decides to
  present it another way

#### Scenario: No transport rule appears in the published vocabulary
- **WHEN** the vocabulary is inspected
- **THEN** it carries no transport-specific naming rule, escaping, ordering
  constraint or presentation hint

### Requirement: A derived revision identifies the vocabulary
The vocabulary SHALL carry a revision identifying its content. The revision
SHALL be DERIVED from the published entries, never stored: two managers serving
the same entries SHALL report the same revision, and a restart SHALL NOT change
it.

A revision SHALL change when and only when the published entries change.

#### Scenario: Restart does not change the revision
- **WHEN** the manager restarts with the same Pipelines Ready
- **THEN** it reports the same revision, and no surface refetches

#### Scenario: A pipeline becoming Ready changes the revision
- **WHEN** a Pipeline becomes Ready
- **THEN** the revision changes and the new entry is served on the next read

### Requirement: A stale copy is told over a connection the surface already holds
The manager SHALL NOT require a surface to poll the vocabulary on a timer to
learn it changed, and SHALL NOT require the ability to reach a surface: the
adapter contract is pull-only and adapters need not be addressable.

The current revision SHALL therefore be carried on the outbound operation
long-poll an adapter already holds open, on both a delivered operation and an
empty result. A surface observing a revision different from its own SHALL
refetch.

This SHALL be additive: an adapter that ignores the revision SHALL remain
conformant and SHALL behave exactly as one that never learned of the
vocabulary.

#### Scenario: A change reaches an idle adapter
- **WHEN** a Pipeline becomes Ready while an adapter is holding an outbound
  long-poll with no operations to deliver
- **THEN** the empty result carries the new revision and the adapter refetches

#### Scenario: No new connection and no timer
- **WHEN** an adapter tracks the vocabulary
- **THEN** it opens no connection beyond those the contract already defines, and
  learns of a change without polling the vocabulary on a timer

#### Scenario: An older adapter is unaffected
- **WHEN** an adapter that knows nothing of the vocabulary long-polls for
  operations
- **THEN** it receives operations exactly as before and the extra revision is
  ignored
