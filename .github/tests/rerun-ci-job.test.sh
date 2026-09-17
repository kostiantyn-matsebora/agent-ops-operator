#!/usr/bin/env bash
# A person's answer re-runs the ONE ci job that reads the conversation, in the
# head's OWN pull_request run -- never a run of its own (#131), and only when
# that job failed. Every case where nothing can be done is a notice, exit 0.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/rerun-ci-job.py"

tmp=$(mktemp -d)
export GH_CALLS="$tmp/calls" RUNS_FILE="$tmp/runs.json" JOBS_FILE="$tmp/jobs" RERUN_FAILS=""
mkdir -p "$tmp/bin"
cat > "$tmp/bin/gh" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$GH_CALLS"
case "$*" in
  "api --method GET repos/"*"/actions/workflows/ci.yml/runs"*) cat "$RUNS_FILE" ;;
  "api --method GET repos/"*"/actions/runs/"*"/jobs"*)         cat "$JOBS_FILE" ;;
  "api --method POST repos/"*"/actions/jobs/"*"/rerun")
      if [ -n "$RERUN_FAILS" ]; then echo "HTTP 403: Resource not accessible by integration" >&2; exit 1; fi ;;
esac
exit 0
STUB
chmod +x "$tmp/bin/gh"; export PATH="$tmp/bin:$PATH"

run() { : > "$GH_CALLS"; python3 "$S" --repo o/r --sha 5b039360000000000000000000000000000000ab --pr 220 --job docs-task 2>&1; }
completed() { cat > "$RUNS_FILE" <<'JSON'
{"workflow_runs":[{"id":100,"status":"completed","conclusion":"failure","run_started_at":"2026-09-17T17:14:05Z"},
                  {"id":99,"status":"completed","conclusion":"failure","run_started_at":"2026-09-13T09:50:13Z"}]}
JSON
}

it "re-runs the FAILED named job of the latest completed pull_request ci run for the head, and nothing else"
completed; printf '{"jobs":[{"id":555,"name":"docs-task","conclusion":"failure"},{"id":556,"name":"ci-green","conclusion":"failure"}]}' > "$JOBS_FILE"
out=$(run); rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$GH_CALLS")" "api --method GET repos/o/r/actions/workflows/ci.yml/runs -f head_sha=5b039360000000000000000000000000000000ab -f event=pull_request"
assert_contains "$(cat "$GH_CALLS")" "actions/runs/100/jobs"
assert_not_contains "$(cat "$GH_CALLS")" "actions/runs/99/jobs"
assert_not_contains "$(cat "$GH_CALLS")" '--jq'
assert_contains "$(cat "$GH_CALLS")" "api --method POST repos/o/r/actions/jobs/555/rerun"
assert_not_contains "$(cat "$GH_CALLS")" "workflow run"
assert_not_contains "$(cat "$GH_CALLS")" "rerun-failed-jobs"
assert_contains "$out" "re-running \`docs-task\` (job 555) of ci run 100"

it "a job that did NOT fail is left alone: nothing to re-evaluate, as a notice"
completed; printf '{"jobs":[{"id":555,"name":"docs-task","conclusion":"success"}]}' > "$JOBS_FILE"
out=$(run); rc=$?
assert_status 0 "$rc"
assert_not_contains "$(cat "$GH_CALLS")" "/rerun"
assert_contains "$out" "::notice::"
assert_contains "$out" "concluded success"

it "the LATEST attempt of the job decides, not an earlier one -- across concatenated pages"
completed; printf '{"jobs":[{"id":554,"name":"docs-task","conclusion":"failure"}]}{"jobs":[{"id":555,"name":"docs-task","conclusion":"success"}]}' > "$JOBS_FILE"
out=$(run); rc=$?
assert_status 0 "$rc"
assert_not_contains "$(cat "$GH_CALLS")" "/rerun"

it "a run still in progress is left alone, as a notice"
cat > "$RUNS_FILE" <<'JSON'
{"workflow_runs":[{"id":101,"status":"in_progress","conclusion":null,"run_started_at":"2026-09-17T18:00:00Z"}]}
JSON
out=$(run); rc=$?
assert_status 0 "$rc"
assert_not_contains "$(cat "$GH_CALLS")" "/rerun"
assert_contains "$out" "::notice::"
assert_contains "$out" "is in_progress"

it "no run for the head is a notice, exit 0"
printf '{"workflow_runs":[]}' > "$RUNS_FILE"
out=$(run); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "::notice::#220: no pull_request ci run exists"

it "a run without the job is a notice, exit 0"
completed; printf '{"jobs":[{"id":9,"name":"operator","conclusion":"failure"}]}' > "$JOBS_FILE"
out=$(run); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "::notice::#220: run 100 has no \`docs-task\` job"

it "a refused re-run (a fork's read-only token) is a notice naming the job, exit 0"
completed; printf '{"jobs":[{"id":777,"name":"docs-task","conclusion":"failure"}]}' > "$JOBS_FILE"
out=$(RERUN_FAILS=1 run); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "::notice::#220: re-running \`docs-task\` (job 777 of run 100) was refused"
assert_contains "$out" "403"

it "uses --method GET on the runs route, so the params never become a request body"
completed; printf '{"jobs":[{"id":555,"name":"docs-task","conclusion":"failure"}]}' > "$JOBS_FILE"
out=$(run); rc=$?
assert_status 0 "$rc"
assert_contains "$(grep 'workflows/ci.yml/runs' "$GH_CALLS")" "api --method GET"

summary
