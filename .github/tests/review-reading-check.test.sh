#!/usr/bin/env bash
# The program that decides whether a component reading is usable. A reading
# that passes here is consolidated; one that fails is the named `unreviewed`
# gap. Both directions are asserted, because a validator that accepts prose
# hands the coordinator a component that was never read.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/review-reading-check.py"

tmp=$(mktemp -d)
good='{"component":"docs","findings":[{"path":"docs/a.md","line":3,"claim":"says the old name","where":["docs/a.md:3"],"rule":"","fix":""}],"changedNames":["FOO_BAR"],"threads":[{"id":"PRRT_1","verdict":"fixed"}]}'

check() {  # check <envelope-json> ; prints exit status
  printf '%s' "$1" > "$tmp/env.json"
  python3 "$S" "$tmp/env.json" --group docs --out "$tmp/out/reading.json" >/dev/null 2>"$tmp/err"
  echo $?
}

it "a valid reading in structured_output passes and is written"
rm -rf "$tmp/out"
assert_status 0 "$(check "{\"type\":\"result\",\"structured_output\":$good}")"
assert_contains "$(cat "$tmp/out/reading.json")" '"FOO_BAR"'

it "a valid reading as the whole result text passes"
assert_status 0 "$(check "{\"type\":\"result\",\"result\":$(python3 -c 'import json,sys;print(json.dumps(sys.argv[1]))' "$good")}")"

it "JSON embedded in prose is extracted"
assert_status 0 "$(check "{\"result\":$(python3 -c 'import json,sys;print(json.dumps("Here is my reading:\n"+sys.argv[1]+"\nDone."))' "$good")}")"

it "prose fails"
rm -rf "$tmp/out"
assert_status 1 "$(check '{"result":"The docs component looks fine to me."}')"
[ ! -e "$tmp/out/reading.json" ] && pass || fail "a failed reading must leave no file"
assert_contains "$(cat "$tmp/err")" "::error::"

it "a missing key fails"
assert_status 1 "$(check '{"structured_output":{"component":"docs","findings":[],"threads":[]}}')"
assert_contains "$(cat "$tmp/err")" "missing key: changedNames"

it "a bad verdict fails"
assert_status 1 "$(check '{"structured_output":{"component":"docs","findings":[],"changedNames":[],"threads":[{"id":"PRRT_1","verdict":"resolved"}]}}')"
assert_contains "$(cat "$tmp/err")" "verdict"

it "a finding without a line fails"
assert_status 1 "$(check '{"structured_output":{"component":"docs","findings":[{"path":"docs/a.md","claim":"x"}],"changedNames":[],"threads":[]}}')"

it "a reading naming another component is recorded under the group it was asked for"
assert_status 0 "$(check '{"structured_output":{"component":"other","findings":[],"changedNames":[],"threads":[]}}')"
assert_contains "$(cat "$tmp/out/reading.json")" '"component": "docs"'

it "a merged component reading keeps its files and unread lists"
merged='{"component":"docs","findings":[],"changedNames":["A -> B"],"files":[{"path":"docs/a.md","declares":["A -> B"],"references":["C"]}],"threads":[],"unread":["docs/b.md"]}'
assert_status 0 "$(check "{\"structured_output\":$merged}")"
assert_contains "$(cat "$tmp/out/reading.json")" '"references"'
assert_contains "$(cat "$tmp/out/reading.json")" '"unread"'
assert_contains "$(cat "$tmp/out/reading.json")" '"A -> B"'

it "a malformed files entry fails"
assert_status 1 "$(check '{"structured_output":{"component":"docs","findings":[],"changedNames":[],"files":[{"path":"docs/a.md","declares":"A"}],"threads":[]}}')"
assert_contains "$(cat "$tmp/err")" "files[0].declares"

it "an unreadable envelope fails"
printf 'not json' > "$tmp/env.json"
python3 "$S" "$tmp/env.json" --group docs --out "$tmp/out/r.json" >/dev/null 2>&1
assert_status 1 "$?"

rm -rf "$tmp"
summary
