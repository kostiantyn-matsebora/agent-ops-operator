#!/usr/bin/env bash
# THE HALF `open` AND `archive` SHARE. Which issue a pull request names, which
# station carries from which reference shape, and the state labels set at the
# moments this program knows them. `gh` and carry-grant.py are stubbed: the
# grant's own logic has carry-grant.test.sh.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/carry-from-pr.sh"

tmp=$(mktemp -d)
export GH_CALLS="$tmp/calls" PR_FILE="$tmp/pr.json" BODY_FILE="$tmp/body" CARRY_OUT="$tmp/carry.out"
mkdir -p "$tmp/bin" "$tmp/repo/.github/scripts"
cat > "$tmp/bin/gh" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$GH_CALLS"
case "$*" in
  "pr view "*"--json isCrossRepository,headRefName"*) cat "$PR_FILE" ;;
  "pr view "*"--json body"*)                          cat "$BODY_FILE" ;;
esac
exit 0
STUB
chmod +x "$tmp/bin/gh"; export PATH="$tmp/bin:$PATH"
# carry-grant.py is REPLACED by a recorder that prints what the test says, and
# review-not-clean.py by one that answers CLEAN (exit 0) or not, as the test says.
cat > "$tmp/repo/.github/scripts/carry-grant.py" <<'PY'
import os, sys
open(os.environ["GH_CALLS"], "a").write("carry-grant.py " + " ".join(sys.argv[1:]) + "\n")
print(open(os.environ["CARRY_OUT"]).read())
PY
export THREADS_OPEN=""
cat > "$tmp/repo/.github/scripts/review-not-clean.py" <<'PY'
import os, sys
open(os.environ["GH_CALLS"], "a").write("review-not-clean.py " + " ".join(sys.argv[1:]) + "\n")
sys.exit(1 if os.environ.get("THREADS_OPEN") else 0)
PY
cp "$ROOT/.github/scripts/conveyor-state.py" "$tmp/repo/.github/scripts/"
cp "$ROOT/.github/review-triage.json" "$tmp/repo/.github/"

run() { : > "$GH_CALLS"; (cd "$tmp/repo" && GITHUB_REPOSITORY=o/r CI_CONCLUSION="${CI_CONCLUSION-success}" bash "$S" "$@" 2>&1); }
same_repo() { printf 'false change/thing\n' > "$PR_FILE"; }

it "fix from a Refs pull request: carries fix onto the pull request, then -- no review thread open -- marks it mergeable and the issue at merge"
same_repo; printf 'Summary.\n\nRefs #51\n' > "$BODY_FILE"; printf 'carried conveyor:run (from maintainer) to conveyor:fix on pr #220\n' > "$CARRY_OUT"
out=$(run 220 fix); rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$GH_CALLS")" "carry-grant.py --repo o/r --issue 51 --station fix --pr 220"
assert_contains "$(cat "$GH_CALLS")" "review-not-clean.py --repo o/r --pr 220"
assert_contains "$(cat "$GH_CALLS")" "issue edit 220 --repo o/r --add-label loop:mergeable"
assert_contains "$(cat "$GH_CALLS")" "issue edit 51 --repo o/r --add-label station:merge"
assert_contains "$out" "carried conveyor:run"

it "fix after a RED ci (the open job runs on failure too, since #221): the carry happens, and NO mergeable or merge state is set"
out=$(CI_CONCLUSION=failure run 220 fix); rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$GH_CALLS")" "carry-grant.py --repo o/r --issue 51 --station fix --pr 220"
assert_not_contains "$(cat "$GH_CALLS")" "review-not-clean.py"
assert_not_contains "$(cat "$GH_CALLS")" "loop:mergeable"
assert_not_contains "$(cat "$GH_CALLS")" "station:merge"
assert_contains "$out" "ci concluded 'failure'"

it "fix with no conclusion handed in at all marks nothing"
out=$(CI_CONCLUSION= run 220 fix); rc=$?
assert_status 0 "$rc"
assert_not_contains "$(cat "$GH_CALLS")" "loop:mergeable"
assert_not_contains "$(cat "$GH_CALLS")" "station:merge"

