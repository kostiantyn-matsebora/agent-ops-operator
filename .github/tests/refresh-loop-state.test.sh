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

it "no thread open (exit 0) leaves the label untouched"
out=$(CLEAN_EXIT=0 run); rc=$?
assert_status 0 "$rc"
assert_not_contains "$(cat "$GH_CALLS")" "issue edit"
assert_contains "$out" "leaving the loop label as it is"

it "a missing helper script is a notice, never a failure"
mv "$tmp/repo/.github/scripts/review-not-clean.py" "$tmp/repo/.github/scripts/review-not-clean.py.bak"
out=$(run); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "::notice::"
mv "$tmp/repo/.github/scripts/review-not-clean.py.bak" "$tmp/repo/.github/scripts/review-not-clean.py"

summary
