## 1. Verify before writing

- [ ] 1.1 Read `console/auth.go` and copy `forwardAuthHeaders` verbatim, in
      order. That list is the page's, and it is copied from the source, never
      from `docs/console.md`. Record the six names here.
- [ ] 1.2 Confirm from `console/auth.go` what admits a request in each mode
      (session cookie, bearer token, external declaration) and what identity is
      recorded in each. The page states consequences, so each one must be a fact
      the code supports.
- [ ] 1.3 Confirm the console's routes from `console/ui/src/App.tsx` and settle
      the six tour views against them: Overview, Topology, Conversations,
      Conversation, Queues, Configuration. A view the app does not have is not a
      tab.
- [ ] 1.4 Confirm the values the authentication section names, with
      `helm show values ./chart`: `console.auth.uiToken`,
      `console.auth.existingSecret`, `console.auth.enabled`,
      `console.auth.externalAuthenticator`, `console.write.enabled`. Exact key
      paths and exact defaults.
- [ ] 1.5 Confirm `docs/console.md` is served verbatim by the build and carries
      no front matter, so a site page at `/console/` cannot collide with it.
      Check the built `_site` output, not the source directory.
- [ ] 1.6 Read `docs/installation.md` and `docs/getting-started.md` end to end.
      The new page sits between them and must not restate either.

## 2. The tabbed component

- [ ] 2.1 Add `docs/assets/js/tabs.js`: for each `ul.ao-tabs`, build a
      `role="tablist"` of buttons from each `li`'s leading `<strong>`, wrap the
      remainder as a `role="tabpanel"`, and wire `aria-selected`,
      `aria-controls`, `tabindex` roving and Left/Right/Home/End keys.
- [ ] 2.2 Give each tab an id derived from its label, select the tab named by
      `location.hash` on load, and update the hash on selection without
      scrolling the page.
- [ ] 2.3 In the same script, resolve the screenshot variant: rewrite each panel
      image's `src` from `-light.png` to `-dark.png` when
      `document.documentElement` resolves dark, and observe `data-theme` so a
      theme toggle repaints the open panel. Nothing in the page knows there are
      two themes.
- [ ] 2.4 Mark every panel image but the first `loading="lazy"` from the script,
      so a reader who never opens a tab never fetches it.
- [ ] 2.5 Add the component's rules to `docs/assets/css/agentops.css` under a
      new `--- tabbed panels ---` block, beside the card grid. Tokens only — a
      literal colour anywhere but the two palette blocks is a defect.
- [ ] 2.6 The tab strip scrolls horizontally on a narrow viewport rather than
      wrapping, and the active tab scrolls into view when selected.
- [ ] 2.7 `docs/_layouts/default.html`: load `tabs.js` deferred, gated on a
      front-matter flag, exactly as `toc.js` is gated on `page.toc`.
- [ ] 2.8 Confirm the no-script behaviour by disabling JavaScript: every panel
      visible, in order, each under its label, nothing hidden. This is the
      content, not a fallback.
- [ ] 2.9 Run `grep -n '#[0-9a-fA-F]\{3,6\}' docs/assets/css/agentops.css` and
      confirm hits only inside the two palette blocks.

## 3. The screenshots

- [ ] 3.1 Create `console/ui/screenshots/fixture.ts`: one curated install —
      invented names only, no cluster, namespace, host or identity from a real
      one. Three pipelines, two runtimes, a handful of conversations with a mix
      of phases, one live run, one non-`True` condition worth seeing on Overview.
- [ ] 3.2 Create `console/ui/screenshots/capture.spec.ts` on the harness
      technique in `e2e/stream-chip.spec.ts`: serve `dist/` from a local
      `node:http` server and answer every `/api/*` route from the fixture.
- [ ] 3.3 Drive each of the six views, wait for the view to settle (no
      in-flight query, no running animation), and write
      `docs/assets/img/console/<view>-light.png` and `-dark.png` at one fixed
      viewport.
- [ ] 3.4 Add `"screenshots"` to `console/ui/package.json` scripts. It is NOT
      wired into `npm test` — the same reason `test:e2e` is not.
- [ ] 3.5 Run it twice and confirm the PNGs are byte-identical between runs. A
      capture that differs every run makes every future diff unreadable.
- [ ] 3.6 Open each PNG and confirm it shows something worth showing: no empty
      state, no spinner, no truncated table, no invented name that reads like a
      real one.
- [ ] 3.7 Commit the twelve PNGs. The site build never runs the capture.

## 4. Write the page

- [ ] 4.1 Create `docs/console-guide.md` with front matter: `title: The console`,
      `permalink: /console/`, a one-sentence `description:`, the tabs flag from
      2.7, and a `next:` card pointing at Installation. No `layout:` key. Open
      the body with an `##`, never an `#`.
- [ ] 4.2 **What it is** — a few sentences: a browser view of the whole install,
      that is also a channel you can reply from and also a source you can start a
      conversation on. Ships enabled by default, one value turns it off.
