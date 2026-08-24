## Why

**The adopter site has no security story — not a thin one, none.** `index.md`,
`introduction.md`, `getting-started.md` and `README.md` contain the word zero
times. A reader deciding whether to run model output inside their cluster has no
entry point at all.

The content is not missing. It is **indexed by the wrong axis**: almost all of it
sits in `installation.md`, organised as *values an operator sets*, and a security
reviewer reads by *threat*. The one document written threat-first,
`docs/adr/0001-bound-component-reach.md`, is not on the site. `SECURITY.md` is
forge-only.

Five things are implemented and stated **nowhere a reader will find**: the
manager holds no `secrets` verbs at all, per-adapter tokens are HMAC-derived
rather than stored, runtime pods are non-root with per-conversation workspace
isolation, a conversation's accumulated context is isolated structurally rather
than by permission, and every image ships an SBOM and max-mode provenance.

The context one is the sharpest, because it cuts both ways. Under `contextSync`
the agent container holds the live context on ephemeral pod-local storage and
holds **no mount of the durable volume at all** — an agent cannot read another
conversation's context because there is nothing to read from, while a sidecar
keeps it durable. Since `context-sync-by-default` landed that is what a DEFAULT
install runs, and it is a real property no adopter can find. Its limit is equally
unwritten: the mode needs a runtime that declares its context paths, and one that
declares none — a second vendor's, until its own entry states them — falls back
to the pod that mounts the whole shared volume into the agent container. Neither
half of that is written down anywhere an adopter reads.

## What Changes

- **A new site page, `docs/security.md` at `/security/`**, shaped as a THREAT
  MODEL: a drawing of the trust boundaries and the flows crossing them, a
  register keyed to it by number, then defence in depth control by control, the
  platform's own posture, and the **residual risk**.
- **It uses a security reviewer's vocabulary**, not names invented here —
  defence in depth, network segmentation, egress control, authorization,
  residual risk.
- **It is illustrated throughout**, not once: the threat model plus a drawing at
  each claim prose states poorly — the unauthenticated surfaces, the allowlist
  an agent with a shell walks around, the Secret reached through the kubelet,
  and the context mount that is absent by design.
- **It is inserted into the *Start here* reading order** between the Console page
  and Installation — it is an evaluation gate, not an install step. One
  navigation entry, and the what-next cards on both sides adjusted together.
- **It restates no chart value.** `installation.md` keeps the keys, the defaults
  and the values tables; the security page states the threat, the posture and the
  cost of closing it, and links the key. The two index different axes, which is
  what makes covering the same subject not duplication.
- **Two sections carry content that exists nowhere on the site today** — what
  agent-ops itself holds, and what is not addressed.
- **Context isolation is stated per mode, and the default install's mode is
  stated first.** The structural isolation `contextSync` gives is what a default
  install runs and is described as that — with the three conditions the mode
  needs named, never as unconditional. The unsynchronised pod's shared volume is
  named in the not-addressed section too, because the reader who skips there is
  the one most exposed to it.
- **The supply-chain answer is stated as it actually is**: images carry a
  BuildKit SBOM and `provenance: mode=max`; nothing is signed and the chart is not
  attested. Three lines under what agent-ops holds, two entries in the gap list —
  not a section of its own, which would read as more than ships.
- **Egress mediation and network policy are named as decisions, briefly, with the
  reasoning referenced to ADR 0001** rather than reproduced. The ADR stays a
  reference document at its own path and gains no front matter.
- **`docs/index.md` and `README.md` each gain one line** naming the page, so the
  question is visible on both entry paths.
- **`docs/installation.md` gains links out to `/security/`** from its
  security-shaped sections. Links only — no content moves, and no section is
  removed from it.

## Capabilities

### New Capabilities
- `security-posture-page`: what the site's security page owes a reader — the
  threat model and the register keyed to it, the default-grants-nothing
  statement, the three defence-in-depth controls each stated as threat, control,
  cost and residual risk, what agent-ops itself holds, the load-bearing
  residual-risk section, and the rule that it states no chart value
  `installation.md` owns.

### Modified Capabilities
- `docs-site`: the site's deliverable page set gains `docs/security.md`. The
  requirement enumerating deliverables names it.
- `documentation-structure`: the *Start here* reading-order chain gains a page
  between the Console page and Installation; the document routing rule gains the
  security/installation split, in the shape it already uses for the
  console-guide/console pair.

## Impact

**The adopter site**

- `docs/security.md` — **new**, the page itself.
- `docs/diagrams/threat-model.py` — **new**, and its two committed SVGs under
  `docs/assets/img/security/`. A hand-run generator like `readme-flow.py`, since
  no CI job renders either.
- `.github/scripts/docs_diagrams.py` and `docs-generate.py` — four more diagram
  specs, plus a `dir` key so a spec can land outside `assets/img/guides/`. Guide
  entries state none and are untouched.
- `docs/_data/nav.yml` — one entry, in *Start here*, between the Console page and
  Installation.
- `docs/index.md` — one line naming the page. Without it the question is
  invisible on the landing page.
- `docs/console-guide.md` — its what-next card points at Installation today and
  must point at the new page, or a card skips an entry in its own group.
- `docs/installation.md` — links out to `/security/` from *The agent's power*,
  *Who may reach what* and *Enforcing the toolset*. **No content moves out of it**
  and no section is removed.
- `docs/getting-started.md` and `docs/introduction.md` — reviewed, not
  necessarily edited. Neither sits adjacent to the insertion point in the chain.

**The reference docs and the forge**

- `README.md` — one line in the links-onward index. It is at 203 lines against a
  215-line budget, and this adds no section, so the bound is unchanged.
- `docs/CLAUDE.md` — the site's-pages table gains a row saying what the security
  page owns.
- `.claude/rules/documentation.md` — the routing table gains a row, so the next
  writer does not have to choose between the security page and `installation.md`
  at random.
- `docs/adr/0001-bound-component-reach.md` — **linked, not edited.** It carries no
  front matter and stays a reference page.
- `SECURITY.md` — **linked, not edited.** Reporting stays where it is.

**Not affected**

- `docs/CHANGELOG.md` — no behaviour changes, nothing to upgrade, no migration
  step. A documentation-only change earns no entry.
- `docs/concepts.md`, `docs/contracts.md`, `docs/console.md`, the bundle pages —
  untouched reference pages.
- No code, no chart template, no CRD. Nothing in this change alters what an
  install grants; it states what it already grants.
