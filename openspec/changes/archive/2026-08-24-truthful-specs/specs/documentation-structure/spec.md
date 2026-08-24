## ADDED Requirements

### Requirement: A published spec describes the system as it is

`openspec/specs/` is published with the repository and read as the current
contract. Every requirement in it SHALL be true of the code at the moment it is
published.

Where a spec and the code disagree, the CODE is authoritative and the spec is
the defect — this is a documentation rule, and correcting a spec SHALL never
change behaviour to suit it.

A capability SHALL carry a written `Purpose`. The scaffolding tool's placeholder
is not one: a capability whose purpose nobody wrote is a capability whose
boundary nobody has had to justify.

Retired vocabulary — a removed CRD field, a withdrawn rule, a superseded command
— SHALL NOT appear in a spec as a current claim. It MAY appear as an explicit
record that the thing was removed, and the difference between those two is the
whole of this requirement.

#### Scenario: A spec asserts a field the API does not have

- **WHEN** a published spec names a CRD field that the API types do not carry
- **THEN** the spec is corrected, and the code is not changed to add the field

#### Scenario: Two specs disagree

- **WHEN** two published specs make incompatible claims about one behaviour
- **THEN** both are corrected against the code, and the one that was wrong says
  what replaced its claim rather than dropping it silently

#### Scenario: The audit finds the code wrong

- **WHEN** a requirement is true, desirable, and NOT satisfied by the code
- **THEN** it is recorded as a defect and raised as its own change
- **AND** the requirement is NOT weakened to match the behaviour, because that
  turns a bug into a decision nobody made

#### Scenario: A capability has no written purpose

- **WHEN** a capability's `Purpose` is the scaffolding placeholder
- **THEN** a purpose is written, or the capability is merged into the one that
  absorbed it

#### Scenario: Retired vocabulary returns

- **WHEN** a change reintroduces a removed field name, rule or command as a
  current claim in any spec
- **THEN** CI fails, naming the term and the file
