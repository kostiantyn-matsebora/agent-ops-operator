## Purpose

Gives every signal two attributes a route and an agent can rely on: a severity drawn from one closed vocabulary the solution owns, and a type the adopter defines as a reserved label, with each shipped adapter mapping its own scale onto the vocabulary once.

## ADDED Requirements

### Requirement: Severity is a closed vocabulary owned by the contract
A normalized signal SHALL carry an optional `severity` field, distinct from its labels.

- The value SHALL be one of exactly `critical`, `error`, `warning` or `info`.
- An absent severity SHALL mean unrated.
- The inbound endpoint SHALL refuse a batch carrying any other value with a 400 that names the vocabulary, creating nothing from that batch.
- No adapter SHALL invent a value, and no configuration SHALL widen the set.

#### Scenario: A vocabulary value is accepted
- **WHEN** an adapter posts a signal with `severity: critical`
- **THEN** the signal is admitted and the resulting conversation records `critical`

#### Scenario: An unknown value is refused loudly
- **WHEN** an adapter posts a signal with `severity: Warning`
- **THEN** the manager responds 400 naming the four accepted values, and nothing is created

#### Scenario: An unrated signal is ordinary
- **WHEN** a `kind: chat` or `kind: task` signal is posted with no severity
- **THEN** it is admitted exactly as before, and its conversation records no severity

### Requirement: Severity is kept with the conversation and with each input
The conversation SHALL record the severity of the signal that opened it in its signal provenance. Each input SHALL record the severity of the signal that produced it.

- A later signal of a different severity landing on an existing conversation SHALL be recorded on its input.
- It SHALL NOT rewrite the provenance. The provenance says how the conversation was opened, the inputs say how it went on.

#### Scenario: A recurrence rated higher is visible on the input
- **WHEN** a conversation opened by a `warning` signal receives a recurrence rated `error`
- **THEN** the conversation's provenance still says `warning` and the new input says `error`

### Requirement: Type is a reserved label key
`type` SHALL be a reserved key in the shared signal label vocabulary, with open values chosen by the adopter.

- An adapter that forwards a sender's labels SHALL pass `type` through unchanged.
- An adapter that derives labels from an observed object SHALL never let that object's own label named `type` overwrite one it did not set itself.

#### Scenario: A rule label reaches routing
- **WHEN** a Prometheus rule carries the label `type: security` and its alert fires
- **THEN** the signal's labels carry `type: security`, and a matcher on that key sees it

#### Scenario: A pod label cannot forge the key
- **WHEN** a pod carrying the label `type: maintenance` emits a Warning event
- **THEN** the event's signal carries no `type` label the adapter did not itself set

### Requirement: Each shipped adapter maps its own scale once
Every signal adapter the repository ships SHALL send the severity field mapped from its own scale, with a fixed mapping stated on the adapter's page. It SHALL stop copying severity into the payload text.

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

### Requirement: The agent is told what it is handling
The prompt of a signal-opened work unit SHALL carry the signal's kind, its severity and its labels as a metadata block above the payload.

- An agent then reads how the signal was rated and classified without parsing the payload.
- A profile supplying its own prompt file SHALL receive the same values as template variables.

#### Scenario: The investigation prompt names the severity
- **WHEN** a `critical` alert carrying `type: security` opens a conversation
- **THEN** the dispatched prompt names `critical` and `security` before the payload

#### Scenario: A custom prompt receives the variables
- **WHEN** the profile names its own prompt file
- **THEN** the work unit's variables include the severity and the labels
