## 1. Verify before writing

- [x] 1.1 Read `console/auth.go` and copy `forwardAuthHeaders` verbatim, in
      order. That list is the page's, and it is copied from the source, never
      from `docs/console.md`. Record the six names here.
- [x] 1.2 Confirm from `console/auth.go` what admits a request in each mode
      (session cookie, bearer token, external declaration) and what identity is
      recorded in each. The page states consequences, so each one must be a fact
      the code supports.
- [x] 1.3 Confirm the console's routes from `console/ui/src/App.tsx` and settle
      the six tour views against them: Overview, Topology, Conversations,
      Conversation, Queues, Configuration. A view the app does not have is not a
      tab.
- [x] 1.4 Confirm the values the authentication section names, with
      `helm show values ./chart`: `console.auth.uiToken`,
      `console.auth.existingSecret`, `console.auth.enabled`,
      `console.auth.externalAuthenticator`, `console.write.enabled`. Exact key
      paths and exact defaults.
- [x] 1.5 Confirm `docs/console.md` is served verbatim by the build and carries
      no front matter, so a site page at `/console/` cannot collide with it.
      Check the built `_site` output, not the source directory.
- [x] 1.6 Read `docs/installation.md` and `docs/getting-started.md` end to end.
      The new page sits between them and must not restate either.

## 2. The tabbed component

- [x] 2.1 Add `docs/assets/js/tabs.js`: for each `ul.ao-tabs`, build a
      `role="tablist"` of buttons from each `li`'s leading `<strong>`, wrap the
      remainder as a `role="tabpanel"`, and wire `aria-selected`,
      `aria-controls`, `tabindex` roving and Left/Right/Home/End keys.
- [x] 2.2 Give each tab an id derived from its label, select the tab named by
      `location.hash` on load, and update the hash on selection without
      scrolling the page.
- [x] 2.3 In the same script, resolve the screenshot variant: rewrite each panel
      image's `src` from `-light.png` to `-dark.png` when
      `document.documentElement` resolves dark, and observe `data-theme` so a
      theme toggle repaints the open panel. Nothing in the page knows there are
      two themes.
- [x] 2.4 Mark every panel image but the first `loading="lazy"` from the script,
      so a reader who never opens a tab never fetches it.
- [x] 2.5 Add the component's rules to `docs/assets/css/agentops.css` under a
      new `--- tabbed panels ---` block, beside the card grid. Tokens only — a
      literal colour anywhere but the two palette blocks is a defect.
- [x] 2.6 The tab strip scrolls horizontally on a narrow viewport rather than
      wrapping, and the active tab scrolls into view when selected.
- [x] 2.7 `docs/_layouts/default.html`: load `tabs.js` deferred, gated on a
      front-matter flag, exactly as `toc.js` is gated on `page.toc`.
- [x] 2.8 Confirm the no-script behaviour by disabling JavaScript: every panel
      visible, in order, each under its label, nothing hidden. This is the
      content, not a fallback.
- [x] 2.9 Run `grep -n '#[0-9a-fA-F]\{3,6\}' docs/assets/css/agentops.css` and
      confirm hits only inside the two palette blocks.

## 3. The screenshots

- [x] 3.1 Create `console/ui/screenshots/fixture.ts`: one curated install —
      invented names only, no cluster, namespace, host or identity from a real
      one. Three pipelines, two runtimes, a handful of conversations with a mix
      of phases, one live run, one non-`True` condition worth seeing on Overview.
- [x] 3.2 Create `console/ui/screenshots/capture.spec.ts` on the harness
      technique in `e2e/stream-chip.spec.ts`: serve `dist/` from a local
      `node:http` server and answer every `/api/*` route from the fixture.
- [x] 3.3 Drive each of the six views, wait for the view to settle (no
      in-flight query, no running animation), and write
      `docs/assets/img/console/<view>-light.png` and `-dark.png` at one fixed
      viewport.
- [x] 3.4 Add `"screenshots"` to `console/ui/package.json` scripts. It is NOT
      wired into `npm test` — the same reason `test:e2e` is not.
- [x] 3.5 Run it twice and confirm the PNGs are byte-identical between runs. A
      capture that differs every run makes every future diff unreadable.
- [x] 3.6 Open each PNG and confirm it shows something worth showing: no empty
      state, no spinner, no truncated table, no invented name that reads like a
      real one.
- [x] 3.7 Commit the twelve PNGs. The site build never runs the capture.

## 4. Write the page

