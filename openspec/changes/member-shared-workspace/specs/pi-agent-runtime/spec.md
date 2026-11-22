## MODIFIED Requirements

### Requirement: Reading the checkout's `.claude/` configuration is optional, on by default, and adds no tool
The image SHALL carry the `pi-code` package at an exact version from the lockfile.

- The runtime SHALL load it only when `PI_CODE_ENABLED` is not `false`, `0`, `no` or `off`, AND the conversation's work unit does not carry `repoContext: omit`.
- The bundle SHALL expose one boolean, `pi.piCode`, default true, that becomes that env. It is a ceiling for the whole install: a unit carrying `repoContext: load` or none SHALL NOT turn the package on when the env is off.
- The package SHALL add no tool to a session.
- The runtime's permission extension (its `tool_call` hook) SHALL still answer every call from the composed allowlist.
- The runtime SHALL record its own workspace as approved, so the package loads the checkout's configuration on a headless run, except when `repoContext` is `omit`, when it SHALL record no approval for that conversation.
- With it off, whether by the env or by `omit`, the session SHALL load exactly the runtime's own two extensions.
- With `repoContext: omit` the runtime SHALL still append the agent definition's body named by the unit to the system prompt, and SHALL NOT load the checkout's `AGENTS.md` or `CLAUDE.md`, which its own session setup already declines to do.

#### Scenario: The flag is off
- **WHEN** `pi.piCode` is `false`
- **THEN** the runtime pod carries `PI_CODE_ENABLED=false`, the session loads only the permission and MCP extensions, and no approval is recorded

#### Scenario: The flag is unset
- **WHEN** the bundle is enabled and `pi.piCode` is not set
- **THEN** the pod carries `PI_CODE_ENABLED=true` and the package's extensions load beside the runtime's own, with no load error

#### Scenario: A member omits repository context
- **WHEN** the pod carries `PI_CODE_ENABLED=true` and a work unit carries `repoContext: omit`
- **THEN** the session loads only the permission and MCP extensions, no approval is recorded, and the unit's agent definition body is still in the system prompt

#### Scenario: A unit cannot override the ceiling
- **WHEN** the pod carries `PI_CODE_ENABLED=false` and a work unit carries `repoContext: load`
- **THEN** the package is not loaded
