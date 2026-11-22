## ADDED Requirements

### Requirement: The runtime honours repoContext
When a work unit carries `repoContext: omit`, the runtime SHALL load none of the checkout's repository context into the session: no repository instruction file, skill, hook or agent definition other than the one the unit names. The unit's own agent definition, system prompt and tool allowlist SHALL still apply. When the field is absent or `load`, behaviour SHALL be unchanged.

Where the vendor SDK offers no way to stop it loading a class of repository context, the runtime SHALL fail the run with a reason naming `repoContext`, as `runtime-contract` states, rather than load it silently.

#### Scenario: Omit suppresses repository context
- **WHEN** a unit carries `repoContext: omit` and the checkout holds repository instruction files and skills
- **THEN** none of them is in the session, and the unit's agent definition is

#### Scenario: An unsuppressable class fails the run
- **WHEN** a unit carries `repoContext: omit` and the SDK cannot be told to skip a class of repository context the checkout holds
- **THEN** the run is reported failed with a reason naming `repoContext`

#### Scenario: Absent is unchanged
- **WHEN** a unit carries no `repoContext`
- **THEN** the session is built exactly as before this change
