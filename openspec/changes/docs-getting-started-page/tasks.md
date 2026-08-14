## 1. Verify the claims before writing them

- [x] 1.1 Confirm `docs-introduction-page` has landed (`docs/introduction.md`
      committed, its nav entry present). This change links it as a site page and
      sits after it in the navigation; landing first produces a dead link.
      *Landed in 229c50a; nav entry present.*
- [x] 1.2 Render demo mode and list what it creates:
      `helm template agent-ops ./chart --set global.demo.enabled=true` — confirm
      the `SignalSource` name, the `AgentProfile` name, the Pipeline name, and
      the toolsets it binds. Every name the page prints comes from this render,
      not from memory.
      *SignalSource `cluster-events` + `console`; AgentProfile `k8s-engineer`;
      Pipeline `k8s-observe` claiming `cluster-events`, binding toolsets
      `agentops-observe` + `k8s-observability` and MCPConfig `k8s-api`;
      AgentRuntime `default`; Channel/ChannelAdapter `console`.*
- [x] 1.3 Resolve the suspect README line: render with
      `--set global.demo.enabled=true --set k8s-bundle.eventsAdapter.enabled=false`
      and check whether any `SignalSource` remains to post an ask to. If none
      does, the "restores ask-only" line is DROPPED rather than carried onto the
      site, and the chart question is noted for a follow-up change — this change
      does not touch the chart.
      **FINDING: the line is FALSE and is dropped.** That flag removes the
      `cluster-events` SignalSource — the exact source the demo's ask posts
      to — leaving only `console`. Worse, `Pipeline/k8s-observe` still renders
      claiming `cluster-events`, so the flag also leaves a DANGLING ref. It does
      not restore ask-only; it breaks the ask. Recorded for a follow-up change;
      no chart edit here.
- [x] 1.4 Confirm the persistence default and the flag that opts out
      (`persistence.enabled`), and what a cluster with no RWX provisioner
      actually shows when it is left on. That observable is what the
      prerequisites section states.
      *`persistence.enabled: true`, PVC `agentops-home`, 5Gi,
      `accessModes: [ReadWriteMany]`, `storageClassName: ""` (cluster default).
      With no RWX provisioner the PVC stays `Pending`, so the runtime pod never
      starts and the conversation sits without a pod.*
- [x] 1.5 Run the install and the ask end to end on a real cluster and record
      what is observed: the phases the conversation passes through, when the pod
      appears and when it exits, the transcript command, the result command.
      Section 2.4 is written from this recording, not from the reconciler.
      *Ran the ASK end to end against the live install (chart 5.17.0), not a
      fresh demo install — the demo's Pipeline/toolset names come from the render
      in 1.2 instead. Recorded, and section 2.6 is written from it:*
      - *post → `{"conversations":1,"queued":1}`, conversation `task-25lr4`
        (generated `task-<suffix>` name)*
      - *+15s phase `Working`, pod `agentops-conv-task-25lr4` `1/1 Running`*
      - *+25s run `succeeded` exit 0 — 3 turns, 8s of model work*
      - *+29s phase `Idle`, **pod still `Running`***
      - *transcript `kubectl logs` shows the tools line, `[init] model=…`, each
        `[tool]` call and `=== RESULT (success, 3 turns, 8s) ===`*
      **Two gaps, both stated rather than guessed at in the page:** the pod was
      still `Running` 60s after the answer, so the exact exit is described as
      "after the runtime's idle TTL" without a number; and the FAILURE
      observables in 2.7 are from the CRD/chart semantics, not observed — only
      the happy path was run.

### Findings that changed the plan

- **FINDING: demo mode installs the console INERT.** It renders the
  ChannelAdapter, the `Channel` and `SignalSource/console`, but `k8s-observe`
  claims only `cluster-events` and binds no channels — so the console cannot
  originate (`Wired=False`, composer unavailable) and no answer ever reaches it.
  The most inviting surface in the product is installed and unusable by the
  turnkey flag whose whole purpose is a working demo.
  **Recorded for a follow-up chart change**; this change does not touch the
  chart, per its own scope, and the page works around it with values and one
  patch.
