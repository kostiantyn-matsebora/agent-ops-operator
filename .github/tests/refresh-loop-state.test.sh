#!/usr/bin/env bash
# STATE, NEVER A CHECK'S VERDICT. refresh-loop-state.py sends the loop machine
# `thread:opened` when a review thread is open (measured on #226: `mergeable`
# for an hour after three findings landed), and it must never fail the gate that
# calls it. WHAT THAT EVENT DOES TO A LABEL IS THE MACHINE'S TABLE, not this
# program's: it moves `mergeable` to `stalled`, and leaves `running` and the rest.
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
  "issue view "*"--json labels"*) [ -z "${GH_READ_FAILS:-}" ] || exit 7; printf '%s\n' ${CURRENT_LABELS:-} ;;
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
cp "$ROOT/.github/scripts/conveyor-state.py" "$ROOT/.github/scripts/conveyor.py" "$tmp/repo/.github/scripts/"
cp "$ROOT/.github/review-triage.json" "$tmp/repo/.github/"

run() { : > "$GH_CALLS"; (cd "$tmp/repo" && python3 "$S" --repo o/r --pr 226 2>&1); }
edits() { grep -c '^issue edit' "$GH_CALLS"; }

it "a thread open on a MERGEABLE pull request stalls the label"
out=$(CURRENT_LABELS="conveyor:fix loop:mergeable" CLEAN_EXIT=1 run); rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$GH_CALLS")" "issue edit 226 --repo o/r --add-label loop:stalled"
assert_contains "$out" "sent thread:opened to the loop machine"

it "a round is running: the machine leaves the label to that round's own ending"
out=$(CURRENT_LABELS="conveyor:fix loop:running" CLEAN_EXIT=1 run); rc=$?
assert_status 0 "$rc"
assert_equals "0" "$(edits)"
assert_contains "$out" "does not move it"

it "every other loop state is left as it is, since only a mergeable label can have gone stale"
for state in loop:stalled loop:capped ""; do
  out=$(CURRENT_LABELS="conveyor:fix $state" CLEAN_EXIT=1 run)
  assert_equals "0" "$(edits)"
done

it "no thread open: nothing is written and the label is not even read"
out=$(CURRENT_LABELS="loop:mergeable" CLEAN_EXIT=0 run); rc=$?
assert_status 0 "$rc"
assert_equals "0" "$(edits)"
assert_contains "$out" "no review thread open"

it "the labels cannot be read: the machine's writer stops, and nothing is guessed"
out=$(GH_READ_FAILS=1 CLEAN_EXIT=1 run); rc=$?
assert_status 0 "$rc"
assert_equals "0" "$(edits)"
assert_contains "$out" "the labels could not be read"

it "the thread check crashes (not 0 or 1): leaves the label, says so accurately"
out=$(CURRENT_LABELS="loop:mergeable" CLEAN_EXIT=2 run); rc=$?
assert_status 0 "$rc"
assert_equals "0" "$(edits)"
assert_contains "$out" "could not read the review threads"

it "a missing helper script is a notice, never a failed gate"
mv "$tmp/repo/.github/scripts/review-not-clean.py" "$tmp/rnc.bak"
out=$(run); rc=$?
mv "$tmp/rnc.bak" "$tmp/repo/.github/scripts/review-not-clean.py"
assert_status 0 "$rc"
assert_contains "$out" "a helper script is missing"

it "conveyor-state.py itself failing to run reports the label was NOT corrected, never claims success"
mv "$tmp/repo/.github/scripts/conveyor-state.py" "$tmp/repo/.github/scripts/conveyor-state.py.bak"
cat > "$tmp/repo/.github/scripts/conveyor-state.py" <<'PY'
import sys
sys.exit(9)
PY
out=$(CLEAN_EXIT=1 run); rc=$?
mv "$tmp/repo/.github/scripts/conveyor-state.py.bak" "$tmp/repo/.github/scripts/conveyor-state.py"
assert_status 0 "$rc"
assert_contains "$out" "the label was NOT corrected"

summary
