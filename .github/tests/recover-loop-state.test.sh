#!/usr/bin/env bash
# recover-loop-state.py corrects `loop:running` left stale by a
# review-dispatch run superseded before it ever ran `gate` -- measured on
# #243, where two runs completed `cancelled` with zero jobs and nothing
# moved the label off a round that had already ended. It must never fail
# the workflow that calls it.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/recover-loop-state.py"

tmp=$(mktemp -d)
export GH_CALLS="$tmp/calls" CLEAN_EXIT="0" ROUND_COMMENTS='[]'
mkdir -p "$tmp/bin" "$tmp/repo/.github/scripts"
cat > "$tmp/bin/gh" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$GH_CALLS"
case "$*" in
  "pr view "*"--json labels"*|"issue view "*"--json labels"*) printf '%s\n' ${CURRENT_LABELS:-loop:running} ;;
  "api repos/o/r/issues/226/comments --paginate") printf '%s' "$ROUND_COMMENTS" ;;
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
cat > "$tmp/repo/.github/review-triage.json" <<'JSON'
{"round_marker": "<!-- conveyor:round", "grant_marker": "<!-- conveyor:grant -->", "max_rounds": 3,
 "loop_labels": {"running": "loop:running", "stalled": "loop:stalled", "capped": "loop:capped", "mergeable": "loop:mergeable"}}
JSON

round_comment() { printf '{"created_at":"%s","body":"<!-- conveyor:round %s -->x"}' "$1" "$2"; }

run() { : > "$GH_CALLS"
        (cd "$tmp/repo" && python3 "$S" --repo o/r --pr 226 --since 2026-01-01T00:00:00Z \
          --vocabulary .github/review-triage.json "$@" 2>&1); }

it "not the superseded shape (--only-if-superseded, but conclusion is not cancelled): does nothing"
out=$(run --only-if-superseded --run-conclusion success --run-job-count 0); rc=$?
assert_status 0 "$rc"
assert_equals "" "$(cat "$GH_CALLS")"
assert_contains "$out" "not a superseded"

it "not the superseded shape (--only-if-superseded, but jobs > 0): does nothing"
out=$(run --only-if-superseded --run-conclusion cancelled --run-job-count 3); rc=$?
assert_status 0 "$rc"
assert_equals "" "$(cat "$GH_CALLS")"

it "the superseded shape (cancelled, zero jobs): proceeds to check state"
out=$(CLEAN_EXIT=0 run --only-if-superseded --run-conclusion cancelled --run-job-count 0); rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$GH_CALLS")" "pr view 226"

it "loop label is not running: nothing to recover"
out=$(CURRENT_LABELS="loop:mergeable" run); rc=$?
assert_status 0 "$rc"
assert_not_contains "$(cat "$GH_CALLS")" "issue edit"
assert_contains "$out" "not \`running\`"

it "rounds used REACH the cap: corrects to capped, without even checking threads"
ROUND_COMMENTS="[$(round_comment 2026-01-02T00:00:00Z 1),$(round_comment 2026-01-02T00:01:00Z 2),$(round_comment 2026-01-02T00:02:00Z 3)]" \
  out=$(run); rc=$?
assert_status 0 "$rc"
assert_not_contains "$(cat "$GH_CALLS")" "review-not-clean.py"
assert_contains "$(cat "$GH_CALLS")" "issue edit 226 --repo o/r --add-label loop:capped"
assert_contains "$out" "sent recover:capped to the loop machine"

it "rounds used EXCEED the cap: corrects to capped too"
ROUND_COMMENTS="[$(round_comment 2026-01-02T00:00:00Z 1),$(round_comment 2026-01-02T00:01:00Z 2),$(round_comment 2026-01-02T00:02:00Z 3),$(round_comment 2026-01-02T00:03:00Z 4)]" \
  out=$(run); rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$GH_CALLS")" "issue edit 226 --repo o/r --add-label loop:capped"
assert_contains "$out" "sent recover:capped to the loop machine"

