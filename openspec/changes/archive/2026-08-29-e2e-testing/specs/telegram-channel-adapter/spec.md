## ADDED Requirements

### Requirement: The Bot API endpoint is configuration, not a compiled-in constant
The adapter SHALL resolve the Bot API base URL from the environment variable
`TELEGRAM_API_BASE`, defaulting to `https://api.telegram.org` when unset, and
SHALL use it for every Bot API call it makes — message sends, document sends,
and forum topic creation and closure alike. The chart SHALL expose it as an
optional value on the telegram bundle that renders no environment entry when
unset, so a default install is byte-identical to today's.

This generalizes to a standing rule: **an adapter's outbound base URL is
configuration, never a constant.** The adapter already parameterizes the manager
URL it connects to; the third-party endpoint being the one hardcoded value is an
inconsistency rather than a decision. The rule costs nothing at every other
adapter — a self-hosted upstream has no fixed address and must be configured
regardless — and without it the adapter's outbound half cannot be exercised
without a real bot token and a real Telegram account.

The value SHALL be an operator-level setting supplied through the deployment,
NOT a field of `Channel.spec.config`. Because a `ChannelAdapter` CR carries no
`env` and the reconciler owns the adapter's Deployment, the chart reaches the
pod through the bot's credential Secret: an `apiBase` key beside `botToken`,
projected as `AGENTOPS_CRED_<CHANNEL>_apiBase`, overrides the process default
for that surface. The Secret already holds the token, so redirecting it there
grants nothing the token did not — unlike a `spec.config` field. Precedence is
the Secret's key, then `TELEGRAM_API_BASE`, then the real host: a surface that
names a root is served at that root whatever the process default says.

What is forbidden is an override in `Channel.spec.config`. That field is
editable by anyone who may edit a served channel, and an endpoint there would
let them redirect that channel's bot token to a host of their choosing — a
configuration edit turned into credential exfiltration. The Secret key is not
that: whoever can write the Secret already holds the token.

#### Scenario: Default installs are unchanged
- **WHEN** the adapter runs with `TELEGRAM_API_BASE` unset
- **THEN** it calls `https://api.telegram.org` exactly as before, and the rendered Deployment carries no additional environment entry

#### Scenario: Every Bot API call honours the override
- **WHEN** `TELEGRAM_API_BASE` is set to a local endpoint and the adapter sends a message, sends a document, creates a forum topic and closes one
- **THEN** all four calls are made against the configured endpoint, with none falling back to the public host

#### Scenario: A channel cannot redirect its own token
- **WHEN** a served `Channel`'s `spec.config` contains an API base or endpoint key
- **THEN** it has no effect on where the adapter sends the bot token

#### Scenario: The credential Secret may name the endpoint
- **WHEN** a served `Channel`'s credential Secret carries an `apiBase` key
- **THEN** that surface's token is sent to that root, since whoever can write the Secret already holds the token

#### Scenario: The Secret's root wins over the process default
- **WHEN** `TELEGRAM_API_BASE` is set AND a served `Channel`'s credential Secret carries a different `apiBase` key
- **THEN** that surface's calls go to the Secret's root, and every other surface's to `TELEGRAM_API_BASE`
