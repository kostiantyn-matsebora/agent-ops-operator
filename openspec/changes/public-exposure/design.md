# Design — public exposure

## D1. Apache-2.0, not MIT

The author's other public repository is MIT, so this is a deliberate divergence
rather than an oversight.

Kubernetes-ecosystem operators are overwhelmingly Apache-2.0, and the reason is
the **patent grant** MIT does not carry. An operator is infrastructure other
companies run; the licence that lets their lawyers say yes is the one the
ecosystem already standardised on.

**It is the one file that cannot be quietly changed later.** Once a fork exists,
relicensing needs every contributor's agreement — today that is one person, and
that is the cheapest this decision will ever be.

## D2. The README stops restating the site

The existing README requirement was written when there was **no site**. It
therefore made the README carry the pitch, the kind list, the behaviours, the
start and the index — correctly, because nothing else could.

The site now owns the model, the walkthrough, the console tour and the install.
A README that repeats them is not merely long, it is a **second source of truth
that drifts**, and the drift is invisible until an adopter follows the wrong one.

So the README answers what a stranger asks in the first two minutes:

```
   what is this        →  the pitch and the diagram
   is it real          →  the licence, the status, one honest sentence
   can I try it        →  ONE command that works without cloning
   where do I go next  →  the site first, then the reference pages
```

Everything else is a link. **What moves out is not deleted** — the existing
requirement's rule that removed content stays reachable in one hop still holds,
and the site is where it goes.

### D2a. "Stops restating" is not "stops covering", and TWO drafts got it wrong

D2 as first stated produced a thin index. It answered the four questions and
said almost nothing a stranger could evaluate the project on, because every
claim had been ruled a repetition of the landing page.

**That over-read the rule.** The landing page and the README are the same story
for two audiences — a reader who arrived at the site, and a stranger who landed
on the forge and may never leave it. The forge audience is the larger one, and
sending it away to learn what the project IS is not a boundary, it is a gap.

So the line moved:

| | |
|---|---|
| **covering** | naming what this is, what it is for, HOW IT WORKS, what a reader declares and why it is built that way. **The README does this.** |
| **restating** | reproducing a site page's DETAIL — the walkthrough, the installation decisions, the console tour. **The site owns this.** |

**And the README says outright that the site is the main source**, above the
index rather than inside it, so a reader who wants more never has to infer which
document is authoritative.

**THE SECOND DRAFT FAILED THE SAME TEST ONE LAYER DOWN**, and is why this is
written as a section list rather than a principle. It kept the diagram and cut
the prose the diagram contained — the `pipeline.yaml`, the three seams, the
three build reasons, how it works at all — reasoning that the picture already
said it. The result was a page that named the project and then stopped.

**A picture is not content.** It cannot be selected, searched, copied or diffed,
a screen reader gets only its alt text, and a reader skimming a forge page reads
headings. The README's sections now TRACK the landing page's, one for one, and a
section added there is considered here.

### D2b. The media split is what makes the drift argument moot

D2's real worry was two copies of one text drifting apart. That worry does not
survive contact with how the two surfaces actually render:

| Surface | Carries the story as |
|---|---|
| the landing page | its `.ao-presentation` tab set and the console recordings |
| `README.md` | a mermaid flowchart, rendered by GitHub from source text |

- **GitHub renders neither tabs nor autoplaying video.** The README needs a
  picture precisely where the site needs a presentation, so there is no shared
  text to drift.
- **THE EXPORTED SVG WAS TRIED FIRST AND IS TOO BIG.** `agent-ops-light.svg` is
  1778×1349, composed for a page; GitHub's ~1012px column shrinks it past
  legibility. It is now a CLICK-THROUGH under the diagram, which keeps the asset
  consumed without asking a forge column to hold a page-scale drawing.
- **Mermaid wins on three counts an image loses:** it scales to the column, it
  follows the reader's theme without a `<picture>` shipping both halves, and it
  is TEXT — diffable in review, and current without an export step.
  `docs/diagrams/message-flow.mmd` is the precedent.
- **The SVG pair had NO consumer** after the landing-page rebuild moved the site
  onto the presentation. It is build output of `docs/diagrams/export.py`, kept
  current through the terminology sweep, and pointed at nothing — the README is
  now what it is for.
- **Whatever the diagram also carries, the prose carries anyway.** See D2a:
  cutting text because a picture has it was tried and read as an empty page.

**The budget went 150 → 240 with this**, because 150 was the number for a README
that was three documents wearing one filename. **The section list is the bound
and the number follows it** — the hard rule, no reference material, did not move.
The mermaid source is ~40 of those lines, which is what a picture the forge
renders, scales and themes actually costs.

