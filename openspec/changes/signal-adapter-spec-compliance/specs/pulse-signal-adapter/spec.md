# pulse-signal-adapter delta

## MODIFIED Requirements

### Requirement: The adapter registers its webhook with Pulse at pod start, and keeps healing it

At pod start the adapter SHALL call Pulse's `POST /api/notifications/webhooks`
(the Generic service) to create or update a webhook receiver pointing at its
own `/webhook/<source>` endpoint, authenticated with the configured Pulse API
token.

The call SHALL be idempotent. A webhook already registered with the same
body SHALL be left alone. One registered with a different body SHALL be
updated in place — matching `signals/alertmanager`'s create-or-repair
posture for its own sender object.

Pulse redacts the stored signing secret on read, so comparing the stored
object can never see a rotated secret.

The comparison SHALL instead key on a non-secret one-way digest of the
configured signing secret.

The digest is registered beside the webhook, in the one field Pulse returns
unredacted: an `agentops_secret_digest` query parameter on the registered
webhook URL. Pulse masks every registered header's value on read, not only
a field named "secret", so a header cannot carry this digest.

A digest mismatch SHALL update the webhook in place, pushing the current
secret.

A failed registration attempt SHALL NOT crash-loop the pod. The adapter
SHALL retry on a fixed interval until registration succeeds, and SHALL keep
re-checking afterward.

Rotation heals without manual re-registration, and each credential heals as
its delivery permits:

| Rotated | Heals |
|---|---|
| the Pulse API token | on the first ensure pass that runs with the new value — the token arrives as kubelet-resolved environment, fixed for a pod's lifetime, so that pass follows the pod next starting |
| the signing secret | the same way — the digest mismatch is what detects it, and the updated webhook carries the new secret |
| nothing — the endpoint was briefly unreachable | on the retry interval, with no pod restart |

The webhook SHALL be served regardless of registration outcome. An
unregistered sender means no alerts arrive, not that the endpoint refuses
them.

#### Scenario: Registration succeeds at startup
- **WHEN** the adapter starts with valid registration config and Pulse
  reachable
- **THEN** Pulse's notification-webhooks list contains one Generic webhook
  pointing at this adapter's endpoint, and the source's Ready condition
  names it

#### Scenario: A rotated signing secret reaches Pulse
- **WHEN** the operator rotates `signingSecret` and the adapter next runs its
  ensure with the new value
- **THEN** the registered webhook is updated in place and subsequent
  deliveries signed with the new secret verify, with no manual
  re-registration

#### Scenario: Redaction does not blind the repair
- **WHEN** Pulse returns the stored webhook with its signing secret redacted
- **THEN** the adapter still detects rotation, because the comparison reads
  the registered digest rather than the redacted field

#### Scenario: Pulse unreachable at startup still serves the webhook
- **WHEN** Pulse's API does not respond when the adapter starts
- **THEN** the adapter keeps serving `POST /webhook/{source}`, reports a
  degraded Ready reason naming the registration failure, and keeps retrying
  on its fixed interval

#### Scenario: A later-successful retry heals Ready without a restart
- **WHEN** a prior registration attempt failed and a subsequent retry
  succeeds
- **THEN** the source's Ready condition updates to reflect successful
  registration, with no pod restart

### Requirement: Inbound webhook signatures are verified when a signing secret is configured

When the served source's credentials carry a signing secret, the registered
webhook SHALL set it as Pulse's `signingSecret`.

Verification SHALL follow the secret, never the registration. A configured
signing secret SHALL be enforced whether or not registration is configured,
attempted or succeeded — a hand-registered Pulse presenting the same secret
verifies identically.

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

#### Scenario: A signing secret is enforced without registration
- **WHEN** `signingSecret` is configured and the source carries no usable
  registration config
- **THEN** inbound posts are still verified, and an unsigned post is refused
  with 401

#### Scenario: Unsigned mode accepts any body
- **WHEN** no signing secret is configured
- **THEN** the adapter accepts an inbound post carrying no signature headers
