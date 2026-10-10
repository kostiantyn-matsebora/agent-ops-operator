## 1. Rename the Go pack and its build tag — backend-developer

- [x] 1.1 `git mv platform/manager/test/e2e platform/manager/test/system` and change every `//go:build e2e` tag inside it to `//go:build system`. Verify `grep -rl "build e2e" platform/manager/test/system/` returns nothing.
- [x] 1.2 Rename every `E2E_*` env var read inside the moved files (`cluster.go`, `install.go`, `harness_test.go`, and the rest) to `SYSTEM_*`. Verify `grep -rn "E2E_" platform/manager/test/system/` returns nothing.
- [x] 1.3 Update the handful of comments elsewhere in `platform/manager/` (`internal/integration/channelread_test.go`, `internal/integration/storagebreaker_test.go`, `internal/httpapi/status.go`, `internal/chat/ops.go`) and in `test/conformance/channel_test.go` that name this pack specifically, leaving any generic "end to end" English usage untouched. Verify by re-reading each hit from `grep -rn -i "e2e" platform/manager/ test/`.
- [x] 1.4 Verify `cd platform/manager && go build ./... && go vet ./...` succeeds and `go test -tags system -c -o /dev/null ./test/system/` compiles the renamed pack's test binary.

## 2. Rename the CI workflows, scripts and env vars — deployment-engineer

- [x] 2.1 Rename `.github/workflows/e2e.yml` to `system.yml`, its `name:` field to `system`, and the job's displayed name to `system / ${{ inputs.tier }}`. Verify the file still parses with `python3 -c "import yaml,sys;yaml.safe_load(open('.github/workflows/system.yml'))"`.
- [x] 2.2 Rename `.github/workflows/e2e-smoke.yml` to `system-smoke.yml` and `.github/workflows/e2e-full.yml` to `system-full.yml`, updating their `uses:` targets and comments. Verify both still parse as valid YAML.
- [x] 2.3 Update `.github/workflows/ci.yml`: the `discover` job's output key `e2e` to `system`, its path filter (the `.github/workflows/e2e\.yml` pattern) to match `system.yml`, and every comment naming the old workflow. Verify `python3 -c "import yaml,sys;yaml.safe_load(open('.github/workflows/ci.yml'))"` and that the renamed output key is the only one `grep -n "outputs.e2e\b" .github/workflows/ci.yml` would have matched.
- [x] 2.4 Update `.github/workflows/release.yml`'s references to the renamed workflow and env vars.
- [x] 2.5 Rename `.github/scripts/e2e-report.py` to `system-report.py`, update the `E2E_ARTIFACT_DIR` reads inside it, and update the CI step that invokes it. Rename `.github/tests/e2e-report.test.sh` to `system-report.test.sh` and update it to match. Verify `bash .github/tests/system-report.test.sh` passes.
- [x] 2.6 Update `.github/scripts/smoke-evidence.py`'s `SUFFIX` constant and docstring from `" / e2e / smoke"` to `" / system / smoke"`. Update `.github/tests/smoke-evidence.test.sh` to match. Verify the test passes.
- [x] 2.7 Update the "e2e pack" comments in `.github/components.sh` to say "system pack". Verify `.github/components.sh modules` and `.github/components.sh images` return the same lists as before (comment-only change).

## 3. Rename the openspec task-section convention — deployment-engineer

- [x] 3.1 In `openspec/config.yaml`, change the injected rule text from "E2E TESTS" to "SYSTEM TESTS" and update its reference to `platform/manager/test/e2e/` to `test/system/`.
- [x] 3.2 In `.github/scripts/docs-task-guard.py`, change the `TEST_SECTIONS` pattern from `r"e2e|end.to.end"` to `r"system"`, the label string "an e2e-test section" to "a system-test section", and the `TAIL`/docstring prose that names the section. Verify `bash .github/scripts/docs-task-guard-test.sh` passes.
- [x] 3.3 Update the fixtures under `.github/tests/docs-task/complete/tasks.md`, `tests-out-of-order/tasks.md`, `unticked/tasks.md` and `unticked-tests/tasks.md` to use a "System tests" heading in place of "E2E tests". Verify the guard's own test suite still passes against them.
- [x] 3.4 Update the "e2e tests" mention in `.claude/hooks/require-docs-task.sh`'s comment.

