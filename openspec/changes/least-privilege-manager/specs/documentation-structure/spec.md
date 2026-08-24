## MODIFIED Requirements

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
— SHALL NOT appear as a current claim in ANY PUBLISHED DOCUMENT. It MAY appear as
an explicit record that the thing was removed, and the difference between those
two is the whole of this requirement.

**The guard's scan set SHALL cover every document the repository publishes**, not
a subset of them. Specs, the site's pages, the README and the shipped values are
not the whole of what a stranger reads: the root policy files —
security reporting, contributing, the code of conduct, the issue and pull-request
templates — and the architecture decision records are published on the same
terms and SHALL be scanned on the same terms.

- **A rule enforced over a subset is enforced nowhere the subset does not
  reach**, and the gap is invisible from a passing build. A retired key survived
  in the security policy for exactly as long as that file sat outside the scan.
- **Adding a published document means adding it to the scan set**, in the change
  that publishes it.

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

#### Scenario: Retired vocabulary sits outside the scan set

- **WHEN** a published document that the guard does not scan names a retired
  field, rule or command as a current claim
- **THEN** the document is corrected AND the scan set is widened to reach it, so
  the same file cannot drift again

#### Scenario: A new published document is added

- **WHEN** a change publishes a document a stranger will read
- **THEN** that document is in the guard's scan set before the change is done
