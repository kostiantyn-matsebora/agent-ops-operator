## MODIFIED Requirements

### Requirement: Browser access is authenticated
Read access SHALL require a bearer token or a session established from it, sourced from the console Channel's projected `uiToken` credential, UNLESS the release declares that an external authenticator fronts the console. An unconfigured token SHALL authorize nobody and SHALL be indistinguishable from a wrong token.

Disabling the console's own authentication SHALL require naming what authenticates instead: the chart SHALL fail to render when authentication is disabled and no external authenticator is declared. A configuration that removes the only gate SHALL cost more than flipping one boolean, and the name SHALL be recoverable from the release so that "what protects this console?" is answerable without asking the operator.

The chart SHALL still render the token Secret when authentication is disabled, because the console Channel projects that credential into the adapter pod and a missing Secret would prevent the pod from starting.

Write actions — replying in a conversation and originating one — SHALL additionally require `console.write.enabled` and a resolved identity, taken from a trusted forward-auth header when present and recorded as the token identity otherwise. Every write SHALL be logged with that identity. The Service SHALL remain `ClusterIP` and Ingress disabled by default; OIDC via forward-auth SHALL be documented as the answer for any Ingress exposure.

Because that token is the whole boundary — it reads every conversation payload and instructs any joined agent — the Ingress template SHALL make its exposure in transit explicit: TLS SHALL be configurable by naming a certificate or a cert-manager issuer without restating hostnames, and enabling the Ingress without TLS SHALL be reported in the post-install notes as sending the token in clear text. The chart SHALL NOT refuse to render in that case, since TLS is frequently terminated upstream, and SHALL NOT claim the exposure is safe.

When authentication is disabled the post-install notes SHALL state that the console authenticates nobody itself, name the declared external authenticator, and say what is reachable by anyone who reaches the Service without passing it.

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

#### Scenario: Disabling authentication without naming its replacement is refused
- **WHEN** an operator disables the console's authentication and declares no external authenticator
- **THEN** the render FAILS, naming the value to set and why it is required

#### Scenario: External authentication is recorded in the release
- **WHEN** an operator disables authentication and names the fronting authenticator
- **THEN** the render succeeds, the name is visible in the release values, and the notes state that the console authenticates nobody itself

#### Scenario: The credential survives the switch
- **WHEN** authentication is disabled
- **THEN** the token Secret is still rendered, so the adapter pod starts and re-enabling authentication is a single value change
