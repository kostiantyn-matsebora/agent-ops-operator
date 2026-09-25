#!/usr/bin/env bash
# THE CARRIES, END TO END OVER A STUBBED gh. carry.py gathers facts, asks the
# machine, and acts; the machine's own table of every fact combination is in
# conveyor.test.py, so this suite covers what only an adapter can get wrong:
# reading the pull request and the timeline, writing the labels and the ONE
# comment, the state events, and the step outputs the workflow reads.
#
# Every case that failed on #248, #254 and #255 is here as a case, and so is
# each way the adapter could disagree with the machine about a fact.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)

tmp=$(mktemp -d)
export GH_CALLS="$tmp/calls" FX="$tmp/fx" GITHUB_OUTPUT="$tmp/out" THREADS_OPEN=""
mkdir -p "$tmp/bin" "$FX" "$tmp/repo/.github/scripts"
cat > "$tmp/bin/gh" <<'STUB'
#!/usr/bin/env bash
call="${*//$'\n'/ }"; printf '%s\n' "$call" >> "$GH_CALLS"
n() { printf '%s' "$*" | awk '{print $3}'; }
case "$*" in
  "pr view "*"--json isCrossRepository"*) cat "$FX/pr.json" ;;
  "issue view "*"--json labels --jq"*) tr ' ' '\n' < "$FX/labels-$(n "$@")" ;;
  "issue view "*"--json labels"*) python3 -c 'import json,sys;print(json.dumps({"labels":[{"name":x} for x in open(sys.argv[1]).read().split()]}))' "$FX/labels-$(n "$@")" ;;
  "api repos/o/r/issues/"*"/timeline"*) cat "$FX/timeline.json" ;;
  "api repos/o/r/collaborators/"*"/permission"*) l=$(printf '%s' "$*" | sed 's#.*collaborators/\([^/]*\)/.*#\1#'); cat "$FX/perm-$l" 2>/dev/null || echo none ;;
  "api repos/o/r/issues/"*"/comments"*) cat "$FX/comments" 2>/dev/null ;;
  "run list "*) cat "$FX/review-runs" 2>/dev/null || echo 0 ;;
esac
exit 0
STUB
chmod +x "$tmp/bin/gh"; export PATH="$tmp/bin:$PATH"
for f in carry.py conveyor.py conveyor_io.py conveyor-state.py; do cp "$ROOT/.github/scripts/$f" "$tmp/repo/.github/scripts/"; done
cp "$ROOT/.github/review-triage.json" "$tmp/repo/.github/"
cat > "$tmp/repo/.github/scripts/review-not-clean.py" <<'PY'
import os, sys
sys.exit(1 if os.environ.get("THREADS_OPEN") else 0)
PY

pr() {  # pr <body> [branch] [cross] [state]
  python3 - "$(printf '%b' "$1")" "${2:-change/thing}" "${3:-false}" "${4:-OPEN}" > "$FX/pr.json" <<'PY'
import json, sys
print(json.dumps({"body": sys.argv[1], "headRefName": sys.argv[2], "isCrossRepository": sys.argv[3] == "true",
                  "state": sys.argv[4], "headRefOid": "abc123", "labels": [{"name": "conveyor:fix"}] if sys.argv[4] == "OPEN" else []}))
PY
}
issue_has() { printf '%s' "$2" > "$FX/labels-$1"; }               # issue_has <n> "<labels>"
placed() {                                                        # placed <login> <permission>
  python3 - "$1" > "$FX/timeline.json" <<'PY'
import json, sys
print(json.dumps([{"event": "labeled", "label": {"name": n}, "actor": {"login": sys.argv[1]}, "created_at": "2026-09-25T10:00:00Z"}
                  for n in ("conveyor:run", "conveyor:archive")]))
PY
  printf '%s' "$2" > "$FX/perm-$1"
}
change() {                                                        # change <issue> <tasks text>
  rm -rf "$tmp/repo/openspec"; mkdir -p "$tmp/repo/openspec/changes/thing"
  printf '%s\n' "$1" > "$tmp/repo/openspec/changes/thing/.github-issue"
  printf '%b' "$2" > "$tmp/repo/openspec/changes/thing/tasks.md"
}
no_change() { rm -rf "$tmp/repo/openspec"; }
FINISHED='## 1. x\n- [x] a\n- [x] b\n'
PENDING='## 1. x\n- [x] a\n- [ ] b\n'
run() { : > "$GH_CALLS"; : > "$GITHUB_OUTPUT"; rm -f "$FX/comments"; (cd "$tmp/repo" && python3 .github/scripts/carry.py --repo o/r "$@" 2>&1); }
edited() { grep -c "^issue edit $1 .*--add-label $2" "$GH_CALLS"; }
commented() { grep -c "^issue comment $1 " "$GH_CALLS"; }
output() { grep "^$1=" "$GITHUB_OUTPUT" | tail -1 | cut -d= -f2-; }
reset() { : > "$FX/comments"; }

