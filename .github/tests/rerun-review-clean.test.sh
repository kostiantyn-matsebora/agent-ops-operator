#!/usr/bin/env bash
# A thread event re-runs `review-clean` of the head's OWN pull_request ci run,
# never a run of its own (#131: only a pull_request run's checks reach the
# merge box). Every case where nothing can be done is a notice and exit 0.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/rerun-review-clean.py"

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

run() { : > "$GH_CALLS"; python3 "$S" --repo o/r --sha 9bd259773b7cd05db20196404d48e9722be599d0 --pr 220 2>&1; }

it "re-runs the review-clean JOB of the latest completed pull_request ci run for the head, and nothing else"
cat > "$RUNS_FILE" <<'JSON'
{"workflow_runs":[{"id":100,"status":"completed","conclusion":"failure","run_started_at":"2026-09-13T17:45:05Z"},
                  {"id":99,"status":"completed","conclusion":"failure","run_started_at":"2026-09-13T09:50:13Z"}]}
JSON
printf '555\n' > "$JOBS_FILE"
out=$(run); rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$GH_CALLS")" "api --method GET repos/o/r/actions/workflows/ci.yml/runs -f head_sha=9bd259773b7cd05db20196404d48e9722be599d0 -f event=pull_request"
assert_contains "$(cat "$GH_CALLS")" "actions/runs/100/jobs"
assert_not_contains "$(cat "$GH_CALLS")" "actions/runs/99/jobs"
assert_contains "$(cat "$GH_CALLS")" "api --method POST repos/o/r/actions/jobs/555/rerun"
assert_not_contains "$(cat "$GH_CALLS")" "workflow run"
assert_not_contains "$(cat "$GH_CALLS")" "actions/runs/100/rerun"
assert_contains "$out" "re-running \`review-clean\` (job 555) of ci run 100"

it "asks for the jobs by name, never by position"
assert_contains "$(cat "$GH_CALLS")" 'select(.name == "review-clean")'

it "a run still in progress is left alone: review-clean reads live when it runs"
cat > "$RUNS_FILE" <<'JSON'
{"workflow_runs":[{"id":101,"status":"in_progress","conclusion":null,"run_started_at":"2026-09-13T18:00:00Z"}]}
JSON
out=$(run); rc=$?
assert_status 0 "$rc"
assert_not_contains "$(cat "$GH_CALLS")" "/rerun"
assert_contains "$out" "is in_progress"

it "no run for the head is a notice, exit 0, no re-run"
printf '{"workflow_runs":[]}' > "$RUNS_FILE"
out=$(run); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "::notice::#220: no pull_request ci run exists"
assert_not_contains "$(cat "$GH_CALLS")" "/rerun"

it "a run without a review-clean job (a draft) is a notice, exit 0"
cat > "$RUNS_FILE" <<'JSON'
{"workflow_runs":[{"id":102,"status":"completed","conclusion":"success","run_started_at":"2026-09-13T18:00:00Z"}]}
JSON
: > "$JOBS_FILE"
out=$(run); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "::notice::#220: run 102 has no \`review-clean\` job"

it "a refused re-run (a fork's read-only token) is a notice naming the job and exit 0"
printf '777\n' > "$JOBS_FILE"
out=$(RERUN_FAILS=1 run); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "::notice::#220: re-running \`review-clean\` (job 777 of run 102) was refused"
assert_contains "$out" "403"

it "uses --method GET on the runs route, so the params never become a request body"
assert_contains "$(grep 'workflows/ci.yml/runs' "$GH_CALLS")" "api --method GET"

summary