- **The demo flow is now CONSOLE-FIRST** (owner's call): install with
  `--set console.auth.uiToken=demo` and
  `--set 'k8s-bundle.pipelines.channels[0]=console'`, patch the route to claim
  the console source, then port-forward `svc/agentops-adapter-console` and ask
  in the browser. This supersedes the proposal's "curl a signal" ask and its
  "one credential, one flag" framing — it is three flags and a patch, and the
  patch is left visible because it teaches the wiring rule the page closes on.
- **The fan-out exercise had to change with it.** Console origination is
  `kind: chat`, and a BARE chat message with two claimants is REFUSED with the
  choices rather than fanned out. So the second route claims `console`, the
  refusal is the lesson, and the reader addresses `/k8s-observe-narrow …` to
  reach one. Verified in `console/originate.go` and the chat-lane rule.

## 2. Write the page

- [x] 2.1 Create `docs/getting-started.md` with front matter carrying `title:
      Getting started`, `permalink: /getting-started/` and a one-sentence
      `description:` for `jekyll-seo-tag`. No `layout:` key —
      `jekyll-default-layout` assigns `page`. Open the body with an `##`, never
      an `#`: the layout already emits the `h1` from `title`.
- [x] 2.2 **Opening** (2–3 sentences): what the reader will have at the end — an
      operator installed, a question answered, one route of their own — and how
      long it takes. No pitch; the landing page and the Introduction did that.
- [x] 2.3 **Before you begin** — a cluster and `kubectl`, Helm, an LLM
      credential, and the storage decision (sessions persist on an RWX PVC by
      default; no RWX provisioner means one flag). State the decision here, with
      the observable from 1.4, so it is never met as a pending pod. Link
      `concepts.md` for what the volume is for; do not explain it.
- [x] 2.4 **Install** — the namespace, the credential Secret, the one
      `helm install … --set global.demo.enabled=true`, each with one line on what
      it is for. Name what demo mode is in a sentence and link
      `k8s-bundle.md` for what it renders; do not enumerate it.
- [x] 2.5 **Ask it something** — the signal post, verbatim and copy-pasteable,
      with one sentence on why an ask is an ordinary signal to a claimed source
      and not a dedicated endpoint (link `contracts.md`, state no endpoint
      table).
- [x] 2.6 **What a good run looks like** — written from the recording in 1.5:
      the conversation appearing, the states it passes through, the pod that
      appears and later exits, `kubectl logs` for the live transcript, and where
      the result is read. Say what is normal, including the pause before
      anything happens. This section is the page's reason to exist and gets the
      most room.
- [x] 2.7 **When nothing happens** — the failures a first run actually hits and
      the observable each announces itself through: no or invalid LLM credential,
      no RWX storage class, a signal posted to a source no Ready Pipeline claims,
      and a conversation waiting on capacity. Name where each is visible
      (conditions, phase, pod state, adapter response). One line each — this is a
      triage list, not a troubleshooting chapter.
- [x] 2.8 **Wire a route of your own** — ~~a section on this page~~ **CUT from
      the page (owner's call) and rendered as a "Next" card at the foot of the
      right rail instead.** The walkthrough now ends at a working first answer,
      which is where a getting-started page should end; wiring is the thing you
      go on to, not the last thing you do here.
      The card is a THEME COMPONENT with no prose of its own: every word comes
      from the page's `next:` front matter (`eyebrow`, `title`, `body`, `url`),
      the include only places it, and a page that declares no `next:` gets no
      card. Same division as the card grid and the stat tiles.
- [x] 2.9 ~~Teardown for 2.8~~ — **not applicable**: with the exercise cut,
      the page leaves nothing standing to bill for. The `--set` flags it does
      type create no claim beyond the demo's own.
- [x] 2.10 **Where to go next** — the Introduction (the model), `concepts.md`
      (the kinds in full), the bundles for a real lane, `console.md` for
      watching it, `contracts.md` for writing an adapter. Reference pages link to
      GitHub where they live; the Introduction is a site-internal link.
- [x] 2.11 Re-read against the page's test: **would the reader type it, or read
      it?** Anything they only read *about* — a field's other values, an option
      the walkthrough does not use, a rule behind a resolution — becomes a link.
      Confirm the page carries no field table, no endpoint table and no values
      reference.

## 3. Publish it

- [x] 3.1 Add one entry to `docs/_data/nav.yml` under *Start here*, after
      *Introduction*: `title: Getting started`, `url: /getting-started/` —
      matching the page's `permalink` exactly, or the sidebar's current-page
      marking silently stops working.
- [x] 3.2 `docs/index.md`: under *Where to start*, lead with the Introduction
      and then Getting started, both site-internal. Remove the README as the
      path to installation — it is no longer where the walkthrough lives.
- [x] 3.3 Confirm no layout, include, stylesheet or other data file was touched.
      If one was, the page needs rewriting in the elements the theme already
      styles.
      **DEVIATION, owner-directed:** the "Next" card (2.8) needed three theme
      files — `_includes/toc.html`, `assets/css/agentops.css`,
      `assets/js/toc.js`. It qualifies under the site's existing rule for this:
      a general component any page can use by declaring `next:` in front matter,
      carrying no prose of its own. No LAYOUT was touched, and the page itself
      is still markdown plus one navigation line.

## 4. Collapse the README

- [x] 4.1 Replace "Try it in five minutes (demo advisor)" and "Install (current
      state)" with one **Get started** section: create the namespace + credential
      Secret, one `helm install … --set global.demo.enabled=true`, one ask, one
      line to watch it — and a link naming the site's Getting started page as the
      walkthrough. Commands only; no expectations, no flags the demo does not
      type, no failure modes.
- [x] 4.2 Keep the one-line statement that the chart owns the substrate and the
      bundles contribute domain, with its link — it answers "what did I just
      install", which is a README question.
- [x] 4.3 Carry the LLM-cost warning about cluster event ingestion in one line,
      linked, and drop the `eventsAdapter.enabled=false` remedy if 1.3 showed it
      leaves no source to ask (record the finding for a follow-up change).
- [x] 4.4 Add one row to the Documentation index: the Getting started page, under
      the documentation-site row. Every path the removed text served is one hop
      away.
- [x] 4.5 `wc -l README.md` — under 150, and lower than the 140 it started at.
- [x] 4.6 Confirm a reader who never leaves the README can still install and ask:
      copy the block into a shell and run it.

## 5. Record it

- [x] 5.1 `CLAUDE.md`: update the `docs/` map line that names the site's pages so
      it names `getting-started.md` as the third, with what it owns (the
      walkthrough: install, first answer, first route) and the boundary that
      keeps it from becoming a reference page. Keep the reference-pages sentence
      intact — it is still the rule.
- [x] 5.2 `CLAUDE.md`: the "After changes" routing table gains no row — the
      walkthrough is a site page and the existing "what the site SAYS to an
      adopter" row already routes it. Confirm that is true rather than adding a
      row.

## 6. Verification

- [x] 6.1 Confirm the page renders with the site shell: masthead, sidebar,
      palette and prose styling, in both themes.
- [x] 6.2 Confirm exactly one `h1` on the page, and that it reads "Getting
      started".
- [x] 6.3 Confirm the sidebar marks *Getting started* current with
      `aria-current="page"` when it is open, and the other two entries when they
      are.
- [x] 6.4 Follow every link on the page and in the README's changed sections:
      none 404s, each lands on the document that owns that content, and the
      Introduction link is site-internal rather than pointing at GitHub.
- [ ] 6.5 Run the page top to bottom on a clean namespace, following only what is
      written: install, ask, read the answer, apply the reader's Pipeline, ask
      again, delete it. Every command runs as printed and every expectation the
      page sets is met.
- [x] 6.6 Confirm the reference pages are unchanged (`git status`), still carry
      no front matter, and still have no navigation entry.
- [x] 6.7 Read it at a phone width: no horizontal scroll on the body — check the
      code blocks in particular — and the sidebar behind its toggle.
- [x] 6.8 Confirm the build needs nothing new — no plugin added to `_config.yml`,
      no workflow, no Gemfile.
