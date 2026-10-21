## ADDED Requirements

### Requirement: SignalSource may filter by signal attribute before routing

A `SignalSource` MAY declare `spec.attributeFilters`, a list of
requirements shaped like a Kubernetes `matchExpressions` entry:

| Field | Holds |
|---|---|
| `key` | the attribute label name to test |
| `operator` | `In`, `NotIn`, `Exists`, or `DoesNotExist` |
| `values` | a string list, required for `In`/`NotIn`, empty for `Exists`/`DoesNotExist` |

An unknown or malformed `operator` SHALL be rejected at the API level, not
silently ignored at ingest time.

Each requirement is evaluated against the normalized signal's attribute
labels, with the same matching semantics a Kubernetes label selector
requirement applies.

A signal SHALL be admitted only when it satisfies EVERY requirement in
the list. Failing any one of them drops the signal.

An empty or absent `attributeFilters` SHALL admit every signal. That is
identical to today's behavior with no filter declared at all.

Filtering SHALL be evaluated before cooldown is consulted or spent, and
before the source's `Wired` pipelines are routed to. A signal the filter
drops:

- SHALL NOT count toward cooldown suppression
- SHALL NOT reach any serving pipeline
- SHALL NOT be counted as received by the source

#### Scenario: A signal matching every requirement routes normally

- **WHEN** a SignalSource declares `attributeFilters` and a signal arrives
  whose labels satisfy every requirement in the list
- **THEN** the signal proceeds to cooldown and routing exactly as it would
  with no filter declared

#### Scenario: A signal failing one requirement is dropped before cooldown

- **WHEN** a signal arrives whose labels fail at least one requirement in
  the source's `attributeFilters`
- **THEN** the signal is dropped before cooldown is consulted, consumes no
  cooldown state, and never reaches a serving pipeline

#### Scenario: Empty or absent attributeFilters admits everything

- **WHEN** a SignalSource sets `attributeFilters` to an empty list, or
  omits the field entirely
- **THEN** every signal is admitted to cooldown and routing unchanged, as
  if no filter existed

#### Scenario: An unknown operator is rejected at creation

- **WHEN** a SignalSource is applied with an `attributeFilters` entry whose
  `operator` is not one of `In`, `NotIn`, `Exists`, or `DoesNotExist`
- **THEN** the API server rejects the request with a validation error
  rather than accepting it and ignoring the requirement at ingest time

### Requirement: SignalSource controls whether a filtered drop is recorded

A `SignalSource` MAY declare `spec.logFiltered`, a nullable boolean. When
unset or null, it behaves as `true`.

It governs ONLY whether a signal dropped by `attributeFilters` is
recorded as a dropped-signal activity event. It SHALL have no effect on
whether the signal itself is dropped.

It SHALL have no effect on the recording of any OTHER drop reason. An
unwired source, an at-capacity backlog, or an ambiguous bare chat message
SHALL always be recorded regardless of this field's value.

#### Scenario: logFiltered false suppresses only the filtered-drop record

- **WHEN** a SignalSource sets `logFiltered: false` and a signal is
  dropped by `attributeFilters`
- **THEN** no dropped-signal activity event is recorded for that signal,
  though the signal is still dropped

#### Scenario: Other drop reasons still log regardless of logFiltered

- **WHEN** a SignalSource sets `logFiltered: false` and a separate signal
  on the same source is dropped for being unwired or at capacity
- **THEN** a dropped-signal activity event is still recorded for that
  signal

#### Scenario: logFiltered unset behaves as true

- **WHEN** a SignalSource omits `logFiltered` (or sets it to null) and a
  signal is dropped by `attributeFilters`
- **THEN** a dropped-signal activity event is recorded for that signal,
  the same as if `logFiltered: true` had been set explicitly