it "rounds within the cap, a thread is open: corrects to stalled"
ROUND_COMMENTS="[$(round_comment 2026-01-02T00:00:00Z 1)]" out=$(CLEAN_EXIT=1 run); rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$GH_CALLS")" "issue edit 226 --repo o/r --add-label loop:stalled"
assert_contains "$out" "sent recover:stalled to the loop machine"

it "rounds within the cap, no thread open: leaves running -- a round may genuinely be in flight"
ROUND_COMMENTS="[$(round_comment 2026-01-02T00:00:00Z 1)]" out=$(CLEAN_EXIT=0 run); rc=$?
assert_status 0 "$rc"
assert_not_contains "$(cat "$GH_CALLS")" "issue edit"
assert_contains "$out" "leaving \`running\` as it is"

it "a grant comment extends the cap, so a round count under the grown cap is not treated as capped"
ROUND_COMMENTS="[$(round_comment 2026-01-02T00:00:00Z 1),$(round_comment 2026-01-02T00:01:00Z 2),$(round_comment 2026-01-02T00:02:00Z 3),
  {\"created_at\":\"2026-01-02T00:02:30Z\",\"body\":\"<!-- conveyor:grant -->x\"},
  $(round_comment 2026-01-02T00:03:00Z 4)]" out=$(CLEAN_EXIT=0 run); rc=$?
assert_status 0 "$rc"
assert_not_contains "$(cat "$GH_CALLS")" "issue edit"

it "a check-script crash (not exit 1) is UNKNOWN, never treated as an open thread"
cat > "$tmp/repo/.github/scripts/review-not-clean.py" <<'PY'
import sys
sys.exit(2)
PY
ROUND_COMMENTS="[$(round_comment 2026-01-02T00:00:00Z 1)]" out=$(run); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "::notice::"
assert_not_contains "$out" "corrected the loop label"
assert_not_contains "$(cat "$GH_CALLS")" "issue edit"
cat > "$tmp/repo/.github/scripts/review-not-clean.py" <<'PY'
import os, sys
open(os.environ["GH_CALLS"], "a").write("review-not-clean.py " + " ".join(sys.argv[1:]) + "\n")
sys.exit(int(os.environ.get("CLEAN_EXIT", "0")))
PY

it "gh pr view fails to read the current labels: STOPS here"
cat > "$tmp/bin/gh" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$GH_CALLS"
case "$*" in
  "pr view "*"--json labels"*) exit 7 ;;
esac
exit 0
STUB
out=$(run); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "::notice::"
assert_contains "$out" "could not read the current labels"
cat > "$tmp/bin/gh" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$GH_CALLS"
case "$*" in
  "pr view "*"--json labels"*|"issue view "*"--json labels"*) printf '%s\n' ${CURRENT_LABELS:-loop:running} ;;
  "api repos/o/r/issues/226/comments --paginate") printf '%s' "$ROUND_COMMENTS" ;;
esac
exit 0
STUB

it "a missing helper script is a notice, never a failure"
mv "$tmp/repo/.github/scripts/review-not-clean.py" "$tmp/repo/.github/scripts/review-not-clean.py.bak"
out=$(run); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "::notice::"
mv "$tmp/repo/.github/scripts/review-not-clean.py.bak" "$tmp/repo/.github/scripts/review-not-clean.py"

it "capped, but conveyor-state.py itself fails to run: reports it was NOT corrected, never claims success"
mv "$tmp/repo/.github/scripts/conveyor-state.py" "$tmp/repo/.github/scripts/conveyor-state.py.bak"
cat > "$tmp/repo/.github/scripts/conveyor-state.py" <<'PY'
import sys
sys.exit(1)
PY
ROUND_COMMENTS="[$(round_comment 2026-01-02T00:00:00Z 1),$(round_comment 2026-01-02T00:01:00Z 2),$(round_comment 2026-01-02T00:02:00Z 3)]" \
  out=$(run); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "::notice::"
assert_contains "$out" "was NOT corrected"
mv "$tmp/repo/.github/scripts/conveyor-state.py.bak" "$tmp/repo/.github/scripts/conveyor-state.py"

summary
