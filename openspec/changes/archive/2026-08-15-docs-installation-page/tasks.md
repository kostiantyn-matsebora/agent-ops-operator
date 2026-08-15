## 1. Verify before writing

- [x] 1.1 Read `chart/values.yaml` end to end and list every top-level key. The
      page's groups come from that list, not from memory — a key that exists and
      is in no group is a deliberate omission, not an oversight.
      *22 keys: `image`, `maxActiveConversations`, `maxQueuedConversations`,
      `maxRuntimes`, `runtimeIdleTtlMinutes`, `serviceAccounts`, `rbac`,
      `runtime`, `global`, `pipelines`, `service`, `adapterAuth`, `metrics`,
      `console`, `telegram-bundle`, `persistence`, `extraEnv`, `nodeSelector`,
      `resources`, `retention`, `housekeeping`, `crds`.*
      **Deliberate omissions:** `service` (ports), `metrics` (scrape config),
      `extraEnv`/`nodeSelector`/`resources` (manager placement),
      `serviceAccounts.manager`, `maxRuntimes` (deprecated alias) — none is a
      decision an operator makes before installing. `console` links to
      `console.md`, `telegram-bundle` to the bundle table, `pipelines` to the
      wiring section.
- [x] 1.2 Confirm each value the page will name, with `helm show values ./chart`:
      exact key path and exact default. Every default printed is copied from
      that output.
      *`maxActiveConversations: 5`, `maxQueuedConversations: 50`,
      `runtimeIdleTtlMinutes: 1`, `persistence.enabled: true`,
      `persistence.size: 5Gi`, `persistence.accessModes: [ReadWriteMany]`,
      `persistence.storageClassName: ""`,
      `global.agentops.runtime.rbacMode: ""`,
      `global.agentops.runtime.serviceAccountName: agentops-runtime`,
      `runtime.image: kmatsebora/agentops-runtime-claude:0.6.0`,
      `runtime.credentialsSecret.name: agentops-claude`,
      `image: kmatsebora/agentops-manager:0.33.0`,
      `adapterAuth.secretName: agentops-adapter-token`,
      `retention.autoclose.enabled: false` / `idleAge: 168h`,
      `retention.autodelete.enabled: false` / `closedAge: 720h`,
      `housekeeping.enabled: false`, `crds.enabled: true`, `crds.keep: true`.*
- [x] 1.3 Confirm the three bundles and their enable flags from
      `chart/Chart.yaml` — including that `k8s-bundle` carries **no** Helm
      condition, because it turns on via `enabled` OR `global.demo.enabled` and
      a condition evaluates only the first existing path.
      *All three default `enabled: false` in their own values.
      `prometheus-bundle` and `telegram-bundle` carry Helm conditions,
      `k8s-bundle` does not and self-gates in every template.*
- [x] 1.4 Confirm what an uninstall leaves behind: `crds.keep` and the
      `helm.sh/resource-policy: keep` annotations on CRDs and the session claim.
      State only what a render or the chart's own comments support.
      *`crds.enabled: true`, `crds.keep: true` — uninstall deletes neither the
      CRDs nor the CRs behind them, nor the session PVC.*
- [x] 1.5 Render a minimal non-demo install and confirm the claim the page ends
      on: with no Pipeline, a bundle's source is `Wired=False` and signals are
      dropped. `helm template` shows the objects, the concepts page owns the
      semantics.
      *A bare install renders `AgentRuntime/default`, `Channel/console` and
      `SignalSource/console` — and **no Pipeline at all**. So the default
      install has a source nothing claims, which is exactly the section's
      point.*

## 2. Write the page

- [x] 2.1 Create `docs/installation.md` with front matter: `title: Installation`,
      `permalink: /installation/`, a one-sentence `description:`, and a `next:`
      card. No `layout:` key. Open the body with an `##`, never an `#`.
- [x] 2.2 **Opening** (2 sentences): this is a real install, and Getting started
      is the demo. Link it.
- [x] 2.3 **Decide first** — storage, RBAC posture, CRD ownership. One short
      block each, stating the consequence rather than the mechanism, linking
      `concepts.md` for semantics. These come BEFORE the install because each is
      expensive to reverse.
- [x] 2.4 **Install** — numbered steps: namespace, credential Secret,
      `helm install`, verify. Each step is a bold action and its command.
- [x] 2.5 **Enable a bundle** — one table: bundle, flag, link to its page. Note
      the k8s bundle's missing Helm condition from 1.3. Name no bundle values.
