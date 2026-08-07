# signal-adapter-contract — delta

## MODIFIED Requirements

### Requirement: Signal endpoints authenticated with type-scoped derived tokens
Signal endpoints SHALL authenticate with the master adapter token or a per-`SignalAdapter` derived token (distinct derivation context from channel adapters, so same-named adapters on the two surfaces never share a token), scoped to that adapter's NAME. Listing SHALL be requested as `GET /signal/sources?adapter=<name>` — the parameter names the adapter, matching `SignalSource.spec.adapter`, and the retired `?type=` SHALL fail with 400 naming the replacement instead of returning an empty list. A token scoped to one adapter SHALL receive 403 when addressing another adapter's sources.

#### Scenario: Own sources listed, others refused
- **WHEN** an adapter authenticates with its derived token and requests `/signal/sources?adapter=<its own name>`
- **THEN** the request succeeds, while the same token addressing another adapter's name is refused with 403

#### Scenario: Retired parameter fails loudly
- **WHEN** an adapter built against the old contract requests `/signal/sources?type=cron`
- **THEN** the manager responds 400 naming `adapter` as the expected parameter
