## ADDED Requirements

### Requirement: The runtime loads no repository context beyond the unit's own definition
The runtime SHALL compose its model input from the unit's system prompt, the agent definition the unit names and the unit's input. It SHALL NOT load the checkout's `CLAUDE.md`, `AGENTS.md`, rules, skills or other agent definitions, so a unit carrying `repoContext: omit` is honoured by construction and a unit carrying `load` or nothing loads no more than that either.

A future addition of any repository context loading to this runtime SHALL be gated on `repoContext` so that `omit` suppresses it.

#### Scenario: Omit changes nothing here
- **WHEN** a unit carries `repoContext: omit` for a checkout containing a `CLAUDE.md`
- **THEN** the model input contains the unit's system prompt and agent definition and not the `CLAUDE.md`

#### Scenario: Load adds nothing here
- **WHEN** a unit carries no `repoContext` for the same checkout
- **THEN** the model input is the same
