## MODIFIED Requirements

### Requirement: Browser access is authenticated
The console UI and its APIs SHALL require the chart-provisioned bearer token (projected to the adapter via the console Channel's `credentialsSecretRef`), validated with constant-time comparison; unauthenticated requests receive 401. The Service SHALL default to ClusterIP with an optional Ingress template.

Because that token is the whole boundary — it reads every conversation payload and instructs any joined agent — the Ingress template SHALL make its exposure in transit explicit: TLS SHALL be configurable by naming a certificate or a cert-manager issuer without restating hostnames, and enabling the Ingress without TLS SHALL be reported in the post-install notes as sending the token in clear text. The chart SHALL NOT refuse to render in that case, since TLS is frequently terminated upstream, and SHALL NOT claim the exposure is safe.

#### Scenario: No anonymous access
- **WHEN** a browser requests any console page or API without the token (or session established from it)
- **THEN** the console responds 401 / login prompt and serves no topology, CR, or conversation data

#### Scenario: Exposed without TLS, and told so
- **WHEN** an operator enables the console Ingress with no TLS configured
- **THEN** the install succeeds and the notes state that the bearer token crosses the network in clear text, naming TLS termination and a fronting authenticating proxy as the remedies
