#!/usr/bin/env bash
# STATE, NOT A GRANT. conveyor-state.py moves one of two label families to one
# value so a person can read where the line is. It must never fail the job
# that called it, and it must never leave two values of one family standing.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/conveyor-state.py"

tmp=$(mktemp -d)
export GH_CALLS="$tmp/calls"
mkdir -p "$tmp/bin"
cat > "$tmp/bin/gh" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$GH_CALLS"
if [ -n "${GH_FAILS:-}" ]; then echo "could not add label: 'station:fix' not found" >&2; exit 1; fi
exit 0
STUB
chmod +x "$tmp/bin/gh"; export PATH="$tmp/bin:$PATH"

run() { : > "$GH_CALLS"; python3 "$S" --repo o/r "$@" 2>&1; }

it "sets one station label and removes its four siblings in one edit"
out=$(run --target 51 --station fix); rc=$?
assert_status 0 "$rc"
assert_equals "1" "$(wc -l < "$GH_CALLS")"
call=$(cat "$GH_CALLS")
assert_contains "$call" "issue edit 51 --repo o/r --add-label station:fix"
for other in implement merge archive done; do assert_contains "$call" "--remove-label station:$other"; done
assert_not_contains "$call" "--remove-label station:fix"
assert_not_contains "$call" "loop:"

it "sets one loop label on a pull request the same way"
call=$(run --target 220 --loop stalled >/dev/null; cat "$GH_CALLS")
assert_contains "$call" "issue edit 220 --repo o/r --add-label loop:stalled"
for other in running capped mergeable; do assert_contains "$call" "--remove-label loop:$other"; done
assert_not_contains "$call" "station:"

it "reads the names from the vocabulary, never from its own text"
names=$(python3 -c 'import json;v=json.load(open("'"$ROOT"'/.github/review-triage.json"));print(" ".join(sorted(v["station_labels"].values())+sorted(v["loop_labels"].values())))')
assert_equals "station:archive station:done station:fix station:implement station:merge loop:capped loop:mergeable loop:running loop:stalled" "$names"

it "an unknown value is a notice and exit 0, and nothing is edited"
out=$(run --target 51 --station shipped); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "::notice::"
assert_equals "0" "$(wc -l < "$GH_CALLS")"

it "a failed edit (the label does not exist yet) is a notice and exit 0"
out=$(GH_FAILS=1 run --target 51 --station fix); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "::notice::conveyor state not recorded on #51"
assert_contains "$out" "not found"

it "refuses to take both families at once, and requires one"
python3 "$S" --repo o/r --target 1 --station fix --loop running >/dev/null 2>&1; assert_status 2 "$?"
python3 "$S" --repo o/r --target 1 >/dev/null 2>&1; assert_status 2 "$?"

summary
