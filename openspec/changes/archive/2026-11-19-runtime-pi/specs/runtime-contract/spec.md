## MODIFIED Requirements

### Requirement: A specification change carries conformance

A change that edits `docs/contracts/runtime.md` MUST, in the same change,
bring every shipped runtime — `runtimes/claude`, `runtimes/claude-cli`,
`runtimes/ollama`, `runtimes/copilot`, `runtimes/pi` — into conformance with
the new version.

The same change MUST move the version each shipped runtime page claims. The
change is not finished while any shipped runtime conforms only to the
previous version, so the specification never runs ahead of what ships.

#### Scenario: The specification version moves

- **WHEN** a change bumps the specification version
- **THEN** the same change updates every shipped runtime and every shipped
  runtime page's claimed version, or the change is not archivable

#### Scenario: A new runtime joins the shipped set

- **WHEN** a change adds a shipped runtime
- **THEN** that runtime's page claims the specification's current version
- **AND** the runtime has been walked against the numbered conformance
  checklist, each item with its own verdict
