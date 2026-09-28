#!/usr/bin/env bash
# WHICH RED CHECKS THE FIXING LOOP MAY ACT ON. Getting this wrong is not a
# crash, it is a loop working on the wrong thing:
#
#   too wide   the review's own failed job, or `ci-green` itself, handed to the
#              fixer as something to fix. `ci-green` fails BECAUSE one of its
#              needs did; reporting both gives the fixer the symptom beside the
#              cause with no way to tell them apart.
#   too narrow a required list copied into the program, stale the first time
#              somebody adds a gate — which in this project is one line in
#              `ci-green`'s `needs:`, so the list is READ from there.
#   too eager  reporting the checks clean before CI has run on the head, or
#              while it is still running. Same rule the analysis collector
#              states: not reported is a FLAG, and not CONCLUDED is too.
#
# NO NETWORK: `gh` is a stub answering from fixtures.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/failed-checks.py"

setup() {
  DIR=$(mktemp -d); BIN="$DIR/bin"; mkdir -p "$BIN"
  OUT="$DIR/checks.json"
  # A ci.yml naming three required jobs. The fixture is the POINT: the program
  # must read this rather than carry its own list.
  cat > "$DIR/ci.yml" <<'YML'
name: ci
on: [pull_request]
jobs:
  operator: {runs-on: ubuntu-latest, steps: []}
  chart: {runs-on: ubuntu-latest, steps: []}
  images: {runs-on: ubuntu-latest, steps: []}
  ci-green:
    needs: [operator, chart, images]
    runs-on: ubuntu-latest
    steps: []
YML
}

# `gh` answering the check-runs route from a fixture, and a log from another.
stub_checks() {  # stub_checks <runs-json-lines-file>
  cat > "$BIN/gh" <<STUB
#!/usr/bin/env bash
printf '%s\n' "\$*" >> "$DIR/calls"
case "\$*" in
  *"check-runs"*) cat "$1" ;;
  *"--log-failed"*) printf 'operator\tRun tests\tFAIL TestThing\noperator\tRun tests\texit 1\n' ;;
  *) : ;;
esac
STUB
  chmod +x "$BIN/gh"
}

runs_file() { RUNS="$DIR/runs.jsonl"; : > "$RUNS"; }
add_run() {  # add_run <name> <conclusion> [status]   (a concluded run unless told otherwise)
  python3 - "$RUNS" "$1" "$2" "${3:-completed}" <<'PY'
import json, sys
with open(sys.argv[1], "a") as f:
    f.write(json.dumps({
        "name": sys.argv[2], "conclusion": None if sys.argv[4] != "completed" else sys.argv[3], "status": sys.argv[4],
        "details_url": "https://github.com/o/r/actions/runs/555/job/1",
        "html_url": "https://github.com/o/r/actions/runs/555/job/1",
    }) + "\n")
PY
}

run_it() { PATH="$BIN:$PATH" python3 "$S" --repo o/r --pr 5 --sha deadbeef --out "$OUT" --ci "$DIR/ci.yml" "$@" 2>&1; }
read_out() { python3 -c "import json,sys;print(json.load(open(sys.argv[1]))[sys.argv[2]])" "$OUT" "$1"; }

# --- green -------------------------------------------------------------------

it "a green head is an empty list, and the checks WERE consulted"
setup; runs_file
add_run operator success; add_run chart success; add_run images success
stub_checks "$RUNS"
out=$(run_it); assert_status 0 "$?"
assert_equals "[]" "$(read_out items)"
assert_equals "True" "$(read_out consulted)"

# --- one failure -------------------------------------------------------------

it "one failed required job is one item, with the job, the run and a log tail"
setup; runs_file
add_run operator failure; add_run chart success
stub_checks "$RUNS"
out=$(run_it); assert_status 0 "$?"
assert_equals "1" "$(python3 -c 'import json,sys;print(len(json.load(open(sys.argv[1]))["items"]))' "$OUT")"
item=$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["items"][0])' "$OUT")
assert_contains "$item" "'job': 'operator'"
assert_contains "$item" "'source': 'check'"
assert_contains "$item" "actions/runs/555"
assert_contains "$item" "FAIL TestThing"

it "the tail is cut at the bound, so a huge log cannot become the work list"
setup; runs_file; add_run operator failure
cat > "$BIN/gh" <<STUB
#!/usr/bin/env bash
case "\$*" in
  *"check-runs"*) cat "$RUNS" ;;
  *"--log-failed"*) for i in \$(seq 500); do echo "operator\tstep\tline \$i"; done ;;
  *) : ;;
esac
STUB
chmod +x "$BIN/gh"
run_it --tail-lines 5 >/dev/null
assert_equals "5" "$(python3 -c 'import json,sys;print(len(json.load(open(sys.argv[1]))["items"][0]["tail"].splitlines()))' "$OUT")"

# --- what is excluded --------------------------------------------------------

