## ADDED Requirements

### Requirement: The specification is a versioned reference page

The channel adapter specification SHALL live at
`docs/contracts/channel-adapter.md` as a reference page with no front matter.

It SHALL carry a semver version, starting at `1.0.0`, and its own changelog
at its foot recording every version with what changed in it.

It SHALL be the normative statement of the channel adapter contract. No
second statement of the contract SHALL survive beside it in
`docs/contracts.md`.

#### Scenario: An adapter author needs the contract

- **WHEN** an author sets out to build a channel adapter
- **THEN** `docs/contracts/channel-adapter.md` states every obligation of
  both parties
- **AND** the author needs no shipped adapter's source to build against it

#### Scenario: A normative statement changes

- **WHEN** a normative statement in the specification is added, changed or
  removed
- **THEN** the version moves as the compatibility rules direct, and the
  changelog records the change under the new version

### Requirement: The specification states both parties

The specification SHALL state the manager's provisions and the adapter's
obligations as two named sections, each exhaustive for its party.

The manager's provisions SHALL include at least:

| Provision | Content |
|---|---|
| the ops long-poll | `GET /channel/ops?adapter=<name>&contract=<version>&wait=<seconds>`, at-least-once delivery, duplicates tolerated by op id |
| the claim window | a claimed op carries `reclaimAfterSeconds`, and an op not completed within the reclaim interval returns to the queue for another claimant |
| typed operations | `send`, `ensure-topic`, `close-topic` and `delete-conversation`, each a structured payload and never rendered display text |
| the vocabulary revision | every long-poll response carries the current revision, and the vocabulary endpoint serves the entries to the same authentication |
| derived-token auth | the master token plus per-adapter tokens derived as `HMAC(masterKey, adapter name)`, validated by re-derivation and scoped to the adapter name |
| delivery recording | completions land on the Conversation — the thread id in `status.threads[]`, the archive in `status.threadsArchived[]`, read watermarks on the thread bindings |

The adapter's obligations SHALL include at least:

- consume the `/channel/*` endpoints it needs — poll, complete, inbound
  replies, channel listing, cursor state, channel status
- parse the body grammar — the markdown subset plus the block grammar, under
  the shared recognition rules
- absorb retryable backpressure inside the claim window
- own its outbound pacing
- never re-ingest its own outbound post as inbound

#### Scenario: The pacing obligation is looked up

- **WHEN** a reader asks which party paces calls to the transport
- **THEN** the specification names the adapter as the party that paces, and
  no page claims the manager models any transport's limits

#### Scenario: The loop rule is looked up

- **WHEN** a reader asks what an adapter does with a message it posted itself
- **THEN** the specification states it is never re-ingested as inbound, as a
  named obligation of the adapter

### Requirement: Schema tables are hand-written, traced to the Go structs

Every operation and payload field table whose shape a named Go struct
carries SHALL be hand-written, in the `| Field | Type | Required |
Description |` shape `docs/contracts/runtime.md` settled on, traced to that
struct's json tags.

No generator renders these tables. `runtime-specification` tried a
`struct-table` docs-generator marker for this exact purpose and reversed it
before archiving (its own task 3.3): the generated cells copied Go
doc-comment paragraphs verbatim, producing unreadable rows. This
specification follows the hand-written shape it settled on instead.

The struct-backed sources are:

| Table | Struct | File |
|---|---|---|
| the operation envelope | `Op` | `platform/manager/internal/chat/ops.go` |
| the completion result | `OpResult` | `platform/manager/internal/chat/ops.go` |
| the typed message | `Message` | `platform/manager/internal/chat/message.go` |
| a message choice | `Choice` | `platform/manager/internal/chat/message.go` |
| the topic descriptor | `TopicDescriptor` | `platform/manager/internal/chat/message.go` |
| the claimed-op wrapper | `claimedOp` | `platform/manager/internal/httpapi/server.go` |
| the inbound request | `inboundReq` | `platform/manager/internal/httpapi/server.go` |
| a read-report entry | `readEntry` | `platform/manager/internal/httpapi/channelread.go` |
| a read-report outcome | `readOutcome` | `platform/manager/internal/httpapi/channelread.go` |

The one shape no named struct backs — the channel-listing entry, a
function-local type in `internal/httpapi/server.go` — SHALL carry a
hand-written table like every other, noted as backed by no named struct
rather than by a different authoring method.

A change that edits a struct-backed source's json tags MUST edit the
matching table in the same change. Review is what catches a missed row —
no generator exists to catch it for a reviewer.

#### Scenario: A struct's json tags change

- **WHEN** a field is added, removed or renamed on a struct-backed source
- **THEN** the change also edits that source's table, and a reviewer checks
  the table against the struct before approving

### Requirement: Compatibility rules make additive change safe

