## ADDED Requirements

### Requirement: The specification is a versioned reference page

The signal adapter specification SHALL live at `docs/contracts/signal-adapter.md`
as a reference page with no front matter. The page is moved there by
`runtime-specification`, and this change makes it a versioned specification.

The same change SHALL add a row to `.claude/rules/documentation.md`'s routing
table for this page and point `docs/contracts.md` at it.

It SHALL carry a semver version, starting at `1.0.0`, and its own changelog
recording every version with what changed in it.

It SHALL state the manager's provisions and the adapter's obligations as two
named sections, each exhaustive for its party.

The manager's provisions SHALL include at least:

| Provision | Content |
|---|---|
| one inbound endpoint | `POST /signal/inbound` accepts every normalized signal, and it is the only way work originates from outside a chat surface |
| type-scoped derived-token auth | the master adapter token or a per-`SignalAdapter` derived token scoped to the adapter's name, with a derivation context distinct from channel adapters |
| manager-side grouping and cooldown | fingerprint cooldown, signature from `labels` × `signatureLabels`, window reuse and recurrence-on-session are the manager's, never the adapter's |
| no Kubernetes access required | source listing, cursor state and status reporting reach the adapter through the contract's own endpoints |

The adapter's obligations SHALL include at least: normalize its transport into
signals and push them, fingerprint honestly so at-least-once re-sends collapse
on the same event, and never emit a signal about agent-ops' own machinery, per
the self-exclusion invariant.

#### Scenario: An adapter author needs the contract

- **WHEN** an author sets out to build a signal adapter
- **THEN** `docs/contracts/signal-adapter.md` states every obligation of both
  parties
- **AND** the author needs no shipped adapter's source to build against it

#### Scenario: A normative statement changes

- **WHEN** a normative statement in the specification is added, changed or
  removed
- **THEN** the version moves as the compatibility rules direct, and the
  changelog records the change under the new version

### Requirement: Schema tables are generated from the Go structs

The inbound signal payload table in the specification SHALL be rendered by the
docs generator's `struct-table` marker from the Go structs' json tags, never
typed by hand. The marker capability is introduced by `runtime-specification`.

The source of truth is `NormalizedSignal` in
`platform/manager/internal/httpapi/signals.go`, with the envelope fields
(`source`, `signals`) of the inbound request stated beside it.

CI SHALL regenerate the tables and fail on a difference, so a field added in
code without a specification row fails the build.

#### Scenario: A field is added in code only

- **WHEN** a json-tagged field is added to the normalized signal struct and the
  specification is not regenerated
- **THEN** CI fails on the stale generated block

#### Scenario: A generated table is hand-edited

- **WHEN** someone edits a generated field table by hand
- **THEN** the next generator run reverts the edit and CI fails on the
  difference

### Requirement: Compatibility rules make additive change safe

The manager MUST ignore inbound payload fields it does not know. This codifies
the decoding behavior already shipped, and it makes an additive field a MINOR
change.

| Version part | Moves when |
|---|---|
| MAJOR | an adapter conformant with the previous version stops conforming |
| MINOR | a new optional capability or field is added |
| PATCH | a clarification that changes no obligation |

Section anchors SHALL be stable names. The guide and the shipped adapter pages
link by anchor, so renaming a published anchor is a breaking change to the
document and moves MAJOR.

#### Scenario: An unknown field arrives

- **WHEN** an inbound payload carries a field the manager does not know
- **THEN** the manager ignores it and the post proceeds, so the change that
  added the field was MINOR

#### Scenario: A published anchor is renamed

- **WHEN** a change renames a section anchor the guide or a shipped adapter
  page links
- **THEN** the change is MAJOR, because a conformant reader's reference
  stopped resolving

### Requirement: A numbered conformance checklist is the spine

The specification SHALL end with a numbered conformance checklist. Each item
SHALL be individually verifiable against a running adapter or its source, and
every normative obligation SHALL be reachable from an item.

Where a shipped adapter test exercises an item, the item SHALL cite it. The
conformance suite in `platform/manager/test/conformance/` runs every shipped
signal adapter's built binary against a fake manager.

#### Scenario: An adapter is judged conformant

- **WHEN** someone claims a signal adapter conforms to a specification version
- **THEN** the claim is the checklist walked item by item, each with its own
  verdict, never an impression of the whole

#### Scenario: An item has a shipped test

- **WHEN** a checklist item is covered by a shipped adapter's conformance test
- **THEN** the item names that test, so a reader can run the verification
  rather than reconstruct it

### Requirement: The adopter guide is the station procedure

`docs/guides/signal-adapter.md` SHALL be the exact ordered procedure for
introducing a signal adapter. Each step SHALL cite the specification section it
implements by anchor, and each SHALL end in a verification.

References SHALL run one direction. The guide cites the specification, and the
specification never depends on the guide. The guide SHALL state no requirement
of its own.

#### Scenario: A step is followed

- **WHEN** a reader completes a step of the guide
- **THEN** the step's verification tells them it worked before the next step
  begins
- **AND** the cited anchor names where the obligation is normative

#### Scenario: The guide and the specification disagree

- **WHEN** a statement in the guide contradicts the specification
- **THEN** the specification wins and the guide is the defect, because the
  guide states no requirement of its own

### Requirement: Shipped signal adapters claim a specification version

Each shipped signal adapter's documentation page SHALL state the specification
version that adapter conforms to.

| Adapter | Claim lives on |
|---|---|
| `signals/alertmanager` | `docs/integrations/prometheus.md` |
| `signals/k8s-events` | `docs/integrations/kubernetes.md` |
| `signals/ha` | `docs/integrations/home-assistant.md` |
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
`signals/k8s-events`, `signals/ha`, `signals/telegram` — into conformance with
the new version.

The same change MUST move the version every shipped adapter page claims. The
change is not finished while any shipped adapter conforms only to the previous
version, so the specification never runs ahead of what ships.

A PATCH-only change alters no obligation, so it is exempt from the adapter walk
and the claimed versions stay as they are.

The first version, `1.0.0`, documents shipped behavior. It needs no adapter
change, but the change MUST walk the checklist for all five adapters and fix
any gap it finds before the version is claimed.

#### Scenario: The specification version moves

- **WHEN** a change bumps the specification's MAJOR or MINOR version
- **THEN** the same change updates every shipped signal adapter and every
  claimed version, or the change is not archivable

#### Scenario: A PATCH clarification

- **WHEN** a change moves only the PATCH version
- **THEN** no adapter or claimed version needs to change

### Requirement: The specification governs the agent's own changes

The existing scoped rule `.claude/rules/signal-rules.md` SHALL be extended, not
duplicated in a new file. It already loads for `signals/**` and
`internal/ingest/`, and SHALL also load for the signal endpoints in
`internal/httpapi/`.

The rule SHALL state that a change there is verified against the specification
before it is claimed done. It SHALL state that the specification itself is
NEVER edited without the user's explicit approval.

#### Scenario: A governed surface changes

- **WHEN** an agent session changes a signal adapter or the manager's ingest
  surface
- **THEN** the loaded rule directs it to verify the change against
  `docs/contracts/signal-adapter.md` before claiming the work done

#### Scenario: A session reaches for the specification itself

- **WHEN** an agent session would edit `docs/contracts/signal-adapter.md`
  without the user's explicit approval
- **THEN** the rule directs it to stop and ask, and the session reports the
  conflict instead of editing
