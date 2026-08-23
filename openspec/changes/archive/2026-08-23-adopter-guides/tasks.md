# Tasks — adopter guides

## 1. Generation

- [x] 1.1 Write the template generator: read `chart/files/crds/`, emit a minimal
      resource per kind (required fields plus the ones a guide teaches) and the
      full field reference. Field descriptions come from the CRD, which is where
      the Go doc comments already land — so the comments in a template cannot
      disagree with the type
- [x] 1.2 Write the example renderer: `helm template` a bundle with its own
      values, extract the custom resources it produces, and emit them per tier.
      No invented values — see `design.md` D4
- [x] 1.3 Emit `docs/cr-reference.md`: every field of every kind, no front
      matter, no nav entry. It is a reference file beside `concepts.md`, not a
      site page
- [x] 1.4 One command regenerates everything, and it is named in
      `.claude/rules/` beside the screenshots and the recording

## 2. The drift check

- [x] 2.1 CI regenerates both and fails on a non-empty diff. Without this the
      generation buys nothing — see `design.md` D2
- [x] 2.2 Prove it fails: rename a CRD field on a scratch branch and confirm the
      job reports which file is stale
- [x] 2.3 The failure message says which command to run. A drift check that
      reports a diff without the fix is a puzzle

## 3. Tier 1 — an agent of your own

- [x] 3.0 The Pipeline is its OWN page, and the FIRST one. It creates nothing:
      every object it names a demo install already ships, so the fundamental
      lesson costs the reader no new resources. Teaching the profile first makes
      an inert object whose purpose is a Pipeline they have not met
- [x] 3.1 `AgentProfile` with an inline `systemPrompt` and no repository, on its
      own page, linking back to the Pipeline guide for the route that reaches it
- [x] 3.2 State what a wrong wiring costs: a source no Ready Pipeline lists
      DROPS its signals, and an unaddressed chat message on a surface several
      Pipelines serve is answered with the list rather than by an agent
- [x] 3.3 The advanced page: `repository`, an agent definition under
      `.claude/agents/`, and the deploy key. Name the key's format trap — LF
      only, trailing newline — because it fails as a crypto error rather than as
      a bad key
- [x] 3.4 Both pages link `concepts.md` for the fields they do not use

## 4. Tier 2 — capabilities

- [x] 4.1 `MCPToolset` and `MCPConfig`, bound from the Pipeline and nowhere
      else. A profile carries no capabilities, and mistaking the profile for the
      counterpart has already deleted a field once
- [x] 4.2 How `toolsMode` composes against the AGENT DEFINITION's own `tools:`,
      not against the profile
- [x] 4.3 State the cost plainly, and no smaller than tier 3's: this tier is
      pure YAML and grants more than an adapter's code ever could
- [x] 4.4 Example rendered from ha-bundle, which splits two toolsets by
      privilege — the split IS the lesson

## 5. Tier 3 — adapters

- [x] 5.1 Signal adapter: `/signal/inbound`, the `SignalAdapter` CR, and what
      the manager does rather than the adapter — grouping, cooldown and
      recurrence are manager-side
- [x] 5.2 State the loop hazard first: an observing adapter that emits about
      agent-ops' own machinery creates a cycle nothing downstream catches, and
      `signals/k8s-events` implements three independent breakers because of it
- [x] 5.3 Channel adapter: `/channel/*`, the ops long-poll, and that an op
      carries a TYPED message rather than rendered text — the adapter composes
      presentation, the manager composes meaning
- [x] 5.4 State the relay hazard: an implementation that re-ingests its own
      outbound posts loops rather than merely duplicating
- [x] 5.5 Each page points at its reference implementation as the thing to copy

## 6. Tier 4 — runtimes

- [x] 6.1 Open with what a runtime is trusted with: `--allowedTools` is the sole
      permission authority and the runtime applies it, so one that ignores it
      voids every toolset binding in the install and nothing detects that
- [x] 6.2 The work contract: long-poll `/work`, report `/work/done`, and the
      context handle the manager stores and never interprets
- [x] 6.3 Continuity: what `contextStorage` promises, and that a promised-and-
      lost context FAILS the run rather than answering fresh
- [x] 6.4 Point at `runtimes/claude` as the implementation, and at
      `egress-proxy` as what exists because this trust is real

## 7. Wiring it up

- [x] 7.1 Replace the Introduction's "Guides are being written. There are none
      yet." with the list. That sentence is the only thing the site currently
      says about guides
- [x] 7.2 One nav line per published guide
- [x] 7.3 Getting started's "Where to go next" leads with tier 1 rather than
      with reference
- [x] 7.4 Every link resolves, including the GitHub links to `cr-reference.md`
      and the contracts

## 8. Verification

- [x] 8.1 Follow tier 1 end to end on a live cluster from the demo install, with
      nothing but the page open. A guide that needs the reference open is not
      finished
- [x] 8.2 The generated templates apply: `kubectl apply --dry-run=server` over
      each minimal resource
- [x] 8.3 Run the generators after `scrub-identity` lands and confirm the
      examples carry the chart's placeholder rather than anything real
- [x] 8.4 The page lint the site already applies passes on all seven new pages
- [x] 8.5 Every page follows the five-part shape — what it IS, Before you start,
      The overall shape, task-named sections, What comes next. Two earlier drafts
      failed it in opposite directions, see `design.md` D8

## 9. Documentation

Both halves, listed separately because they are skipped independently.

### 9.1 Reference docs

- [x] 9.1.1 `docs/cr-reference.md` — every field of every kind, generated by
      `python3 .github/scripts/docs-generate.py`. No front matter and no nav
      entry: a reference file beside `concepts.md`, not a site page
- [x] 9.1.2 `docs/CHANGELOG.md` — the seven guides, `cr-reference.md` and the CI
      drift check, under `[Unreleased] → Added`. It is not a breaking change and
      needs no `### Upgrade` block
- [x] 9.1.3 `.claude/rules/documentation.md` names the generator beside the
      screenshots and the recording, so the next reader learns that a template,
      an example and the reference are BUILD OUTPUT and a hand-edit is reverted
      by the next run

### 9.2 The adopter site

- [x] 9.2.1 Seven pages under `docs/guides/`, one per tier, each following the
      five-part shape
- [x] 9.2.2 `docs/_data/nav.yml` — one Guides entry per published guide, in
      learning order, the Pipeline first
- [x] 9.2.3 `docs/introduction.md` — the kind cards each link their guide, and
      "Follow the guides" replaces "Guides are being written. There are none
      yet."
- [x] 9.2.4 `docs/getting-started.md` — "Where to go next" leads with the
      Pipeline guide rather than with reference
- [x] 9.2.5 `README.md` — the Documentation index rows for the guides and for
      `docs/cr-reference.md`, still inside the 150-line budget
- [x] 9.2.6 Every link resolves, including the GitHub links out to
      `cr-reference.md` and the contracts
