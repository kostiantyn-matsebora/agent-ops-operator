## Purpose

Gives every signal an optional severity drawn from one closed vocabulary the
contract owns, with each shipped adapter mapping its own scale onto it once,
so a route or a reader can tell how urgent a signal was without parsing an
adapter-specific label.

## ADDED Requirements

### Requirement: Severity is a closed vocabulary owned by the contract
A normalized signal SHALL carry an optional `severity` field, distinct from
its labels.

- The value SHALL be one of exactly `critical`, `error`, `warning` or
  `info`.
- An absent severity SHALL mean unrated.
- The inbound endpoint SHALL refuse a batch carrying any other value with a
  400 that names the vocabulary, creating nothing from that batch.
- No adapter SHALL invent a value, and no configuration SHALL widen the
  set.

#### Scenario: A vocabulary value is accepted
- **WHEN** an adapter posts a signal with `severity: critical`
- **THEN** the signal is admitted and the resulting conversation records
  `critical`

#### Scenario: An unknown value is refused loudly
- **WHEN** an adapter posts a signal with `severity: Warning`
- **THEN** the manager responds 400 naming the four accepted values, and
  nothing is created

#### Scenario: An unrated signal is ordinary
- **WHEN** a `kind: chat` or `kind: task` signal is posted with no
  severity
- **THEN** it is admitted exactly as before, and its conversation records
  no severity

### Requirement: Severity is kept with the conversation and with each input
The conversation SHALL record the severity of the signal that opened it in
its signal provenance. Each input SHALL record the severity of the signal
that produced it.

- A later signal of a different severity landing on an existing
  conversation SHALL be recorded on its input.
- It SHALL NOT rewrite the provenance. The provenance says how the
  conversation was opened, the inputs say how it went on.

#### Scenario: A recurrence rated higher is visible on the input
- **WHEN** a conversation opened by a `warning` signal receives a
  recurrence rated `error`
- **THEN** the conversation's provenance still says `warning` and the new
  input says `error`

### Requirement: Each shipped adapter maps its own scale once
Every signal adapter the repository ships SHALL send the severity field
mapped from its own scale, with a fixed mapping stated on the adapter's
page. It SHALL stop copying severity into the payload text where it did.

| Adapter | Maps |
|---|---|
| k8s-events | Event type `Warning` to `warning`, `Normal` to `info` |
| ha | `CRITICAL` to `critical`, `ERROR` to `error`, `WARNING` to `warning`, `INFO` and `DEBUG` to `info` |
| alertmanager | the alert's `severity` label through the source's `config.severityMap`, identity for `critical`, `warning` and `info` when the map is absent |
| cron | none, the tick is unrated |

#### Scenario: A Kubernetes warning arrives rated
- **WHEN** a `Warning` event is normalized
- **THEN** the signal's severity is `warning`

#### Scenario: A Home Assistant error arrives rated
- **WHEN** an `ERROR` log record passes the adapter's rules
- **THEN** the signal's severity is `error`
