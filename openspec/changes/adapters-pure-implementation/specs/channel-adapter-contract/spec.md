# channel-adapter-contract — delta

## MODIFIED Requirements

### Requirement: Adapter endpoints are authenticated without manager secret reads
All `/channel/*` endpoints SHALL require a bearer token that the manager receives via its own deployment environment (no Secret API reads by the manager — the manager SHALL perform zero secret reads after this change). Requests with a missing or invalid token SHALL receive 401. Comparison SHALL be constant-time. In addition to the master token (full scope, hand-deployed adapters), the manager SHALL accept per-adapter tokens derived as `HMAC(masterKey, adapter name)` — validated by re-derivation (stateless, no storage) and **scoped to the routing key the adapter serves, which is its name**: a per-adapter token presented for another key's ops, state, or status SHALL receive 403.

#### Scenario: Valid adapter token
- **WHEN** an adapter calls any `/channel/*` endpoint with the shared bearer token
- **THEN** the request is served

#### Scenario: Missing or wrong token
- **WHEN** a `/channel/*` request lacks the token or presents a wrong one
- **THEN** the manager responds 401 without processing the operation or message

#### Scenario: Per-adapter token is scoped to its name
- **WHEN** the `slack` adapter's derived token is used to poll `/channel/ops?type=telegram`
- **THEN** the manager responds 403

#### Scenario: Token validation survives manager restart
- **WHEN** the manager restarts and an adapter re-polls with its previously issued derived token
- **THEN** the token validates by re-derivation with no Secret reads or stored state
