# pulse-signal-adapter Specification

## Purpose
The webhook-receiving Pulse signal adapter in `signals/pulse/`. A
dependency-free module serving SignalSources through the signal adapter
contract.

Self-registers against Pulse's own outgoing-webhooks API at pod start.
Owns both the registered payload template and the parser that decodes it,
so the two halves cannot drift apart.

## Requirements

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
`spec.credentialsSecretRef` (kubelet-resolved `envFrom`, under the single
documented keys `token` and `signingSecret`).

This bundle still ships exactly one default source. The config shape is
three flat fields — `{"baseUrl": "...", "webhookUrl": "...", "webhookHeaders": {...}}`,
`webhookHeaders` optional, `webhookUrl` required for registration exactly
like `baseUrl` is (see the delivery-URL requirement below) — never
`signals/alertmanager`'s `register` sub-block.

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

### Requirement: The registered delivery URL comes from the source, never the adapter
The adapter is a dumb transport — the same posture as `channels/telegram`
and `signals/ha`. It SHALL hold no connectivity information of its own.
It SHALL register exactly the URL `spec.config.webhookUrl` carries,
verbatim, never deriving one itself.

A served source carrying `spec.config.baseUrl` but no `webhookUrl` SHALL be
treated as not configured for registration. That is the same treatment a
missing `baseUrl` gets.

The bundle chart renders `webhookUrl` for the one source it ships:

- Default: the in-cluster Service DNS form
  (`http://agentops-signal-<adapter>.<namespace>.svc:<port>/webhook/<source>`),
  computed at the template level, where the namespace and port are already
  known.
- Override: an operator-set external URL, for a Pulse outside the cluster.
  Pulse commonly runs this way — it watches Proxmox/Docker hosts that may
  sit entirely outside Kubernetes, unreachable from the in-cluster form.

A hand-authored second source carries its own `webhookUrl` the same way.
The adapter makes no attempt to expose itself externally either way.

#### Scenario: Pulse outside the cluster registers an external URL
- **WHEN** the served source carries a `spec.config.webhookUrl` naming an
  externally reachable address
- **THEN** that exact URL is registered with Pulse

#### Scenario: A missing webhookUrl is not configured, not derived
- **WHEN** the served source carries `spec.config.baseUrl` but no
  `spec.config.webhookUrl`
- **THEN** the adapter registers nothing and reports the same
  not-configured state a missing `baseUrl` produces

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
`X-Pulse-Timestamp` and `X-Pulse-Signature`.

`X-Pulse-Signature` SHALL always carry a `v1=` prefix before the hex
digest, confirmed against Pulse's own
`internal/notifications/webhook_signing_test.go`.

A digest compared without accounting for the prefix SHALL reject every
genuinely signed delivery. This is not a cosmetic detail — it is the one
place this adapter's behavior has to match Pulse's actual source rather
than a best guess at the Generic webhook contract.

A missing or invalid webhook signature, or one missing the `v1=` prefix,
SHALL be rejected with 401 and push nothing. The timestamp SHALL be bound
to a tolerance window to reject stale replays.

With no signing secret configured, the endpoint accepts unsigned posts —
parity with `signals/alertmanager`'s opt-in-per-source credential posture.

#### Scenario: Valid webhook signature is accepted
- **WHEN** a webhook arrives with a `v1=`-prefixed signature computed from
  the configured signing secret, inside the timestamp tolerance
- **THEN** the adapter parses the body and pushes the normalized signal

#### Scenario: Invalid webhook signature is refused before parsing
- **WHEN** a webhook arrives with a signature that does not match, that is
  missing the `v1=` prefix, or outside the timestamp tolerance
- **THEN** the adapter returns 401 and never parses the body

#### Scenario: Unsigned mode accepts any body
- **WHEN** no signing secret is configured
- **THEN** the adapter accepts an inbound post carrying no signature headers

### Requirement: Pulse-attached headers are registered independently of signing
`spec.config.webhookHeaders`, when set, SHALL be registered verbatim as
Pulse's own `WebhookConfig.Headers` field (confirmed real against Pulse's
`internal/notifications/notifications.go`) — static headers PULSE attaches
to every delivery to this adapter.

