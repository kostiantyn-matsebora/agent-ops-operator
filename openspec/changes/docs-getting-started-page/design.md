## Context

The site has a landing page (what this is) and, from the in-flight
`docs-introduction-page` change, an Introduction (how it is put together).
Neither gets anyone to a running agent, and the only walkthrough that exists is
the README's — two sections holding a credential, a `helm install`, a curl, four
`kubectl` lines and a caveat, all inside a 150-line budget shared with the pitch,
the CRD table and the documentation index. There is no room in it for what a
first run *looks* like, what goes wrong first, or what to do once it answers.

Constraints this change inherits and does not renegotiate:

- **The theme holds no prose and the pages hold no theme.** Publishing a page is
  a markdown file plus one line in `_data/nav.yml`. Needing a layout edit is the
  signal the page has grown wrong, not a licence to edit the layout.
- **The build is GitHub Pages branch deploy.** No workflow, no Gemfile, no
  plugin outside the default set.
- **The reference pages under `docs/` are untreated.** No front matter, no
  navigation entry; they are linked where they live on GitHub.
- **`README.md` is capped at 150 lines** and holds no reference material. It is
  at 140 today.
- **A page declares its own `permalink`** — no permalink style is configured, and
  the sidebar marks the current entry by comparing `page.url` to `entry.url`.

## Goals / Non-Goals

**Goals:**

- A reader with a cluster and an LLM credential finishes the page with an agent
  installed, one question answered, and one Pipeline they wrote themselves.
- The page states what a *successful* run looks like, not only the commands that
  start one — the phases, the pod appearing and exiting, where the transcript
  is — so "nothing happened" becomes a diagnosable state rather than a shrug.
- The walkthrough has exactly one home. Every expectation, flag and failure mode
  is maintained on the site; the README keeps only what a GitHub visitor needs
  to start without leaving the file.
- The reader ends the page understanding, from something they typed, that
  capability comes from the routing and not from the agent.

**Non-Goals:**

- Not a second `concepts.md`. No field tables, no endpoint tables, no values
  reference. Where the page would have to explain a field, it links.
- Not the full adoption path. Persistence choices, credential variants, the
  Telegram and Prometheus lanes and the console each have a document that owns
  them; this page links to them from *where to go next* and does not absorb
  them.
- No chart, values or sample changes. The page installs what ships today.
- Not the change that publishes the reference pages onto the site.

## Decisions

### The page is a walkthrough, so it carries commands — and that is not a licence for reference prose

The Introduction's test ("would a sentence have to change if a CRD field were
renamed?") cannot be applied here unchanged: every command names a flag and the
one Pipeline names fields. The test for this page is different and narrower:

> **Would the reader type it, or read it?** What they type — a command, a flag,
> a manifest they apply — belongs on the page. What they only read *about* — the
> other values of a field, what else the kind can express, the rules behind the
> resolution — belongs in the document that owns it, behind a link.

Concretely: `--set global.demo.enabled=true` is on the page because they type it;
the list of what demo mode renders is a link to `k8s-bundle.md`. `toolsets:` in
the reader's own Pipeline is on the page because they write it; `mode: merge`
versus `overwrite` is a link to `concepts.md`.

Alternative considered — keeping the page command-free and linking the README for
every step. Rejected: it recreates the split this change exists to close, and a
walkthrough the reader must leave at every step is not a walkthrough.

### "Wire one route of your own" is a second Pipeline on the source the demo already installed

The reader has, after the demo: the `cluster-events` source, the `k8s-engineer`
profile, the observe toolsets, and one route claiming all of it. The cheapest
honest exercise on that inventory is a **second** Pipeline claiming the same
source with a deliberately narrower toolset — then one task signal, and two
conversations open, one able to see the cluster and one not.

That single step demonstrates three things the reader would otherwise take on
faith: sources are shareable and fan out; the same profile has two different
reaches; and the reach came from the route, because nothing about the profile
changed between them.

Alternatives considered:

