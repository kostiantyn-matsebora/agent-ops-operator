## Purpose

How every component other than the manager reads conversation and signal
state: one read-only surface the manager serves, whichever backend holds the
state, so a backend can be selected without any reader going blind.

## ADDED Requirements

### Requirement: State is read by every other component through the manager
The manager SHALL serve a state read surface, and it SHALL be the only path a
component other than the manager reads a state record through. It SHALL serve
exactly:

- `GET /state/conversations` — every conversation, each with its runs, and the
  revision to resume the feed from
- `GET /state/conversations/{name}` — one conversation with its runs, or
  not-found naming it
- `GET /state/conversations/stream` — the conversation feed
- `GET /state/signals` — every signal and the revision to resume from
- `GET /state/signals/stream` — the signal feed

Each listing SHALL be the store's whole-set index and each single read the
store's combined read. A single signal is not served on its own: no reader
reads one, since a reader takes its signals from the listing and the feed. The surface SHALL accept no selector, filter or field
list: a lookup a reader needs is added here, through a change, never expressed
in a query.

A record SHALL be served under its kind's published field names, `spec` and
`status` as the kind's reference page documents them, with the record's own
`name`, `id`, `revision`, `createdAt` and `deletionRequestedAt` (`expiresAt`
on a signal), and no other metadata.

#### Scenario: The whole set with runs
- **WHEN** a reader lists conversations
- **THEN** it receives every record the whole-set index returns, Closed and
  deletion-requested ones included, each with its runs and their recorded
  inputs, and a revision

#### Scenario: One conversation
- **WHEN** a reader reads a conversation by name
- **THEN** it receives the record and runs the combined read returns, or a
  not-found response naming the conversation

#### Scenario: One signal is read from the listing
- **WHEN** a reader needs one signal
- **THEN** it takes the signal from the listing or the feed, and the surface offers no per-signal read

#### Scenario: Nothing beyond the declared reads is expressible
- **WHEN** a reader sends a query parameter other than the feed's resume point
- **THEN** it is refused as unknown, naming the parameter

### Requirement: The read surface's feed is resumable
Each feed SHALL be a server-sent event stream over the store's change feed.
Events SHALL be `put` carrying the record (a conversation's with its runs),
`delete` carrying the name, `bookmark` carrying only a resume point, and
`resync` when the reader's point is unknown or too old. Every frame SHALL
carry the revision to resume from. A reader resuming from a kept point SHALL
observe every change made after it. Duplicates are permitted and a gap is not.

#### Scenario: Resuming sees what happened while away
- **WHEN** a reader disconnects, a conversation is updated and another deleted,
  and the reader reconnects from its kept revision
- **THEN** it observes the put and the delete

#### Scenario: A stale point is told to relist
- **WHEN** a reader presents a revision the backend no longer serves
- **THEN** the first event is `resync` with a reason, and a listing afterwards
  is consistent with the feed that follows it

### Requirement: Under `kube` the read surface serves the object's own fields
Under the `kube` backend the `spec` and `status` of a served record SHALL be
the stored object's own, and `id`, `revision`, `createdAt` and
`deletionRequestedAt` SHALL be its UID, resourceVersion, creation and deletion
timestamps, so a reader renders exactly what a direct read rendered.

#### Scenario: The console renders the same pages
- **WHEN** the console's browser API is exercised against a fixture install
  through the read surface and through a direct read of the same objects
- **THEN** every response is identical

### Requirement: Housekeeping decides orphans from the read surface
The reclaiming workload SHALL read conversations only through
`GET /state/conversations`, reading each record's name and context handle, and
SHALL hold no Kubernetes grant. The listing SHALL remain phase-blind: a Closed
or deletion-requested conversation protects its workspace and transcripts
exactly as an open one does, under every backend.

#### Scenario: The same set is reclaimed under every backend
- **WHEN** the reclaiming run executes against an install with open, Closed and
  deletion-requested conversations and one orphan directory
- **THEN** it retains every directory and transcript a listed conversation
  names and reclaims only the orphan, whichever backend is selected

#### Scenario: An unreachable manager reclaims nothing
- **WHEN** the listing fails
- **THEN** the run removes nothing, reports the failure and exits non-zero

### Requirement: A state reader states its reach
The read surface SHALL accept the master adapter token. It SHALL accept a
derived adapter token only when that adapter's `ChannelAdapter` or
`SignalAdapter` declares `spec.stateAccess: true`, which defaults to false. It
SHALL accept a dedicated state-read credential the chart renders as its own
Secret, never derived from or equal to the adapter or control-plane
credential, projected by the kubelet into the manager and into each
non-adapter reader. The manager SHALL read no Secret for any of them. A
refusal SHALL name its reason. The state-read credential SHALL open the read
surface alone.

#### Scenario: An adapter without the declaration is refused by name
- **WHEN** a channel adapter whose CR does not declare `stateAccess` presents
  its derived token
- **THEN** the read surface refuses it and the body names the adapter and the
  field

#### Scenario: The housekeeping credential opens the read surface alone
- **WHEN** the state-read credential is presented on `/state/*`, then on
  `/channel/*` and `/control-plane/*`
- **THEN** the first is served and the other two are refused

#### Scenario: One credential is refused as two
- **WHEN** the chart is rendered with `stateReadAuth.token` equal to
  `adapterAuth.token` or `controlPlaneAuth.token`
- **THEN** the render fails, naming the two values

### Requirement: The read surface is served by every replica
Every manager replica SHALL serve `/state/*` from its own store client,
without proxying to the leader. A resume point obtained from one replica SHALL
be valid on another.

#### Scenario: A non-leader serves a listing and a feed
- **WHEN** a reader connects to a replica that does not hold the lease
- **THEN** the listing and the feed are served locally and a point kept from
  the leader resumes without a gap
