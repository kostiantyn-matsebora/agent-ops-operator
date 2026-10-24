# signal-adapter-contract

## Purpose

The inbound-only HTTP contract between the manager and out-of-process signal adapters: one normalized-signal ingestion endpoint, plus source listing, cursor state, and status reporting — all authenticated with type-scoped derived tokens so adapters need no Kubernetes access.

## Requirements

### Requirement: A missing correlation id is rejected, never synthesized

The manager SHALL NOT mint, derive or default an `initiationId` for any inbound signal. A signal arriving without one SHALL be rejected with `4xx` at `POST /signal/inbound`, the response naming the missing field.

A `1.x` adapter during rollout gets the same answer — a loud contract violation, never a silently merged conversation.

The specification's COMPAT section SHALL state this rule: there is no compatibility shim, because a manager-synthesized id would re-mint grouping policy in the manager, which this contract forbids.

Replacing `fingerprint` with `initiationId` was a MAJOR move. The changelog SHALL record the replacement under version `2.0.0`, whatever later additive versions the page carries.

#### Scenario: A 1.x adapter is refused loudly

- **WHEN** an adapter posts a signal carrying the retired `fingerprint` and no `initiationId`
- **THEN** the manager responds `4xx` naming `initiationId`, creates nothing, and synthesizes no id

#### Scenario: The contract names its own break

- **WHEN** a reader opens the published specification
- **THEN** the changelog records the `fingerprint` → `initiationId` replacement under `2.0.0`, and the COMPAT section states that a missing id is rejected rather than repaired

### Requirement: The id is the sender's, never the manager's

Every signal adapter MUST supply its own `initiationId`, one rule for every lane, in one of two ways:

| Way | Means |
|---|---|
| minted | where the sender's aggregation happens — one id per "one problem" as the sender defines it, the same recurring problem reusing the same id |
| propagated | a round trip handed back a manager-issued `conversationId` — the sender or external system carries it, and the adapter passes it on VERBATIM, no re-derivation, no transformation |

The manager never determines an id and MUST treat every `initiationId` opaquely, whatever its provenance: pure equality, no parsing, no branch on the lane.

Chat is an instance of this rule, not a special lane. A fresh human request is its own aggregation unit (a fresh mint), and a reply in a bound thread is the propagation case — the thread carries the manager-issued id back. `chat-signal-origination` states both concretely.

#### Scenario: A chat reply routes by the propagated id

- **WHEN** a `chat` signal carries as `initiationId` the `conversationId` the manager stamped on an open conversation's thread binding
- **THEN** equality routing appends it to that conversation, through the same rule every other signal routes by

#### Scenario: An external system's round-trip response lands on its conversation

- **WHEN** an adapter forwards its external system's response under an id that system attached, one that originated as a `conversationId` carried out to it
- **THEN** the adapter propagates the id unchanged and equality routing appends the response to that open conversation, with no new addressing mode and no manager-side parsing

#### Scenario: The grouping decision is never the manager's

- **WHEN** any signal is posted
- **THEN** its id was supplied sender-side — minted from the sender's own aggregation or propagated verbatim from a round trip — and the manager only compares it for equality

### Requirement: Normalized signals enter through one inbound endpoint
The manager SHALL expose `POST /signal/inbound` (non-leader-gated) accepting `{source, signals: [{initiationId, labels, title?, payload?, kind?, severity?}]}` where `kind` is `alert` (default), `job`, `task`, or `chat`.

`initiationId` is the sender-minted correlation id and is REQUIRED on every signal. "These N signals are one problem" is decided wherever the id was minted — the sender or its adapter — never by the manager.

`severity` is OPTIONAL. When present, it SHALL be one of exactly `critical`, `error`, `warning`, or `info` — the vocabulary `signal-severity` owns. Absent SHALL mean unrated, the only state a signal carried before this field existed.

A signal carrying any other value SHALL be refused with `400` naming the four accepted values. Validation SHALL run before any routing, so a refused signal creates nothing.

The manager SHALL route each signal by pure id equality through its single ingest core:

- an OPEN conversation whose `conversationId` equals the signal's `initiationId`, originated by the claiming pipeline, receives the signal appended as a recurrence
- otherwise a new conversation is created, adopting the `initiationId` as its `conversationId`

The manager hosts no signal transports of its own. Every signal reaches it through this endpoint, which SHALL be the ONLY way work originates from outside a chat surface.

`kind: job` SHALL route as a job-lane input (task-style prompt) with recurrence-on-session, so later ticks resume the agent.