An adapter MUST ignore operation fields it does not know. The manager MUST
ignore fields it does not know in completions, inbound requests and read
reports.

Both rules codify the lenient decoding already shipped on both sides, and
together they make an additive field a MINOR change.

| Version part | Moves when |
|---|---|
| MAJOR | an adapter conformant with the previous version stops conforming |
| MINOR | a new optional capability or field is added |
| PATCH | a clarification that changes no obligation |

The document version is NOT the wire contract version. The `contract=<version>`
query parameter the adapter declares on the long-poll keeps its own integer
series, and the specification SHALL state the mapping between the two.

Section anchors SHALL be stable names. The guide and the shipped adapter
pages link by anchor, so renaming a published anchor is a breaking change to
the document and moves MAJOR.

#### Scenario: An unknown field arrives

- **WHEN** an operation carries a field the adapter does not know, or a
  completion carries one the manager does not know
- **THEN** the receiver ignores it and the operation proceeds, so the change
  that added the field was MINOR

#### Scenario: The document moves without the wire moving

- **WHEN** the specification gains a MINOR version for an additive field
- **THEN** the `contract` parameter a conformant adapter declares on the
  long-poll does not change

#### Scenario: A published anchor is renamed

- **WHEN** a change renames a section anchor the guide or a shipped adapter
  page links
- **THEN** the change is MAJOR, because a conformant reader's reference
  stopped resolving

### Requirement: A numbered conformance checklist is the spine

The specification SHALL end with a numbered conformance checklist. Each item
SHALL be individually verifiable against a running adapter or its source, and
every normative obligation SHALL be reachable from an item.

Where a shipped adapter's test suite already verifies an item, the item SHALL
cite that test by name, so the claim and its proof are one lookup apart.

#### Scenario: An adapter is judged conformant

- **WHEN** someone claims an adapter conforms to a specification version
- **THEN** the claim is the checklist walked item by item, each with its own
  verdict, never an impression of the whole

#### Scenario: An item with a shipped proof is checked

- **WHEN** a reader checks an item a shipped adapter's test already verifies
- **THEN** the item names that test, and running it is the verification

### Requirement: The adopter guide is the station procedure

`docs/guides/channel-adapter.md` SHALL be the exact ordered procedure for
introducing a channel adapter. Each step SHALL cite the specification section
it implements by anchor, and each SHALL end in a verification.

References SHALL run one direction. The guide cites the specification, and
the specification never depends on the guide. The guide SHALL state no
requirement of its own.

#### Scenario: A step is followed

- **WHEN** a reader completes a step of the guide
- **THEN** the step's verification tells them it worked before the next step
  begins
- **AND** the cited anchor names where the obligation is normative

#### Scenario: The guide and the specification disagree

- **WHEN** a statement in the guide contradicts the specification
- **THEN** the specification wins and the guide is the defect, because the
  guide states no requirement of its own

### Requirement: Shipped channel adapters claim a specification version

Each shipped channel adapter's documentation page SHALL state the
specification version that adapter conforms to. The shipped channel adapters
are `channels/telegram` and `platform/console` — the console is a channel
adapter like any other.

#### Scenario: An adopter checks a shipped adapter

- **WHEN** an adopter opens a shipped channel adapter's page
- **THEN** it names the specification version the adapter conforms to
- **AND** that version exists in the specification's changelog

### Requirement: A specification change carries conformance

A change that edits `docs/contracts/channel-adapter.md` MUST, in the same
change, bring every shipped channel adapter — `channels/telegram` and
`platform/console` — into conformance with the new version.

The same change MUST move the version each shipped adapter's page claims. The
change is not finished while any shipped adapter conforms only to the
previous version, so the specification never runs ahead of what ships.

#### Scenario: The specification version moves

- **WHEN** a change bumps the specification version
- **THEN** the same change updates every shipped channel adapter and every
  claimed version on its page, or the change is not archivable

### Requirement: The specification governs the agent's own changes

A scoped context rule SHALL load for `channels/**`, the console's adapter
surface (`platform/console/*.go`) and the manager's channel surface:
`internal/chat/` and the `/channel/*` endpoints in `internal/httpapi/`.

The rule SHALL state that a change there is verified against the
specification before it is claimed done. It SHALL state that the
specification itself is NEVER edited without the user's explicit approval.

#### Scenario: A governed surface changes

- **WHEN** an agent session changes an adapter or the manager's channel
  surface
- **THEN** the loaded rule directs it to verify the change against
  `docs/contracts/channel-adapter.md` before claiming the work done

#### Scenario: A session reaches for the specification itself

- **WHEN** an agent session would edit `docs/contracts/channel-adapter.md`
  without the user's explicit approval
- **THEN** the rule directs it to stop and ask, and the session reports the
  conflict instead of editing