- *Wire a chat surface.* Needs a bot token and a chat id — prerequisites the
  reader does not have on page one, and an unguessable value is where a
  getting-started page loses people.
- *Wire a new signal source.* Needs an adapter, which means enabling a bundle,
  which means the page becomes the bundle documentation.
- *Edit the demo's own Pipeline.* Teaches the same lesson with none of the
  fan-out, but leaves nothing to compare against — the reader sees one answer
  and has to trust the counterfactual.

**The step ends with teardown.** A route claiming the cluster-events source is a
standing LLM cost on a noisy cluster, so the exercise finishes by deleting the
Pipeline it created and saying why. A getting-started page that leaves a billing
surprise behind is a defect.

### README keeps a runnable minimum, not a bare link

The existing `documentation-structure` requirement demands the demo and install
commands be present "without following a link", and that is worth keeping: a
GitHub visitor evaluating the project should not need the site to try it. So the
collapse is not a deletion — README's **Get started** section keeps the
credential, the one `helm install`, one ask and the "watch it work" line, and
hands off everything else:

| Stays in README | Moves to the site |
|---|---|
| Credential secret, `helm install … --set global.demo.enabled=true` | What each command is for, and what to expect |
| One ask (the signal post) | The phases, the pod, the transcript, the result |
| One line naming what demo mode is, linked | Prerequisites and the storage flag decision |
| Link to Getting started as the walkthrough | Failure modes and how each announces itself |
| | Writing your own route |

The spec delta records this so the requirement and the file agree; the 150-line
budget is unchanged and gains headroom.

### Sequencing: this change follows `docs-introduction-page`

The page links to the Introduction as a *site* page and its navigation entry
sits directly after it. Landing this first would produce a link to a page that
does not exist and a nav entry pointing at a missing URL — which the site's own
rule calls a defect, not a placeholder. Both changes modify the same
"deliverables" requirement in `docs-site`, so this delta is written against the
Introduction's version of it, not against what is in `openspec/specs/` today.

### Every claim is verified against the chart before the page ships

The page states what will happen on a real cluster, which is the class of claim
a render test cannot check (a rendered pod is not a running one). Two specific
items:

- The **storage caveat**: agent sessions persist by default on an RWX PVC;
  clusters with no RWX provisioner need `--set persistence.enabled=false`. The
  page states this as a prerequisite decision rather than letting it surface as
  a pending pod.
- The README's current line that `--set k8s-bundle.eventsAdapter.enabled=false`
  "restores ask-only" is **suspect**: that flag gates the whole events component,
  including the `SignalSource` the ask is posted to, and a route with no source
  has nothing to claim. Implementation verifies it by render. If it does not
  hold, the line is **dropped rather than carried onto the site**, and the chart
  question is recorded for a follow-up change — this change fixes documentation,
  not chart behavior.

## Risks / Trade-offs

- **The page pins commands and flags that can change** → Every command is one an
  existing document already owns the detail of; the page is verified by running
  it at apply time, and the flags it types are the ones the README already typed,
  so the surface is not new.
- **The fan-out exercise doubles conversations per event while it exists** →
  The step is explicitly bounded and ends with `kubectl delete pipeline`, with
  the cost stated in the same breath rather than in a footnote.
- **Two documents still describe the same install** → Accepted and bounded by the
  table above: the README's copy is the commands only, with no expectations and
  no flags to drift. If a command changes, both change — that is one line each.
- **The demo spends LLM credits on a noisy cluster** → Already true of the
  README's demo and already stated there; the page inherits the warning and
  gives it the room to name grouping, cooldown and the cap as the bounds.
- **A reader arrives from a search engine mid-page** → Prerequisites are a
  section, not a preamble, so the storage decision is linkable and skippable
  rather than buried in prose.

## Migration Plan

Not applicable — documentation only. Nothing deploys, and the change is
reversible by reverting the commit. The one ordering constraint is the
dependency on `docs-introduction-page`.

## Open Questions

None blocking. The `eventsAdapter.enabled=false` claim is resolved by
verification during implementation, with the outcome (carry it or drop it)
already decided above.