- [x] 2.6 **Configure** — one subsection per decision group (capacity, storage,
      the agent's power, the runtime, access, housekeeping, lifecycle). Each is
      a small table of key + default + consequence. Defaults come from 1.2.
- [x] 2.7 Close the configure section by naming `helm show values ./chart` as
      the exhaustive list. Do not reproduce it.
- [x] 2.8 **Wire one route** — the smallest real Pipeline, as one YAML block,
      with the reason it is required: an unclaimed source drops every signal.
      Link `concepts.md` for the fields.
- [x] 2.9 **Upgrade and uninstall** — `CHANGELOG.md` owns migration steps, and
      what survives an uninstall from 1.4.
- [x] 2.10 Re-read against the adopter-documentation rules in `CLAUDE.md`:
      structure over prose, numbered steps for procedures, tables for mappings,
      short sentences, emphasis on the load-bearing phrase, **no semicolons**,
      no paragraph past about three lines.
- [x] 2.11 Confirm the page names no bundle value, no CRD field and no HTTP
      endpoint. Where it would have to, it links.

## 3. Publish it

- [x] 3.1 Add one entry to `docs/_data/nav.yml` under *Start here*, after
      *Getting started*, with `url` matching the page's `permalink` exactly.
- [x] 3.2 `docs/getting-started.md`: turn "a page for real installs comes later"
      into a link, and retarget its `next:` card at this page.
- [x] 3.3 `docs/index.md`: add Installation to the paths onward.
- [x] 3.4 Confirm no layout, include or stylesheet was touched.

## 4. Record it

- [x] 4.1 `README.md`: one row in the Documentation index. Check the 150-line
      budget still holds.
- [x] 4.2 `CLAUDE.md`: add the routing row — parent-chart values, installation,
      upgrade and uninstall go to `docs/installation.md` — and state the split
      against a subchart's values, since both are "values".
- [x] 4.3 `CLAUDE.md`: name `installation.md` in the `docs/` map line with what
      it owns.

## 5. Verification

- [x] 5.1 Build the site and confirm the page renders with the shell in both
      themes, exactly one `h1`, and the Next card at the foot of the rail.
- [x] 5.2 Confirm the sidebar marks *Installation* current when it is open.
- [x] 5.3 Follow every link: none 404s, site pages are internal links, reference
      pages point where they live.
- [x] 5.4 Grep the page for semicolons in prose — there must be none.
- [x] 5.5 Check every default printed on the page against
      `helm show values ./chart` one final time.
- [x] 5.6 Read it at a phone width: no horizontal scroll on the body, and the
      tables and YAML block scroll inside their own frames.
- [x] 5.7 Confirm the reference pages are unchanged and still carry no front
      matter.
- [ ] 5.8 Run the page on a cluster: install without demo mode, enable one
      bundle, apply the route, and confirm a signal is answered. This is the one
      claim a render cannot check.

## 6. Added during implementation (owner-directed)

Scope beyond the proposal, folded in rather than deferred because it changes
pages this change is already rewriting.

- [x] 6.1 **Tables: code never wraps.** `table-layout: auto` was breaking
      `persistence.storageClassName` mid-token, inventing a key. The stylesheet
      now forbids it and the column widens instead, with the table scrolling in
      its own frame below 60rem.
- [x] 6.2 **No squeezed columns.** Two long code values in a three-column table
      crushed the description to two words a line. The runtime group became a
      values snippet, the RBAC group two columns, and the bundle table lost its
      `--set ` prefix. Verified by measuring the last column of every table on
      every page.
- [x] 6.3 **The default is visible** in the rbacMode table — `none` is marked as
      the default rather than left to prose.
- [x] 6.4 **Platform tabs.** Every command block on the site is given for
      PowerShell and Linux/macOS. `assets/js/tabs.js` pairs two adjacent fences
      whose languages name the shell, so the PAGE carries no tab markup. The
      choice is page-wide, persisted, and taken from the browser initially.
      Multi-line commands carry real PowerShell continuations, not copies.
- [x] 6.5 **`docs/CLAUDE.md`** — the rules for writing pages in this directory,
      beside the files they govern: which pages are site pages, structure over
      prose, the command-tab rule, the component table, the table rules, and the
      pre-flight lint. Excluded from the Jekyll build, being a contributor file.
- [x] 6.6 Verify the tabs in a browser: 5 widgets per page, one panel visible
      each, switching one switches all, the choice persists, no overflow.
