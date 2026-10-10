## MODIFIED Requirements

### Requirement: A tasks file names the agent that fulfils each implementation section

The rules injected when a tasks file is generated SHALL require each
implementation section to name the role agent that fulfils it, or none
where no role fits.

The three trailing sections — unit tests, E2E tests, documentation — keep
their shape and are not required to name one.

#### Scenario: A tasks file is generated

- **WHEN** a change's tasks file is generated
- **THEN** each implementation section names exactly one of the five role
  agents, and a section no role fits names none and is worked by the session
  itself
