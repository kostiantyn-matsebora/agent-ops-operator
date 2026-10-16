## Purpose

The webhook-receiving Pulse signal adapter in `signals/pulse/`. A
dependency-free module serving SignalSources through the signal adapter
contract.

Self-registers against Pulse's own outgoing-webhooks API at pod start.
Owns both the registered payload template and the parser that decodes it,
so the two halves cannot drift apart.

## ADDED Requirements

### Requirement: The adapter serves sources through the generic signal adapter contract
A signal adapter in `signals/pulse/` (own dependency-free Go module and
image, precedent `signals/alertmanager/`) SHALL serve SignalSources whose
`spec.adapter` names its SignalAdapter CR, through the signal adapter
contract:

- Learns served sources from `GET /signal/sources?adapter=<key>`.
- Accepts webhooks at `POST /webhook/{source}`.
- Returns 404 for an unknown or unserved source name.
- Pushes normalized signals to `POST /signal/inbound`.

#### Scenario: Unknown source is refused
- **WHEN** a webhook is posted for a source name that no served SignalSource
  matches
- **THEN** the adapter returns 404 naming the source and pushes nothing

### Requirement: Registration config comes from the served SignalSource, never a pod env var
`SignalAdapterSpec` SHALL carry no generic env/envFrom passthrough a chart
could use to render `PULSE_URL`/`PULSE_API_TOKEN` directly onto the pod —
the reconciler alone owns the adapter Deployment's env.

This adapter's registration config SHALL instead be read from the served
SignalSource itself: the Pulse base URL from `spec.config.baseUrl`
(schema-less JSON, polled via `GET /signal/sources?adapter=`), and the
Pulse API token plus the optional HMAC signing secret from
`spec.credentialsSecretRef` (kubelet-resolved `envFrom`, accepting both
`token`/`TOKEN` and `signingSecret`/`SIGNING_SECRET` key casings).

This bundle still ships exactly one default source. The config shape is
one flat field — `{"baseUrl": "..."}` — never `signals/alertmanager`'s
`register` sub-block.

Registration follows whichever source the adapter serves, not a
hardcoded identity. A second, hand-authored Pulse SignalSource would also
register — a smaller trade-off than a pod-env mechanism that does not
exist.

#### Scenario: Registration needs the source's own config
- **WHEN** the adapter starts and the bundle's default source carries
  `spec.config.baseUrl` and a `credentialsSecretRef` holding a Pulse API
  token
- **THEN** registration proceeds, driven entirely by that source's own
  config and credential

### Requirement: The registered delivery URL is overridable for a Pulse outside the cluster
The adapter SHALL derive the webhook URL it registers with Pulse from its
own in-cluster Service identity by default
(`http://agentops-signal-<adapter>.<namespace>.svc:<port>/webhook/<source>`).

`spec.config.webhookUrl`, when set, SHALL be registered verbatim instead.
Pulse commonly runs outside the cluster — it watches Proxmox/Docker hosts
that may sit entirely outside Kubernetes — so the derived, cluster-internal
URL is unreachable from such a Pulse.

The adapter makes no attempt to expose itself externally. An operator who
already exposes this adapter's `/webhook/<source>` path (a LoadBalancer
Service, a NodePort, an Ingress, or a tunnel) states the resulting URL
here.

#### Scenario: Pulse outside the cluster registers an external URL
- **WHEN** the served source carries `spec.config.webhookUrl`
- **THEN** that exact URL is registered with Pulse, never the derived
  in-cluster Service URL

#### Scenario: No override still derives the in-cluster URL
- **WHEN** the served source carries no `spec.config.webhookUrl`
- **THEN** the adapter registers its own derived in-cluster Service URL,
  exactly as before this requirement existed

### Requirement: The adapter registers its webhook with Pulse at pod start, and keeps healing it
At pod start the adapter SHALL call Pulse's `POST /api/notifications/webhooks`
(the Generic service) to create or update a webhook receiver pointing at its
own `/webhook/<source>` endpoint, authenticated with the configured Pulse API
token.

The call SHALL be idempotent. A webhook already registered with the same
body SHALL be left alone. One registered with a different body SHALL be
updated in place — matching `signals/alertmanager`'s create-or-repair
posture for its own sender object.

