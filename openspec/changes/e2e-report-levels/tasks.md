## 1. Report script

- [ ] 1.1 Write `.github/scripts/e2e-report.py`: parse a `go test -json`
      (test2json) event stream file, grouping by `Test` (subtests kept
      distinct, per design.md - Decisions), and render markdown at
      `--level summary` (counts + total elapsed + names of any failed or
      skipped test) or `--level full` (every test's name, status, elapsed,
      plus captured `Output` for each failure). Verify by running it by hand
      against a hand-written `events.jsonl` fixture for each level and
      reading the printed markdown.
- [ ] 1.2 Handle zero parseable test events (a build failure, or any file
      whose lines are not test2json JSON) by rendering "no test results were
      parsed" instead of crashing, and exit 0 regardless of what the parsed
      tests' outcomes were (the step's exit code must never gate the job —
      the "Run the pack" step already does). Verify by running it against an
      empty file and a file of plain non-JSON lines.

## 2. Workflow wiring

- [ ] 2.1 In `.github/workflows/e2e.yml`'s "Run the pack" step: create
      `$E2E_ARTIFACT_DIR` before running tests (it is currently created only
      as the upload-artifact source, not by this job), switch the `go test`
      invocation to `-v -json`, and pipe it as
      `| tee "$E2E_ARTIFACT_DIR/events.jsonl" | jq -rR 'try (fromjson |
      select(.Action == "output") | .Output) catch .'` so the live log still
      streams human-readable output. Verify with `bash -n` on the extracted
      step script and by tracing through the pipeline against a small
      canned `go test -json` sample locally (`go test -json ./...` on any Go
      module in this repo, piped through the same `jq` filter, reproduces
      readable `-v`-style output).
- [ ] 2.2 Add a step after "Run the pack", `if: always()`, that runs
      `python3 "$GITHUB_WORKSPACE/.github/scripts/e2e-report.py"
      --events "$E2E_ARTIFACT_DIR/events.jsonl" --level <level> >>
      "$GITHUB_STEP_SUMMARY"`, where `<level>` is `summary` for
      `inputs.tier == 'full'` and `full` for `inputs.tier == 'smoke'` (an
      inline `${{ }}` conditional in the `run:` line, matching the existing
      `CLAUDE_CODE_OAUTH_TOKEN` line's style). Verify by reading the rendered
      step in `actionlint` or by eye against the existing `env:` block's
      conditional syntax already in this file.

## 3. Unit tests

- [ ] 3.1 Add `.github/tests/e2e-report.test.sh`, following
      `.github/tests/lib.sh` and the style of `.github/tests/review-trace.test.sh`:
      a `summary`-level fixture asserting the counts line and the
      failed/skipped names appear and that full per-test output does NOT;
      a `full`-level fixture asserting every test name, its status and
      elapsed appear, and that a failing test's captured output appears; the
      zero-events case asserting the "no test results were parsed" message
      and a zero exit code. Verify with
      `bash .github/tests/e2e-report.test.sh`.
- [ ] 3.2 Verify `.github/tests/run.sh` picks the new test up (it globs
      `*.test.sh`, no registration needed) by running
      `.github/tests/run.sh` and confirming `e2e-report.test.sh` appears in
      its output and the suite still exits 0.

## 4. E2E tests

- [ ] 4.1 Not applicable — nothing in this change is decided by a
      Kubernetes cluster. It changes what a CI job writes to its own run
      summary from an already-running `go test` process; it does not change
      what the pack installs, tests, or asserts against the cluster. No
      lane is added to `platform/manager/test/e2e/`.

## 5. Documentation

- [ ] 5.1 Reference docs: update `docs/testing.md` — the "What the
      repository must hold" section (or immediately after it) to state that
      each tier's CI run now appends a report to the Actions run summary,
      and name the level each tier gets (full tier: summary; smoke tier:
      full) with the one-sentence reason from proposal.md - Why. Verify by
      reading the updated section for accuracy against the shipped
      `e2e.yml`.
- [ ] 5.2 Adopter site: none of the site's pages (landing page, Introduction,
      Getting started, Installation, integration pages, guides) describe CI
      workflow internals or this repository's own Actions runs, so nothing
      there is made untrue by this change. No edit needed.
