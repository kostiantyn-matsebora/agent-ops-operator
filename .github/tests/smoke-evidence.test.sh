#!/usr/bin/env bash
# The lookup that decides whether a release provisions a cluster smoke.
#
# WHAT IS ASSERTED IS THE CLASSIFICATION AND THE WAIT. A release publishes on
# the strength of this answer, so "smoked" must mean a check run with
# conclusion success actually exists for THIS commit, an in-flight one must be
# waited for rather than raced, and any failure on the `gh` side must resolve
# to "run one" -- never to "smoked" on missing evidence. `gh` is stubbed, so
# the suite touches no network and no token.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/smoke-evidence.py"

tmp=$(mktemp -d)
export GH_CALLS="$tmp/calls" PAGES="$tmp/pages"
mkdir -p "$tmp/bin" "$PAGES"

# A `gh api .../check-runs` stub that records every call and answers from a
# per-page fixture file: $PAGES/<page>.json, or an empty page past the last one.
cat > "$tmp/bin/gh" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$GH_CALLS"
if [ -n "${GH_API_FAILS:-}" ]; then
  echo "gh: connection reset" >&2
  exit 1
fi
if [ -n "${GH_API_HANGS:-}" ]; then
  sleep 5
fi
page=1
for a in "$@"; do
  case "$a" in page=*) page="${a#page=}" ;; esac
done
f="$PAGES/$page.json"
if [ -f "$f" ]; then cat "$f"; else echo '{"check_runs":[]}'; fi
STUB
chmod +x "$tmp/bin/gh"; export PATH="$tmp/bin:$PATH"