- [ ] 4.3 **The tour** — the `{: .ao-tabs}` list, six items, each a bold label,
      an em-dashed one-line statement of the question that view answers, and the
      screenshot. Alt text describes what the screenshot SHOWS, not that it is a
      screenshot.
- [ ] 4.4 **What it does for you** — an `{: .ao-cards}` set, the same component
      the other pages use. Peer capabilities, not a feature list.
- [ ] 4.5 **Authentication** — its own `##` section, three subsections in the
      design's order: the shipped token mode, declaring your own authenticator,
      and what that authenticator must do.
- [ ] 4.6 In the token subsection: where the token comes from, that an
      unconfigured token authorizes nobody and is indistinguishable from a wrong
      one, and that a redeploy does not sign anyone out. Values from 1.4.
- [ ] 4.7 In the own-authenticator subsection: the two values that must be set
      together, one YAML block, and **all six headers from 1.1 as a table** in
      preference order. State that the proxy must set every one and that a
      client-supplied copy must never reach the console.
- [ ] 4.8 In the requirements subsection: the proxy must be the only route to
      the Service, must strip all six client-supplied headers, and should forward
      one — and that without an identity the console serves reads and refuses
      writes. Link `docs/console.md` for the ingress annotations rather than
      copying them.
- [ ] 4.9 **What it cannot do** — one short block: no write path to the
      Kubernetes API exists in the module, and its Role carries no write verb.
- [ ] 4.10 Re-read against the adopter-documentation rules in `CLAUDE.md`:
      structure over prose, tables for mappings, bullets for sets, short
      sentences, emphasis on the load-bearing phrase, **no semicolons**, no
      paragraph past about three lines.
- [ ] 4.11 Confirm the page carries no HTTP endpoint, no RBAC table and no
      values reference beyond the keys a decision needs. Where it would, it links
      `docs/console.md`.

## 5. Publish it

- [ ] 5.1 Add one entry to `docs/_data/nav.yml` under *Start here*, between
      *Getting started* and *Installation*, `url` matching the permalink exactly.
- [ ] 5.2 `docs/getting-started.md`: retarget its `next:` card at the console
      page. Note the two spellings — a nav `url` is a site-root path, a
      `next.url` is used raw in an `href` and carries the baseurl.
- [ ] 5.3 `docs/getting-started.md`: drop the console entry from its closing
      "where to go next" list, which now duplicates the card.
- [ ] 5.4 `docs/index.md`: add the console page to the paths onward, and point
      its existing console line at the site page rather than the raw reference.
- [ ] 5.5 Confirm the what-next chain now reads landing → Introduction → Getting
      started → The console → Installation, with no card skipping an entry.
- [ ] 5.6 Confirm `docs/console.md` is byte-identical to before this change.

## 6. Record it

- [ ] 6.1 `README.md`: one row in the Documentation index for the console guide,
      distinguished in its line from `docs/console.md`. Check the 150-line budget
      still holds.
- [ ] 6.2 `CLAUDE.md`: replace the single console routing row with the two-row
      split — what the console is FOR to `docs/console-guide.md`, what it IS to
      `docs/console.md`.
- [ ] 6.3 `CLAUDE.md`: add the row for site screenshots — a change to the
      console's UI is not done until `npm run screenshots` has been re-run.
- [ ] 6.4 `CLAUDE.md`: name `console-guide.md` in the `docs/` map line with what
      it owns, and name `assets/img/console/` as generated output.
- [ ] 6.5 `CLAUDE.md`: add `{: .ao-tabs}` to the list of components a page may
      name, beside `{: .ao-cards}` and `{: .ao-callout}`, with its no-script
      rule.

## 7. Verification

- [ ] 7.1 Build the site and confirm the page renders with the shell in both
      themes, exactly one `h1`, and the Next card at the foot of the rail.
- [ ] 7.2 Confirm the sidebar marks *The console* current when it is open.
- [ ] 7.3 Drive the tabs in a browser: every tab selects, arrow keys move,
      `aria-selected` follows, and each panel's screenshot is the right one.
- [ ] 7.4 Toggle the site theme with a tab open and confirm the screenshot
      swaps variant without a reload.
- [ ] 7.5 Open the page at `#`-a-tab-name and confirm that tab is selected on
      arrival.
- [ ] 7.6 Disable JavaScript and confirm the whole tour is readable as a list.
- [ ] 7.7 Diff the six headers on the page against `console/auth.go` one final
      time — same names, same order, none missing.
- [ ] 7.8 Follow every link: none 404s, site pages are internal links, the
      reference page points where it lives.
- [ ] 7.9 Grep the page for semicolons in prose — there must be none.
- [ ] 7.10 Read it at a phone width: no horizontal scroll on the body, the tab
      strip scrolls inside itself, and the screenshots fit their column.
- [ ] 7.11 Confirm the reference pages are unchanged and still carry no front
      matter, and that `/console/` and `/console.md` both resolve in the built
      site.
- [ ] 7.12 Run the authentication section against a cluster: expose the console
      behind a forward-auth proxy configured only from what the page says, and
      confirm a write is attributed to the signed-in person. This is the one
      claim a render cannot check.