`kind: task` SHALL route as a task-lane input with NO recurrence-on-session, so each posted task is its own request rather than news about a standing one. Unlike `kind: chat` it SHALL NOT require a channel label, because replies reach the claiming Pipeline's channels.

`title` SHALL override the derived conversation title. Conversation binding requires the source's Ready `Pipeline` claim (the pipeline's channels + profile). Signals for an unwired source are dropped with an explicit reason in the response. Payloads SHALL be stored out-of-line (`ConversationInput`).

Delivery is at-least-once. A duplicate `initiationId` within the source's cooldown is safely absorbed.

#### Scenario: Claimed source fans out per its pipeline
- **WHEN** an adapter posts a signal for a source claimed by a Ready Pipeline binding two channels
- **THEN** the resulting conversation carries thread bindings for both pipeline channels and uses the pipeline's profile

#### Scenario: Unwired source drops with a reason
- **WHEN** an adapter posts a signal for a source no Ready Pipeline claims
- **THEN** nothing is created and the response reports queued 0 with a not-wired reason

#### Scenario: A repeated id lands on its open conversation
- **WHEN** a signal arrives whose `initiationId` equals an open conversation's `conversationId` for the claiming pipeline, outside the source's cooldown
- **THEN** the signal is appended to that conversation as a recurrence, and no new conversation is created

#### Scenario: A fresh id opens a fresh conversation
- **WHEN** a signal arrives whose `initiationId` matches no open conversation for the claiming pipeline
- **THEN** a new conversation is created with `conversationId` set to that id

#### Scenario: Distinct ids never merge
- **WHEN** two signals with different `initiationId` values arrive on one source while both conversations are open
- **THEN** each routes to its own conversation, and neither is appended to the other's

#### Scenario: Job-kind signal takes the task lane
- **WHEN** an adapter posts a signal with `kind: job`
- **THEN** the resulting input dispatches with the task/job prompt template, not the read-only investigation template

#### Scenario: Task-kind signal opens its own conversation
- **WHEN** two `kind: task` signals with different `initiationId` values are posted to one source
- **THEN** two conversations are created, each with a task-lane input — the second does not resume the first

#### Scenario: A task needs no chat surface
- **WHEN** a `kind: task` signal is posted carrying no `agentops.dev/channel` label
- **THEN** it is accepted, and the conversation binds the claiming Pipeline's channels

#### Scenario: Unknown source rejected
- **WHEN** `/signal/inbound` names a SignalSource that does not exist
- **THEN** the manager responds 404 and creates nothing

#### Scenario: An unknown severity value is refused
- **WHEN** a signal is posted with `severity: fatal`
- **THEN** the manager responds 400 naming `critical`, `error`, `warning`, and `info`, and no conversation or input is created

#### Scenario: A valid severity is accepted
- **WHEN** a signal is posted with `severity: critical`
- **THEN** the manager accepts it and routes it exactly as a signal with no severity would route

### Requirement: Programmatic origination has no endpoint of its own
The manager SHALL expose no HTTP route that creates a Conversation from a directly named `Pipeline`. A caller that wants to start work SHALL post a signal to a `SignalSource` a Ready Pipeline claims, so that which agent answers, on which channels, with which capabilities is decided by declared wiring rather than chosen by the caller.

#### Scenario: The task endpoint is gone
- **WHEN** a client posts to the removed `/task` with any body
- **THEN** the manager responds 404 — the route does not exist, and no compatibility shim answers it

#### Scenario: A caller cannot pick a pipeline
- **WHEN** a caller wants a specific Pipeline to answer
- **THEN** it posts to a source that Pipeline claims; naming the Pipeline itself is not an available addressing form

### Requirement: Adapters need no Kubernetes access
The contract SHALL carry everything a signal adapter needs: `GET /signal/sources?adapter=<t>` returns that type's sources with opaque `spec.config` and a `credentialEnvPrefix` locating projected credentials (Secret key `K` at env `<prefix>K`); `GET/PUT /signal/state/{source}/{key}` persists cursor state (manager-side, as SignalSource annotations) across restarts; `POST /signal/sources/{name}/status` sets the source's Ready condition. The prefix SHALL derive from projection metadata only — never Secret values or key enumeration.

#### Scenario: Adapter reads its sources through the contract
- **WHEN** an adapter lists `GET /signal/sources?adapter=cron`
- **THEN** it receives each `adapter: cron` source's name, raw config, and credential prefix without Kubernetes API access

