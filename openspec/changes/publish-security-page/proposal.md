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
isolation, a conversation's accumulated context can be isolated structurally
rather than by permission, and every image ships an SBOM and max-mode provenance.

The context one is the sharpest, because it cuts both ways. Under `contextSync`
the agent container holds the live context on ephemeral pod-local storage and
holds **no mount of the durable volume at all** — an agent cannot read another
conversation's context because there is nothing to read from, while a sidecar
keeps it durable. That mode is opt-in. In the default mode an install with a
shared context volume mounts the whole volume into every agent container. Neither
half of that is written down anywhere an adopter reads.

## What Changes

- **A new site page, `docs/security.md` at `/security/`**, indexed by threat:
  what you are trusting, what the default install grants, what each of the three
  walls bounds, what agent-ops itself holds, and what is **not** addressed.
- **It is inserted into the *Start here* reading order** between the Console page
  and Installation — it is an evaluation gate, not an install step. One
  navigation entry, and the what-next cards on both sides adjusted together.
- **It restates no chart value.** `installation.md` keeps the keys, the defaults
  and the values tables; the security page states the threat, the posture and the
  cost of closing it, and links the key. The two index different axes, which is
  what makes covering the same subject not duplication.
- **Two sections carry content that exists nowhere on the site today** — what
  agent-ops itself holds, and what is not addressed.
- **Context isolation is stated per mode, and its default is stated first.** The
  structural isolation `contextSync` gives is described as a mode an install can
  choose, never as the posture it has; the default mode's shared volume is named
  in the not-addressed section too, because the reader who skips there is the one
  most exposed to it.
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
  default-grants-nothing statement, the three walls indexed by threat, what
  agent-ops itself holds, the load-bearing list of what is not addressed, and the
  rule that it states no chart value `installation.md` owns.

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
