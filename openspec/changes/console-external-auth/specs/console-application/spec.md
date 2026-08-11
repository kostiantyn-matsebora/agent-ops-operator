## MODIFIED Requirements

### Requirement: Reads are authenticated; writes require identity and can be disabled
Read access SHALL require a bearer token or a session established from it, UNLESS the deployment declares that authentication is performed upstream. Write actions — replying in a conversation and originating one — SHALL additionally require `console.write.enabled` and a resolved identity, taken from a trusted forward-auth header when present and recorded as the token identity otherwise. An unconfigured token SHALL authorize nobody, and SHALL be indistinguishable from a wrong token.

The two states SHALL remain independent: an absent or empty token SHALL NEVER be interpreted as "authentication not required". Only an explicit declaration that an external authenticator fronts the console SHALL relax the token requirement.

When authentication is declared external, writes SHALL require an identity resolved from a forward-auth header and SHALL be refused when none resolves. The token identity SHALL NOT be used as a fallback in that mode, because no token was proven and a write log naming one would assert something untrue.

Every write SHALL be logged with the resolved identity.

#### Scenario: Unconfigured is closed, not open
- **WHEN** no token is configured
- **THEN** every authenticated route is refused, and login failures do not reveal whether a token exists

#### Scenario: Viewer-only deployment
- **WHEN** `console.write.enabled` is false
- **THEN** the composer and the new-conversation action are absent, and both endpoints reject requests server-side

#### Scenario: Identity is carried, not assumed
- **WHEN** a trusted forward-auth header is present
- **THEN** that identity is recorded on the write and carried as the signal's sender

#### Scenario: External authentication admits the request
- **WHEN** the deployment declares authentication external and a request arrives with no bearer token
- **THEN** reads are served, because the declaration — not the absence of a credential — is what opened the door

#### Scenario: External authentication without identity is read-only
- **WHEN** authentication is declared external, writes are enabled, and a request carries no forward-auth identity header
- **THEN** the write is refused and no identity is invented, while reads continue to be served

#### Scenario: An empty token still closes a console that authenticates
- **WHEN** authentication is NOT declared external and the configured token is empty
- **THEN** every authenticated route is refused, exactly as when a token is set and a wrong one is presented
