# signal-severity Specification

## Purpose
One optional, closed severity vocabulary for every signal, written once onto
a conversation's provenance and again onto each input that carries it, so
routing and display have one urgency field to read instead of parsing an
adapter's own label.

## Requirements

### Requirement: Severity is a closed, optional vocabulary

A normalized signal MAY carry a `severity` field, one of exactly four
values.

| Value | Means |
|---|---|
| `critical` | highest urgency |
| `error` | a failure, below critical |
| `warning` | a degraded or notable condition |
| `info` | informational |

Absent SHALL mean unrated — the only state a signal carried before this
field existed, and the state every signal with no `severity` still carries.

`POST /signal/inbound` SHALL refuse a signal whose `severity` is present
and not one of the four values, responding `400` and naming the accepted
values. Validation SHALL run ahead of any routing decision, so a refused
signal creates nothing and appends nothing to an open conversation.

The refusal is per batch. One signal with an unmapped `severity` SHALL
refuse the whole request, and no signal in that batch is processed.

#### Scenario: An unknown value is refused

- **WHEN** a signal is posted with `severity: fatal`
- **THEN** the manager responds 400 naming `critical`, `error`, `warning`,
  and `info`, and creates nothing

#### Scenario: One bad signal refuses the whole batch

- **WHEN** a batch posts one valid signal and one with `severity: fatal`
- **THEN** the manager responds 400 and processes neither

#### Scenario: Absent severity behaves as today

- **WHEN** a signal is posted with no `severity` field
- **THEN** it is accepted and processed exactly as before this field
  existed

#### Scenario: Each vocabulary value is accepted

- **WHEN** a signal is posted carrying `critical`, `error`, `warning`, or
  `info`
- **THEN** the manager accepts it and proceeds to routing

### Requirement: Severity rides with the conversation's provenance and with each input, same as labels

Severity SHALL be stored in two places, matching where and how `labels`
are already stored.

| Stored on | Written | Rewritten by a later recurrence |
|---|---|---|
| the conversation's provenance | once, at creation, from the signal that opened the conversation | never |
| each input's own record | once per input, from the signal that produced it | never — each occurrence keeps its own |

A later signal with a different severity than the one that opened the
conversation SHALL be recorded on its own input. The conversation's
provenance SHALL keep the opening severity unchanged.

Both SHALL be omitted wherever the originating signal carried no severity.

#### Scenario: Provenance keeps the opening severity

- **WHEN** a conversation opens from a `critical` signal, and a later
  recurrence arrives `warning`
- **THEN** the conversation's provenance still reads `critical`

#### Scenario: A recurrence records its own severity

- **WHEN** a recurrence arrives with a severity different from the one
  that opened its conversation
- **THEN** the new input carries the recurrence's own severity, not the
  provenance's

#### Scenario: No severity on the originating signal leaves both unset

- **WHEN** a conversation opens from a signal with no `severity`
- **THEN** its provenance and its opening input both omit the field