it "a failed check outside ci-green's needs is not the loop's: the review's own job is excluded"
setup; runs_file
add_run consolidate failure; add_run read failure; add_run operator success
stub_checks "$RUNS"
run_it >/dev/null
assert_equals "[]" "$(read_out items)"

it "ci-green itself is never an item: it is the aggregate, not the cause"
setup; runs_file
add_run ci-green failure; add_run operator success
stub_checks "$RUNS"
run_it >/dev/null
assert_equals "[]" "$(read_out items)"

it "a cancelled or timed-out run is not a verdict about the tree"
setup; runs_file
add_run operator cancelled; add_run chart timed_out; add_run images skipped
stub_checks "$RUNS"
run_it >/dev/null
assert_equals "[]" "$(read_out items)"

it "a matrix job's check run is matched to the job ci-green names"
setup; runs_file
add_run "images (manager)" failure
stub_checks "$RUNS"
run_it >/dev/null
assert_equals "1" "$(python3 -c 'import json,sys;print(len(json.load(open(sys.argv[1]))["items"]))' "$OUT")"
assert_contains "$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["items"][0]["job"])' "$OUT")" "images (manager)"

# --- not yet reported --------------------------------------------------------

it "CI not yet reported is NOT consulted, and NOT an empty green"
setup; runs_file
add_run consolidate success
stub_checks "$RUNS"
run_it >/dev/null
assert_equals "False" "$(read_out consulted)"
assert_equals "[]" "$(read_out items)"

# A RUN HALF-WAY THROUGH IS NOT A VERDICT EITHER. Some required jobs have
# reported and some have not; reading that as consulted lets a round say the
# checks are clean while the job that was going to fail has not spoken yet.
it "a partially reported run is NOT consulted: every required job must have spoken"
setup; runs_file
add_run operator success; add_run chart success      # `images` has not reported
stub_checks "$RUNS"
run_it >/dev/null
assert_equals "False" "$(read_out consulted)"

it "every required job reporting IS consulted, whatever they concluded"
setup; runs_file
add_run operator success; add_run chart success; add_run images failure
stub_checks "$RUNS"
run_it >/dev/null
assert_equals "True" "$(read_out consulted)"
assert_equals "1" "$(python3 -c 'import json,sys;print(len(json.load(open(sys.argv[1]))["items"]))' "$OUT")"

it "the checks API refusing is not consulted either, and never a crash"
setup; runs_file
cat > "$BIN/gh" <<'STUB'
#!/usr/bin/env bash
echo "gh: HTTP 403" >&2
exit 1
STUB
chmod +x "$BIN/gh"
out=$(run_it); assert_status 0 "$?"
assert_equals "False" "$(read_out consulted)"

# --- the required list is READ, never copied ---------------------------------

it "the required jobs come from ci-green's needs: a gate added there is required at once"
setup; runs_file
add_run operator failure
stub_checks "$RUNS"
python3 - "$DIR/ci.yml" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1])
p.write_text(p.read_text().replace("needs: [operator, chart, images]", "needs: [chart, images]"))
PY
run_it >/dev/null
# `operator` is no longer required, so its failure is not the loop's.
assert_equals "[]" "$(read_out items)"

# --- a check run that has not concluded is not a verdict ---------------------
#
# A check run EXISTS from the moment its run starts, queued. On #259 every required
# job had one 38 seconds into a ten-minute CI run, the checks read as consulted, and
# the round said "0 failures" over jobs that had not spoken. Consulted means every
# required check run has COMPLETED.

it "a queued or in-progress required check run is NOT consulted, whatever the others say"
setup; runs_file
add_run operator success; add_run chart success; add_run images "" in_progress
stub_checks "$RUNS"
run_it >/dev/null
assert_equals "False" "$(read_out consulted)"
assert_equals "[]" "$(read_out items)"

it "a queued run beside a FAILED one is still not consulted: the red is reported, the verdict is not claimed"
setup; runs_file
add_run operator failure; add_run chart "" queued; add_run images success
stub_checks "$RUNS"
run_it >/dev/null
assert_equals "False" "$(read_out consulted)"
assert_equals "1" "$(python3 -c 'import json,sys;print(len(json.load(open(sys.argv[1]))["items"]))' "$OUT")"

it "every required run completed IS consulted, and each entry carries its status"
setup; runs_file
add_run operator success; add_run chart success; add_run images success
stub_checks "$RUNS"
run_it >/dev/null
assert_equals "True" "$(read_out consulted)"
assert_contains "$(read_out checks)" "'status': 'completed'"

# --- the loop's own guard is no longer a check, so no red is exempt -----------
#
# A `docs-task` that failed only on the dispute guard used to be reported as WAITING,
# apart from the work. The guard left CI (#259): a red no fixer could clear stopped the
# loop on its own refusal. Every failed required check is work now, whatever step failed.