This is the opposite direction from HMAC signing and serves a different
need: infra in front of this adapter (a WAF, a gateway) that requires its
own shared-secret header, independent of whether a signing secret is also
configured.

Pulse's own signing headers win on a name collision, by Pulse's own
behavior, not this adapter's.

#### Scenario: Custom headers reach the registered webhook
- **WHEN** the served source carries `spec.config.webhookHeaders`
- **THEN** the webhook object registered with Pulse carries the same
  headers verbatim

### Requirement: Firing alerts normalize into the signal contract, resolved alerts do not
Matching `signals/alertmanager`'s firing-only parity, the adapter SHALL
forward only Pulse's `"alert"` (firing) events as normalized signals.

`"resolved"` events SHALL be accepted, so Pulse sees a 2xx and does not
retry, but SHALL produce no signal.

The `initiationId` SHALL be derived from the Pulse alert id carried in the
registered template's fields — the resource identity joined with the alert
identity — using one of two strategies selected by the served source's
`spec.config.correlationStrategy`:

- `"id+startTime"` (the default, used when the field is absent, empty, or
  any value other than `"id"`, the last logged as a warning): the resource, the alert id, AND the
  alert's start time. Pulse's own alert id can be reused when the same
  condition fires again later as a new occurrence, so start time SHALL be
  included to keep a later re-fire from colliding with an earlier,
  unrelated occurrence that happened to reuse the same alert id.
- `"id"`: the resource and the alert id alone, with no time component —
  today's behavior, kept for a source that needs to opt back into it.

Either strategy SHALL yield distinct ids for two unrelated alerts (never
sharing an id), and SHALL yield the SAME id for retries and for a
firing/resolved pair of one occurrence.

The no-ID fallback — used only when Pulse sends no alert id at all, a hash
of resource/type/severity/message — is unaffected by `correlationStrategy`
and SHALL continue to omit any time component.

The registered template's `startsAt` field SHALL render the alert's
occurrence start time (`{{.StartTime}}`), which stays the same across one
occurrence's firing and resolved events. It SHALL NOT render a per-reading
timestamp, since that would give one occurrence a different id on every
event.

Cooldown and routing stay manager-side, keyed on the resulting id.

#### Scenario: A firing event produces one signal
- **WHEN** a webhook body carries an `"alert"` event
- **THEN** exactly one normalized signal is pushed, its `initiationId` derived from the resource and alert identity per the source's `correlationStrategy`

#### Scenario: A resolved event produces no signal
- **WHEN** a webhook body carries a `"resolved"` event
- **THEN** the adapter responds 200 and pushes no signal

#### Scenario: Unrelated Pulse alerts never merge
- **WHEN** a node-memory threshold alert and an unreachable-device incident both fire
- **THEN** their signals carry distinct `initiationId` values and open distinct conversations

#### Scenario: Default strategy distinguishes a re-fire from a continuation
- **WHEN** a source sets no `correlationStrategy`, and the same Pulse alert id fires, resolves, and later fires again with a new start time
- **THEN** the firing/resolved pair shares one `initiationId`, and the later re-fire carries a different `initiationId` that opens a new conversation

#### Scenario: Explicit "id" strategy keeps today's collapsing behavior
- **WHEN** a source sets `spec.config.correlationStrategy: "id"`, and the same Pulse alert id fires, resolves, and later fires again with a new start time
- **THEN** every one of those signals carries the same `initiationId`

#### Scenario: An unrecognized strategy value falls back to the default
- **WHEN** a source sets `spec.config.correlationStrategy` to a value other than `"id"` or `"id+startTime"`
- **THEN** the adapter derives `initiationId` as if `"id+startTime"` had been set, and logs a warning naming the source and the unrecognized value

#### Scenario: The no-ID fallback ignores correlationStrategy
- **WHEN** a webhook body carries no Pulse alert id, regardless of the source's `correlationStrategy`
- **THEN** `initiationId` is the existing hash of resource/type/severity/message, with no start time included

#### Scenario: The registered template carries the occurrence start time
- **WHEN** the adapter registers its webhook template with Pulse
- **THEN** the template's `startsAt` field renders `{{.StartTime}}`, not a per-reading timestamp
