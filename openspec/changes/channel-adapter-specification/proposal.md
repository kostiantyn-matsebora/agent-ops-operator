## Why

After `runtime-specification` splits the contracts, the channel adapter
contract page is still the moved narrative. It explains decisions but carries
no version, no exhaustive field tables, no conformance checklist and no
governance.

An adopter building a channel adapter should get what a runtime adopter now
gets: a specification that produces a working adapter on the first attempt,
and a guide that is the exact path to it.

## What Changes

- **Rework `docs/contracts/channel-adapter.md` into a versioned (semver,
  starting `1.0.0`) normative specification.** Two-party: what the manager
  provides (the ops long-poll with its claim window, typed operations, the
  vocabulary revision, derived-token auth, delivery recording) and what the
  adapter must do (consume `/channel/*`, parse the body grammar, absorb
  retryable backpressure, own outbound pacing, never relay its own outbound
  as inbound).
- **Hand-write the struct-backed schema tables**, in the
  `| Field | Type | Required | Description |` shape `docs/contracts/runtime.md`
  settled on. The `struct-table` marker this proposal originally planned to
  reuse was tried in `runtime-specification` and reversed there before this
  change started — generated cells copied Go doc-comment paragraphs verbatim,
  producing unreadable rows. No generator exists for these tables.
- **Add the numbered conformance checklist**, each item verifiable, cited
  against the shipped adapters' tests.
- **Rewrite `docs/guides/channel-adapter.md` as the station procedure** —
  ordered steps, each citing a spec section by anchor, each ending in a
  verification.
- **Shipped channel adapters claim a spec version.** `channels/telegram` and
  `platform/console` are both channel adapters — each one's documentation
  page states the specification version it conforms to.
- **The specification governs the agent too.** A scoped context rule binds
  changes under `channels/**`, the console's adapter surface and the
  manager's channel surface: verify against the specification, never edit it
  without the user's explicit approval.
- **A specification change carries conformance.** An approved edit must, in
  the same change, bring every shipped channel adapter into conformance and
  move every claimed version.
- **Depends on `runtime-specification`** — the page location and the
  hand-written table shape land there. Out of scope: any behavior change to
  the manager, the adapters or the chart.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `channel-adapter-contract`: gains the document-governance requirements —
  the versioned specification page, generated schema tables, compatibility
  rules, the conformance checklist, the station-procedure guide, version
  claims on shipped adapter pages, conformance carried by a spec change, and
  the agent-governance rule.

## Impact

**Reference docs touched:**

- `docs/contracts/channel-adapter.md` — reworked into the specification.
- `.github/scripts/docs-generate.py` markers placed for the op structs.
- The shipped adapters' pages (`docs/integrations/telegram.md`,
  `docs/console.md`) — conformance line.

**Adopter site touched:**

- `docs/guides/channel-adapter.md` — rewritten as the station procedure.
- Any page linking an anchor the rework renames.

**Context touched:**

- A new scoped rule (loads under `channels/**`, the console adapter surface
  and the manager's channel surface) — verify against the specification,
  never edit it unapproved, a spec change carries conformance.

**Code:** none.