reset() { : > "$GH_CALLS"; rm -f "$PAGES"/*.json; unset GH_API_FAILS GH_API_HANGS; }
run() { python3 "$S" --repo o/r --sha deadbeef "$@" 2>"$tmp/err"; }

# --- one page, each classification -----------------------------------------

reset
cat > "$PAGES/1.json" <<'JSON'
{"check_runs":[{"name":"smoke / e2e / smoke","status":"completed","conclusion":"success"}]}
JSON
it "a passed smoke check run on the commit: smoked=true"
out=$(run); rc=$?
assert_status 0 "$rc"
assert_equals "smoked=true" "$out"

reset
cat > "$PAGES/1.json" <<'JSON'
{"check_runs":[]}
JSON
it "no check runs at all: smoked=false, run one"
out=$(run); rc=$?
assert_status 0 "$rc"
assert_equals "smoked=false" "$out"

reset
cat > "$PAGES/1.json" <<'JSON'
{"check_runs":[{"name":"smoke / e2e / smoke","status":"completed","conclusion":"failure"}]}
JSON
it "only a failed smoke: smoked=false, run one"
out=$(run); rc=$?
assert_status 0 "$rc"
assert_equals "smoked=false" "$out"

reset
cat > "$PAGES/1.json" <<'JSON'
{"check_runs":[{"name":"other-check","status":"completed","conclusion":"success"}]}
JSON
it "a passed check run that is not a smoke (name does not end in 'e2e / smoke'): smoked=false"
out=$(run); rc=$?
assert_status 0 "$rc"
assert_equals "smoked=false" "$out"

# --- in-flight: waits, then re-classifies -----------------------------------

reset
cat > "$PAGES/1.json" <<'JSON'
{"check_runs":[{"name":"smoke / e2e / smoke","status":"in_progress"}]}
JSON
it "an in-flight smoke, then success on re-check: waits and reports smoked"
(
  # Flip the fixture to success WELL BEFORE the second poll fires, so the
  # margin (0.2s write vs. a 2s poll interval) makes the ordering
  # deterministic rather than a race between two equal sleeps.
  sleep 0.2
  cat > "$PAGES/1.json" <<'JSON'
{"check_runs":[{"name":"smoke / e2e / smoke","status":"completed","conclusion":"success"}]}
JSON
) &
bgpid=$!
out=$(run --wait-minutes 1 --poll-seconds 2); rc=$?
wait "$bgpid" 2>/dev/null
assert_status 0 "$rc"
assert_equals "smoked=true" "$out"
assert_contains "$(cat "$tmp/err")" "waiting"

reset
cat > "$PAGES/1.json" <<'JSON'
{"check_runs":[{"name":"smoke / e2e / smoke","status":"in_progress"}]}
JSON
it "an in-flight smoke that never resolves before the bound: smoked=false"
out=$(run --wait-minutes 0 --poll-seconds 1); rc=$?
assert_status 0 "$rc"
assert_equals "smoked=false" "$out"
assert_contains "$(cat "$tmp/err")" "still running"

it "the final wait does not OVERSHOOT the deadline by a full poll-seconds"
# --wait-minutes cannot go below one full minute via the CLI, so this
# genuinely exercises wall-clock: a 60s bound against a 55s poll leaves ~5s
# after the first sleep, and the fix caps the SECOND sleep to that remainder
# rather than sleeping another 55s past the bound. Slow (~65s) but exact.
started=$(date +%s)
out=$(run --wait-minutes 1 --poll-seconds 55); rc=$?
elapsed=$(( $(date +%s) - started ))
assert_status 0 "$rc"
assert_equals "smoked=false" "$out"
[ "$elapsed" -lt 75 ] && pass || fail "took ${elapsed}s; expected under 75s (60s bound + one 55s poll would be 115s if uncapped)"

reset
cat > "$PAGES/1.json" <<'JSON'
{"check_runs":[{"name":"smoke / e2e / smoke","status":"in_progress"}]}
JSON
it "an in-flight smoke that finishes FAILED before the bound: smoked=false, run our own"
(
  sleep 0.2
  cat > "$PAGES/1.json" <<'JSON'
{"check_runs":[{"name":"smoke / e2e / smoke","status":"completed","conclusion":"failure"}]}
JSON
) &
bgpid=$!
out=$(run --wait-minutes 1 --poll-seconds 2); rc=$?
wait "$bgpid" 2>/dev/null
assert_status 0 "$rc"
assert_equals "smoked=false" "$out"

# --- gh failure: never smoked on missing evidence ---------------------------

reset
export GH_API_FAILS=1
it "gh unreachable: smoked=false, never smoked on missing evidence"
out=$(run); rc=$?
unset GH_API_FAILS
assert_status 0 "$rc"
assert_equals "smoked=false" "$out"
assert_contains "$(cat "$tmp/err")" "lookup failed"

it "gh MISSING entirely (not merely failing): smoked=false, never a crash"
# An EMPTY PATH, so subprocess.run(["gh", ...]) raises FileNotFoundError --
# python3 itself is invoked by absolute path, so this exercises "gh is not
# installed" rather than "gh failed", which the GH_API_FAILS case above does.
empty_path_dir=$(mktemp -d)
out=$(PATH="$empty_path_dir" $(command -v python3) "$S" --repo o/r --sha deadbeef 2>"$tmp/err"); rc=$?
rmdir "$empty_path_dir"
assert_status 0 "$rc"
assert_equals "smoked=false" "$out"
assert_contains "$(cat "$tmp/err")" "lookup failed"

reset
export GH_API_HANGS=1
it "gh HANGS (not merely failing): smoked=false after --api-timeout, never blocks past it"
started=$(date +%s)
out=$(run --api-timeout 1); rc=$?
elapsed=$(( $(date +%s) - started ))
unset GH_API_HANGS
assert_status 0 "$rc"
assert_equals "smoked=false" "$out"
assert_contains "$(cat "$tmp/err")" "lookup failed"
it "  ...and did not wait for the stub's full 5s sleep"
[ "$elapsed" -lt 5 ] && pass || fail "took ${elapsed}s, expected under 5s (api-timeout was 1s)"

# --- pagination --------------------------------------------------------------

reset
python3 - "$PAGES" <<'PY'
import json, sys
page = {"check_runs": [{"name": f"check-{i}", "status": "completed", "conclusion": "success"} for i in range(100)]}
open(f"{sys.argv[1]}/1.json", "w").write(json.dumps(page))
PY
cat > "$PAGES/2.json" <<'JSON'
{"check_runs":[{"name":"smoke / e2e / smoke","status":"completed","conclusion":"success"}]}
JSON
it "pages through check runs (100 on page 1) to find the smoke on page 2"
out=$(run); rc=$?
assert_status 0 "$rc"
assert_equals "smoked=true" "$out"
assert_contains "$(cat "$GH_CALLS")" "page=2"

# THE INVOCATION SHAPE ITSELF. `gh api <path> -f k=v` with no `--method GET`
# sends the `-f` params as a POST-style request body on some routes, and this
# route answers a bodied GET with 404 rather than the list — a bug that
# reached a live release run once, silently, because every stubbed `gh` here
# answers any invocation identically and could not have caught it.
it "invokes gh api with --method GET, not the ambiguous default"
reset
cat > "$PAGES/1.json" <<'JSON'
{"check_runs":[]}
JSON
run >/dev/null
assert_contains "$(cat "$GH_CALLS")" "api --method GET repos/o/r/commits/deadbeef/check-runs"

rm -rf "$tmp"
summary
