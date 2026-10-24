## Why

`Coordinator` shipped as a real second wiring kind. A coordinating agent
invokes named members by purpose, loops on their results, and escalates to
a human only when it decides to.

Adopter-facing documentation still treats `Pipeline` as the only core
concept. The landing page's entire presentation, the README's flow diagram,
and the README's own "How it works" walkthrough are all built around one
`Pipeline` example. Coordinator surfaces only as a footnote bullet, read
after everything else.

Separately, two follow-up fix PRs (#296, #301) shipped real behavior
changes to the Coordinator with zero documentation commits.

#301 rewrote the coordinating agent's own prompt. It moved from a
dispatcher matching trigger-style descriptions ("a cluster event", "once
the cause is known") to an orchestrator delegating by purpose.

That is the exact anti-pattern `docs/guides/coordinate-agents.md` still
teaches in its own worked examples. An adopter copying that guide
reproduces the incident #301 fixed.

Now is the right time. Chart 14.0 (14.0.0 and 14.0.1) has shipped, clearing
the version-number gate this change was held behind.

The imbalance also compounds. Every release that extends the Coordinator
side of the product without touching the docs that explain it widens the
gap further.

## What Changes

**Coordinator guide, changelog and security catch-up** — content accuracy,
no new behavior:

- Rewrite `docs/guides/coordinate-agents.md`'s worked examples off
  trigger-style descriptions onto the purpose shape PR #301 shipped: what
  the agent IS, what it CAN do, what it CANNOT, what to HAND it.
- Correct the guide's surrounding prose from dispatcher framing ("reads to
  decide when to use it") to the orchestrator framing the shipped prompt
  actually uses.
- Add the missing `docs/CHANGELOG.md` entry for #301 — orchestrator prompt
  reframe, description `maxLength` 512→2048, three bundles' rewritten
  capability descriptions — to the next chart version heading.
- Disclose in `docs/security.md`'s "Agent-invoked agents" section that a
  coordinated member's ask-before-acting consent boundary is currently
  prompt-only (`agentops.memberScopeInstruction`, chart-rendered), not a
  mechanical gate at `/coordinate/invoke`. Name the incident history from
  #296 (several rounds of prompt tuning, reverted twice) as why this
  residual risk is stated rather than assumed solid.

**Elevate Coordinator to core-feature parity in the primary adopter
narrative:**

- Add one clause to README.md's "How it works" step 3, naming `Coordinator`
  as the other route a reader can declare, at the point `Pipeline` is first
  introduced. Align the "Compose agents, another seam" bullet's wording
  with the orchestrator framing used elsewhere. Leave the Pipeline YAML
  worked example and the diagram's own content alone — both are already
  correctly scoped as one worked example, not a place to duplicate a
  second one.
- Redraw `docs/diagrams/readme-flow.py` to branch into two routes after
  "you declare it": `Pipeline` as the existing linear chain, `Coordinator`
  as a hub fanning out to named members. The Coordinator branch needs its
  own result-returns loop — a member's result returns to the coordinator
  before it decides to invoke someone else, escalate, or stop — and
  `escalate` as its own conditional path reaching the channel from
  underneath, never the same automatic arrow `Pipeline`'s `channelRefs`
  draws. A validated mockup of this exact redraw is the reference for the
  real rework: `prototypes/readme-diagram-mockup.html` (see design.md)
- Rework the landing page's presentation (`index.md`'s `.ao-presentation`,
  `assets/js/presentation.js`) to carry Coordinator as a second story with
  equal weight to Pipeline's, not an aside read afterward. It needs a
  hub-and-branch drawing, not a relabeled copy of Pipeline's linear chain,
  built from the site's own tokens and the console's per-kind shapes:
  hexagon trigger, cylinder channel, circle identity, and diamond for the
  Coordinator hub specifically, since it is a decision point. Tab order:
  Orchestrator first, Pipeline second. "Orchestrator" is the human-facing
  label for the Coordinator's role in this narrative — the kind label
  drawn inside the figure stays `Coordinator`, since that is what is
  actually declared. A validated prototype of both stories is the
  reference: `prototypes/landing-presentation-mockup.html` (see design.md)
- Review `introduction.md` and `getting-started.md` for the same
  Pipeline-only framing and correct what is found. Not yet audited in
  detail. This is a real review task, not an assumption that they are
  clean.

**Close out existing verification debt while in this neighborhood:**

- Finish `coordinator-deployment-mode`'s three remaining open tasks,
  workstation-only verification unblocked now that a live cluster with
  `mcp-aops` is running: 5.1 (smoke test against the live install), 8.1
  (run the already-written e2e pack lane), 9b.5 (the conditional
  screenshot and demo re-check). Task 9b.5 is expected to be a no-op,
  since that change touched no console code, but it gets confirmed and
  ticked rather than skipped.

## Capabilities

### New Capabilities
(none — this change corrects and extends documentation and closes existing
verification tasks. It introduces no new system behavior.)

### Modified Capabilities
(none — `coordinator-self-heal` and `wiring-mode`'s specs already describe
the behavior this change documents and verifies. Nothing about that
behavior changes here.)

## Impact

**Reference docs:**

| File | Change |
|---|---|
| `docs/guides/coordinate-agents.md` | worked examples and framing rewritten |
| `docs/CHANGELOG.md` | missing #301 entry added |
| `docs/security.md` | residual-risk disclosure added |
| `docs/cr-reference.md` | unaffected — generated from CRDs, which do not change |

**Adopter site:**

| File | Change |
|---|---|
| `README.md` | "How it works" step 3, "Compose agents" bullet |
| `docs/diagrams/readme-flow.py` + generated `assets/img/readme-flow-{light,dark}.svg` | redrawn to branch |
| `index.md` + `assets/js/presentation.js` | reworked to carry two stories |
| `introduction.md`, `getting-started.md` | reviewed, corrected if needed |

**Code:** none. This change touches no CRD, no manager code, no chart
template. The only non-documentation work is running existing,
already-written verification — `coordinator-deployment-mode`'s 5.1 smoke
test and 8.1 e2e lane — and ticking that change's own tasks file.

**Process:** `coordinator-deployment-mode` is expected to reach 33/33 and
become archivable as a side effect of the third work item above,
independent of this change's own archival.
