## ADDED Requirements

### Requirement: A built-in tool outside the composed allowlist is not offered

Where the vendor lets the runtime choose which built-in tools the model is
OFFERED, the runtime SHALL offer only the built-ins the composed allowlist
grants. An allowlist granting no built-in SHALL offer none.

Where the vendor offers no such control, the per-call denial the runtime
already owes satisfies this requirement.

MCP tools are offered by binding their servers, unchanged. A server the
wiring binds is offered whole, and the allowlist still decides each call.

This narrows what is OFFERED, never what is ALLOWED. The per-call denial
stays as the second wall, so a tool the vendor offers despite this rule is
still refused when called.

A tool the model cannot see is a tool it cannot reason from. A coordinating
conversation granted only the aops server sees the aops tools and nothing
else, and its own toolbox never contradicts "your tools are the agents".

#### Scenario: A coordinator sees only the aops tools
- **WHEN** a conversation's composed allowlist is `mcp__aops__*` alone and the vendor exposes a built-in tool list control
- **THEN** the model is offered no Bash, Read, Glob, Edit, Write or web tool, and every aops tool

#### Scenario: A granted built-in is offered
- **WHEN** a conversation's composed allowlist grants `Read` and `Grep`
- **THEN** the model is offered `Read` and `Grep` among its built-ins, and no other built-in

#### Scenario: Narrowing never widens
- **WHEN** a vendor offers a built-in the allowlist does not grant, with no control to withhold it
- **THEN** a call to it is still denied per call, exactly as today

#### Scenario: Offered and allowed stay separate decisions
- **WHEN** a runtime narrows the offered built-ins to the composed allowlist
- **THEN** it still decides every call against that allowlist, and pre-approves nothing
