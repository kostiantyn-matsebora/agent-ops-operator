# channel-adapter-contract — delta

## MODIFIED Requirements

### Requirement: Adapters need no Kubernetes access
The contract SHALL carry everything an adapter needs beyond ops and inbound: `GET /channel/channels?type=<t>` returns the channels of that type with their opaque `spec.config` **and a `credentialEnvPrefix` locating each channel's projected credentials in the adapter's own environment (Secret key `K` is env `<prefix>K`)**; `GET/PUT /channel/state/{channel}/{key}` persists adapter cursor state (manager-side, as Channel annotations) across adapter restarts; `POST /channel/channels/{name}/status` sets the Channel's Ready condition (adapters report their own config validation results there). The prefix SHALL be derived from projection metadata (the Channel name), never from Secret values or key enumeration.

#### Scenario: Adapter reads its config through the contract
- **WHEN** an adapter lists `GET /channel/channels?type=telegram`
- **THEN** it receives each `type: telegram` Channel's name and raw `spec.config` without any Kubernetes API access

#### Scenario: Adapter locates per-channel credentials
- **WHEN** a served Channel has `credentialsSecretRef` projected under prefix `AGENTOPS_CRED_HOME_OPS_`
- **THEN** the channel listing entry carries `credentialEnvPrefix: "AGENTOPS_CRED_HOME_OPS_"` and the adapter resolves Secret key `botToken` as env `AGENTOPS_CRED_HOME_OPS_botToken`

#### Scenario: Cursor state survives adapter restart
- **WHEN** an adapter PUTs state key `offset` and later restarts and GETs it
- **THEN** the previously written value is returned

#### Scenario: Invalid config surfaces on the Channel
- **WHEN** an adapter reports `ready: false` with a reason for a misconfigured Channel
- **THEN** the Channel's status carries a False Ready condition with that reason

### Requirement: Adapter endpoints are authenticated without manager secret reads
All `/channel/*` endpoints SHALL require a bearer token that the manager receives via its own deployment environment (no Secret API reads by the manager — the manager SHALL perform zero secret reads after this change). Requests with a missing or invalid token SHALL receive 401. Comparison SHALL be constant-time. **In addition to the master token (full scope, hand-deployed adapters), the manager SHALL accept per-adapter tokens derived as `HMAC(masterKey, adapter name)` — validated by re-derivation (stateless, no storage) and scoped to the adapter's declared `spec.type`: a per-adapter token presented for another type's ops, state, or status SHALL receive 403.**

#### Scenario: Valid adapter token
- **WHEN** an adapter calls any `/channel/*` endpoint with the shared bearer token
- **THEN** the request is served

#### Scenario: Missing or wrong token
- **WHEN** a `/channel/*` request lacks the token or presents a wrong one
- **THEN** the manager responds 401 without processing the operation or message

#### Scenario: Per-adapter token is type-scoped
- **WHEN** the `slack` adapter's derived token is used to poll `/channel/ops?type=telegram`
- **THEN** the manager responds 403

#### Scenario: Token validation survives manager restart
- **WHEN** the manager restarts and an adapter re-polls with its previously issued derived token
- **THEN** the token validates by re-derivation with no Secret reads or stored state
