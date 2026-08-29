## ADDED Requirements

### Requirement: The router's Bot API endpoint is configuration
The router SHALL resolve the Bot API base URL from the environment variable
`TELEGRAM_API_BASE`, defaulting to `https://api.telegram.org` when unset, and
SHALL use it for its `getUpdates` poll loop. The chart that owns the router's
Deployment SHALL inject it alongside the existing forwarding targets and bot
token, rendering no entry when unset.

Unlike the forwarding targets and the bot token — whose absence exits the
process at startup, because a router that cannot reach its downstreams or
authenticate is useless — an absent `TELEGRAM_API_BASE` is a normal
configuration, and SHALL take the default without comment.

#### Scenario: Default installs are unchanged
- **WHEN** the router runs with `TELEGRAM_API_BASE` unset
- **THEN** it polls `https://api.telegram.org` and starts normally, since the value is optional rather than required

#### Scenario: The poll loop honours the override
- **WHEN** `TELEGRAM_API_BASE` is set to a local endpoint
- **THEN** `getUpdates` is issued against that endpoint, and the router's classification and verbatim forwarding are unchanged

#### Scenario: Redirecting the endpoint does not create a second consumer
- **WHEN** the router is pointed at a local endpoint for testing
- **THEN** there is still exactly one `getUpdates` consumer per deployment, and the single-consumer invariant is unaffected by where it points
