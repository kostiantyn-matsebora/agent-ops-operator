## MODIFIED Requirements

### Requirement: Browser access is authenticated
Read access SHALL require a bearer token or a session established from it, sourced from the console Channel's projected `uiToken` credential. An unconfigured token SHALL authorize nobody and SHALL be indistinguishable from a wrong token.

Write actions — replying in a conversation and originating one — SHALL additionally require `console.write.enabled` and a resolved identity, taken from a trusted forward-auth header when present and recorded as the token identity otherwise. Every write SHALL be logged with that identity. The Service SHALL remain `ClusterIP` and Ingress disabled by default; OIDC via forward-auth SHALL be documented as the answer for any Ingress exposure.

Because that token is the whole boundary — it reads every conversation payload and instructs any joined agent — the Ingress template SHALL make its exposure in transit explicit: TLS SHALL be configurable by naming a certificate or a cert-manager issuer without restating hostnames, and enabling the Ingress without TLS SHALL be reported in the post-install notes as sending the token in clear text. The chart SHALL NOT refuse to render in that case, since TLS is frequently terminated upstream, and SHALL NOT claim the exposure is safe.

#### Scenario: Unconfigured is closed, not open
- **WHEN** no token is configured
- **THEN** every authenticated route is refused, and failures do not reveal whether a token exists

#### Scenario: Viewer-only deployment
- **WHEN** `console.write.enabled=false`
- **THEN** the composer and the new-conversation action are absent and both endpoints reject requests server-side

#### Scenario: No anonymous access
- **WHEN** a browser requests any console page or API without the token (or session established from it)
- **THEN** the console responds 401 / login prompt and serves no topology, CR, or conversation data

#### Scenario: Exposed without TLS, and told so
- **WHEN** an operator enables the console Ingress with no TLS configured
- **THEN** the install succeeds and the notes state that the bearer token crosses the network in clear text, naming TLS termination and a fronting authenticating proxy as the remedies
