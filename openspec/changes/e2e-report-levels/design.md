## Context

See proposal.md - Why. `.github/workflows/e2e.yml` defines one `pack` job,
called by `e2e-smoke.yml` (tier `smoke`) and `e2e-full.yml` (tier `full`, via
`release.yml` too). The job runs `go test -tags e2e -v ./test/e2e/` and
uploads `$E2E_ARTIFACT_DIR` as an artifact only when the job fails
(`cluster.Dump`, written by the test binary itself on a test failure). There
is no prototype here — this is a CI script, and the "how it looks" question is
settled by the markdown a `GITHUB_STEP_SUMMARY` write renders, checkable by
running the script against a fixture and reading its stdout.

## Goals / Non-Goals

**Goals:**
- Every pack run (pass or fail) leaves a report on the job's Actions run
  summary, without opening the log.
- The full tier's report stays short even when the pack is large: counts plus
  the names of anything that did not pass.
- The smoke tier's report is complete enough to diagnose a failure from the
  summary alone, without downloading the diagnostics artifact.
- A build failure (no test ever ran) produces a report that says so, not a
  crash or an empty page.

**Non-Goals:**
- Not a general-purpose Go test reporting tool — it only needs to serve this
  one workflow's two tiers.
- Not a replacement for `$E2E_ARTIFACT_DIR` (cluster dumps, pod logs) — the
  report is about which *tests* passed, not why the cluster misbehaved.
- Not wired into `ci.yml`'s conformance job or the envtest suite — both are
  already fast enough that a `-v` log is sufficient, and this change touches
  only `.github/workflows/e2e.yml`.

## Decisions

**Parse `go test -json` (test2json), not `-v` text.** The alternative —
regexing `--- PASS:` / `--- FAIL:` lines out of `-v` output — breaks on
multi-line output interleaved between them. `-json` gives one structured event
per line (`Action`: `run`/`output`/`pass`/`fail`/`skip`, `Test`, `Output`,
`Elapsed`), which is exactly test2json's job and is what every existing Go
test-reporting tool (gotestsum included) builds on. `-v` is kept alongside
`-json` so failure `Output` events carry the verbose per-assertion text, not
just the final `FAIL` line.

**Tee the JSON stream to a file under `$E2E_ARTIFACT_DIR`, and still print
human output live.** The job runs up to 55 minutes; a step that goes silent
until the very end is a worse debugging experience than today's live `-v`
log, even though the file is what the report is generated from afterwards.
The run step becomes:

```sh
go test -tags e2e -count=1 -timeout 55m -v -json ./test/e2e/ \
  | tee "$E2E_ARTIFACT_DIR/events.jsonl" \
  | jq -rR '. as $raw | try (fromjson | select(.Action == "output") | .Output) catch $raw'
```

`fromjson` inside a `try`/`catch` falls back to printing the raw line when it
is not valid JSON, which is what a build failure emits before `go test` ever
reaches test2json — so the live log is never blanker than before. **`catch`'s
`.` is jq's ERROR VALUE, not the original input** — `. as $raw` is what makes
the fallback print the actual build-failure line rather than jq's own parse
error message, which reads as noise on top of noise. And
`events.jsonl` still gets every line (including the non-JSON ones), which the
report script filters itself.

`set -o pipefail` (already implied by using `bash` `run:` blocks with
`shell: bash`, GitHub Actions' default) keeps `go test`'s exit code as the
step's exit code through the pipe, so the job still fails exactly when it
does today.

**One script, one `--level {summary,full}` flag, not two scripts.** Both
levels read the same events and differ only in how much of the parsed result
they render — sharing the parse means one place to fix a test2json edge case,
matching the pattern in `.github/scripts/review-trace.py`
(parse once, argparse for the shape of the output).

**Level is chosen by the CALLER (the workflow step), from `inputs.tier`, not
guessed by the script from the file.** The mapping (full tier -> `summary`
level, smoke tier -> `full` level) is a decision about *this workflow's* two
tiers, stated in proposal.md - Why; the script has no opinion about which
tier warrants which level, so it takes `--level` as a plain argument. This
also keeps the script testable without a tier concept — a test picks a level
directly.

**Subtests count as their own test.** test2json emits one `run`/`pass`/`fail`
entry per `t.Run` subtest, with its own `Test` (e.g.
`TestFoo/subtest`) and `Elapsed`. The script groups strictly by `Test` field,
without collapsing subtests into their parent — a `summary` failing-test list
that only ever printed `TestFoo` would hide which of several subtests failed,
which is exactly the case a summary report has to be useful for.

**Zero-events case reports "no test results parsed", exit 0.** A build
failure already fails the "Run the pack" step and therefore the job — the
report step runs `if: always()` so it still gets to say *why* nothing ran,
but it must never itself fail the job (that would double-report the same
failure under a second name in the checks list).

**Markdown, appended directly to `$GITHUB_STEP_SUMMARY`.** GitHub renders
that file as the run's summary page; the step is `python3 e2e-report.py ...
>> "$GITHUB_STEP_SUMMARY"`. No JSON artifact, no separate upload — the
existing `$E2E_ARTIFACT_DIR` upload step (unconditional, `if: always()`)
already carries `events.jsonl` for anyone who wants the raw stream.

## Risks / Trade-offs

- **`jq`'s `try`/`catch` around `fromjson` silently drops a mid-stream
  corruption that happens to look like valid JSON for the wrong reason** →
  accepted: test2json's own output format is what's being trusted here, and a
  malformed line from `go test` itself would be a Go toolchain bug, not
  something this change should try to detect.
- **A `full`-level report on a very large full-tier pack (if this mapping is
  ever flipped) could produce a huge summary page** → the level→tier mapping
  is fixed by this change's own reasoning (full tier is the large one), so
  this is a risk of someone changing the mapping later without re-reading
  why, not a risk of the current design. No code enforces the mapping beyond
  the one line in `e2e.yml`.
- **`GITHUB_STEP_SUMMARY` has a 1 MiB size limit (GitHub-imposed)** → the
  `full` level is only ever used for the smoke tier, which is deliberately
  short (`E2E_BUDGET`-bounded, default 20m); failure `Output` text for a
  handful of tests will not approach it. Not truncated defensively, since
  doing so would hide the one thing the smoke-tier report exists to show.

## Migration Plan

No migration — this only changes what a future CI run writes to its own
summary page. No stored state, no schema, nothing to roll back beyond
reverting the workflow and script files.
