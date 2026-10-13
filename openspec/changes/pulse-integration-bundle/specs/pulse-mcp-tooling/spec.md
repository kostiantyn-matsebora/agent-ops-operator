## Purpose

Pulse access as MCP tools in `pulse-bundle`: a first-party server,
`platform/mcp-pulse/`, fronting Pulse's REST API, plus two risk-split
`MCPConfig`/`MCPToolset` pairs any Pipeline or AgentCapability can bind.

The real wall between the two pairs is which pre-scoped Pulse API token
each one's Secret carries, not the toolset allowlist.

## ADDED Requirements

### Requirement: mcp-pulse forwards the caller's own token and decides no reach itself
`platform/mcp-pulse/` SHALL be a stateless server, shaped like
`platform/mcp-aops`: streamable HTTP, one endpoint, no server-side session
between calls.

It SHALL hold no Pulse credential of its own. Every call SHALL forward
whatever bearer token the caller's bound `MCPConfig` header supplied,
unchanged, to the matching Pulse REST endpoint.

Pulse's own API is what accepts or refuses the call by that token's
scope. `mcp-pulse` makes no scope decision itself.

#### Scenario: Two callers, two scopes, two outcomes
- **WHEN** one caller's bound token carries `monitoring:read` only and
  another's carries `monitoring:read`+`monitoring:write`, and both call an
  operator-state write tool
- **THEN** the first call is refused by Pulse (insufficient scope) and the
  second succeeds, with `mcp-pulse` itself making no distinction between
  the two calls beyond relaying each token

#### Scenario: mcp-pulse needs no Kubernetes RBAC
- **WHEN** `mcp-pulse` is deployed
- **THEN** it reads only Pulse's HTTP API and is granted no Kubernetes RBAC,
  running under its own ServiceAccount distinct from the runtime
  ServiceAccount — setting them equal SHALL fail the render

### Requirement: Two MCPConfig/MCPToolset pairs front the same server
The bundle SHALL render two `MCPConfig`s, `pulse-observer-mcp` and
`pulse-operator-mcp`, both naming the same `mcp-pulse` server key and the
same deployed Service URL. They SHALL differ only in which Secret-backed
header each supplies as the forwarded bearer token.

Each SHALL pair with its own `MCPToolset`: `pulse-observability` naming
read-only tool patterns (inventory, metrics, alerts, storage history), and
`pulse-admin` adding the Community-tier operator-state write tools
(maintenance windows, criticality tags).

Patterns SHALL be enumerated, not wildcarded, since both pairs share one
server key and a wildcard would span both risk tiers.

Neither `MCPConfig` nor `MCPToolset` SHALL express or enforce a permission
scope. The scope lives entirely in which pre-scoped token string the
installer put in which Secret, exactly as `wiring.md` already states for
every other bundle.

#### Scenario: Enabling only the observer component renders only its pair
- **WHEN** the bundle renders with the operator component off
- **THEN** `pulse-operator-mcp` and `pulse-admin` do not render, and no
  route can reference them

#### Scenario: A wildcard pattern is rejected
- **WHEN** either toolset's `tools` list is authored with a
  `mcp__pulse__*` wildcard instead of enumerated names
- **THEN** the render or its test fixture flags it, because a wildcard
  would surface the other tier's tools regardless of which token answers
  the call

### Requirement: Credentials arrive as a token or an existing Secret, doubled per profile
Each profile's Pulse API token SHALL be suppliable either as a raw value
(the bundle creates the Secret) or as an `existingSecret` reference,
mirroring the Home Assistant and Telegram bundles' credential shape.

The two profiles SHALL be configured independently. An install may supply
one without the other.

#### Scenario: A raw token value creates its own Secret
- **WHEN** the observer profile's credential is supplied as a raw token
  value
- **THEN** the bundle creates a Secret holding it and binds the observer
  `MCPConfig`'s header to it

#### Scenario: An existing Secret is referenced, not duplicated
- **WHEN** the operator profile's credential names an existing Secret
- **THEN** the bundle creates no Secret of its own and binds the operator
  `MCPConfig`'s header to the named one

### Requirement: The server's own Pulse backend URL is required when the component is active
`mcp-pulse` SHALL require a values-supplied Pulse base URL to reach,
failing the render when the component is active and the URL is empty —
`mcp-pulse` itself has no in-cluster endpoint to default onto, since Pulse
is always operator-supplied.

#### Scenario: Missing backend URL fails loudly
- **WHEN** the mcp-pulse component is enabled with no backend URL
- **THEN** the render fails naming the required value, rather than
  deploying a server that can reach nothing
