## Why

The work contract is a narrative inside `docs/contracts.md`. It explains
decisions but does not state every field, env var, enum and obligation, so
nobody can build a runtime from it without reading `runtimes/claude`.

It has already drifted: the adopter guide claims the manager checks out the
repository, while every runtime clones it itself.

Three extension points (runtime, channel adapter, signal adapter) share one
page no single adopter needs the whole of.

## What Changes

- **Split `docs/contracts.md` by extension point.** The channel and signal
  adapter contracts move VERBATIM to `docs/contracts/channel-adapter.md` and
  `docs/contracts/signal-adapter.md`. `docs/contracts.md` keeps the
  manager-owned surfaces (aops MCP server, activity, introspection, metrics,
  HTTP API) and its URL. Every link and routing table follows.
- **Write `docs/contracts/runtime.md`** — a versioned (semver, starting
  `1.0.0`) normative runtime specification replacing the work-contract
  narrative. Two-party: what the manager provides, what the runtime must do.
  Exhaustive: lifecycle state machine, environment table, filesystem contract,
  full field tables for the work unit and the report, every enum, bounds,
  refusal semantics, security posture, optional capabilities, a numbered
  conformance checklist, compatibility rules (unknown fields ignored on both
  sides becomes normative).
- **Generate the schema tables from source.** `docs-generate.py` gains a
  marker that renders a field table from a Go struct's json tags, so a field
  added to `dispatch.go` without a spec row fails CI.
- **Rewrite `docs/guides/agent-runtime.md` as the station procedure** — the
  exact ordered steps to introduce a runtime, each station citing a spec
  section and ending in a verification. Fixes the false checkout claim.
- **Shipped runtime pages claim a spec version.** `docs/runtimes/ollama.md`
  and `docs/runtimes/copilot.md` each state the specification version they
  conform to.
- **The specification governs the agent too.** A scoped context rule binds
  every change under `runtimes/**` and the manager's work-contract surface:
  verify the change against `docs/contracts/runtime.md` before claiming it
  done, and never edit the specification itself without the user's explicit
  approval.
- **A specification change carries conformance.** An approved edit to the
  spec must, in the same change, bring every shipped runtime into conformance
  and move the version each vendor page claims (`docs/runtimes/ollama.md`,
  `docs/runtimes/copilot.md` and `docs/claude.md`).
- **Out of scope, named follow-ups:** spec-ifying the channel and signal
  adapter contracts to the same standard (one change each), and a runnable
  conformance harness.

## Capabilities

### New Capabilities

- `runtime-contract`: the versioned runtime specification — the document's
  existence and location, its two-party structure, its generated schema
  tables, its compatibility and versioning rules, the conformance checklist,
  the station-procedure guide that references it, the conformance-version
  claim on every shipped runtime page, and the governance rules (changes to
  the governed surface verified against the spec, the spec never edited
  without the user's approval, a spec change bringing every shipped runtime
  along in the same change).

### Modified Capabilities

- `documentation-structure`: the "reference material split by audience"
  requirement changes — extension-point contracts live one page per contract
  under `docs/contracts/`, `docs/contracts.md` holds only manager-owned
  surfaces, the routing rule and README documentation index name the new
  pages, and reorganization preserves content and links.

## Impact

**Reference docs made untrue or touched:**

- `docs/contracts.md` — loses three sections, keeps manager surfaces.
- `docs/contracts/runtime.md`, `docs/contracts/channel-adapter.md`,
  `docs/contracts/signal-adapter.md` — new pages.
- `docs/runtimes/ollama.md`, `docs/runtimes/copilot.md`, `docs/claude.md`
  (the claude runtime's page, staying where it is) — conformance line.
- `README.md` — Documentation index gains the contract pages.
- `.github/scripts/docs-generate.py` — new struct-table marker.

**Adopter site made untrue or touched:**

- `docs/guides/agent-runtime.md` — rewritten as the station procedure.
- `docs/guides/channel-adapter.md`, `docs/guides/signal-adapter.md`,
  `docs/guides/toolsets.md`, `docs/index.md`, any page linking a
  `contracts.md` anchor that moved — links retargeted.

**Context made untrue:**

- Root `CLAUDE.md` and `.claude/rules/documentation.md` — routing tables name
  `docs/contracts.md` as the home of work/adapter contracts.
- `docs/.claude/site.md` — names `contracts.md` as the single contracts page.
- A new scoped rule (loads under `runtimes/**` and the manager's
  work-contract surface) — verify against the specification, never edit it
  unapproved.

**Code:** none. The manager, runtimes and chart do not change — the
ignore-unknown-fields rule codifies existing `encoding/json` behavior.