it "a failed docs-task is WORK whatever step failed: there is no waiting list and no jobs-route read"
setup; runs_file
python3 - "$DIR/ci.yml" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1]); p.write_text(p.read_text().replace("needs: [operator, chart, images]", "needs: [operator, docs-task]"))
PY
add_run operator success; add_run docs-task failure
stub_checks "$RUNS"
out=$(run_it); assert_status 0 "$?"
assert_equals "1" "$(python3 -c 'import json,sys;print(len(json.load(open(sys.argv[1]))["items"]))' "$OUT")"
assert_equals "['consulted', 'checks', 'items', 'awaiting']" "$(python3 -c 'import json,sys;print(list(json.load(open(sys.argv[1])).keys()))' "$OUT")"
assert_not_contains "$(cat "$DIR/calls")" "actions/jobs/"
assert_not_contains "$(cat "$S")" "guard_step"

# --- a check an earlier round disputed waits for the person ------------------
#
# A disputed check has no thread: its dispute is a pull request comment under the
# marker, naming each item in a hidden line. Red again on the next head, it is not
# disputed twice and not handed to the fixer; it is AWAITING, and the round's
# ending says so. A person's later comment answers it, and it is work again.

stub_checks_and_comments() {  # stub_checks_and_comments <runs-file> <comments-json>
  cat > "$BIN/gh" <<STUB
#!/usr/bin/env bash
printf '%s\n' "\$*" >> "$DIR/calls"
case "\$*" in
  *"check-runs"*) cat "$1" ;;
  *"issues/5/comments"*) printf '%s' '$2' ;;
  *"--log-failed"*) printf 'operator\tRun tests\tFAIL TestThing\n' ;;
  *) : ;;
esac
STUB
  chmod +x "$BIN/gh"
}
DISPUTE='<!-- conveyor:disputed -->\nThe fixing step disputes 1 failed check, for @x.\n\n- the operator check: a rate limit\n<!-- conveyor:disputed-item check:operator -->'

it "a red check an earlier round disputed, with no person's answer since, is AWAITING and not work"
setup; runs_file
add_run operator failure; add_run chart success; add_run images success
stub_checks_and_comments "$RUNS" "[{\"body\":\"$DISPUTE\",\"author\":{\"login\":\"github-actions\",\"__typename\":\"Bot\"}}]"
out=$(run_it); assert_status 0 "$?"
assert_equals "[]" "$(read_out items)"
assert_contains "$(read_out awaiting)" "'id': 'check:operator'"
assert_contains "$out" "awaiting operator: disputed by an earlier round, awaiting the person"

it "once a PERSON has commented after the dispute, the same red check is work again"
setup; runs_file
add_run operator failure; add_run chart success; add_run images success
stub_checks_and_comments "$RUNS" "[{\"body\":\"$DISPUTE\",\"author\":{\"login\":\"github-actions\",\"__typename\":\"Bot\"}},{\"body\":\"not a rate limit, the test is broken\",\"author\":{\"login\":\"maintainer\",\"__typename\":\"User\"}}]"
out=$(run_it); assert_status 0 "$?"
assert_equals "1" "$(python3 -c 'import json,sys;print(len(json.load(open(sys.argv[1]))["items"]))' "$OUT")"
assert_equals "[]" "$(read_out awaiting)"

it "a dispute naming ANOTHER check leaves this one work"
setup; runs_file
add_run images failure; add_run chart success; add_run operator success
stub_checks_and_comments "$RUNS" "[{\"body\":\"$DISPUTE\",\"author\":{\"login\":\"github-actions\",\"__typename\":\"Bot\"}}]"
run_it >/dev/null
assert_equals "1" "$(python3 -c 'import json,sys;print(len(json.load(open(sys.argv[1]))["items"]))' "$OUT")"
assert_equals "[]" "$(read_out awaiting)"

it "a dispute posted BEFORE --since (a removed label's loop) does not hold the check awaiting"
setup; runs_file
add_run operator failure; add_run chart success; add_run images success
stub_checks_and_comments "$RUNS" "[{\"body\":\"$DISPUTE\",\"created_at\":\"2026-09-28T10:00:00Z\",\"author\":{\"login\":\"github-actions\",\"__typename\":\"Bot\"}}]"
out=$(run_it --since 2026-09-28T11:00:00Z); assert_status 0 "$?"
assert_equals "1" "$(python3 -c 'import json,sys;print(len(json.load(open(sys.argv[1]))["items"]))' "$OUT")"
assert_equals "[]" "$(read_out awaiting)"

it "a dispute posted AFTER --since still holds the check awaiting"
setup; runs_file
add_run operator failure; add_run chart success; add_run images success
stub_checks_and_comments "$RUNS" "[{\"body\":\"$DISPUTE\",\"created_at\":\"2026-09-28T12:00:00Z\",\"author\":{\"login\":\"github-actions\",\"__typename\":\"Bot\"}}]"
out=$(run_it --since 2026-09-28T11:00:00Z); assert_status 0 "$?"
assert_equals "[]" "$(read_out items)"
assert_contains "$(read_out awaiting)" "'id': 'check:operator'"

summary