- [x] 4.1 Create `docs/console-guide.md` with front matter: `title: The console`,
      `permalink: /console/`, a one-sentence `description:`, the tabs flag from
      2.7, and a `next:` card pointing at Installation. No `layout:` key. Open
      the body with an `##`, never an `#`.
- [x] 4.2 **What it is** — a few sentences: a browser view of the whole install,
      that is also a channel you can reply from and also a source you can start a
      conversation on. Ships enabled by default, one value turns it off.
- [x] 4.3 **The tour** — the `{: .ao-tabs}` list, six items, each a bold label,
      an em-dashed one-line statement of the question that view answers, and the
      screenshot. Alt text describes what the screenshot SHOWS, not that it is a
      screenshot.
- [x] 4.4 **What it does for you** — an `{: .ao-cards}` set, the same component
      the other pages use. Peer capabilities, not a feature list.
- [x] 4.5 **Authentication** — its own `##` section, three subsections in the
      design's order: the shipped token mode, declaring your own authenticator,
      and what that authenticator must do.
- [x] 4.6 In the token subsection: where the token comes from, that an
      unconfigured token authorizes nobody and is indistinguishable from a wrong
      one, and that a redeploy does not sign anyone out. Values from 1.4.
- [x] 4.7 In the own-authenticator subsection: the two values that must be set
      together, one YAML block, and **all six headers from 1.1 as a table** in
      preference order. State that the proxy must set every one and that a
      client-supplied copy must never reach the console.
- [x] 4.8 In the requirements subsection: the proxy must be the only route to
      the Service, must strip all six client-supplied headers, and should forward
      one — and that without an identity the console serves reads and refuses
      writes. Link `docs/console.md` for the ingress annotations rather than
      copying them.
- [x] 4.9 **What it cannot do** — one short block: no write path to the
      Kubernetes API exists in the module, and its Role carries no write verb.
- [x] 4.10 Re-read against the adopter-documentation rules in `CLAUDE.md`:
      structure over prose, tables for mappings, bullets for sets, short
      sentences, emphasis on the load-bearing phrase, **no semicolons**, no
      paragraph past about three lines.
- [x] 4.11 Confirm the page carries no HTTP endpoint, no RBAC table and no
      values reference beyond the keys a decision needs. Where it would, it links
      `docs/console.md`.

## 5. Publish it

- [x] 5.1 Add one entry to `docs/_data/nav.yml` under *Start here*, between
      *Getting started* and *Installation*, `url` matching the permalink exactly.
- [x] 5.2 `docs/getting-started.md`: retarget its `next:` card at the console
      page. Note the two spellings — a nav `url` is a site-root path, a
      `next.url` is used raw in an `href` and carries the baseurl.
- [x] 5.3 `docs/getting-started.md`: drop the console entry from its closing
      "where to go next" list, which now duplicates the card.
- [x] 5.4 `docs/index.md`: add the console page to the paths onward, and point
      its existing console line at the site page rather than the raw reference.
- [x] 5.5 Confirm the what-next chain now reads landing → Introduction → Getting
      started → The console → Installation, with no card skipping an entry.
- [x] 5.6 Confirm `docs/console.md` is byte-identical to before this change.

## 6. Record it

- [x] 6.1 `README.md`: one row in the Documentation index for the console guide,
      distinguished in its line from `docs/console.md`. Check the 150-line budget
      still holds.
- [x] 6.2 `CLAUDE.md`: replace the single console routing row with the two-row
      split — what the console is FOR to `docs/console-guide.md`, what it IS to
      `docs/console.md`.
- [x] 6.3 `CLAUDE.md`: add the row for site screenshots — a change to the
      console's UI is not done until `npm run screenshots` has been re-run.
- [x] 6.4 `CLAUDE.md`: name `console-guide.md` in the `docs/` map line with what
      it owns, and name `assets/img/console/` as generated output.
- [x] 6.5 `CLAUDE.md`: add `{: .ao-tabs}` to the list of components a page may
      name, beside `{: .ao-cards}` and `{: .ao-callout}`, with its no-script
      rule.

## 7. Verification

- [x] 7.1 Build the site and confirm the page renders with the shell in both
      themes, exactly one `h1`, and the Next card at the foot of the rail.
- [x] 7.2 Confirm the sidebar marks *The console* current when it is open.
- [x] 7.3 Drive the tabs in a browser: every tab selects, arrow keys move,
      `aria-selected` follows, and each panel's screenshot is the right one.
- [x] 7.4 Toggle the site theme with a tab open and confirm the screenshot
      swaps variant without a reload.
- [x] 7.5 Open the page at `#`-a-tab-name and confirm that tab is selected on
      arrival.
