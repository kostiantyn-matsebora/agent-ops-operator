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
#   too eager  reporting the checks clean before CI has run on the head. Same
#              rule the analysis collector states: not reported is a FLAG.
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
add_run() {  # add_run <name> <conclusion>
  python3 - "$RUNS" "$1" "$2" <<'PY'
import json, sys
with open(sys.argv[1], "a") as f:
    f.write(json.dumps({
        "name": sys.argv[2], "conclusion": sys.argv[3],
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

# --- the loop's own guard is not work ----------------------------------------
#
# A `docs-task` that failed ONLY on the guard step is a dispute waiting for a
# person. Handing it to a fixer started rounds that could not answer it, and each
# round's red started the next (#248). It is reported as WAITING instead, read from
# the job's own failed steps -- and the step's name is read from ci.yml, never restated.

GUARD_STEP="No dispute the fixing loop posted waits unanswered"
guard_ci() {
  cat > "$DIR/ci.yml" <<YML
name: ci
on: [pull_request]
jobs:
  operator: {runs-on: ubuntu-latest, steps: []}
  docs-task:
    runs-on: ubuntu-latest
    steps:
      - name: Every change this pull request finishes ends in finished tasks
        run: python3 .github/scripts/docs-task-guard.py --range "\$RANGE"
      - name: $GUARD_STEP
        run: python3 .github/scripts/autofix-guard.py --repo "\$R" --pr "\$N" --purpose ci
  ci-green:
    needs: [operator, docs-task]
    runs-on: ubuntu-latest
    steps: []
YML
}
# `gh` answering check-runs from a fixture AND the jobs route with a step list per job id.
stub_checks_and_steps() {  # stub_checks_and_steps <runs-file> <steps-json-or-FAIL>
  cat > "$BIN/gh" <<STUB
#!/usr/bin/env bash
printf '%s\n' "\$*" >> "$DIR/calls"
case "\$*" in
  *"check-runs"*) cat "$1" ;;
  *"actions/jobs/"*) [ "$2" = FAIL ] && { echo "HTTP 502" >&2; exit 1; }; printf '%s' '$2' ;;
  *"--log-failed"*) printf 'docs-task\tstep\tboom\n' ;;
  *) : ;;
esac
STUB
  chmod +x "$BIN/gh"
}
add_run_id() {  # add_run_id <name> <conclusion> <id>
  python3 - "$RUNS" "$1" "$2" "$3" <<'PY'
import json, sys
with open(sys.argv[1], "a") as f:
    f.write(json.dumps({"id": int(sys.argv[4]), "name": sys.argv[2], "conclusion": sys.argv[3],
        "details_url": "https://github.com/o/r/actions/runs/555/job/" + sys.argv[4],
        "html_url": "https://github.com/o/r/actions/runs/555/job/" + sys.argv[4]}) + "\n")
PY
}

it "a docs-task that failed ONLY on the loop's own guard is WAITING, not work"
setup; guard_ci; runs_file
add_run_id operator success 1; add_run_id docs-task failure 2
stub_checks_and_steps "$RUNS" "[\"$GUARD_STEP\"]"
out=$(run_it); assert_status 0 "$?"
assert_equals "[]" "$(read_out items)"
waiting=$(read_out waiting)
assert_contains "$waiting" "docs-task"
assert_contains "$out" "waiting  docs-task"

it "a docs-task that failed on the TASKS step is work, as before"
setup; guard_ci; runs_file
add_run_id docs-task failure 2
stub_checks_and_steps "$RUNS" '["Every change this pull request finishes ends in finished tasks"]'
out=$(run_it); assert_status 0 "$?"
assert_equals "1" "$(python3 -c 'import json,sys;print(len(json.load(open(sys.argv[1]))["items"]))' "$OUT")"
assert_equals "[]" "$(read_out waiting)"

it "a docs-task that failed on BOTH steps is work: the tasks step is the fixer's to answer"
setup; guard_ci; runs_file
add_run_id docs-task failure 2
stub_checks_and_steps "$RUNS" "[\"Every change this pull request finishes ends in finished tasks\",\"$GUARD_STEP\"]"
out=$(run_it); assert_status 0 "$?"
assert_equals "1" "$(python3 -c 'import json,sys;print(len(json.load(open(sys.argv[1]))["items"]))' "$OUT")"

it "the jobs route failing is not a licence to wave the job through: it stays work"
setup; guard_ci; runs_file
add_run_id docs-task failure 2
stub_checks_and_steps "$RUNS" FAIL
out=$(run_it); assert_status 0 "$?"
assert_equals "1" "$(python3 -c 'import json,sys;print(len(json.load(open(sys.argv[1]))["items"]))' "$OUT")"
assert_equals "[]" "$(read_out waiting)"

it "the guard step's name is READ from ci.yml: a reworded step is still recognised"
setup; guard_ci; runs_file
python3 - "$DIR/ci.yml" "$GUARD_STEP" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1]); p.write_text(p.read_text().replace(sys.argv[2], "A dispute the loop posted has no answer"))
PY
add_run_id docs-task failure 2
stub_checks_and_steps "$RUNS" '["A dispute the loop posted has no answer"]'
out=$(run_it); assert_status 0 "$?"
assert_equals "[]" "$(read_out items)"
assert_contains "$(read_out waiting)" "docs-task"

it "ci.yml with no guard step means nothing is ever waiting"
setup; runs_file
add_run_id operator failure 1
stub_checks_and_steps "$RUNS" '["anything"]'
out=$(run_it); assert_status 0 "$?"
assert_equals "[]" "$(read_out waiting)"

summary
