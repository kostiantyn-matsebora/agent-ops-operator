# agent-definition-tools Specification

## Purpose

The contract between a runtime and the agent definition in the profile's
repository: that a declared `tools:` frontmatter is honoured rather than read as
narrative, how the work unit's mode composes it with the wiring's toolsets, and
that a composition yielding nothing denies rather than substituting a default or
hanging on a permission prompt. The composition lives here because only the
runtime holds the checkout — the manager never reads it.

## Requirements
### Requirement: The agent definition's declared tools are honoured
When a work unit names an agent and the profile's checkout contains that agent's definition (`.claude/agents/<agent>.md`), the runtime SHALL read its YAML frontmatter and treat a declared `tools:` list as that agent's own capability declaration. A definition that is absent, has no frontmatter, or declares no `tools:` SHALL contribute nothing — not an error and not a default. Frontmatter the runtime cannot parse SHALL be logged and treated as contributing nothing, never as a failure that blocks the run: an unreadable role file must not stop an agent from answering.

#### Scenario: A declared tool list participates
- **WHEN** an agent definition declares `tools:` and its conversation's wiring grants a toolset
- **THEN** the agent's declared tools take part in the allowlist the runtime passes, rather than being read as narrative and ignored

#### Scenario: No definition, no contribution
- **WHEN** the profile has no repository, or the named agent's file does not exist
- **THEN** the agent contributes no tools and the wiring's toolsets stand alone

#### Scenario: Unparseable frontmatter degrades, never blocks
- **WHEN** an agent definition's frontmatter cannot be parsed
- **THEN** the runtime logs it, treats the agent as declaring no tools, and runs

### Requirement: The work unit's mode selects how the two compose
The work unit SHALL carry the wiring's tool contribution and the mode that composes it with the agent's own. `overwrite` SHALL pass the wiring's tools alone, ignoring what the agent declared. `merge` SHALL pass the union of the agent's declared tools and the wiring's, deduplicated, with the agent's own retaining their position. Composition SHALL happen in the runtime, because it is the only component with the repository checked out — the manager never reads it.

#### Scenario: Merge extends what the agent declares
- **WHEN** an agent declares `Read, Grep` and its conversation's wiring grants `Bash` in `merge` mode
- **THEN** the runtime allows `Read`, `Grep`, and `Bash`

#### Scenario: Overwrite replaces it
- **WHEN** the same agent's conversation is wired in `overwrite` mode granting `Bash`
- **THEN** the runtime allows `Bash` alone — the agent's own declaration does not apply to this route

#### Scenario: Merge with nothing declared is the wiring alone
- **WHEN** an agent declares no tools and its wiring grants a toolset in `merge` mode
- **THEN** the allowlist is exactly the wiring's tools

### Requirement: An empty allowlist denies rather than defaulting or hanging
The runtime SHALL NOT substitute a tool the operator did not declare. When composition yields nothing, it SHALL pass an empty allowlist and SHALL run in a permission mode that denies unlisted tools outright, because a headless run that falls back to interactive permission prompts hangs until its idle timeout rather than reporting anything. An agent with no declared capabilities SHALL therefore start, find it can do nothing, and say so.

#### Scenario: Nothing declared grants nothing
- **WHEN** neither the agent definition nor the conversation's wiring declares any tools
- **THEN** the runtime passes an empty allowlist and the agent has no tools — no substituted default

#### Scenario: An empty allowlist does not hang the pod
- **WHEN** a work unit dispatches with an empty allowlist
- **THEN** the run completes and reports, rather than blocking on a permission prompt no one can answer

