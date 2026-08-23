## ADDED Requirements

### Requirement: A shipped example carries a placeholder, never a real value

Documentation, chart values, sample manifests and test fixtures are copied by
adopters and read by strangers. Every identifier in them — a hostname, a
repository URL, a chat or group identifier, an address literal, a person's name
— SHALL be a documented placeholder from the publication guard's allowlist, and
never a value from a real deployment or a real person.

This holds even when the real value is more convincing. An example that works
when pasted unchanged is a leak that looks like a courtesy.

Where a placeholder already exists for a kind of value, THAT placeholder SHALL
be used rather than a second one invented beside it. Two placeholders for one
kind is how a real value gets chosen as the "better" example.

#### Scenario: A new field is documented

- **WHEN** a page, a values file or a sample gains an identifier
- **THEN** the value shown is a documented placeholder
- **AND** the publication guard permits it by name

#### Scenario: A real value is more convenient

- **WHEN** an author has a working value to hand and no placeholder exists for
  that kind
- **THEN** a placeholder is added to the allowlist first, and the real value is
  not committed in the meantime

#### Scenario: A fixture needs a person

- **WHEN** a test needs a sender, an author or an operator
- **THEN** the name states the ROLE the fixture exercises rather than naming
  anyone, so the test reads as what it tests

#### Scenario: The rule is enforced rather than remembered

- **WHEN** any change adds an identifier outside the allowlist
- **THEN** CI fails on it, in the tree and in the commit message alike