A failed registration attempt SHALL NOT crash-loop the pod. The adapter
SHALL retry with bounded backoff on a fixed interval until registration
succeeds, and SHALL keep re-checking afterward. A Pulse API token rotated,
or an endpoint made briefly unreachable, SHALL heal without a pod restart.

The webhook SHALL be served regardless of registration outcome. An
unregistered sender means no alerts arrive, not that the endpoint refuses
them.

#### Scenario: Registration succeeds at startup
- **WHEN** the adapter starts with valid registration config and Pulse
  reachable
- **THEN** Pulse's notification-webhooks list contains one Generic webhook
  pointing at this adapter's endpoint, and the source's Ready condition
  names it

#### Scenario: Pulse unreachable at startup still serves the webhook
- **WHEN** Pulse's API does not respond when the adapter starts
- **THEN** the adapter keeps serving `POST /webhook/{source}`, reports a
  degraded Ready reason naming the registration failure, and keeps retrying
  on its backoff schedule

#### Scenario: A later-successful retry heals Ready without a restart
- **WHEN** a prior registration attempt failed and a subsequent retry
  succeeds
- **THEN** the source's Ready condition updates to reflect successful
  registration, with no pod restart

### Requirement: One module owns both the registered template and its parser
The Go-template JSON body the adapter registers as the webhook's payload
SHALL live in the same module as the parser that decodes it on receipt.

Neither SHALL be independently values-configurable. Pulse's Generic webhook
requires a caller-supplied template, unlike Alertmanager's fixed standard
payload.

Letting the template drift from the parser's expected shape would fail
silently at the first malformed webhook, rather than at build time. A
pinned test fixture SHALL assert the two agree.

#### Scenario: A pinned fixture would catch drift
- **WHEN** the registered template's field set changes without a matching
  parser change
- **THEN** the pinned fixture test fails, rather than the drift surfacing
  only when a real webhook body fails to parse

### Requirement: Inbound webhook signatures are verified when a signing secret is configured
When the adapter's registration config carries a signing secret, the
registered webhook SHALL set it as Pulse's `signingSecret`.

Every inbound POST SHALL then be verified before the body is parsed:
HMAC-SHA256 over `timestamp + "." + body`, checked against
`X-Pulse-Timestamp` and `X-Pulse-Signature`, per Pulse's own webhook
documentation.

A missing or invalid signature SHALL be rejected with 401 and push
nothing. The timestamp SHALL be bound to a tolerance window to reject
stale replays.

With no signing secret configured, the endpoint accepts unsigned posts —
parity with `signals/alertmanager`'s opt-in-per-source credential posture.

#### Scenario: Valid signature is accepted
- **WHEN** a webhook arrives with a signature computed from the configured
  signing secret, inside the timestamp tolerance
- **THEN** the adapter parses the body and pushes the normalized signal

#### Scenario: Invalid signature is refused before parsing
- **WHEN** a webhook arrives with a signature that does not match, or
  outside the timestamp tolerance
- **THEN** the adapter returns 401 and never parses the body

#### Scenario: Unsigned mode accepts any body
- **WHEN** no signing secret is configured
- **THEN** the adapter accepts an inbound post carrying no signature headers

### Requirement: Firing alerts normalize into the signal contract, resolved alerts do not
Matching `signals/alertmanager`'s firing-only parity, the adapter SHALL
forward only Pulse's `"alert"` (firing) events as normalized signals.

`"resolved"` events SHALL be accepted, so Pulse sees a 2xx and does not
retry, but SHALL produce no signal. Grouping, cooldown and recurrence stay
manager-side.

The fingerprint SHALL be derived from the resource identity and the alert
identity carried in the registered template's fields. The same underlying
Pulse alert occurrence SHALL always yield the same fingerprint, across
retries and across a firing/resolved pair.

#### Scenario: A firing event produces one signal
- **WHEN** a webhook body carries an `"alert"` event
- **THEN** exactly one normalized signal is pushed, fingerprinted on the
  resource and alert identity

#### Scenario: A resolved event produces no signal
- **WHEN** a webhook body carries a `"resolved"` event
- **THEN** the adapter responds 200 and pushes no signal
