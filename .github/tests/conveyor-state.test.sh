#!/usr/bin/env bash
# STATE, NOT A GRANT, AND ONLY EVENTS MOVE IT. conveyor-state.py reads the
# target's live label, asks the machine's table what an event does, and writes
# that, or writes nothing where the table skips. It must never fail the job
# that called it, and never leave two values of one family standing.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/conveyor-state.py"

tmp=$(mktemp -d)
export GH_CALLS="$tmp/calls" LABELS_ON="$tmp/labels"
mkdir -p "$tmp/bin"
cat > "$tmp/bin/gh" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$GH_CALLS"
case "$*" in
  "issue view "*) [ -z "${GH_READ_FAILS:-}" ] || { echo "rate limited" >&2; exit 1; }; tr ' ' '\n' < "$LABELS_ON" ;;
  "issue edit "*) if [ -n "${GH_FAILS:-}" ]; then echo "could not add label: 'station:fix' not found" >&2; exit 1; fi ;;
esac
exit 0
STUB
chmod +x "$tmp/bin/gh"; export PATH="$tmp/bin:$PATH"

has() { printf '%s' "$*" > "$LABELS_ON"; }
run() { : > "$GH_CALLS"; python3 "$S" --repo o/r "$@" 2>&1; }
edits() { grep -c '^issue edit' "$GH_CALLS"; }

it "an event the table maps writes ONE station label and removes its five siblings, in one edit"
has "opsx:review station:implement"
out=$(run --target 51 --station-event round:start); rc=$?
assert_status 0 "$rc"
assert_equals "1" "$(edits)"
call=$(grep '^issue edit' "$GH_CALLS")
assert_contains "$call" "issue edit 51 --repo o/r --add-label station:fix"
for other in implement merge stalled archive done; do assert_contains "$call" "--remove-label station:$other"; done
assert_not_contains "$call" "--remove-label station:fix"
assert_not_contains "$call" "loop:"
assert_contains "$out" "was implement, round:start"

it "the same for a loop event on a pull request: one label added, its three siblings removed"
has "conveyor:fix loop:stalled"
run --target 220 --loop-event round:start >/dev/null
call=$(grep '^issue edit' "$GH_CALLS")
assert_contains "$call" "issue edit 220 --repo o/r --add-label loop:running"
for other in stalled capped mergeable; do assert_contains "$call" "--remove-label loop:$other"; done
assert_not_contains "$call" "station:"

it "a target with no state label starts from none"
has "conveyor:fix"
run --target 220 --loop-event ci:green_clean >/dev/null
assert_contains "$(grep '^issue edit' "$GH_CALLS")" "--add-label loop:mergeable"

it "an event the table SKIPS writes nothing: a running round keeps its label against a green CI on an older head"
has "conveyor:fix loop:running"
out=$(run --target 220 --loop-event ci:green_clean); rc=$?
assert_status 0 "$rc"
assert_equals "0" "$(edits)"
assert_contains "$out" "does not move it"

it "done is terminal: no station event moves it"
for e in fire:implement fire:archive round:start pr:green merge:proposal_or_apply merge:finished_carried merge:finished_stalled merge:plain merge:archive_pr; do
  has "opsx:archived station:done"
  run --target 51 --station-event "$e" >/dev/null
  assert_equals "0" "$(edits)"
done

it "an implement session never pulls a change back from the archive station"
has "station:archive"
run --target 51 --station-event fire:implement >/dev/null
assert_equals "0" "$(edits)"

it "a target somehow showing two values is read as the most advanced one"
has "station:implement station:merge"
run --target 51 --station-event merge:finished_carried >/dev/null
assert_contains "$(grep '^issue edit' "$GH_CALLS")" "--add-label station:archive"

it "reads the names from the vocabulary, never from its own text"
names=$(python3 -c 'import json;v=json.load(open("'"$ROOT"'/.github/review-triage.json"));print(" ".join(sorted(v["station_labels"].values())+sorted(v["loop_labels"].values())))')
assert_equals "station:archive station:done station:fix station:implement station:merge station:stalled loop:capped loop:mergeable loop:running loop:stalled" "$names"

it "an unknown event is a notice and exit 0, and nothing is edited"
has "station:fix"
out=$(run --target 51 --station-event shipped); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "::notice::"
assert_equals "0" "$(edits)"

it "labels that cannot be READ are a notice and exit 0, never a guess"
has ""
out=$(GH_READ_FAILS=1 run --target 51 --station-event round:start); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "the labels could not be read"
assert_equals "0" "$(edits)"

it "a failed edit (the label does not exist yet) is a notice and exit 0"
has ""
out=$(GH_FAILS=1 run --target 51 --station-event round:start); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "::notice::conveyor state not recorded on #51"
assert_contains "$out" "not found"

it "there is no way to name a value: only events are accepted, exactly one, and one is required"
python3 "$S" --repo o/r --target 1 --station fix >/dev/null 2>&1; assert_status 2 "$?"
python3 "$S" --repo o/r --target 1 --loop running >/dev/null 2>&1; assert_status 2 "$?"
python3 "$S" --repo o/r --target 1 --station-event round:start --loop-event round:start >/dev/null 2>&1; assert_status 2 "$?"
python3 "$S" --repo o/r --target 1 >/dev/null 2>&1; assert_status 2 "$?"

summary
