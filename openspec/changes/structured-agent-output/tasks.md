## 1. Grammar and parser (manager)

- [ ] 1.1 Define the block model in `internal/chat`: an ordered list of blocks,
      each with a role (`title` | `section` | `details`), a label for named
      sections, and text in the existing markdown subset.
- [ ] 1.2 New parser package: recognize a tag only when alone on its own line at
      line start, well-formed as an open/close pair, and outside fenced and
      inline code. Everything else is literal text.
- [ ] 1.3 Unclosed region closes at end of output; no content is discarded.
- [ ] 1.4 Input with no recognized tag yields exactly one above-the-fold block
      holding the whole text.
- [ ] 1.5 Normalize: `<title>` first regardless of where it appeared, at most
      one, single line; named sections keep written order.
- [ ] 1.6 Cap above-the-fold length and DEMOTE overflow into the fold. Assert in
      test that no input loses a character.
- [ ] 1.7 Flatten blocks back to the markdown subset. Assert flattening is total:
      `body` is never empty when `blocks[]` is populated.
- [ ] 1.8 Table-driven tests for the adversarial cases: `if x < y`,
      `Deployment<T>`, a shell redirect, a fenced block containing `<details>`,
      an inline code span containing a tag, an unpaired tag, nested tags, a tag
      with trailing whitespace.

## 2. Message contract

- [ ] 2.1 `chat.Message` gains `blocks[]`; `body` stays populated alongside.
- [ ] 2.2 `ContractVersion` 2 → 3.
- [ ] 2.3 `/channel/ops` serves `contract=2` the message with `body` populated
      and `blocks[]` omitted; `contract=1` stays refused with 400.
- [ ] 2.4 Parse where the reply is COMPOSED, never where the run is reported.
      `chat.RunReplyMessage` builds a run's reply FROM THE RECORDED RUN ALONE so
      that `/work/done` and the reconciler backstop compose the same message, and
      a pure parse called there keeps that true. Parsing on the `/work/done` path
      instead leaves the backstop's re-derived reply blockless.
- [ ] 2.5 Blocks are DERIVABLE from `status.runs[].result` and are NOT persisted.
      Nothing new goes in status.
- [ ] 2.6 `notice` gains `blocks[]`, populated only when its body is
      agent-reported — the FAILED-run case, where `RunReplyMessage` sends the
      agent's own explanation as `Warn(body)`. Manager-composed notices
      (listings, refusals, usage errors) carry none.
- [ ] 2.7 Signal bodies parsed only when the inbound signal declares it; default
      raw.
- [ ] 2.8 `/signal/inbound` accepts and records the declaration.

## 3. AgentProfile flag

- [ ] 3.1 Add the output-format field to `AgentProfile` in `api/v1alpha1`,
      default OFF.
- [ ] 3.2 Regenerate deepcopy and CRDs into `chart/files/crds`.
- [ ] 3.3 Gate injection of the format specification on the flag in prompt
      assembly. Unset injects nothing.
- [ ] 3.4 Test: flag unset + agent emits tags → still parsed and folded (the flag
      gates the prompt, never the parse).

## 4. format.md

- [ ] 4.1 Rewrite in the block grammar: the grammar itself, the fold rule, the
      inline markdown subset, the length budget, the emoji conventions, the
      anti-patterns.
- [ ] 4.2 Turn the five report templates into the DEFAULT section set — a
      starting point, not a mandatory form.
- [ ] 4.3 Keep the no-HTML rule for inline text and state the block-level
      exception explicitly, so the two rules do not read as contradicting.

## 5. Telegram adapter

- [ ] 5.1 Render blocks: title, labelled sections in order, fold.
- [ ] 5.2 Verify the Bot API minimum version for expandable blockquote; fall back
      to a plain quote with a visible marker if unavailable.
- [ ] 5.3 Split at block boundaries; assert the first chunk carries title and
      named sections.
- [ ] 5.4 A message with no blocks renders from `body`, unchanged.
- [ ] 5.5 Bump `ContractVersion` to 3.

## 6. Console

- [ ] 6.1 Block components: heading, labelled section, disclosure control
      collapsed by default.
- [ ] 6.2 Render blocks in the conversation transcript and the run-result view.
- [ ] 6.3 `Text.tsx` keeps its plain-text rule for every other string; assert no
      `dangerouslySetInnerHTML` is introduced anywhere.
- [ ] 6.4 Messages with no blocks render as text, unchanged.
- [ ] 6.5 Bump `ContractVersion` to 3.
- [ ] 6.6 Re-run BOTH `npm run screenshots` and `npm run demo` in
      `platform/console/ui`. The screenshots and the landing recording are both
      build output, and the change is not done until both match.

## 7. Chart and profiles

- [ ] 7.1 Expose the flag on the chart-shipped profiles, default OFF.
- [ ] 7.2 Turn it on for `k8s-engineer`, `alert-investigator`, `ha-user`,
      `ha-operator` once step 6 is verified.
- [ ] 7.3 Chart render test pins the flag's default OFF.

## 8. Documentation

- [ ] 8.1 `docs/concepts.md` — the `AgentProfile` flag and what it injects.
- [ ] 8.2 `docs/contracts.md` — contract version 3, `blocks[]`, the grammar and
      its recognition rules, the signal opt-in.
- [ ] 8.3 `docs/console-guide.md` — the fold as something a reader interacts
      with, if the views change visibly.
- [ ] 8.4 `CHANGELOG.md` — the template removal and the flag, newest first.
- [ ] 8.5 Check the adopter pages: does `introduction.md`, `getting-started.md`
      or the landing page now say something untrue about agent output?
- [ ] 8.6 Read every changed adopter page against `docs/CLAUDE.md`'s writing
      rules BY HAND — structure over prose, no semicolons, both shell fences.
      There is no lint script in this repo.
- [ ] 8.7 `.claude/rules/invariants.md` — "THE MANAGER COMPOSES MEANING" names
      the four message kinds' fields and says `format.md` binds the agent to the
      markdown subset. Contract 3, `blocks[]` and the block grammar make both
      lines incomplete.

## 9. Verification

- [ ] 9.1 Integration test: an answer with a fold reaches a bound thread with its
      blocks intact.
- [ ] 9.2 Integration test: a chat signal containing tag-shaped text delivers
      those characters verbatim.
- [ ] 9.3 Live smoke on a real install — a task signal to a claimed source, with
      the flag on, read on BOTH surfaces. A rendered pod is not a running one.
