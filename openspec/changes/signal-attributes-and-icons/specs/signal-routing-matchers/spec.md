## Purpose

Lets a Pipeline decide, per source claim and per channel binding, which signals it takes and which surfaces a conversation reaches, with the Kubernetes label-selector expression shape and nothing invented.

## ADDED Requirements

### Requirement: A ref may carry a matcher in the Kubernetes expression shape
Each entry of `Pipeline.spec.signalSourceRefs` and `Pipeline.spec.channelRefs` MAY carry `match`, a list of expressions.

- Each expression is `{key, operator, values}`, the Kubernetes `LabelSelectorRequirement` shape.
- The operator SHALL be `In` or `NotIn`. Any other operator SHALL be refused at admission.
- A ref with no `match` SHALL match every signal, so a Pipeline written before this field is unchanged.
- Several expressions SHALL all have to hold, as a Kubernetes selector's do.
- `In` with an empty `values` SHALL match nothing, and `NotIn` with an empty `values` SHALL match everything, as a Kubernetes selector's do.

#### Scenario: An unsupported operator is refused
- **WHEN** a Pipeline is applied with an expression whose operator is `Exists`
- **THEN** the API server rejects it naming the two accepted operators

#### Scenario: A Pipeline without matchers is unchanged
- **WHEN** a Pipeline lists sources and channels with no `match` on any ref
- **THEN** every signal on its sources opens a conversation bound to every channel, exactly as before

### Requirement: Expressions evaluate over one map
A matcher SHALL evaluate over one map built from the signal: its labels, plus `severity` and `kind` projected in as keys.

- An unrated signal SHALL have no `severity` key, so `In` on that key does not match it and `NotIn` does.
- A label named `severity` or `kind` SHALL NOT shadow the projected field. The field wins.

#### Scenario: Severity is matched as a key
- **WHEN** a ref carries `{key: severity, operator: In, values: [critical, error]}` and a `critical` signal arrives
- **THEN** the expression holds

#### Scenario: An unrated signal against NotIn
- **WHEN** a ref carries `{key: severity, operator: NotIn, values: [info]}` and a signal with no severity arrives
- **THEN** the expression holds

### Requirement: A source matcher decides whether this Pipeline opens a conversation
On a source claim, a signal whose map fails the matcher SHALL NOT open or continue a conversation on that Pipeline.

- Per-source ingest policy (cooldown, signature grouping) SHALL still be evaluated once, above the fan-out, before any matcher.
- Two Ready Pipelines whose matchers both hold SHALL both open a conversation. Sources stay shareable, and there is no tiebreak.
- A signal that matches no claiming Pipeline SHALL be reported as dropped with a reason that names matching, distinct from the unwired reason.
- Matching SHALL NOT affect `Wired` or `Ready`. A claim with a matcher is still a claim.

#### Scenario: Two Pipelines partition one source
- **WHEN** Pipeline `page` claims a source with `severity In [critical]` and Pipeline `triage` claims it with `severity NotIn [critical]`, and a `warning` alert arrives
- **THEN** one conversation opens, on `triage`

#### Scenario: A recurrence rated higher reaches the other Pipeline
- **WHEN** the same source later delivers a `critical` signal with the same signature
- **THEN** a conversation opens on `page`, and the conversation on `triage` is neither continued nor closed

#### Scenario: Nothing matches
- **WHEN** every claiming Pipeline's matcher fails for a signal
- **THEN** nothing is created and the response reports the signal dropped by matching, naming the source

### Requirement: A channel matcher decides which surfaces the conversation binds
On a channel binding, a conversation SHALL snapshot only the channels whose matcher holds for the signal that opened it.

- The snapshot is taken once, at creation, exactly as the unmatched binding set is.
- A later signal on the conversation SHALL NOT re-evaluate the binding. Escalation is a new conversation on a Pipeline whose source matcher fits.
- A conversation whose channel matchers leave no channel SHALL still open, with no threads, and its Pipeline SHALL NOT report it as a fault.
- A chat command addressing a Pipeline by name SHALL bind the surface it was typed on whatever the matchers say.

#### Scenario: Critical pages, warning does not
- **WHEN** a Pipeline binds `pager` with `severity In [critical]` and `console` with no matcher, and a `warning` alert opens a conversation
- **THEN** the conversation opens a thread on `console` only

#### Scenario: Type keeps maintenance off the pager
- **WHEN** the same Pipeline's `pager` binding also carries `{key: type, operator: NotIn, values: [maintenance]}` and a `critical` alert labelled `type: maintenance` arrives
- **THEN** the conversation opens a thread on `console` only
