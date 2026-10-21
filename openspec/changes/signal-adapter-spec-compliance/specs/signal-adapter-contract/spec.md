# signal-adapter-contract delta

## MODIFIED Requirements

### Requirement: A missing correlation id is rejected, never synthesized

The manager SHALL NOT mint, derive or default an `initiationId` for any inbound signal. A signal arriving without one SHALL be rejected with `4xx` at `POST /signal/inbound`, the response naming the missing field.

A `1.x` adapter during rollout gets the same answer — a loud contract violation, never a silently merged conversation.

The specification's COMPAT section SHALL state this rule: there is no compatibility shim, because a manager-synthesized id would re-mint grouping policy in the manager, which this contract forbids.

Replacing `fingerprint` with `initiationId` was a MAJOR move. The changelog SHALL record the replacement under version `2.0.0`, whatever later additive versions the page carries.

#### Scenario: A 1.x adapter is refused loudly

- **WHEN** an adapter posts a signal carrying the retired `fingerprint` and no `initiationId`
- **THEN** the manager responds `4xx` naming `initiationId`, creates nothing, and synthesizes no id

#### Scenario: The contract names its own break

- **WHEN** a reader opens the published specification
- **THEN** the changelog records the `fingerprint` → `initiationId` replacement under `2.0.0`, and the COMPAT section states that a missing id is rejected rather than repaired

### Requirement: Shipped signal adapters claim a specification version

Each shipped signal adapter's documentation page SHALL state the specification
version that adapter conforms to.

| Adapter | Claim lives on |
|---|---|
| `signals/alertmanager` | `docs/integrations/prometheus.md` |
| `signals/k8s-events` | `docs/integrations/kubernetes.md` |
| `signals/ha` | `docs/integrations/home-assistant.md` |
| `signals/pulse` | `docs/integrations/pulse.md` |
| `signals/telegram` | `docs/integrations/telegram.md` |
| `signals/cron` | `docs/guides/signal-adapter.md` — it has no page of its own |

#### Scenario: An adopter checks a shipped adapter

- **WHEN** an adopter opens a shipped signal adapter's page
- **THEN** it names the specification version the adapter conforms to
- **AND** that version exists in the specification's changelog

#### Scenario: An adapter with no page of its own

- **WHEN** an adopter looks for `signals/cron`'s conformance claim
- **THEN** the guide carries it, because the guide is the page that owns cron

### Requirement: A specification change carries conformance

A change that moves the MAJOR or MINOR version of
`docs/contracts/signal-adapter.md` MUST, in the same change, bring every
shipped signal adapter — `signals/cron`, `signals/alertmanager`,
`signals/k8s-events`, `signals/ha`, `signals/pulse`, `signals/telegram` —
into conformance with the new version.

The same change MUST move the version every shipped adapter page claims. The
change is not finished while any shipped adapter conforms only to the previous
version, so the specification never runs ahead of what ships.

A PATCH-only change alters no obligation, so it is exempt from the adapter walk
and the claimed versions stay as they are.

The first version, `1.0.0`, documented shipped behavior and walked that
checklist for the five adapters that shipped then. `signals/pulse` joined the
enumeration at `2.2.0` — a shipped adapter a list omits is exempt from
nothing, so the omission was the defect, never an exemption.

#### Scenario: The specification version moves

- **WHEN** a change bumps the specification's MAJOR or MINOR version
- **THEN** the same change updates every shipped signal adapter and every
  claimed version, or the change is not archivable

#### Scenario: A PATCH clarification

- **WHEN** a change moves only the PATCH version
- **THEN** no adapter or claimed version needs to change

## ADDED Requirements

### Requirement: A post-emit refractory window is the one named allowance in the no-dedup rule

The no-dedup obligation stands: an adapter MUST NOT implement dedup, cooldown
or recurrence. The manager's cooldown on `initiationId` is the one dedup
authority.

The specification SHALL name ONE carve-out beside that obligation — the
post-emit refractory window. It bounds an adapter's own re-post chatter, and
decides nothing about recurrence.

| Bound | Rule |
|---|---|
| post-emit only | the window opens only when the adapter itself emitted a signal — a condition never reported is never suppressed |
| one id | it suppresses only re-posts carrying the emitted signal's own `initiationId`, never collapsing distinct ids |
| length | no longer than the emitting rule's own dwell, with a floor of one minute |
| no substitute | the manager's cooldown stays the dedup authority — the window only saves round trips cooldown would have absorbed |
| declared | an adapter relying on the window SHALL declare it, with its length, in that adapter's own capability spec |

Naming the allowance is an additive change. The specification version SHALL
move to `2.2.0`, and its changelog SHALL record the allowance under that
version.

#### Scenario: A suppressed re-post changes nothing the manager would have kept

- **WHEN** an adapter emitted a signal and the same condition recurs within the declared window
- **THEN** no re-post is made, and the conversation's state equals what the manager's cooldown would have produced for that re-post

#### Scenario: Distinct ids are never collapsed by the window

- **WHEN** a different condition with its own `initiationId` arises while another id's window is open
- **THEN** its signal is posted normally

#### Scenario: Recurrence past the window repeats the id

- **WHEN** the emitted condition still recurs after the window closes
- **THEN** the adapter re-posts under the same `initiationId` and the manager routes it as a recurrence

#### Scenario: An undeclared suppression is a conformance gap

- **WHEN** an adapter suppresses its own post-emit re-posts and its capability spec declares no window
- **THEN** the adapter is judged against the no-dedup obligation as written, and the suppression is a conformance gap
