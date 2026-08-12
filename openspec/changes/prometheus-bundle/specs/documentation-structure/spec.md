## MODIFIED Requirements

### Requirement: The document routing rule is recorded for contributors

`CLAUDE.md` SHALL state which document receives which kind of update —
concepts to `docs/concepts.md`, contracts to `docs/contracts.md`, a bundle's
components or values to that bundle's own `docs/<bundle>.md` page, upgrade steps
to `CHANGELOG.md`, README only for pitch, kind list, demo, or install — and SHALL
state the README line budget as a number.

The routing rule SHALL name bundle pages by the bundle they document, so renaming
a subchart renames its page and the rule together. `docs/prometheus-bundle.md` is
the destination for the Alertmanager and Prometheus bundle's content;
`docs/vm-bundle.md` no longer exists.

#### Scenario: Contributor finishes a behavior change

- **WHEN** a contributor consults `CLAUDE.md` for what to document after a change
- **THEN** the routing rule names the destination file for that kind of content
- **AND** the README line budget is stated numerically so overrun is checkable

#### Scenario: A subchart is renamed

- **WHEN** a bundle subchart is renamed
- **THEN** its `docs/` page is renamed with it and the routing rule names the new
  page, so no rule points at a file that no longer exists
