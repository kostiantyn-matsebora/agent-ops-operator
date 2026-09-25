## MODIFIED Requirements

### Requirement: Sources are shareable and signals fan out
A `SignalSource` MAY be listed by any number of Ready Pipelines, of any signal kind.

- Doing so SHALL NOT produce a conflict condition and SHALL NOT affect any Pipeline's `Ready`.
- Listing a source means "I watch this", not "I own this". It makes the source wired and, on a chat surface, makes the Pipeline addressable there.

A signal admitted on a source served by several Ready Pipelines SHALL produce one conversation PER Pipeline whose claim matcher holds, each carrying that Pipeline's own profile, channel set and capabilities.

- Per-source ingest policy — fingerprint cooldown and signature grouping — SHALL be evaluated ONCE, before the fan-out and before any matcher, so a fingerprint is admitted once and then offered to each server rather than being suppressed for all but the first.
- A claim with no matcher takes every signal. A claim with one takes the signals its expressions hold for.
- Disjoint matchers partition a source. Overlapping ones fan out, exactly as unmatched claims do. Neither is a conflict.

Channels MAY likewise be referenced by multiple Pipelines. A conversation's binding set comes from the Pipeline that originated it, filtered by each binding's matcher against the signal that opened it.

#### Scenario: Two pipelines watching one alert source both investigate
- **WHEN** an alert arrives on a source two Ready Pipelines list with no matchers
- **THEN** two conversations are created, one per Pipeline, each with its own profile and capabilities, and neither Pipeline reports a conflict

#### Scenario: Cooldown is not spent by the first server
- **WHEN** a fingerprint is admitted on a source two Ready Pipelines list
- **THEN** both Pipelines receive it, and the fingerprint is recorded as admitted once for the source

#### Scenario: Several pipelines serve one chat surface
- **WHEN** two Ready Pipelines both list the same chat SignalSource
- **THEN** neither reports a conflict, both stay `Ready=True`, and both appear in the `/pipelines` listing on that surface

#### Scenario: Channel shared by two pipelines stays valid
- **WHEN** two Ready Pipelines both reference channel `web`
- **THEN** neither reports a conflict, and each pipeline's sources produce conversations bound per their own pipeline

#### Scenario: Two pipelines partition one source by severity
- **WHEN** two Ready Pipelines claim one source, one with `severity In [critical]` and one with `severity NotIn [critical]`, and a `warning` alert arrives
- **THEN** one conversation is created, on the second Pipeline, and neither reports a conflict

## ADDED Requirements

### Requirement: A ref carries its matcher beside its name
Each entry of `spec.signalSourceRefs` and `spec.channelRefs` SHALL accept an optional `match` beside `name`, in the shape and with the semantics the `signal-routing-matchers` capability states.

- The matcher is WIRING, so it lives on the Pipeline and nowhere else. SignalSource and Channel carry none.
- An entry with only `name` is the entry as it was, so no existing manifest changes meaning.

#### Scenario: A source claim with a matcher is still a claim
- **WHEN** a Ready Pipeline claims a source with `match` on the ref
- **THEN** the source reports `Wired=True` and the Pipeline reports `Ready=True`, whether or not any signal has yet matched

#### Scenario: A matcher on a Channel is refused
- **WHEN** a manifest sets a `match` on a `Channel` or a `SignalSource`
- **THEN** the field is not part of that schema and is pruned rather than honoured
