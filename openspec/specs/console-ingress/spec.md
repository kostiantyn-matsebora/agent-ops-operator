# console-ingress Specification

## Purpose
TBD - created by archiving change console-ingress-tls. Update Purpose after archive.
## Requirements
### Requirement: The console is not exposed unless an operator says so
The chart SHALL render no Ingress by default. The console's Service SHALL remain
`ClusterIP`, reachable only by port-forward or by a deliberate exposure
decision. Disabling the console SHALL remove its Ingress along with every other
console object.

#### Scenario: Default install exposes nothing
- **WHEN** the chart is installed with no ingress values supplied
- **THEN** no Ingress object is rendered and the console is reachable only in-cluster

#### Scenario: Disabling the console removes its Ingress
- **WHEN** the console is disabled while ingress values are still set
- **THEN** no console object renders at all, the Ingress included

### Requirement: The ingress values surface is conventional and non-breaking
`console.ingress` SHALL accept the keys operators expect from a Helm chart:
`enabled`, `host`, `extraHosts`, `className`, `annotations`, `labels`, `path`,
`pathType` and `tls`. Every key that exists today SHALL keep its name and
meaning, so an existing values file continues to render the same Ingress.

`host` SHALL remain REQUIRED when ingress is enabled, and its absence SHALL fail
the render with a message naming the value — a hostname cannot be guessed.

Additional hostnames SHALL be declared in `extraHosts[]` and SHALL each produce
a rule serving the same console backend.

#### Scenario: Existing values keep working
- **WHEN** a values file that set only `enabled`, `host`, `className` and `annotations` is rendered against the new chart
- **THEN** the resulting Ingress is equivalent to the one the previous chart produced

#### Scenario: Missing host fails the render
- **WHEN** ingress is enabled with no `host`
- **THEN** the render fails naming `console.ingress.host`, rather than producing an Ingress with an empty host

#### Scenario: Extra hostnames serve the same console
- **WHEN** `extraHosts` names two additional hostnames
- **THEN** the Ingress carries a rule per hostname, each routing to the same backend

### Requirement: The backend is the reconciler-owned Service, never a chart-shipped one
The Ingress SHALL route to the Service the `ChannelAdapter` reconciler owns,
named `agentops-adapter-<console name>`, on the configured port. The chart SHALL
NOT render a Service, a Deployment, or any other connectivity for the console —
doing so would make the console deployable only by this chart.

#### Scenario: Ingress points at the reconciler's Service
- **WHEN** the console Ingress is rendered
- **THEN** its backend names `agentops-adapter-<console name>` and the chart ships no Service of its own

### Requirement: Only root-path hosting is offered, and a non-root path fails loudly
The embedded single-page application is built to be served from the root of a
hostname and emits absolute asset URLs. The chart SHALL therefore accept
`path` and `pathType` but SHALL FAIL the render when `path` is anything other
than the root, stating that the console must be served at the root of its
hostname. A configuration that cannot work SHALL NOT render successfully and
break at first page load.

#### Scenario: Root path renders
- **WHEN** `path` is left at its default
- **THEN** the Ingress rule serves the root path with the configured `pathType`

#### Scenario: Sub-path hosting is refused at render time
- **WHEN** an operator sets `path` to a sub-path such as `/console`
- **THEN** the render fails explaining that the console must be served at the root of a hostname, and no Ingress is produced

### Requirement: TLS is configured by naming a certificate, not by restating hostnames
`console.ingress.tls` SHALL accept:

- `secretName` — an existing certificate Secret. The rendered `tls[]` entry
  SHALL cover `host` and every `extraHosts` entry, derived rather than restated.
- `clusterIssuer` — a cert-manager issuer. Setting it SHALL emit the issuer
  annotation and SHALL derive `secretName` when none is given.
- `existing[]` — a verbatim `tls:` list for anything the above does not cover,
  taking precedence over the derived form.

When none of these is set, no `tls` block SHALL be rendered.

#### Scenario: Named certificate covers every hostname
- **WHEN** `tls.secretName` is set alongside a `host` and two `extraHosts`
- **THEN** the rendered `tls[]` entry names that Secret and lists all three hostnames

#### Scenario: cert-manager needs one value
- **WHEN** `tls.clusterIssuer` is set and `tls.secretName` is not
- **THEN** the Ingress carries the issuer annotation and a `tls[]` entry with a derived Secret name covering every configured hostname

#### Scenario: Raw list wins when supplied
- **WHEN** `tls.existing` is supplied
- **THEN** it is rendered verbatim and no derivation occurs

#### Scenario: No TLS configuration renders no TLS block
- **WHEN** ingress is enabled with no TLS values
- **THEN** the Ingress carries no `tls` section and renders successfully

### Requirement: The legacy list form of tls keeps working
A `console.ingress.tls` supplied as a LIST — the shape the chart accepted
before — SHALL be rendered verbatim, so existing values files and
`helm upgrade --reuse-values` continue to work. The list form SHALL be
documented as legacy with the map form as its replacement.

#### Scenario: Upgrade with the old shape
- **WHEN** an existing release carrying `console.ingress.tls` as a list is upgraded
- **THEN** the render succeeds and produces the same `tls` section as before

### Requirement: Plaintext exposure is announced, never silently allowed
Enabling the Ingress without TLS SHALL render successfully — termination
upstream at a load balancer or service mesh is legitimate — but the post-install
notes SHALL state that the console's bearer token crosses the network in clear
text and name both remedies: terminating TLS, and putting an authenticating
proxy in front. The notes SHALL NOT print this when TLS is configured.

The render SHALL NOT fail on a missing certificate: the chart cannot see what
terminates in front of it.

#### Scenario: Plaintext exposure is called out
- **WHEN** ingress is enabled with no TLS configured
- **THEN** the install succeeds and the notes state that the UI token travels in clear text, with the two remedies

#### Scenario: Configured TLS is not nagged about
- **WHEN** ingress is enabled with a certificate configured
- **THEN** the notes carry no plaintext warning

