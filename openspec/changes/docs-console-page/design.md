## Context

The site has four pages — Overview, Introduction, Getting started, Installation
— and one theme with three components a page may name: `{: .ao-cards}`,
`{: .ao-callout}`, and the front-matter-driven `next:` card. Nothing on the site
shows a product screenshot, and the shell has no component that could carry one.

`docs/console.md` is 636 lines of reference. It carries no front matter, so
Jekyll treats it as a static file and copies it verbatim — it is not a site page
and nothing links to it as one. It is also being edited right now by the
in-flight `console-unread-conversations` change.

Getting started now ends in the console and hands its Next card to Installation.
The reader's first sight of the product is the screen they were just told to
port-forward, and no page explains it.

Three constraints bound everything below:

- **GitHub Pages builds `/docs` from master with no plugins.** A feature needing
  one is implemented in the theme's own assets or dropped.
- **The theme holds no prose and the pages hold no theme.** A page may carry a
  kramdown attribute list and nothing else — no `<div>`, no inline style.
- **The palette is copied from the console's own stylesheet.** No new colour is
  invented here.

## Goals / Non-Goals

**Goals:**

- An adopter who finishes Getting started can see what the console does without
  installing it, and knows what each view is for.
- An operator can decide the console's authentication from the site, including
  putting their own authenticator in front of it.
- Screenshots regenerate from a command, so they cannot silently rot.
- The theme grows exactly one component, on the terms the existing two use.

**Non-Goals:**

- Documenting the console's endpoints, RBAC grant or full values list. That is
  `docs/console.md` and stays there.
- Changing any console behaviour, chart value or CRD.
- Publishing the reference pages onto the site. That is still a later change.
- A general screenshot pipeline for the other pages.

## Decisions

### 1. The adopter page is a NEW file, and the reference page is not renamed

`docs/console-guide.md` with `permalink: /console/`. `docs/console.md` keeps its
name, its content and its URL.

The obvious alternative — rename the reference to `console-reference.md` and let
the site page take `console.md` — buys filename tidiness for eight inbound link
edits and a merge conflict with uncommitted work in that exact file. Jekyll
resolves the two by itself: a file with front matter is a page at its declared
permalink (`/console/`), a file without is a static copy at its own path
(`/console.md`). They do not collide.

The cost is two documents whose names are one word apart. That is paid for in
`CLAUDE.md`, with a routing rule stating the split in one line: **what the
console is FOR is the guide, what it IS is the reference.**

When a later change publishes the reference pages onto the site, that page takes
a `/reference/…` permalink. The guide keeps `/console/`.

### 2. Tabs are a list the page writes, and a script the theme owns

The page writes an ordinary markdown list, named with an attribute list — the
same contract `{: .ao-cards}` already uses:

```markdown
{: .ao-tabs}
- **Overview** — every non-`True` condition across every kind, newest first.
  ![The console's Overview page …](/assets/img/console/overview-light.png)
- **Topology** — the install as a live graph …
  ![…](/assets/img/console/topology-light.png)
```

`assets/js/tabs.js` reads each `<li>`, takes the leading `<strong>` as the tab
label and the rest as the panel, and builds a `role="tablist"` with arrow-key
navigation. Deep links work: each tab gets an id from its label, and a
`#console-queues` in the URL opens that tab.

**Without JavaScript the page is a labelled list of screenshots** — every word
and every image still present, in order. That is why the label and the
description live in the markdown rather than in the script: the fallback is the
content, not a degraded copy of it.

Alternatives rejected:

- **A `_data` file plus an include.** Moves the prose out of the page, which the
  content/theme division forbids in both directions.
- **Front matter, like `stats:`.** Front matter places a component where the
  layout says, and the tour has to sit at a specific point in the body.
- **A Jekyll tabs plugin.** Pages enables none.

### 3. The theme swaps the screenshot variant, not the page

The page names ONE image per tab, always the `-light` path. `tabs.js` rewrites
the `src` to `-dark.png` when `<html>` resolves dark, and watches that attribute
so a theme toggle repaints the open tab.

The landing diagram does this differently — two `<img>` elements, hidden by CSS —
because it is placed by a layout from front matter, where markup is free. A body
component cannot use that trick without the page writing both images and naming
which is which, and "there are two themes" is theme knowledge in a page.

The trade is honest and small: **with JavaScript off, the screenshots are the
light variant on a dark page.** The tabs are already gone in that case. A reader
without JavaScript gets the whole content in one theme, which is a mismatch, not
a loss.

### 4. Screenshots are a build output, produced against a fixture