# ==== the fix station ==========================================================================

it "fix: a Refs pull request under a standing conveyor:run is labelled, once, and a round is dispatched when the review already completed"
reset; pr "Refs #51"; issue_has 51 "conveyor:run opsx:review"; placed maintainer write; change 51 "$PENDING"; echo 1 > "$FX/review-runs"
out=$(run --pr 220 --station fix --ci-conclusion success)
assert_equals "1" "$(edited 220 conveyor:fix)"
assert_equals "1" "$(commented 220)"
assert_contains "$(grep '^issue comment 220' "$GH_CALLS")" "carrying @maintainer's standing instruction"
assert_equals "true" "$(output dispatch_round)"

it "fix: the state events follow: loop mergeable on the pull request, station merge on the issue, when CI passed with no thread open"
assert_contains "$(cat "$GH_CALLS")" "issue edit 220 --repo o/r --add-label loop:mergeable"
assert_contains "$(cat "$GH_CALLS")" "issue edit 51 --repo o/r --add-label station:merge"

it "fix: no round is dispatched while the review has not completed on the head: its own completion is the round"
reset; echo 0 > "$FX/review-runs"
out=$(run --pr 220 --station fix --ci-conclusion success)
assert_equals "1" "$(edited 220 conveyor:fix)"
assert_equals "false" "$(output dispatch_round)"

it "fix: CI red still carries the grant, and marks nothing mergeable"
reset; echo 1 > "$FX/review-runs"
out=$(run --pr 220 --station fix --ci-conclusion failure)
assert_equals "1" "$(edited 220 conveyor:fix)"
assert_not_contains "$(cat "$GH_CALLS")" "loop:mergeable"
assert_not_contains "$(cat "$GH_CALLS")" "station:merge"
assert_equals "true" "$(output dispatch_round)"

it "fix: a review thread open carries the grant but is not mergeable"
reset
out=$(THREADS_OPEN=1 run --pr 220 --station fix --ci-conclusion success)
assert_equals "1" "$(edited 220 conveyor:fix)"
assert_not_contains "$(cat "$GH_CALLS")" "loop:mergeable"

it "fix: MEASURED ON #254, the archive pull request (Closes) under conveyor:archive ALONE is carried"
reset; pr "Archives the change.\n\nCloses #51"; issue_has 51 "conveyor:archive opsx:archived"; placed maintainer write; no_change
out=$(run --pr 254 --station fix --ci-conclusion success)
assert_equals "1" "$(edited 254 conveyor:fix)"
assert_contains "$(grep '^issue comment 254' "$GH_CALLS")" "conveyor:archive"

it "fix: conveyor:archive alone is NOT a grant for an ordinary (Refs) pull request"
reset; pr "Refs #51"; issue_has 51 "conveyor:archive opsx:review"
out=$(run --pr 220 --station fix --ci-conclusion success)
assert_equals "0" "$(edited 220 conveyor:fix)"
assert_contains "$out" "carries no grant for this pull request's fix station"

it "fix: nothing is carried for a pull request that names no issue, or that is not a same-repository change/* branch"
reset; pr "No reference here"; out=$(run --pr 220 --station fix --ci-conclusion success)
assert_equals "0" "$(edited 220 conveyor:fix)"
pr "Refs #51" "feature/x"; out=$(run --pr 220 --station fix --ci-conclusion success)
assert_equals "0" "$(edited 220 conveyor:fix)"
pr "Refs #51" "change/thing" true; out=$(run --pr 220 --station fix --ci-conclusion success)
assert_equals "0" "$(edited 220 conveyor:fix)"

it "fix: a placer who lost write access carries nothing and leaves ONE access-lost comment, once"
reset; pr "Refs #51"; issue_has 51 "conveyor:run"; placed maintainer read
out=$(run --pr 220 --station fix --ci-conclusion success)
assert_equals "0" "$(edited 220 conveyor:fix)"
assert_equals "1" "$(commented 220)"
assert_contains "$(grep '^issue comment 220' "$GH_CALLS")" "access-lost"
: > "$FX/comments"; printf '<!-- carry-grant:fix:access-lost -->\n' > "$FX/comments"
: > "$GH_CALLS"; (cd "$tmp/repo" && python3 .github/scripts/carry.py --repo o/r --pr 220 --station fix --ci-conclusion success >/dev/null 2>&1)
assert_equals "0" "$(commented 220)"

it "fix: a timeline that shows nobody placing the grant carries nothing"
reset; echo '[]' > "$FX/timeline.json"; issue_has 51 "conveyor:run"
out=$(run --pr 220 --station fix --ci-conclusion success)
assert_equals "0" "$(edited 220 conveyor:fix)"

