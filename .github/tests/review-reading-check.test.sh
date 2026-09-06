#!/usr/bin/env bash
# The program that decides whether a reading is usable, and merges a
# directory of file and verdict readings into one component reading. A
# reading that passes is consolidated; one that fails is a named gap.
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

# --- the unbuilt shape (--kind component, the default) ---------------------

it "the unbuilt shape is valid on its own"
assert_status 0 "$(check '{"structured_output":{"component":"docs","unbuilt":"go build failed","findings":[],"changedNames":[],"files":[],"threads":[],"unread":["docs/a.md"]}}')"
assert_contains "$(cat "$tmp/out/reading.json")" '"unbuilt": "go build failed"'

it "unbuilt beside a non-empty list fails"
assert_status 1 "$(check '{"structured_output":{"component":"docs","unbuilt":"x","findings":[{"path":"docs/a.md","line":1,"claim":"y"}],"changedNames":[],"files":[],"threads":[],"unread":[]}}')"
assert_contains "$(cat "$tmp/err")" "unbuilt is set alongside non-empty"

# --- the file shape (--kind file), the blind reader's own return -----------

it "a valid file reading passes"
printf '%s' '{"structured_output":{"path":"docs/a.md","findings":[],"declares":["+X"],"references":["Y"]}}' > "$tmp/f.json"
python3 "$S" "$tmp/f.json" --group docs --kind file --out "$tmp/file-out.json" >/dev/null 2>"$tmp/ferr"
assert_status 0 "$?"
assert_contains "$(cat "$tmp/file-out.json")" '"path": "docs/a.md"'

it "a file reading with a threads key is still accepted (extra keys ignored) but a missing declares fails"
printf '%s' '{"structured_output":{"path":"docs/a.md","findings":[],"references":["Y"]}}' > "$tmp/f2.json"
python3 "$S" "$tmp/f2.json" --group docs --kind file --out "$tmp/file-out2.json" >/dev/null 2>"$tmp/ferr2"
assert_status 1 "$?"
assert_contains "$(cat "$tmp/ferr2")" "missing key: declares"

# --- the verdict shape (--kind verdict) -------------------------------------

it "a fixed/standing/gone verdict passes with no finding"
printf '%s' '{"structured_output":{"threads":[{"id":"PRRT_1","verdict":"fixed"},{"id":"PRRT_2","verdict":"standing"}]}}' > "$tmp/v.json"
python3 "$S" "$tmp/v.json" --group docs --kind verdict --out "$tmp/v-out.json" >/dev/null 2>"$tmp/verr"
assert_status 0 "$?"

it "a detached verdict REQUIRES a finding"
printf '%s' '{"structured_output":{"threads":[{"id":"PRRT_1","verdict":"detached"}]}}' > "$tmp/v2.json"
python3 "$S" "$tmp/v2.json" --group docs --kind verdict --out "$tmp/v-out2.json" >/dev/null 2>"$tmp/verr2"
assert_status 1 "$?"
assert_contains "$(cat "$tmp/verr2")" "detached but carries no finding"

it "a detached verdict WITH a finding passes"
printf '%s' '{"structured_output":{"threads":[{"id":"PRRT_1","verdict":"detached","finding":{"path":"docs/a.md","line":9,"claim":"moved"}}]}}' > "$tmp/v3.json"
python3 "$S" "$tmp/v3.json" --group docs --kind verdict --out "$tmp/v-out3.json" >/dev/null 2>"$tmp/verr3"
assert_status 0 "$?"

it "a non-detached verdict carrying a finding fails"
printf '%s' '{"structured_output":{"threads":[{"id":"PRRT_1","verdict":"fixed","finding":{"path":"docs/a.md","line":9,"claim":"x"}}]}}' > "$tmp/v4.json"
python3 "$S" "$tmp/v4.json" --group docs --kind verdict --out "$tmp/v-out4.json" >/dev/null 2>"$tmp/verr4"
assert_status 1 "$?"
assert_contains "$(cat "$tmp/verr4")" "must carry no finding"

# --- merge mode: a directory of file-*.json / verdict-*.json ---------------

it "merge combines file readings, folds a detached verdict's finding into findings, and names unread by comparing against --paths"
mkdir -p "$tmp/merge"
echo '{"path":"docs/a.md","findings":[{"path":"docs/a.md","line":1,"claim":"a problem"}],"declares":["+X"],"references":[]}' > "$tmp/merge/file-a.json"
echo '{"path":"docs/b.md","findings":[],"declares":[],"references":["X"]}' > "$tmp/merge/file-b.json"
echo '{"threads":[{"id":"PRRT_1","verdict":"detached","finding":{"path":"docs/b.md","line":2,"claim":"moved claim"}}]}' > "$tmp/merge/verdict-b.json"
python3 "$S" "$tmp/merge" --group docs --paths "docs/a.md,docs/b.md,docs/c.md" --out "$tmp/merged.json"
assert_status 0 "$?"
m=$(cat "$tmp/merged.json")
assert_contains "$m" '"component": "docs"'
assert_contains "$m" '"a problem"'
assert_contains "$m" '"moved claim"'
assert_contains "$m" '"id": "PRRT_1"'
assert_contains "$m" '"unread": [
  "docs/c.md"
 ]'

it "a corrupt file-*.json in the merge directory is skipped, not fatal"
echo 'not json' > "$tmp/merge/file-corrupt.json"
python3 "$S" "$tmp/merge" --group docs --paths "docs/a.md,docs/b.md" --out "$tmp/merged2.json"
assert_status 0 "$?"

rm -rf "$tmp"
summary
