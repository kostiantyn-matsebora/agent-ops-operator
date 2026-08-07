# signal-adapter-contract — delta

## MODIFIED Requirements

### Requirement: Signal endpoints authenticated with type-scoped derived tokens
All `/signal/*` endpoints SHALL require a bearer token: the master token (env-provided, full scope) or a per-SignalAdapter token derived with the distinct context `HMAC(masterKey, "signal-adapter:" + name)` — validated by stateless re-derivation against the SignalAdapter list and **scoped to the routing key the adapter serves, which is its name**. A ChannelAdapter and a SignalAdapter sharing a name SHALL NOT share a token, and a signal-adapter token SHALL NOT authorize `/channel/*` (nor vice versa). Missing/invalid → 401; out-of-scope key → 403; constant-time comparison; zero manager Secret reads.

#### Scenario: Cross-key signal poll refused
- **WHEN** the `cron` SignalAdapter's token calls `GET /signal/sources?type=pagerduty`
- **THEN** the manager responds 403

#### Scenario: Cross-surface token refused
- **WHEN** a ChannelAdapter's derived token calls any `/signal/*` endpoint
- **THEN** the manager responds 401 (the token validates against no SignalAdapter)

#### Scenario: Validation survives manager restart
- **WHEN** the manager restarts and an adapter re-uses its previously issued token
- **THEN** the token validates by re-derivation with no stored state
