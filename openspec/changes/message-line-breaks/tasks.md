## 1. The contract says what a newline means

- [ ] 1.1 State the line-break rule in `docs/contracts.md` where the subset is
      named — verify the subset now defines every character it relies on.
- [ ] 1.2 Add the hard-wrapping corollary to
      `internal/dispatch/templates/format.md`, in one line under the global
      rules — verify it names the consequence, not just the prohibition.

## 2. The console honours it

- [ ] 2.1 Add `remark-breaks` to `console/ui` and enable it in
      `Markdown.tsx` — verify `npm run build` and `npm test` pass.
- [ ] 2.2 Unit-test a Template 1 message: label and content on separate lines,
      and the bullets each on their own — verify the test fails without the
      plugin.
- [ ] 2.3 Unit-test that a fenced block and a table are byte-identical with the
      plugin enabled — verify no break is inserted into code.

## 3. The other surface is pinned

- [ ] 3.1 Add a `channel-telegram` render test asserting that a two-line section
      survives rendering as two lines — verify it passes today, since the point
      is to stop the surfaces drifting apart later.

## 4. The fixture and the published assets

- [ ] 4.1 Unwrap the screenshot fixture's agent answer so no sentence is broken
      across source lines — verify the text reads as paragraphs, not as a column.
- [ ] 4.2 Re-run `npm run screenshots` and `npm run demo` — verify the twelve
      images and the recording show the answer's sections on their own lines.

## 5. Release

- [ ] 5.1 Build and push a multi-arch console image, bump its tag in the chart
      values — verify `docker manifest inspect` lists both architectures.
- [ ] 5.2 Add the `docs/CHANGELOG.md` entry under the version that ships it —
      verify it names the behaviour change and needs no upgrade steps.
