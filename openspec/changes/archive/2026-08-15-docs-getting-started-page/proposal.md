## Why

The site can now tell an adopter what agent-ops *is* (the landing page) and how
it is *put together* (the Introduction). What it cannot do is get them to a
running agent. The only walkthrough that exists is the README's "Try it in five
minutes" — written for a GitHub visitor, wedged against a 150-line budget, and
therefore unable to say anything about what to expect, what to check when
nothing happens, or what to do once the demo answers.

That last part is the real gap. The demo ends with an agent that can read the
cluster and nothing wired to the adopter's own situation, and the next step —
declaring one Pipeline of your own — is currently spread across `concepts.md`,
the bundle pages and `config/samples/`. Nobody assembles that from three
reference documents on their first afternoon.

So: a Getting started page that runs the demo, shows what a first answer looks
like, and ends with the reader having written one route. And a README that stops
trying to be that page.

## What Changes

- **A new site page, `docs/getting-started.md`**, published at
  `/getting-started/`, written to be followed top to bottom in one sitting:
  - **before you begin** — what a cluster must already have (an LLM credential,
    a namespace, and the storage caveat that decides one flag), stated as
    prerequisites rather than discovered as a failure;
  - **install the demo** — the one-credential, one-flag install, verbatim
    copy-paste, with what each command is for;
  - **your first answer** — ask a question, watch the conversation, read the
    result, and *what it should look like*: the phases it passes through, the
    pod that appears and exits, where the transcript is. Plus the two or three
    things that actually go wrong first (no credential, no RWX storage class,
    a signal to a source no Pipeline claims) and how each announces itself;
  - **wire one route of your own** — a single Pipeline the reader writes,
    against the sources and profile the demo already installed, so the point
    lands that capability comes from wiring and not from the agent. One YAML
    block, applied and re-asked;
  - **where to go next** — the Introduction for the model, `concepts.md` for the
    fields, the bundles for a real lane, the console for watching it.
- **One navigation entry** in `docs/_data/nav.yml`, under *Start here*, after
  *Introduction*.
- **The landing page's paths onward** lead with Introduction then Getting
  started, and stop pointing at the README for installation.
- **README.md collapses its walkthrough into a pointer.** "Try it in five
  minutes" and "Install (current state)" become one short **Get started**
  section: the credential, the one `helm install`, one ask, and a link to the
  Getting started page as the walkthrough's home. A GitHub visitor can still
  copy-paste their way to a running install without leaving the file; the
  expectations, the failure modes, the flags and the first route live on the
  site, in one place, and are maintained there.

Explicitly NOT in this change:

- **No reference prose.** The page carries commands and exactly one YAML block —
  it is a walkthrough, and a walkthrough with no commands is a brochure — but it
  explains no CRD field, no HTTP endpoint and no values key beyond the flags it
  actually types. Where it would have to, it links.
- **No theme work.** A page is a markdown file plus one navigation line.
- **The reference pages stay untreated** — no front matter, no navigation entry,
  still linked where they live. Publishing them remains its own change.
- **No new chart behavior, values or samples.** The page installs what ships
  today; if a step needs a chart change to be true, the page is wrong, not the
  chart.

## Capabilities

### New Capabilities

None. The published site is one capability, and this page belongs to it.

### Modified Capabilities

- `docs-site`: the site's deliverables grow by the Getting started page, and the
  site gains a requirement stating what that page must get a reader to — an
  install, a first answer and one route of their own — and what it must not
  become (a second concepts reference). Builds on the in-flight
  `docs-introduction-page` delta, which is the version of the deliverables
  requirement this one modifies.
- `documentation-structure`: the README requirement that it carry "the
  five-minute demo, install" becomes a bounded **start** — a credential, an
  install command and one ask, with the walkthrough itself owned by the site.
  The rule that removed content must be one hop away is what makes this safe,
  and the 150-line budget is unchanged.

## Impact

- `docs/getting-started.md` (new), `docs/_data/nav.yml` (one entry),
  `docs/index.md` (paths onward).
- `README.md` — two sections replaced by one shorter one; the Documentation
  index gains the Getting started link.
- `CLAUDE.md` — the `docs/` map line naming the site's pages.
- No Go code, no chart, no CRDs. Nothing ships or deploys differently.
- **Sequencing:** depends on `docs-introduction-page` landing first — this page
  links to the Introduction as a site page and its navigation entry sits after
  it.
