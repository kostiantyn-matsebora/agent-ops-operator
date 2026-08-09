## MODIFIED Requirements

### Requirement: Chat grouping defaults preserve human behavior

A chat `SignalSource` SHALL default to `cooldownHours: 0` and SHALL NOT apply
signature grouping unless explicitly configured, so each general-surface message
opens its own conversation.

The no-grouping half of that default SHALL NOT be chat-specific machinery. It is
the general rule for one-shot lanes — `chat` and `task` alike — that a source
declaring no `signatureLabels` keys on the signal's own fingerprint, while the
recurring-subject lanes (`alert`, `job`) keep the default alert labels. Chat
inherits that rule rather than owning it. The `cooldownHours: 0` default remains
chat's own, because a person repeating themselves means it twice while a machine
re-delivering a fingerprint does not.

#### Scenario: Repeating yourself is not dedup

- **WHEN** a user sends the same text twice
- **THEN** two conversations are created, not one suppressed by cooldown

#### Scenario: Chat takes the task lane

- **WHEN** a chat signal opens a conversation
- **THEN** it uses the task-lane prompt, and a later message does not resume the
  earlier session as a recurrence

#### Scenario: Chat keying is the general one-shot rule

- **WHEN** a `chat` signal and a `task` signal each arrive at a source declaring
  no `signatureLabels`
- **THEN** both key on their own fingerprint by the same rule, with no
  kind-specific branch applying to chat alone
