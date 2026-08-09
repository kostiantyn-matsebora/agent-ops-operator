# signal-adapter-contract

## Purpose

The inbound-only HTTP contract between the manager and out-of-process signal adapters: one normalized-signal ingestion endpoint, plus source listing, cursor state, and status reporting — all authenticated with type-scoped derived tokens so adapters need no Kubernetes access.

## Requirements

### Requirement: Normalized signals enter through one inbound endpoint
The manager SHALL expose `POST /signal/inbound` (non-leader-gated) accepting `{source, signals: [{fingerprint, labels, title?, payload?, kind?}]}` where `kind` is `alert` (default), `job`, `task`, or `chat`. For each signal the manager SHALL apply the source's grouping policy (cooldown by fingerprint, signature from `labels` × `signatureLabels`, window reuse, recurrence-on-session) through its single ingest core — the manager hosts no signal transports of its own; every signal reaches it through this endpoint, which SHALL be the ONLY way work originates from outside a chat surface. `kind: job` SHALL route as a job-lane input (task-style prompt) carrying the source name as its job name. `kind: task` SHALL route as a task-lane input carrying NO job name and NO recurrence-on-session, so each posted task is its own request rather than news about a standing one; unlike `kind: chat` it SHALL NOT require a channel label, because replies reach the claiming Pipeline's channels. `title` SHALL override the derived conversation title. Conversation binding requires the source's Ready `Pipeline` claim (the pipeline's channels + profile); signals for an unwired source are dropped with an explicit reason in the response. Payloads SHALL be stored out-of-line (`ConversationInput`). Delivery is at-least-once; duplicate fingerprints within cooldown are safely absorbed.

#### Scenario: Claimed source fans out per its pipeline
- **WHEN** an adapter posts a signal for a source claimed by a Ready Pipeline binding two channels
- **THEN** the resulting conversation carries thread bindings for both pipeline channels and uses the pipeline's profile

#### Scenario: Unwired source drops with a reason
- **WHEN** an adapter posts a signal for a source no Ready Pipeline claims
- **THEN** nothing is created and the response reports queued 0 with a not-wired reason

#### Scenario: Job-kind signal takes the task lane
- **WHEN** an adapter posts a signal with `kind: job`
- **THEN** the resulting input dispatches with the task/job prompt template, not the read-only investigation template

#### Scenario: Task-kind signal opens its own conversation
- **WHEN** two `kind: task` signals with different fingerprints are posted to one source declaring no `signatureLabels`
- **THEN** two conversations are created, each with a task-lane input and no job name — the second does not resume the first

#### Scenario: A task needs no chat surface
- **WHEN** a `kind: task` signal is posted carrying no `agentops.dev/channel` label
- **THEN** it is accepted, and the conversation binds the claiming Pipeline's channels

#### Scenario: Unknown source rejected
- **WHEN** `/signal/inbound` names a SignalSource that does not exist
- **THEN** the manager responds 404 and creates nothing

### Requirement: Programmatic origination has no endpoint of its own
The manager SHALL expose no HTTP route that creates a Conversation from a directly named `Pipeline`. A caller that wants to start work SHALL post a signal to a `SignalSource` a Ready Pipeline claims, so that which agent answers, on which channels, with which capabilities is decided by declared wiring rather than chosen by the caller.

#### Scenario: The task endpoint is gone
- **WHEN** a client posts to `/task` with any body
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