#### Scenario: Cursor survives adapter restart
- **WHEN** an adapter PUTs state key `last-fire` and later restarts and GETs it
- **THEN** the previously written value is returned

#### Scenario: Invalid config surfaces on the source
- **WHEN** an adapter reports `ready: false` with a reason for a misconfigured source
- **THEN** the SignalSource's status carries a False Ready condition with that reason

### Requirement: Signal endpoints authenticated with type-scoped derived tokens
Signal endpoints SHALL authenticate with the master adapter token or a per-`SignalAdapter` derived token (distinct derivation context from channel adapters, so same-named adapters on the two surfaces never share a token), scoped to that adapter's NAME. Listing SHALL be requested as `GET /signal/sources?adapter=<name>` — the parameter names the adapter, matching `SignalSource.spec.adapter`, and the retired `?type=` SHALL fail with 400 naming the replacement instead of returning an empty list. A token scoped to one adapter SHALL receive 403 when addressing another adapter's sources.

#### Scenario: Cross-surface token refused
- **WHEN** a ChannelAdapter's derived token calls any `/signal/*` endpoint
- **THEN** the manager responds 401 (the token validates against no SignalAdapter)

#### Scenario: Validation survives manager restart
- **WHEN** the manager restarts and an adapter re-uses its previously issued token
- **THEN** the token validates by re-derivation with no stored state

#### Scenario: Own sources listed, others refused
- **WHEN** an adapter authenticates with its derived token and requests `/signal/sources?adapter=<its own name>`
- **THEN** the request succeeds, while the same token addressing another adapter's name is refused with 403

#### Scenario: Retired parameter fails loudly
- **WHEN** an adapter built against the old contract requests `/signal/sources?type=cron`
- **THEN** the manager responds 400 naming `adapter` as the expected parameter

### Requirement: The specification is a versioned reference page

The signal adapter specification SHALL live at `docs/contracts/signal-adapter.md`
as a published reference page (front matter, permalink, a Specifications
nav entry). The page is moved there by
`runtime-specification`, and this change makes it a versioned specification.