## 4. Retired vocabulary and spec validation — deployment-engineer

- [x] 4.1 Add entries to `.github/retired-vocabulary.json` for `test/e2e/` as this pack's path, `-tags e2e`, the three old workflow file names, and the five `E2E_*` env vars, each naming its `system`-named replacement. Verify `python3 .github/scripts/retired-vocabulary-guard.py` reports no violation from any file this change leaves behind. (The two capability specs this change touches — `continuous-integration` and `end-to-end-testing` — were synced early via `openspec sync`, ahead of archiving, since the guard runs over the published tree and would otherwise flag its own not-yet-archived delta.)
- [x] 4.2 Run `python3 .github/scripts/retired-vocabulary-guard.py --show` locally and confirm it does NOT flag `e2e-live`, `platform/console/ui/e2e/`, or any generic "end to end" prose — those stay legitimate.
- [x] 4.3 Run `openspec validate rename-e2e` and confirm the change validates cleanly.

## 5. Unit tests

- [x] 5.1 `cd platform/manager && go build ./... && go vet ./... && go test ./...` passes.
- [x] 5.2 `go test -tags system -c -o /dev/null ./test/system/` compiles the renamed pack's test binary.
- [x] 5.3 `.github/tests/run.sh` (the Python script suite, which runs `docs-task-guard-test.sh`, `e2e-report.test.sh`/`system-report.test.sh`, `smoke-evidence.test.sh` and `retired-vocabulary-guard.py`'s own tests) passes. The suite as a whole reports 3 failures in `cloud-bootstrap.test.sh`, reproduced identically against a clean `origin/master` checkout with none of this change's commits — pre-existing, environment-dependent flakiness unrelated to this rename, in a file this change does not touch.
- [x] 5.4 `python3 .claude/scripts/rules_compliance.py` over every `.md` file this change edits reports clean. (Verified by diffing violations before/after this change's edits per file. Every surviving violation is a pre-existing one at a shifted line number, and this change introduces none. The pre-existing debt is unrelated to this rename and out of scope.)

## 6. System tests

- [ ] 6.1 Dispatch the renamed smoke tier against this branch — `gh workflow run system-smoke.yml --ref change/rename-e2e` — and confirm the run succeeds. This is the one direct proof that the renamed build tag, package path, workflow files and env vars still cohere end to end against a real k3d cluster.

## 7. Documentation

- [x] 7.1 Update `docs/testing.md`: rename the "End to end" tier row to "System" and adjust its prose to match, including the "What the repository must hold" table's workflow names.
- [x] 7.2 Update `README.md`'s nightly-workflow badge link and label from `e2e-full.yml`/"E2E nightly" to `system-full.yml`/"System nightly".
- [x] 7.3 Update `.claude/rules/build-test.md`, `change-tests.md`, `documentation.md`, `remote-session.md`, `structure.md` and `CLAUDE.md` to describe the pack, its tag, its env vars and its workflows under their new names.
- [x] 7.4 Update `CONTRIBUTING.md` and `.github/routines/implement-issue.md` the same way.
- [x] 7.5 Confirm `.claude/rules/gotchas.md` is UNCHANGED for every historical incident narrative (it keeps the file and workflow names as they existed when the incident happened), and leave a note only where a forward-looking "what to run today" statement needed updating.
- [x] 7.6 Adopter site: no page changes. This tier is internal CI/dev-tooling with no adopter-facing behavior, and no landing page, Introduction, Getting started, Installation page or guide names it.
