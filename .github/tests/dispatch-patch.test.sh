#!/usr/bin/env bash
# The cut between the fixing job and the landing job: what crosses, and what the
# fixer left behind that must not.
#
# THE GIT HALF IS REAL. A throwaway repository, a tracked file the fixer changed,
# a new file it declared, and the scratch it did not -- because `git ls-files`,
# `git add -N` and `git diff --binary` are the behaviour under test. Measured on
# #243: six rounds each landed a helper script at the repository root.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/dispatch-patch.py"

tmp=$(mktemp -d)

fresh_repo() {
  rm -rf "$tmp/work"
  git init -q -b master "$tmp/work"
  git -C "$tmp/work" config user.email t@example.com
  git -C "$tmp/work" config user.name T
  printf 'package a\n\nfunc A() {}\n' > "$tmp/work/a.go"
  printf '*.out\n' > "$tmp/work/.gitignore"
  git -C "$tmp/work" add . && git -C "$tmp/work" commit -qm seed
}

cut() { (cd "$tmp/work" && python3 "$S" --report "${REPORT:-$tmp/report.json}" \
          --out "$tmp/fix.patch" --dropped "$tmp/dropped.json" 2>&1); }

# The fixer's working tree after a round: one tracked file changed, one new
# file it declared, two helpers it wrote to read its inputs, and the coverage
# profile a reproduced check leaves behind.
fixer_ran() {
  fresh_repo
  printf 'package a\n\nfunc A() int { return 1 }\n' > "$tmp/work/a.go"
  printf 'package a\n\nfunc TestA(t *testing.T) {}\n' > "$tmp/work/a_test.go"
  : > "$tmp/work/.tmp_getenv.py"
  printf 'cat "$WORK_LIST"\n' > "$tmp/work/.tmp_read_worklist.sh"
  printf 'mode: set\n' > "$tmp/work/coverage.out"
}

printf '{"items":[{"id":"x","action":"fixed","reason":""}],"created":["a_test.go"]}' > "$tmp/report.json"

it "a changed tracked file and a DECLARED new file cross; an undeclared new file does not"
fixer_ran
out=$(cut); rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$tmp/fix.patch")" "+++ b/a.go"
assert_contains "$(cat "$tmp/fix.patch")" "+++ b/a_test.go"
assert_not_contains "$(cat "$tmp/fix.patch")" ".tmp_getenv.py"
assert_not_contains "$(cat "$tmp/fix.patch")" ".tmp_read_worklist.sh"

it "the undeclared files are deleted from the checkout before the cut, and recorded"
[ ! -e "$tmp/work/.tmp_getenv.py" ] && [ ! -e "$tmp/work/.tmp_read_worklist.sh" ] && pass || fail "scratch still in the checkout"
assert_equals '[".tmp_getenv.py", ".tmp_read_worklist.sh"]' \
  "$(python3 -c 'import json;print(json.dumps(sorted(json.load(open("'"$tmp"'/dropped.json"))["dropped"])))')"
assert_contains "$out" "::warning::.tmp_getenv.py: created by the fixing step but not declared"

it "an ignored file is neither landed nor dropped -- git never staged it before either"
[ -f "$tmp/work/coverage.out" ] && pass || fail "the ignored file was deleted"
assert_not_contains "$(cat "$tmp/fix.patch")" "coverage.out"
assert_not_contains "$(cat "$tmp/dropped.json")" "coverage.out"

it "the patch applies to a clean checkout as the lander applies it, new file included"
fresh_repo
(cd "$tmp/work" && git apply --check "$tmp/fix.patch" && git apply --index "$tmp/fix.patch") && pass || fail "the patch does not apply"
assert_equals "A  a_test.go
M  a.go" "$(git -C "$tmp/work" status --porcelain | sort)"

it "a declared path is normalised as git names it, so ./a_test.go declares a_test.go"
fixer_ran
printf '{"items":[],"created":["./a_test.go"]}' > "$tmp/report-dot.json"
REPORT="$tmp/report-dot.json" cut >/dev/null
assert_contains "$(cat "$tmp/fix.patch")" "+++ b/a_test.go"

it "no report means nothing declared: every new file is dropped and the tracked change alone crosses"
fixer_ran
REPORT="$tmp/absent.json" cut >/dev/null
assert_contains "$(cat "$tmp/fix.patch")" "+++ b/a.go"
assert_not_contains "$(cat "$tmp/fix.patch")" "a_test.go"
assert_equals "3" "$(python3 -c 'import json;print(len(json.load(open("'"$tmp"'/dropped.json"))["dropped"]))')"

it "a report with no created list, or an unreadable one, declares nothing rather than failing the cut"
fixer_ran
printf '{"items":[]}' > "$tmp/report-none.json"
REPORT="$tmp/report-none.json" cut >/dev/null; rc=$?
assert_status 0 "$rc"
assert_not_contains "$(cat "$tmp/fix.patch")" "a_test.go"
fixer_ran
printf 'not json' > "$tmp/report-bad.json"
REPORT="$tmp/report-bad.json" cut >/dev/null; rc=$?
assert_status 0 "$rc"

it "a declared file that is not there is a warning, never a failure"
fresh_repo
printf '{"items":[],"created":["missing.go"]}' > "$tmp/report-missing.json"
out=$(REPORT="$tmp/report-missing.json" cut); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "::warning::missing.go: declared as created but not present"

it "a changed binary crosses whole: the diff is cut with --binary"
fresh_repo
printf '\x89PNG\000\r\n\x1a\n' > "$tmp/work/mark.png"
git -C "$tmp/work" add mark.png && git -C "$tmp/work" commit -qm png
printf '\x89PNG\000\r\n\x1a\nchanged' > "$tmp/work/mark.png"
printf '{"items":[]}' > "$tmp/report-none.json"
REPORT="$tmp/report-none.json" cut >/dev/null
assert_contains "$(cat "$tmp/fix.patch")" "GIT binary patch"

it "nothing changed is an empty patch and an empty dropped list, exit 0"
fresh_repo
out=$(REPORT="$tmp/report-none.json" cut); rc=$?
assert_status 0 "$rc"
assert_equals "0" "$(wc -c < "$tmp/fix.patch" | tr -d ' ')"
assert_equals '{"dropped": []}' "$(cat "$tmp/dropped.json")"

rm -rf "$tmp"

summary
