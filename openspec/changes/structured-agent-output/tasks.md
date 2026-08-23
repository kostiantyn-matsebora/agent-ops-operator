> **REVISED 2026-08-23, mid-implementation.** The manager parsed and shipped
> `blocks[]`; it now passes the agent's text through and each adapter parses.
> See design D1. Sections below marked **[was manager-side]** are rework, not
> new work — the parser exists and moves.

## 1. Grammar and parser

- [x] 1.1 Block model: an ordered list of blocks, each with a role
      (`title` | `section` | `details`), a label for named sections, and text in
      the existing markdown subset.
- [x] 1.2 Recognize a tag only when alone on its own line at line start,
      well-formed as an open/close pair, and outside fenced and inline code.
      Everything else is literal text.
- [x] 1.3 Unclosed region closes at end of output; no content is discarded.
- [x] 1.4 Input with no recognized tag yields exactly one above-the-fold block
      holding the whole text.
- [x] 1.5 Normalize: `<title>` first regardless of where it appeared, at most
      one, single line; named sections keep written order.
- [x] 1.6 **CAP DELETED.** A length budget cut a markdown table from its header
      and buried `<fix>` because it was last. There is no value that makes
      trimming markdown safe — see design D4. A test now fails if one returns.
- [x] 1.7 Flatten blocks back to the markdown subset — kept for a renderer that
      wants the whole thing as one string.
- [x] 1.8 Table-driven tests for the adversarial cases: `if x < y`,
      `Deployment<T>`, a shell redirect, a fenced block containing `<details>`,
      an inline code span containing a tag, an unpaired tag, nested tags, a tag
      with trailing whitespace.

## 2. The manager passes text through **[was manager-side]**

- [x] 2.1 Remove `Message.Blocks`, the `Block` alias and the parse calls from
      `AnswerMessage`, `agentWarn` and `SignalMessage`.
- [x] 2.2 Revert `ContractVersion` to `2`; delete `PreviousContractVersion`.
- [x] 2.3 Delete `downgrade()` and its call in `handleChannelOps`; restore the
      original `contractOK`.
- [x] 2.4 `RunReplyMessage` still composes from the RECORDED RUN ALONE — that is
      why `/work/done` and the reconciler backstop agree — and now carries the
      agent's text verbatim.
- [x] 2.5 Nothing new in status. Still true, and now trivially so.
- [x] 2.6 Delete `InputOrigin.StructuredBody`, `NormalizedSignal.StructuredBody`
      and `allStructured`. A signal is a card, never prose (design D8).
- [x] 2.7 Move `internal/blocks` out of the manager once both adapters have
      their own parser. Delete it, do not leave it unused.
- [x] 2.8 Regenerate deepcopy and CRDs.

## 3. AgentProfile flag

- [x] 3.1 `AgentProfile.spec.sharedOutputFormat`, default OFF.
- [x] 3.2 Regenerate deepcopy and CRDs into `chart/files/crds`.
- [x] 3.3 Injected via the work unit's SYSTEM PROMPT, in one place before any
      lane branches — a profile with its own `prompt` file sends the runtime a
      path, which nothing manager-side can append to.
- [x] 3.4 Test: flag unset injects nothing; set reaches both prompt lanes; the
      profile's own role text is appended to, never replaced.
- [ ] 3.5 It is now also the COMPATIBILITY boundary (design D5). Say so in the
      field's doc comment: an adapter with no parser is protected by the flag
      being off, not by a wire version.

## 4. format.md

- [x] 4.1 Rewritten in the block grammar: the grammar, the fold rule, the inline
      markdown subset, the emoji conventions, the anti-patterns.
- [x] 4.2 The five report templates became a DEFAULT SECTION SET — a starting
      point, not a mandatory form.
- [x] 4.3 The no-HTML rule for inline text, with the block-level exception
      stated explicitly so the two do not read as contradicting.
- [x] 4.4 NO PROSE above the fold: a what-goes-where table, ≤2 lines or ≤3
      bullets per section, "drop the connective tissue".
- [x] 4.5 A content→shape table mirroring `.claude/rules/authoring.md` — facts
      to bullets, comparisons to tables, sequences to numbered lists.
- [x] 4.6 Lists are MARKDOWN lists (`- `), never a literal `•`. A typed glyph
      renders as one running paragraph on any surface that has real lists.
- [x] 4.7 Every fence carries its language, because both surfaces highlight from
      the tag.
- [x] 4.8 The three lane templates no longer name the removed numbered
      templates, and their unread `INVESTIGATE:` / `AGENT-TASK:` / `REPLY:`
      markers are gone — no code reads them and the parser made them a visible
      stray block.
- [ ] 4.9 Length budget line: it now says ~600 characters, which was the cap.
      Keep the guidance, drop the implication that anything enforces it.

## 5. Telegram adapter

- [x] 5.1 **Port the parser** into the adapter (Go, dependency-free).
      **[was manager-side]**
