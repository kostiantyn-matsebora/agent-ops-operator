## Why

After `runtime-specification` splits the contracts, the signal adapter
contract page is still the moved narrative. It explains decisions but carries
no version, no exhaustive field tables, no conformance checklist and no
governance.

An adopter building a signal adapter should get what a runtime adopter now
gets: a specification that produces a working adapter on the first attempt,
and a guide that is the exact path to it.

## What Changes

- **Rework `docs/contracts/signal-adapter.md` into a versioned (semver,
  starting `1.0.0`) normative specification, published like
  `docs/contracts/runtime.md`** (front matter, permalink, a Specifications
  nav entry). Two-party: what the manager provides (the inbound/listing/state
  endpoints, type-scoped derived-token auth, manager-side grouping and
  cooldown, no Kubernetes access required) and what the adapter must do
  (normalize and push, fingerprint honestly, never emit a signal about
  agent-ops' own machinery).
- **Hand-write the request/response field tables**, requirement-ID style,
  matching `runtime.md`'s own shape. `runtime-specification` shipped a
  `struct-table` marker and then removed it in the same change — generated
  tables read as unreadable doc-comment dumps for a spec page, so the
  established convention is hand-written, concise tables, not generation.
- **Add a Conformance section matching `runtime.md`'s final shape**: a
  minimal/optional split rather than a per-item test registry. Bringing all
  five shipped adapters to conformance is still owed in this change — just
  verified by reading each adapter's own tests, not citation.
- **Rewrite `docs/guides/signal-adapter.md` as the station procedure** —
  ordered steps, each citing a spec section by anchor, each ending in a
  verification.
- **Shipped signal adapters claim a spec version.** `signals/cron`,
  `signals/alertmanager`, `signals/k8s-events`, `signals/ha` and
  `signals/telegram` — each one's documentation page states the specification
  version it conforms to, and an adapter whose page is the guide itself is
  claimed there.
- **The specification governs the agent too.** The existing scoped rule
  `signal-rules.md` is extended to bind changes under `signals/**` and the
  manager's ingest surface: verify against the specification, never edit it
  without the user's explicit approval.
- **A specification change carries conformance.** An approved edit must, in
  the same change, bring every shipped signal adapter into conformance and
  move every claimed version.
- **Depends on `runtime-specification`** — now archived. It landed the page
  split, the versioned-spec convention and the governance pattern, but NOT a
  generator: its own `struct-table` marker was added and then removed in the
  same change, in favor of hand-written tables. This change follows the
  convention runtime-specification actually shipped, not the one its
  proposal originally described. Out of scope: any behavior change to the
  manager, the adapters or the chart.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `signal-adapter-contract`: gains the document-governance requirements —
  the versioned specification page, hand-written requirement-ID tables,
  compatibility rules, the Conformance section, the station-procedure guide,
  version claims on shipped adapter pages, conformance carried by a spec
  change, and the agent-governance rule.

## Impact

**Reference docs touched:**

- `docs/contracts/signal-adapter.md` — reworked into the specification, and
  promoted to a published page (front matter, permalink, nav entry), same as
  `runtime.md`.
- The shipped adapters' pages (`docs/integrations/home-assistant.md`,
  `docs/integrations/prometheus.md`, `docs/integrations/kubernetes.md`,
  `docs/integrations/telegram.md`) — conformance line.

**Adopter site touched:**

- `docs/guides/signal-adapter.md` — rewritten as the station procedure, and
  the claim point for adapters with no page of their own (cron).
- Any page linking an anchor the rework renames.

**Context touched:**

- `.claude/rules/signal-rules.md`, extended rather than a new file. It
  already scopes `signals/**` and `internal/ingest/`, and gains the signal
  endpoints in `internal/httpapi/` plus the three obligations: verify against
  the specification, never edit it unapproved, a spec change carries
  conformance.
- `.claude/rules/documentation.md` routing table — a row sending signal
  adapter contract changes to `docs/contracts/signal-adapter.md`.
- `docs/contracts.md` — points to the signal page.
- `docs/.claude/site.md` — signal-adapter.md moves from "awaiting the
  versioned-spec rewrite" to published, alongside `runtime.md`.
- `docs/_data/nav.yml` — Specifications group gains the new page.

**Code:** none expected. 1.0.0 documents shipped behavior, and a conformance
gap found while verifying is fixed in this change.