The same change SHALL verify that `.claude/rules/documentation.md`'s routing
table has a row naming this page (the existing "Runtime/channel/signal
contract" row, `docs/contracts/<contract>.md`, does) and point
`docs/contracts.md` at it.

It SHALL carry a semver version, starting at `1.0.0`, and its own changelog
recording every version with what changed in it.

It SHALL state the manager's provisions and the adapter's obligations as two
named sections, each exhaustive for its party.

The manager's provisions SHALL include at least:

| Provision | Content |
|---|---|
| one inbound endpoint | `POST /signal/inbound` accepts every normalized signal, and it is the only way work originates from outside a chat surface |
| type-scoped derived-token auth | the master adapter token or a per-`SignalAdapter` derived token scoped to the adapter's name, with a derivation context distinct from channel adapters |
| id-equality routing and cooldown | an open conversation whose `conversationId` equals the signal's `initiationId` for the claiming pipeline receives a recurrence, else a new conversation adopts the id — and cooldown keys on `initiationId`, both manager-side |
| no Kubernetes access required | source listing, cursor state and status reporting reach the adapter through the contract's own endpoints |

The adapter's obligations SHALL include at least: normalize its transport into
signals and push them, mint `initiationId` where its aggregation already
happens and mint it deterministically so at-least-once re-sends repeat the
same id, and never emit a signal about agent-ops' own machinery, per the
self-exclusion invariant.

#### Scenario: An adapter author needs the contract

- **WHEN** an author sets out to build a signal adapter
- **THEN** `docs/contracts/signal-adapter.md` states every obligation of both
  parties
- **AND** the author needs no shipped adapter's source to build against it

#### Scenario: A normative statement changes

- **WHEN** a normative statement in the specification is added, changed or
  removed
- **THEN** the version moves as the compatibility rules direct, and the
  changelog records the change under the new version

### Requirement: Request and response field tables are hand-written

The specification's field tables — the inbound signal payload, the envelope
(`source`, `signals`), the source listing entry and the state/status bodies —
SHALL be hand-written, requirement-ID style, in the shape
`docs/contracts/runtime.md` uses.

`runtime-specification` added a `struct-table` marker to `docs-generate.py`
and removed it in the same change: the Go doc comments it copied verbatim
carried rationale normal for Go source but unreadable in a spec table. No
generator targets this page, so `docs-generate.py --check` SHALL stay clean
without regenerating any block here.

The source of truth for every field named stays `NormalizedSignal` in
`platform/manager/internal/httpapi/signals.go` — a reviewer traces each row to
it by reading the source, not by running a generator.

Dropping generated regeneration leaves no CI check of its own over this
drift.

The compensating control is the "A spec change carries conformance"
governance rule. A specification change is not archivable until every
shipped adapter's own conformance suite passes against it, so a table a
reviewer missed by hand still fails there before the change lands.

#### Scenario: A field is added in code only

- **WHEN** a json-tagged field is added to the normalized signal struct
- **THEN** the specification's table is edited by hand to match, and the
  change is incomplete until it does

#### Scenario: Someone looks for a generated block here

- **WHEN** someone runs `docs-generate.py --check` expecting it to touch this
  page
- **THEN** it reports nothing to regenerate, because no marker names this
  page's tables

### Requirement: Compatibility rules make additive change safe

This is the COMPAT statement "A missing correlation id is rejected, never
synthesized" promises above: there is no compatibility shim for a missing
`initiationId`.

A `1.x` adapter, or any signal with no `initiationId`, gets the same `4xx`
naming the field, never a manager-synthesized id, matching
`docs/contracts/signal-adapter.md`'s own `COMPAT-1`.

The manager MUST ignore inbound payload fields it does not know. This codifies
the decoding behavior already shipped, and it makes an additive field a MINOR
change.

| Version part | Moves when |
|---|---|
| MAJOR | an adapter conformant with the previous version stops conforming |
| MINOR | a new optional capability or field is added |
| PATCH | a clarification that changes no obligation |

Section anchors SHALL be stable names. The guide and the shipped adapter pages
link by anchor, so renaming a published anchor is a breaking change to the
document and moves MAJOR.

#### Scenario: An unknown field arrives

- **WHEN** an inbound payload carries a field the manager does not know
- **THEN** the manager ignores it and the post proceeds, so the change that
  added the field was MINOR

#### Scenario: A published anchor is renamed

- **WHEN** a change renames a section anchor the guide or a shipped adapter
  page links
- **THEN** the change is MAJOR, because a conformant reader's reference
  stopped resolving

### Requirement: Conformance is a minimal/optional split

The specification SHALL end with a Conformance section stating that an
adapter implementing every requirement on the page except its Optional
capabilities is fully conformant. This matches `docs/contracts/runtime.md`'s
own shape, not a per-requirement test registry.

A capability is Optional only by being a row of the page's `Optional
capabilities` table, each naming its trigger. Every other requirement is
mandatory.

Judging a shipped adapter conformant SHALL mean reading that adapter's own
tests against every requirement on the page, including the conformance suite
in `platform/manager/test/conformance/` (which runs every shipped signal
adapter's built binary against a fake manager), and fixing any gap before its
claimed version moves.

#### Scenario: An adapter is judged conformant

- **WHEN** someone claims a signal adapter conforms to a specification
  version
- **THEN** every requirement on the page except Optional capabilities holds
  for that adapter, verified by reading its own tests

#### Scenario: An optional capability is missing

- **WHEN** a shipped adapter implements none of the Optional capabilities
- **THEN** it stays fully conformant, because none is required

### Requirement: The adopter guide is the station procedure

`docs/guides/signal-adapter.md` SHALL be the exact ordered procedure for
introducing a signal adapter. Each step SHALL cite the specification section it
implements by anchor, and each SHALL end in a verification.

References SHALL run one direction. The guide cites the specification, and the
specification never depends on the guide. The guide SHALL state no requirement
of its own.

#### Scenario: A step is followed

- **WHEN** a reader completes a step of the guide
- **THEN** the step's verification tells them it worked before the next step
  begins
- **AND** the cited anchor names where the obligation is normative

#### Scenario: The guide and the specification disagree

- **WHEN** a statement in the guide contradicts the specification
- **THEN** the specification wins and the guide is the defect, because the
  guide states no requirement of its own

### Requirement: Shipped signal adapters claim a specification version

Each shipped signal adapter's documentation page SHALL state the specification
version that adapter conforms to.

| Adapter | Claim lives on |
|---|---|
| `signals/alertmanager` | `docs/integrations/prometheus.md` |
| `signals/k8s-events` | `docs/integrations/kubernetes.md` |
| `signals/ha` | `docs/integrations/home-assistant.md` |
| `signals/pulse` | `docs/integrations/pulse.md` |
| `signals/telegram` | `docs/integrations/telegram.md` |
| `signals/cron` | `docs/guides/signal-adapter.md` — it has no page of its own |

#### Scenario: An adopter checks a shipped adapter

- **WHEN** an adopter opens a shipped signal adapter's page
- **THEN** it names the specification version the adapter conforms to
- **AND** that version exists in the specification's changelog

#### Scenario: An adapter with no page of its own

- **WHEN** an adopter looks for `signals/cron`'s conformance claim
- **THEN** the guide carries it, because the guide is the page that owns cron

### Requirement: A specification change carries conformance

A change that moves the MAJOR or MINOR version of
`docs/contracts/signal-adapter.md` MUST, in the same change, bring every
shipped signal adapter — `signals/cron`, `signals/alertmanager`,
`signals/k8s-events`, `signals/ha`, `signals/pulse`, `signals/telegram` —
into conformance with the new version.

The same change MUST move the version every shipped adapter page claims. The
change is not finished while any shipped adapter conforms only to the previous
version, so the specification never runs ahead of what ships.

A PATCH-only change alters no obligation, so it is exempt from the adapter walk
and the claimed versions stay as they are.

The first version, `1.0.0`, documented shipped behavior and walked that
checklist for the five adapters that shipped then. `signals/pulse` joined the
enumeration at `2.2.0` — a shipped adapter a list omits is exempt from
nothing, so the omission was the defect, never an exemption.

#### Scenario: The specification version moves

- **WHEN** a change bumps the specification's MAJOR or MINOR version
- **THEN** the same change updates every shipped signal adapter and every
  claimed version, or the change is not archivable

#### Scenario: A PATCH clarification

- **WHEN** a change moves only the PATCH version
- **THEN** no adapter or claimed version needs to change

### Requirement: The specification governs the agent's own changes

The existing scoped rule `.claude/rules/signal-rules.md` SHALL be extended, not
duplicated in a new file. It already loads for `signals/**` and
`internal/ingest/`, and SHALL also load for the signal endpoints in
`internal/httpapi/`.

The rule SHALL state that a change there is verified against the specification
before it is claimed done. It SHALL state that the specification itself is
NEVER edited without the user's explicit approval.

#### Scenario: A governed surface changes

- **WHEN** an agent session changes a signal adapter or the manager's ingest
  surface
- **THEN** the loaded rule directs it to verify the change against
  `docs/contracts/signal-adapter.md` before claiming the work done

#### Scenario: A session reaches for the specification itself

- **WHEN** an agent session would edit `docs/contracts/signal-adapter.md`
  without the user's explicit approval
- **THEN** the rule directs it to stop and ask, and the session reports the
  conflict instead of editing

### Requirement: A post-emit refractory window is the one named allowance in the no-dedup rule

An adapter MUST NOT implement dedup, cooldown or recurrence. That is the
no-dedup obligation, and it is stated here as a requirement of its own.
The manager's cooldown on `initiationId` is the one dedup authority.

The specification SHALL name ONE carve-out beside that obligation — the
post-emit refractory window. It bounds an adapter's own re-post chatter, and
decides nothing about recurrence.

| Bound | Rule |
|---|---|
| post-emit only | the window opens only when the adapter itself emitted a signal — a condition never reported is never suppressed |
| one id | it suppresses only re-posts carrying the emitted signal's own `initiationId`, never collapsing distinct ids |
| length | no longer than the emitting rule's own dwell, with a floor of one minute |
| no substitute | the manager's cooldown stays the dedup authority — the window only saves round trips cooldown would have absorbed |
| declared | an adapter relying on the window SHALL declare it, with its length, in that adapter's own capability spec |

Naming the allowance is an additive change. The specification version SHALL
move to `2.2.0`, and its changelog SHALL record the allowance under that
version.

#### Scenario: A suppressed re-post changes nothing the manager would have kept

- **WHEN** an adapter emitted a signal and the same condition recurs within the declared window
- **THEN** no re-post is made, and the conversation's state equals what the manager's cooldown would have produced for that re-post

#### Scenario: Distinct ids are never collapsed by the window

- **WHEN** a different condition with its own `initiationId` arises while another id's window is open
- **THEN** its signal is posted normally

#### Scenario: Recurrence past the window repeats the id

- **WHEN** the emitted condition still recurs after the window closes
- **THEN** the adapter re-posts under the same `initiationId` and the manager routes it as a recurrence

#### Scenario: An undeclared suppression is a conformance gap

- **WHEN** an adapter suppresses its own post-emit re-posts and its capability spec declares no window
- **THEN** the adapter is judged against the no-dedup obligation as written, and the suppression is a conformance gap