- [x] 7.6 Disable JavaScript and confirm the whole tour is readable as a list.
- [x] 7.7 Diff the six headers on the page against `console/auth.go` one final
      time — same names, same order, none missing.
- [x] 7.8 Follow every link: none 404s, site pages are internal links, the
      reference page points where it lives.
- [x] 7.9 Grep the page for semicolons in prose — there must be none.
- [x] 7.10 Read it at a phone width: no horizontal scroll on the body, the tab
      strip scrolls inside itself, and the screenshots fit their column.
- [x] 7.11 Confirm the reference pages are unchanged and still carry no front
      matter, and that `/console/` and `/console.md` both resolve in the built
      site.
- [x] 7.12 Run the authentication section against a cluster: expose the console
      behind a forward-auth proxy configured only from what the page says, and
      confirm a write is attributed to the signed-in person. This is the one
      claim a render cannot check.

## Record

**1.1 The six headers**, copied from `console/auth.go` (`forwardAuthHeaders`), in
order: `X-Forwarded-Preferred-Username`, `X-Forwarded-Email`,
`X-Forwarded-User`, `X-Auth-Request-Preferred-Username`,
`X-Auth-Request-Email`, `X-Auth-Request-User`. Diffed against the source again
at 7.7 — same names, same order.

**1.4 Values**, from `helm show values ./chart`: `console.enabled: true`,
`console.write.enabled: true`, `console.auth.enabled: true`,
`console.auth.uiToken: ""`, `console.auth.existingSecret: ""`,
`console.auth.externalAuthenticator: ""`. Token Secret
`agentops-console-<console.name>`, key `uiToken`, generated on install only and
carrying `helm.sh/resource-policy: keep`.

## Deviations from the plan

- **2.1 / 2.7 — `docs/assets/js/tabs.js` already existed.** The Installation
  page landed platform tabs in it after this change was written, and that script
  already owns the `.ao-tabs` / `.ao-tablist` / `.ao-tab` / `.ao-tabpanel`
  classes. The panel component was therefore ADDED to that file as a second
  block rather than created, and it builds the same markup so the two tab
  widgets look alike. The front-matter gate in 2.7 was dropped: the file is
  already loaded on every page for the command tabs, and gating it would break
  them. The script does nothing on a page with no `ul.ao-tabs`.
- **3.3 — fixed WIDTH, measured height.** The console scrolls inside its main
  region, so one fixed viewport either crops a view or pads it: at 1920x1400 the
  Overview's Problems table was only just in frame while Queues carried 700px of
  empty background. The capture fixes the width at 1920 (the layout decision)
  and sizes the height to the view, bounded 420..1600. Reproducibility is
  unaffected, since the fixture pins the content.
- **3.3 — Configuration is captured as the Pipelines inventory** (`/config/
  pipelines`), not the kind index. The tab asks "what is wired to what", and a
  grid of object counts does not answer it.
- **3.7 — the PNGs are written and untracked-but-not-ignored.** No commit was
  made: committing is the repository owner's call.
- **5.5 — `introduction.md` had no `next:` card**, so the chain broke between
  Introduction and Getting started. One was added, pointing at Getting started.
  The landing page carries none by construction: `_layouts/home.html` renders no
  on-this-page rail, and its "Where to start" list is the onward path.

## 7.12, against the live cluster

Both modes were run on the home cluster. The console was temporarily switched to
token mode, then restored to its oauth2-proxy gate. Every claim the page makes:

| The page says | Observed |
|---|---|
| Token in Secret `agentops-console-<name>`, key `uiToken` | present, 40 chars, `resource-policy: keep` |
| A redeploy does not sign anyone out | the upgrade rendered no Secret, and the same token still authenticated |
| No credential serves nothing | `GET /api/overview` -> 401 |
| A wrong token is refused | `POST /api/login` -> 401 `invalid token` |
| A token proves a credential, not a person | session `identity: token` |
| Headers in preference order | both set, `X-Forwarded-Preferred-Username` won |
| No identity: reads served | `GET /api/overview` -> 200 |
| No identity: writes refused, no name invented | `POST .../reopen` -> 403, naming the proxy |
| An identity: the write is logged against it | `console write: action=reopen identity=dana` |
| The proxy is the only route | unauthenticated request to the host -> 302 to `/oauth2/start` |

Write refusal was checked on a real write route against a conversation that does
not exist, so the guard was reached before any side effect. `/conversations/read`
is NOT one — it sits behind `auth`, not `write`, because a read watermark is not
a write to the cluster.

The `_gitops` change was reverted and re-applied. That repo is back to
its own uncommitted work and nothing of this remains.
