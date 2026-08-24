## Context

See `proposal.md` — Why. What shapes the approach is where the content already
sits, and what the site's own rules already constrain.

**The content exists. It is indexed by chart value.** `installation.md` carries
the agent's power, the Secrets boundary, who may reach what, and toolset
enforcement — all organised by the key an operator sets.
`docs/adr/0001-bound-component-reach.md` is the one document written threat-first
and is not on the site. `SECURITY.md` is forge-only.

Four constraints are already binding, and they decide most of the design:

| Constraint | Where | Effect |
|---|---|---|
| the site's deliverables are enumerated | `docs-site` | publishing is front matter plus one nav line, and the enumeration must name the page |
| the *Start here* order is a chain | `documentation-structure` | inserting a page adjusts the what-next cards on both sides |
| where two documents cover one subject, the routing rule says which gets what | `documentation-structure` | the security/installation split must be written down |
| reference pages carry no front matter and are not edited for the site | `docs-site` | the ADR is linked where it lives |

**This page describes the system as it is today.** `contextSync` is opt-in, so
context isolation is a mode an install can choose. The change
`context-sync-by-default` will make it the default; when it lands, this page is
revised by `/opsx:update` rather than being written ahead of it. A page
describing behaviour that does not exist yet is the failure this project already
guards against in published specs.

## Goals / Non-Goals

**Goals:**

- One place a reader can evaluate the security posture without reading the
  installation page.
- Every claim on the page verified against the code or the chart before it is
  written, because a public security claim that is wrong is worse than silence.
- The gaps stated as plainly as the controls.

**Non-Goals:**

- **Moving content out of `installation.md`.** Explicitly decided. It keeps every
  key, default and values table it has.
- **Publishing the ADR**, or any other reference page. That is its own change.
- **Editing `SECURITY.md`.** Reporting stays on the forge; the page links it.
- **A security section on every page.** Two entry points gain one line each.
- **Auditing or changing what the product grants.** This states the posture; it
  does not alter it.

## Decisions

### D1 — One page, not a set

The current failure is scattering. A set of security pages reproduces it with a
security label on top, and forces a reader evaluating the product to assemble an
answer from several places — which is what they cannot do today.

**Alternative considered: a page per wall.** Rejected: the walls are only
meaningful against each other. A reader needs to know that closing one leaves the
next one open, and three pages state that nowhere.

### D2 — The two pages index different axes, and that is what makes this not duplication

`installation.md` is indexed by KEY: here is the value, its default, its YAML.
The security page is indexed by THREAT: here is what can go wrong, what bounds
it, what bounding it costs, and what remains.

The rule that keeps them apart is mechanical rather than editorial: **the
security page carries no values table and no default.** It names a control in
prose and links the key. A default stated in one place cannot drift; stated in
two, nothing fails when they disagree.

**Alternative considered: move the threat prose out of `installation.md`.** Some
of it is already threat prose in a values page — "turn it on only if you accept
the agent reading every Secret in the cluster". Rejected by decision: nothing
moves. The cost is accepted — that sentence exists in both registers — and it is
bounded, because the security page states the consequence while the installation
page states it beside the key that causes it.

### D3 — It sits before Installation in the reading order

Security is an evaluation gate, not an install step. A reader decides whether to
run model output in their cluster before they choose values, not after.

Mechanically that is one nav entry between the Console page and Installation,
`console-guide.md`'s what-next card repointed at it, and its own card pointing at
Installation — all three together, or a card skips an entry in its own group.

**Alternative considered: after Installation.** That serves the reader who has
already decided, which is the reader who needed it least.

### D4 — The ADR is referenced, never reproduced and never published

The page states each decision in a sentence and links the record. Copying the
trade-off analysis onto the page creates a second copy of reasoning that the ADR
owns, and publishing the ADR means giving a reference page front matter — which
`docs-site` forbids outside the change that publishes reference pages.

### D5 — Supply chain is in scope, as facts and gaps, not a section

Verified against the workflows: images carry `provenance: mode=max` and
`sbom: true`; `actions/attest-build-provenance` appears nowhere, and
`build-chart.yml` has no `attestations:` permission at all.

So the answer is partial. It goes as three lines under what agent-ops holds plus
two entries in the gap list. **A heading promises a subject is handled**, and a
"Supply chain" section over a partial answer reads as more than ships.

Noted and not acted on here: `release.yml` already grants `attestations: write`
and `id-token: write`, unused. That is a code change and belongs to its own
change; the page will make it visible.

### D6 — The not-addressed section is load-bearing, and the spec says so

A security page listing only what is handled is read as a claim that the rest is
handled. This section is the reason the page is worth publishing, and it is the
first thing a later edit will tidy into a callout — so
`security-posture-page` states that it stays a section, and that requirement is
the mechanism, not this paragraph.

### D7 — Every claim is verified before it is written, and the verification is a task

The page will assert that the manager holds no `secrets` verb, that adapter
tokens are derived, that runtime pods are non-root, and what images carry. Each
is checkable in one command, and each is a public claim about a security
property. The tasks name the check beside the claim rather than trusting this
design's own research.

## Risks / Trade-offs

- **Publishing a gap list tells an attacker where to look** → Accepted, and it is
  the project's existing posture rather than a new one: ADR 0001 already
  documents the same unauthenticated surfaces in the public repository, and
  `SECURITY.md` already declares a chart default that grants too much to be a
  finding rather than a configuration choice. Publishing on the site changes
  reach, not disclosure. An adopter who cannot see a gap cannot compensate for
  it, and the surfaces are discoverable from a running install regardless.

- **The page dates as controls change** → Bounded by D2. It states no defaults,
  so a chart change makes at most its prose imprecise rather than wrong, and the
  routing rule tells the next writer which document takes the edit.

- **It duplicates `installation.md` over time** → The axis rule is the guard, and
  the spec states it as "no chart value the installation page owns". A values
  table appearing on the page is the observable regression.

- **`contextSync` becoming the default invalidates a section** → Known, dated and
  handled by `/opsx:update` after `context-sync-by-default` lands. Recorded in
  Context so the next reader does not treat the section as stale drift.

- **One line on the landing page reads as an afterthought** → It is one line
  because the landing page's section list is a bound the site's own spec holds.
  Its job is to prove the question was considered and route the reader, not to
  answer it.

## Migration Plan

Not applicable — no behaviour changes, and nothing an adopter must do.

**The one ordering that matters is publication.** The nav entry, the page's own
front matter, and both what-next cards land together. Any of them alone leaves
the *Start here* chain inconsistent: an entry unreachable from the page before
it, or a card skipping an entry.

**Rollback** is removing the nav line, the page, and the two entry-point lines,
and repointing `console-guide.md`'s card at Installation.