## D3. The install command decides whether the README can be honest

`helm install ./chart` cannot appear in a README a stranger reads: it requires a
clone the previous line did not tell them to make.

The OCI chart from `sdlc-setup` is what makes a one-line install true. Until it
publishes, this change **cannot finish the README** — which is a dependency
between changes, not a note. The gate carries it.

## D4. The gate, because the flip cannot be undone

Publication is one switch and no rollback: after it, the history is public and a
rewrite breaks every clone.

So the last task is a **gate**, and each condition is something that becomes
impossible or expensive afterwards:

| Condition | Owner | Why before |
|---|---|---|
| The publication guard is green | `sdlc-setup` | it is what proves the next two conditions |
| Identifying content is gone from tree, archive AND history | `scrub-identity` | a rewrite after publication breaks every clone |
| The published specs are true | `truthful-specs` | a contradiction read by a stranger is read once and remembered |
| Images are public and the chart installs by OCI | `sdlc-setup` | the README's one command must work on the day it is readable |
| Licence, community files and templates exist | here | an issue filed before there is a template is a template nobody will retrofit |

**Pages was to be enabled BEFORE the flip**, not after: it is what makes the
README's links resolve, and a first visitor who hits a 404 does not come back
for the fix.

### D4a. THAT ORDERING IS IMPOSSIBLE ON THIS PLAN, and the gate absorbs it

Both remaining §6 items were REFUSED against the private repository, and the
refusals are the plan's, not a permission mistake:

| Item | What the API answers while private |
|---|---|
| Pages (6.3) | 422 — "your current plan does not support GitHub Pages for this repository" |
| Branch protection (6.5) | 403 — "upgrade to GitHub Pro or make this repository public" |

**GitHub Pages is unavailable for a PRIVATE repository on the Free plan**, so
the one condition D4 wanted settled beforehand cannot be. The choice is
therefore between paying for Pro and accepting a WINDOW, and the window is
minutes rather than never:

1. 7.6 flips the repository.
2. Pages and branch protection are applied AT ONCE, as the next action.
3. Only then is the README's index followed, which is 7.6's own verification.

- **The 404 risk is not eliminated, it is BOUNDED.** D4's reasoning survives —
  a first visitor meeting a 404 really does not come back — so the mitigation
  is that nothing announces the repository until step 3 has passed.
- **Do not read this as "settings come after publication".** Description,
  topics, homepage and the feature toggles were all applied while private, and
  they are what a stranger meets first. Only the two the plan refuses moved.
- **The branch-protection SHAPE was decided before it could be applied**, so
  the move costs no thinking later: deletion blocked; the six stable CI jobs
  required (`operator`, `modules`, `console-ui`, `chart`, `docs`,
  `publication` — `images` is a matrix and its name is not stable enough to
  require); **admins exempt**, because direct commits to `master` are how this
  project actually works and a policy that blocks them is the one bypassed on
  the first hotfix; no required reviews, since a solo maintainer cannot
  self-approve and the branch would simply freeze.
- **Force pushes stay ALLOWED until the flip**, then are blocked. Blocking them
  earlier would have prevented the history rewrite that §5 needed.

## D5. Security reporting is a contact link, never an issue type

A security issue template invites the report into a public issue, which is the
disclosure it was meant to prevent.

Blank issues are disabled and the security route is a `contact_links` entry
pointing at private advisories, so the only paths are a template that fits or a
private channel.

**The acknowledgement target is one a single maintainer can actually keep.** A
policy promising 24 hours from one person is a promise that will be broken in
public.

## D6. The hygiene items are not incidental

Two of them would be noise on their own and are not here:

- **The committed binary** is over half the packed repository. Publishing means
  every clone pays for it forever, and a git object is not removed by deleting
  the file — it goes when history is rewritten, which `scrub-identity` is
  already doing once. **Removing the file here and letting that rewrite drop the
  object is the only way to do it without a second rewrite.**
- **The ignore list no longer matches the tree** after the component
  restructure, which is why the binary was committed at all. Correcting it after
  publication means the next contributor commits the next one.

`.gitattributes` is the third: line endings flipped under editing twice in one
week here. A public repository adds contributors on other platforms, which is
the condition that turns an occasional annoyance into recurring churn.

## D7. The CRD kind table stays in the README

Eleven kinds IS the product. A reader who cannot see the shape of the model
without following a link has not been told what this is.

The two-minute budget is met by cutting what the site says better — the
behaviours section, the expanded start — not by removing the one table that
answers the first question.