`console/ui/screenshots/` — a Playwright spec, run by `npm run screenshots`,
that:

1. serves the built `dist/` bundle over a local `node:http` server, exactly as
   `e2e/stream-chip.spec.ts` already does;
2. answers every `/api/*` call from **one curated fixture** — a small, plausible
   install with three pipelines, two runtimes, a handful of conversations and
   one live run;
3. drives each route, waits for the view to settle, and writes
   `docs/assets/img/console/<view>-{light,dark}.png` at a fixed viewport.

Why a fixture rather than a live cluster:

| | Fixture | Live capture |
|---|---|---|
| Reproducible | yes, byte-stable | no |
| Leaks real names | no | yes, and silently |
| Survives a UI change | re-run the command | someone must notice |
| Shows an interesting install | as designed | whatever the cluster had |

It reuses the existing harness technique, so it is not a new dependency —
Playwright is already a devDependency and already downloads a browser
deliberately.

**It is NOT part of `npm test`**, on the same grounds `test:e2e` is not: a suite
that cannot run offline is a suite people skip. The PNGs are committed, so the
site builds without ever running it.

Fixed viewport, deterministic content, no animation mid-capture: the tour is a
picture of the product, and a diff that moves every pixel on every run is a
picture of nothing.

### 5. Six views, chosen by what they answer

| Tab | The question it answers |
|---|---|
| Overview | is anything wrong right now? |
| Topology | what is moving between components? |
| Conversations | what has the fleet been asked, and what is unread? |
| Conversation | what did one agent actually do? |
| Queues | what is waiting, and what is stuck? |
| Configuration | what is wired to what? |

Six is the count the console has. The tab bar scrolls horizontally on a narrow
viewport rather than wrapping, so the strip stays one line and the reader can
tell there are more.

### 6. Authentication is a chapter, not a paragraph

The user asked for it as its own chapter, and it is the one console decision an
operator cannot defer. It has three parts, in this order:

1. **The default: a shared token.** Where it comes from, that an unconfigured
   token authorizes nobody, and that a redeploy does not sign anyone out.
2. **Your own authenticator.** The two values that must be set together, and
   the **exact six headers** the console trusts, in preference order:
   `X-Forwarded-Preferred-Username`, `X-Forwarded-Email`, `X-Forwarded-User`,
   `X-Auth-Request-Preferred-Username`, `X-Auth-Request-Email`,
   `X-Auth-Request-User`.
3. **What the proxy must do** — be the only route to the Service, **strip**
   client-supplied copies of all six, and forward one. Without an identity the
   console serves reads and refuses writes.

The header list is **verified against `console/auth.go`**, not copied from
memory, and the tasks pin that check. Publishing five of six on a page that
tells an operator what to allow-list is how a header gets left passing through
from the client.

The chapter states the decision and its consequence. `docs/console.md` keeps the
mechanism and the full values — the guide links to it rather than restating it.

### 7. The chain grows a link

`Getting started → The console → Installation`. Getting started's Next card
moves to `/console/`, the console guide's points at `/installation/`, and the nav
entry sits between them.

Note the two spellings the theme already requires: `nav.yml` entries are
site-root paths (`/console/`) because the sidebar compares them to `page.url`,
and a `next.url` is used raw in an `href` and therefore carries the baseurl
(`/agent-ops-operator/console/`). Getting started's existing card shows both.

## Risks / Trade-offs

- **The screenshots go stale when the console UI changes** → they regenerate
  from one command, and the tasks add the note to `CLAUDE.md`'s console entry so
  the next UI change knows to re-run it. Nothing in CI enforces it, which is a
  deliberate limit rather than an oversight.
- **PNG weight on a documentation page** → twelve images, one tab shown at a
  time. Panels other than the first carry `loading="lazy"`, so a reader who never
  opens Queues never fetches it.
- **`{: .ao-tabs}` is a third theme component to maintain** → it is the last one
  this page needs, and it reuses the card grid's contract exactly, so there is
  one pattern to learn rather than two.
- **Two documents named for the console** → mitigated by an explicit routing
  rule, not by hoping. Without the rule, the next writer picks one at random.
- **The authentication chapter and `docs/console.md` can drift** → the guide
  carries decisions and the reference carries mechanism, and the header list is
  the one fact stated in both. The tasks verify it against the source, which is
  the copy that cannot be wrong.

## Migration Plan

None. The change is additive to the site — one new page, one new component, one
new asset directory — plus three small edits to published pages. Rollback is
deleting the page and its nav entry and restoring Getting started's Next card.

## Open Questions

None. Screenshot source, theme variants and page order were settled before this
document was written.