it "fix when the threads cannot be read (review-not-clean.py crashes) is a notice, not an open finding, and marks nothing"
cat > "$tmp/repo/.github/scripts/review-not-clean.py" <<'PY'
import sys; sys.exit(2)
PY
out=$(run 220 fix); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "::notice::could not read #220's review threads (exit 2)"
assert_not_contains "$out" "has a review thread open"
assert_not_contains "$(cat "$GH_CALLS")" "loop:mergeable"
cat > "$tmp/repo/.github/scripts/review-not-clean.py" <<'PY'
import os, sys
open(os.environ["GH_CALLS"], "a").write("review-not-clean.py " + " ".join(sys.argv[1:]) + "\n")
sys.exit(1 if os.environ.get("THREADS_OPEN") else 0)
PY

it "fix with a review thread still open: the carry happens, and NO mergeable or merge state is set"
out=$(THREADS_OPEN=1 run 220 fix); rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$GH_CALLS")" "carry-grant.py --repo o/r --issue 51 --station fix --pr 220"
assert_not_contains "$(cat "$GH_CALLS")" "loop:mergeable"
assert_not_contains "$(cat "$GH_CALLS")" "station:merge"
assert_contains "$out" "has a review thread open"

it "fix from a Closes pull request (the archive one): carries fix onto it too, from the same issue"
same_repo; printf 'Archives the change.\n\nCloses #51\n' > "$BODY_FILE"
out=$(run 230 fix); rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$GH_CALLS")" "carry-grant.py --repo o/r --issue 51 --station fix --pr 230"
assert_contains "$(cat "$GH_CALLS")" "issue edit 230 --repo o/r --add-label loop:mergeable"

it "fix from a pull request naming no issue: nothing carried, nothing marked"
same_repo; printf 'No reference at all.\n' > "$BODY_FILE"
out=$(run 231 fix); rc=$?
assert_status 0 "$rc"
assert_not_contains "$(cat "$GH_CALLS")" "carry-grant.py"
assert_not_contains "$(cat "$GH_CALLS")" "issue edit"
assert_contains "$out" "names no issue"

it "archive from a Refs pull request: carries archive to the issue"
same_repo; printf 'Refs #51\n' > "$BODY_FILE"; printf 'carried conveyor:run (from maintainer) to conveyor:archive on issue #51\n' > "$CARRY_OUT"
out=$(run 220 archive); rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$GH_CALLS")" "carry-grant.py --repo o/r --issue 51 --station archive"
assert_not_contains "$(cat "$GH_CALLS")" "station:done"

it "archive from a Closes pull request: the line ENDED at this merge -- station done, nothing carried"
same_repo; printf 'Closes #51\n' > "$BODY_FILE"
out=$(run 230 archive); rc=$?
assert_status 0 "$rc"
assert_not_contains "$(cat "$GH_CALLS")" "carry-grant.py"
assert_contains "$(cat "$GH_CALLS")" "issue edit 51 --repo o/r --add-label station:done"
assert_contains "$out" "the line ended at this merge"

it "archive from a plain-lane Refs pull request: carry-grant declines, and the station is done"
same_repo; printf 'Refs #60\n' > "$BODY_FILE"; printf '#60 is on the plain lane, which has no archive station; its line ended at the merge\n' > "$CARRY_OUT"
out=$(run 240 archive); rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$GH_CALLS")" "carry-grant.py --repo o/r --issue 60 --station archive"
assert_contains "$(cat "$GH_CALLS")" "issue edit 60 --repo o/r --add-label station:done"

it "a fork's pull request, or a non-change branch, carries nothing and marks nothing"
printf 'true change/thing\n' > "$PR_FILE"; printf 'Refs #51\n' > "$BODY_FILE"
out=$(run 250 fix); rc=$?
assert_status 0 "$rc"
assert_not_contains "$(cat "$GH_CALLS")" "carry-grant.py"
assert_not_contains "$(cat "$GH_CALLS")" "issue edit"
printf 'false fix/thing\n' > "$PR_FILE"
out=$(run 251 fix); assert_not_contains "$(cat "$GH_CALLS")" "carry-grant.py"

it "a bad station is usage, exit 64"
bash "$S" 1 merge >/dev/null 2>&1; assert_status 64 "$?"

summary