- [x] 5.2 Render blocks: title, labelled sections in order, fold.
- [x] 5.3 Expandable blockquote needs Bot API 7.2, and there is no version
      probe — the send is the probe. Refused for the quote tag, latch the
      feature off and re-render with a plain quote plus a visible marker.
- [x] 5.4 Split at block boundaries; the first chunk carries title and named
      sections.
- [x] 5.5 A body with no tags renders from the body, unchanged.
- [x] 5.6 `ContractVersion` stays `2`. **Revert the bump to 3.**
- [x] 5.7 Markdown lists become bullets, one blank line between items.
- [x] 5.8 Fenced code carries its language as
      `<pre><code class="language-X">`, which Telegram highlights.
- [x] 5.9 A signal payload is quoted, and folded once tall enough to dominate
      the thread.

## 6. Console

- [x] 6.1 **Port the parser** into the SPA (TypeScript). **[was manager-side]**
      It must run on a rehydrated transcript as well as a live message — that is
      the whole point of the move.
- [x] 6.2 Block components: heading, labelled section, disclosure control
      collapsed by default.
- [x] 6.3 Render blocks in the conversation transcript, live AND rehydrated.
      The run-result view reads the same recorded text and can render the same
      way — the earlier "leave it raw" decision was forced by the console not
      being allowed to parse, and that constraint is gone.
- [x] 6.4 `Text.tsx` keeps its plain-text rule, asserted by a test that scans
      every source file for `dangerouslySetInnerHTML`.
- [x] 6.5 Messages with no tags render as text, unchanged.
- [x] 6.6 `ContractVersion` stays `2`. **Revert the bump to 3.**
- [x] 6.7 Single newlines render as line breaks, so consecutive lines are lines.
- [x] 6.8 Syntax highlighting via `rehype-highlight`, which emits an element
      tree — `highlight.js` directly returns an HTML string and would need
      `dangerouslySetInnerHTML`. Ten registered grammars, its own cached chunk.
- [x] 6.9 Code blocks wrap rather than scroll sideways, and are height-bounded.
- [x] 6.10 Signal card: source stated plainly, labels as a table with redundant
      ones dropped, payload carried apart so it can be collapsed.
- [ ] 6.11 Re-run BOTH `npm run screenshots` and `npm run demo` in
      `platform/console/ui`, once the parser move is done.

## 7. Chart and profiles

- [x] 7.1 The flag is exposed on all four chart-shipped profiles, default OFF.
- [ ] 7.2 Turn it on for `k8s-engineer`, `alert-investigator`, `ha-user`,
      `ha-operator` once 9.3 passes on each surface those profiles reach.
- [x] 7.3 Chart render test pins the default OFF and that it renders when set.


## 8. Verification

- [ ] 8.1 Integration test: an answer with a fold reaches a bound thread with
      its tags intact and unaltered by the manager.
- [x] 8.2 A chat signal containing tag-shaped text delivers those characters
      verbatim.
- [ ] 8.3 Live smoke on a real install, with the flag on, read on BOTH surfaces
      — including a conversation reopened AFTER a console restart, which is the
      case the whole rework exists for.

## 9. Documentation — THE LAST TASK, and it is not optional

**Both halves, and they are skipped independently.** The change is not finished
while a reader meets the old model — and this change REVERSED itself midway, so
several published pages describe a design that was abandoned.

### Reference docs

- [ ] 9.1 `docs/concepts.md` — the flag, and that it is the compatibility
      boundary.
- [ ] 9.2 `docs/contracts.md` — **REWRITE.** The body is markdown plus the block
      grammar and the ADAPTER reads both. No `blocks[]`, no contract 3, no
      downgrade table, no `structuredBody`. A `signal` is a card.
- [ ] 9.3 `docs/console-guide.md` — the fold as something a reader interacts
      with.
- [ ] 9.4 `CHANGELOG.md` — **REWRITE.** The published entry currently announces
      contract 3 and `blocks[]`.
- [ ] 9.5 Check the adopter pages for anything now untrue about agent output.
- [ ] 9.6 Read every changed page against `docs/CLAUDE.md` by hand, and run the
      prose lint. There is no lint script in this repo.
- [ ] 9.7 `.claude/rules/invariants.md` — **REWRITE.** It currently says the
      manager parses. It must say the manager passes agent text through and the
      adapters read the grammar.
- [ ] 9.8 `.claude/rules/adapters.md` — parsing the body grammar is now part of
      what a channel adapter does. It is not mentioned there at all.

### The ADOPTER SITE

- [ ] 9.9 Build the site and LOOK at every changed page. A squeezed column and a
      wrapped key are invisible until rendered.
- [ ] 9.10 `docs/guides/channel-adapter.md` — an adapter author now has to implement
      the grammar. This is the page the change most affects.
- [ ] 9.11 Re-run BOTH `npm run screenshots` and `npm run demo` in
      `platform/console/ui`, and decide whether the curated fixture should carry
      the grammar so the published screenshot shows a fold at all.
