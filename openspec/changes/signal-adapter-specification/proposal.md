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
  starting `1.0.0`) normative specification.** Two-party: what the manager
  provides (one inbound endpoint, type-scoped derived-token auth,
  manager-side grouping and cooldown, no Kubernetes access required) and what
  the adapter must do (normalize and push, fingerprint honestly, never emit a
  signal about agent-ops' own machinery).
- **Generate the struct-backed schema tables** with the `struct-table` marker
  `runtime-specification` adds to `docs-generate.py`, so the inbound payload
  fields cannot drift from the Go structs.
- **Add the numbered conformance checklist**, each item verifiable, cited
  against the shipped adapters' tests.
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
- **Depends on `runtime-specification`** — the page location and the
  generator capability land there. Out of scope: any behavior change to the
  manager, the adapters or the chart.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `signal-adapter-contract`: gains the document-governance requirements —
  the versioned specification page, generated schema tables, compatibility
  rules, the conformance checklist, the station-procedure guide, version
  claims on shipped adapter pages, conformance carried by a spec change, and
  the agent-governance rule.

## Impact

**Reference docs touched:**

- `docs/contracts/signal-adapter.md` — reworked into the specification.
- `.github/scripts/docs-generate.py` markers placed for the inbound structs.
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

**Code:** none expected. 1.0.0 documents shipped behavior, and a conformance
gap the checklist finds is fixed in this change.
