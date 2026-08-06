# signal-adapter-contract

## Purpose

The inbound-only HTTP contract between the manager and out-of-process signal adapters: one normalized-signal ingestion endpoint, plus source listing, cursor state, and status reporting — all authenticated with type-scoped derived tokens so adapters need no Kubernetes access.

## Requirements

### Requirement: Normalized signals enter through one inbound endpoint
The manager SHALL expose `POST /signal/inbound` (non-leader-gated) accepting `{source, signals: [{fingerprint, labels, title?, payload?, kind?}]}` where `kind` is `alert` (default) or `job`. For each signal the manager SHALL apply the source's grouping policy (cooldown by fingerprint, signature from `labels` × `signatureLabels`, window reuse, recurrence-on-session) through the same core as the built-in webhook; `kind: job` SHALL route as a job-lane input (task-style prompt), and `title` SHALL override the derived conversation title. **Conversation binding requires the source's Ready `Pipeline` claim (the pipeline's channels + profile); signals for an unwired source are dropped with an explicit reason in the response.** Payloads SHALL be stored out-of-line (`ConversationInput`) as today. Delivery is at-least-once; duplicate fingerprints within cooldown are safely absorbed.

#### Scenario: Claimed source fans out per its pipeline
- **WHEN** an adapter posts a signal for a source claimed by a Ready Pipeline binding two channels
- **THEN** the resulting conversation carries thread bindings for both pipeline channels and uses the pipeline's profile

#### Scenario: Unwired source drops with a reason
- **WHEN** an adapter posts a signal for a source no Ready Pipeline claims
- **THEN** nothing is created and the response reports queued 0 with a not-wired reason

#### Scenario: Job-kind signal takes the task lane
- **WHEN** an adapter posts a signal with `kind: job`
- **THEN** the resulting input dispatches with the task/job prompt template, not the read-only investigation template

#### Scenario: Unknown source rejected
- **WHEN** `/signal/inbound` names a SignalSource that does not exist
- **THEN** the manager responds 404 and creates nothing

### Requirement: Adapters need no Kubernetes access
The contract SHALL carry everything a signal adapter needs: `GET /signal/sources?type=<t>` returns that type's sources with opaque `spec.config` and a `credentialEnvPrefix` locating projected credentials (Secret key `K` at env `<prefix>K`); `GET/PUT /signal/state/{source}/{key}` persists cursor state (manager-side, as SignalSource annotations) across restarts; `POST /signal/sources/{name}/status` sets the source's Ready condition. The prefix SHALL derive from projection metadata only — never Secret values or key enumeration.

#### Scenario: Adapter reads its sources through the contract
- **WHEN** an adapter lists `GET /signal/sources?type=cron`
- **THEN** it receives each `type: cron` source's name, raw config, and credential prefix without Kubernetes API access

#### Scenario: Cursor survives adapter restart
- **WHEN** an adapter PUTs state key `last-fire` and later restarts and GETs it
- **THEN** the previously written value is returned

#### Scenario: Invalid config surfaces on the source
- **WHEN** an adapter reports `ready: false` with a reason for a misconfigured source
- **THEN** the SignalSource's status carries a False Ready condition with that reason

### Requirement: Signal endpoints authenticated with type-scoped derived tokens
All `/signal/*` endpoints SHALL require a bearer token: the master token (env-provided, full scope) or a per-SignalAdapter token derived with the distinct context `HMAC(masterKey, "signal-adapter:" + name)` — validated by stateless re-derivation against the SignalAdapter list and scoped to that adapter's `spec.type`. A ChannelAdapter and a SignalAdapter sharing a name SHALL NOT share a token, and a signal-adapter token SHALL NOT authorize `/channel/*` (nor vice versa). Missing/invalid → 401; out-of-scope type → 403; constant-time comparison; zero manager Secret reads.

#### Scenario: Cross-type signal poll refused
- **WHEN** the `cron` SignalAdapter's token calls `GET /signal/sources?type=pagerduty`
- **THEN** the manager responds 403

#### Scenario: Cross-surface token refused
- **WHEN** a ChannelAdapter's derived token calls any `/signal/*` endpoint
- **THEN** the manager responds 401 (the token validates against no SignalAdapter)

#### Scenario: Validation survives manager restart
- **WHEN** the manager restarts and an adapter re-uses its previously issued token
- **THEN** the token validates by re-derivation with no stored state