it "fix: the label is re-asserted every time, but the comment only once"
reset; placed maintainer write; printf '<!-- carry-grant:fix -->\nold\n' > "$FX/comments"
: > "$GH_CALLS"; (cd "$tmp/repo" && python3 .github/scripts/carry.py --repo o/r --pr 220 --station fix --ci-conclusion failure >/dev/null 2>&1)
assert_equals "1" "$(edited 220 conveyor:fix)"
assert_equals "0" "$(commented 220)"

# ==== the archive station ======================================================================

it "archive: a FINISHED change under a standing conveyor:run: the issue is labelled, the station recorded, and its archive session started"
reset; pr "Refs #51" "change/thing" false MERGED; issue_has 51 "conveyor:run opsx:review"; placed maintainer write; change 51 "$FINISHED"
out=$(run --pr 220 --station archive)
assert_equals "1" "$(edited 51 conveyor:archive)"
assert_contains "$(cat "$GH_CALLS")" "issue edit 51 --repo o/r --add-label station:archive"
assert_equals "51" "$(output fire_issue)"

it "archive: MEASURED ON #255, a merge of a change that is NOT finished carries nothing, and the line moves to implement"
reset; change 51 "$PENDING"
out=$(run --pr 220 --station archive)
assert_equals "0" "$(edited 51 conveyor:archive)"
assert_equals "" "$(output fire_issue)"
assert_contains "$(cat "$GH_CALLS")" "issue edit 51 --repo o/r --add-label station:implement"
assert_contains "$out" "not finished"

it "archive: finished, but no conveyor:run to carry: the merge station is stalled, and nothing fires"
reset; issue_has 51 "opsx:review"; change 51 "$FINISHED"
out=$(run --pr 220 --station archive)
assert_equals "0" "$(edited 51 conveyor:archive)"
assert_equals "" "$(output fire_issue)"
assert_contains "$(cat "$GH_CALLS")" "issue edit 51 --repo o/r --add-label station:stalled"

it "archive: finished, and conveyor:archive was placed directly: it already fired, so nothing is carried and nothing is stalled"
reset; issue_has 51 "conveyor:archive opsx:review"
out=$(run --pr 220 --station archive)
assert_equals "" "$(output fire_issue)"
assert_not_contains "$(cat "$GH_CALLS")" "station:stalled"

it "archive: the archive pull request's own merge (Closes only) ends the line: station done, nothing carried"
reset; pr "Archives it.\n\nCloses #51" "change/thing" false MERGED; issue_has 51 "opsx:archived"
out=$(run --pr 254 --station archive)
assert_equals "0" "$(edited 51 conveyor:archive)"
assert_contains "$(cat "$GH_CALLS")" "issue edit 51 --repo o/r --add-label station:done"

it "archive: the plain lane has no archive station: its line ends at the merge"
reset; pr "Refs #51" "change/thing" false MERGED; issue_has 51 "conveyor:run"; no_change
out=$(run --pr 220 --station archive)
assert_equals "0" "$(edited 51 conveyor:archive)"
assert_contains "$(cat "$GH_CALLS")" "issue edit 51 --repo o/r --add-label station:done"

it "archive: a placer who lost write access carries nothing and leaves one access-lost comment on the ISSUE"
reset; issue_has 51 "conveyor:run opsx:review"; placed maintainer read; change 51 "$FINISHED"
out=$(run --pr 220 --station archive)
assert_equals "0" "$(edited 51 conveyor:archive)"
assert_equals "1" "$(commented 51)"
assert_contains "$(grep '^issue comment 51' "$GH_CALLS")" "access-lost"

it "archive: a pull request with no Refs carries nothing"
reset; pr "Nothing here" "change/thing" false MERGED
out=$(run --pr 220 --station archive)
assert_equals "" "$(output fire_issue)"
assert_equals "0" "$(grep -c '^issue edit' "$GH_CALLS")"

it "a done line stays done: no later carry moves a closed issue's station"
reset; pr "Refs #51" "change/thing" false MERGED; issue_has 51 "station:done opsx:archived conveyor:run"; placed maintainer write; change 51 "$PENDING"
out=$(run --pr 220 --station archive)
assert_not_contains "$(cat "$GH_CALLS")" "add-label station:implement"

# ==== the interface ============================================================================

it "there is no way to ask for an unknown station, and a pull request number is required"
(cd "$tmp/repo" && python3 .github/scripts/carry.py --repo o/r --pr 1 --station merge >/dev/null 2>&1); assert_status 2 "$?"
(cd "$tmp/repo" && python3 .github/scripts/carry.py --repo o/r --station fix >/dev/null 2>&1); assert_status 2 "$?"

summary
