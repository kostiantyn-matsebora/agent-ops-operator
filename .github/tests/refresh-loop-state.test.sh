#!/usr/bin/env bash
# STATE, NEVER A CHECK'S VERDICT. refresh-loop-state.py corrects
# loop:mergeable when it has gone stale (measured on #226), and it must
# never fail the gate that calls it.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/refresh-loop-state.py"

tmp=$(mktemp -d)
export GH_CALLS="$tmp/calls" CLEAN_EXIT="0"
mkdir -p "$tmp/bin" "$tmp/repo/.github/scripts"
cat > "$tmp/bin/gh" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$GH_CALLS"
case "$*" in
  "pr view "*"--json labels"*) printf '%s' "${CURRENT_LABELS:-}" ;;
esac
exit 0
STUB
chmod +x "$tmp/bin/gh"; export PATH="$tmp/bin:$PATH"

# review-not-clean.py stands in for the real check: exits per $CLEAN_EXIT.
cat > "$tmp/repo/.github/scripts/review-not-clean.py" <<'PY'
import os, sys
open(os.environ["GH_CALLS"], "a").write("review-not-clean.py " + " ".join(sys.argv[1:]) + "\n")
sys.exit(int(os.environ.get("CLEAN_EXIT", "0")))
PY
cp "$ROOT/.github/scripts/conveyor-state.py" "$tmp/repo/.github/scripts/"
cp "$ROOT/.github/review-triage.json" "$tmp/repo/.github/"

run() { : > "$GH_CALLS"; (cd "$tmp/repo" && python3 "$S" --repo o/r --pr 226 2>&1); }

it "a review thread open (exit 1) corrects the label to stalled"
out=$(CLEAN_EXIT=1 run); rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$GH_CALLS")" "issue edit 226 --repo o/r --add-label loop:stalled"
assert_contains "$out" "corrected the loop label to stalled"

it "a round is already running (loop:running): the label is left to that round's own ending, no thread check made"
out=$(CURRENT_LABELS="loop:running" CLEAN_EXIT=1 run); rc=$?
assert_status 0 "$rc"
assert_not_contains "$(cat "$GH_CALLS")" "review-not-clean.py"
assert_not_contains "$(cat "$GH_CALLS")" "issue edit"
assert_contains "$out" "a round is currently running"

it "loop:running alongside other labels is still detected, not just as the sole label"
out=$(CURRENT_LABELS=$'conveyor:fix\nloop:running\nopsx:review' CLEAN_EXIT=1 run); rc=$?
assert_status 0 "$rc"
assert_not_contains "$(cat "$GH_CALLS")" "issue edit"

it "no thread open (exit 0) leaves the label untouched"
out=$(CLEAN_EXIT=0 run); rc=$?
assert_status 0 "$rc"
assert_not_contains "$(cat "$GH_CALLS")" "issue edit"
assert_contains "$out" "leaving the loop label as it is"

it "a check-script crash (not exit 1) is UNKNOWN, never treated as an open thread"
cat > "$tmp/repo/.github/scripts/review-not-clean.py" <<'PY'
import sys
sys.exit(2)
PY
out=$(run); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "::notice::"
assert_contains "$out" "could not read"
assert_not_contains "$out" "corrected the loop label to stalled"
assert_not_contains "$(cat "$GH_CALLS")" "issue edit"
cat > "$tmp/repo/.github/scripts/review-not-clean.py" <<'PY'
import os, sys
open(os.environ["GH_CALLS"], "a").write("review-not-clean.py " + " ".join(sys.argv[1:]) + "\n")
sys.exit(1 if os.environ.get("CLEAN_EXIT") == "1" else 0)
PY

it "a missing helper script is a notice, never a failure"
mv "$tmp/repo/.github/scripts/review-not-clean.py" "$tmp/repo/.github/scripts/review-not-clean.py.bak"
out=$(run); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "::notice::"
mv "$tmp/repo/.github/scripts/review-not-clean.py.bak" "$tmp/repo/.github/scripts/review-not-clean.py"

it "a thread open, but conveyor-state.py itself fails to run: reports it was NOT corrected, never claims success"
mv "$tmp/repo/.github/scripts/conveyor-state.py" "$tmp/repo/.github/scripts/conveyor-state.py.bak"
cat > "$tmp/repo/.github/scripts/conveyor-state.py" <<'PY'
import sys
sys.exit(1)
PY
out=$(CLEAN_EXIT=1 run); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "::notice::"
assert_contains "$out" "was NOT corrected"
assert_not_contains "$out" "corrected the loop label to stalled"
mv "$tmp/repo/.github/scripts/conveyor-state.py.bak" "$tmp/repo/.github/scripts/conveyor-state.py"

summary
