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
When a work unit names an agent and the profile's checkout contains that agent's definition, the runtime SHALL read its YAML frontmatter and treat a declared `tools:` list as that agent's own capability declaration. WHERE the definition lives is the RUNTIME's fact, not the contract's: `runtime-claude` and `runtime-ollama` read `.claude/agents/<agent>.md`, `runtime-copilot` reads `.github/agents/<agent>.agent.md`, and another backend may read somewhere else. A definition that is absent, has no frontmatter, or declares no `tools:` SHALL contribute nothing — not an error and not a default. Frontmatter the runtime cannot parse SHALL be logged and treated as contributing nothing, never as a failure that blocks the run: an unreadable role file must not stop an agent from answering.

"Contributes nothing" is the CONTRACT's rule and binds every runtime, including one whose vendor reads an absent declaration as a grant of everything. A runtime SHALL neutralise such a default at the boundary — passing the composed allowlist explicitly, empty when the composition produced nothing — rather than letting the vendor's reading of an omitted field widen a route nobody widened.

#### Scenario: A declared tool list participates
- **WHEN** an agent definition declares `tools:` and its conversation's wiring grants a toolset
- **THEN** the agent's declared tools take part in the allowlist the runtime passes, rather than being read as narrative and ignored

#### Scenario: No definition, no contribution
- **WHEN** the profile has no repository, or the named agent's file does not exist at the path this runtime reads
- **THEN** the agent contributes no tools and the wiring's toolsets stand alone

#### Scenario: Unparseable frontmatter degrades, never blocks
- **WHEN** an agent definition's frontmatter cannot be parsed
- **THEN** the runtime logs it, treats the agent as declaring no tools, and runs

#### Scenario: Each runtime reads its own vendor's path
- **WHEN** one profile's repository is executed by a route on a claude runtime and by a route on a copilot runtime
- **THEN** each runtime reads the definition path its vendor defines, and neither treats the other's absent path as an error

#### Scenario: A permissive vendor default does not become a grant
- **WHEN** a runtime's vendor would read an agent definition with no `tools:` as granting every tool
- **THEN** that runtime passes the composed allowlist explicitly instead, so the agent still contributes nothing and an empty composition stays empty

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
The runtime SHALL NOT substitute a tool the operator did not declare. When
composition yields nothing, the agent SHALL have no tools.

A headless run SHALL never fall back to an interactive permission prompt. A
prompt no one can answer hangs the run until its idle timeout rather than
reporting anything.

An agent with no declared capabilities SHALL therefore start, find it can do
nothing, and say so.

HOW the allowlist is enforced is the runtime's choice, and the rules bind
every form of it. Passing the composed list to the vendor with a mode that
denies unlisted tools satisfies this.

So does deciding each invocation against the composed list in a permission
callback, provided the callback is reached for EVERY call it is meant to
govern and answers every one of them.

A runtime taking the callback form SHALL NOT also pre-approve those tools by
another mechanism. A pre-approval that resolves a call before the callback
sees it makes the callback's rules unenforced while appearing to enforce
them.

It SHALL NOT select a mode that skips the callback either, since a rule set
that is never consulted denies for the wrong reason.

This requirement is this project's own expression of
`docs/contracts/runtime.md`'s RT-3 (Composing the allowlist). It states
which enforcement mechanisms satisfy RT-3, and SHALL NOT be read as an
exception to it.

#### Scenario: Nothing declared grants nothing
- **WHEN** neither the agent definition nor the conversation's wiring declares any tools
- **THEN** the agent has no tools — no substituted default

#### Scenario: An empty allowlist does not hang the pod
- **WHEN** a work unit dispatches with an empty allowlist
- **THEN** the run completes and reports, rather than blocking on a permission prompt no one can answer

#### Scenario: A per-invocation decision satisfies the same rules
- **WHEN** a runtime enforces the composed allowlist by deciding each tool invocation rather than by passing a list
- **THEN** an unlisted tool is denied, a denial is answered immediately rather than prompting, and an empty composition denies everything

#### Scenario: No mechanism may shadow the decision point
- **WHEN** a runtime decides invocations in a callback
- **THEN** it does not additionally pre-approve those tools elsewhere, and does not run in a mode that resolves calls without consulting the callback
