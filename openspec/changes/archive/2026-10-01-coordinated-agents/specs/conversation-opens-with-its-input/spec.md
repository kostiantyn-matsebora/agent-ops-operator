## ADDED Requirements

### Requirement: A member result is an input with a member origin

A result the manager appends to a root SHALL be an input whose origin names the
member conversation and its entry name, so the record shows what was asked and
what each member answered. It SHALL be delivered to the root's bound channels
per the ordinary per-destination rule — which is every channel, since no
surface displayed it.

#### Scenario: A person on an escalated thread sees member results
- **WHEN** a root is escalated and a member later reports
- **THEN** the escalated thread receives the result attributed to the member entry
