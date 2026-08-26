#!/usr/bin/env bash
# The step that holds `contents: write` for a dispatch, and runs no model.
#
# WHAT IS ASSERTED IS THE ORDER AND THE REFUSALS. A reply naming a commit that
# was not yet pushed is a claim; a thread resolved for a patch that did not
# land is a thread resolved on nothing. The git half is REAL — a throwaway
# repository with a bare origin — because `git apply` and `git push` are the
# behaviour under test. Only `gh` is stubbed.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/land-dispatch.py"

tmp=$(mktemp -d)
export GH_CALLS="$tmp/calls" GH_FIXTURE="$tmp/threads.json" ORIGIN="$tmp/origin.git"

# A `gh` that, on every thread reply, records what origin's branch pointed at
# AT THAT MOMENT. That is how "push before reply" becomes observable.
mkdir -p "$tmp/bin"
cat > "$tmp/bin/gh" <<'STUB'
#!/usr/bin/env bash
case "$*" in
  "api repos/"*"/replies"*)
    printf '%s @origin=%s\n' "$*" "$(git -C "$ORIGIN" rev-parse --short "$BRANCH" 2>/dev/null || echo none)" >> "$GH_CALLS" ;;
  *) printf '%s\n' "$*" >> "$GH_CALLS" ;;
esac
case "$*" in
  "api graphql"*) cat "$GH_FIXTURE" ;;
esac
exit 0
STUB
chmod +x "$tmp/bin/gh"; export PATH="$tmp/bin:$PATH"
export BRANCH="change/thing"

# The resolver's view of the threads: both are the review's and open.
cat > "$GH_FIXTURE" <<'JSON'
{"data":{"repository":{"pullRequest":{"reviewThreads":{
  "pageInfo":{"hasNextPage":false,"endCursor":null},
  "nodes":[
    {"id":"PRRT_a","isResolved":false,"isOutdated":false,"path":"a.go",
     "comments":{"nodes":[{"author":{"login":"claude","__typename":"Bot"}}]}},
    {"id":"PRRT_b","isResolved":false,"isOutdated":false,"path":"b.go",
     "comments":{"nodes":[{"author":{"login":"claude","__typename":"Bot"}}]}},
    {"id":"PRRT_human","isResolved":false,"isOutdated":false,"path":"c.go",
     "comments":{"nodes":[{"author":{"login":"a-maintainer","__typename":"User"}}]}}
  ]}}}}}
JSON

# A repository with a bare origin, a branch, and two files with a finding each.
fresh_repo() {
  rm -rf "$ORIGIN" "$tmp/work"
  git init -q --bare "$ORIGIN"
  git init -q -b master "$tmp/work"
  git -C "$tmp/work" config user.email t@example.com
  git -C "$tmp/work" config user.name T
  printf 'package a\n\nfunc A() {}\n' > "$tmp/work/a.go"
  printf 'package b\n\nfunc B() {}\n' > "$tmp/work/b.go"
  git -C "$tmp/work" add . && git -C "$tmp/work" commit -qm seed
  git -C "$tmp/work" checkout -q -b "$BRANCH"
  git -C "$tmp/work" remote add origin "$ORIGIN"
  git -C "$tmp/work" push -q origin master "$BRANCH"
}

# A patch that fixes a.go only.
fresh_repo
printf 'package a\n\nfunc A() int { return 1 }\n' > "$tmp/work/a.go"
git -C "$tmp/work" diff > "$tmp/fix.patch"
git -C "$tmp/work" checkout -q -- a.go

cat > "$tmp/work.json" <<'JSON'
[{"threadId":"PRRT_a","commentId":11,"path":"a.go","line":3,"finding":"A returns nothing.","acceptedBy":"a-maintainer","reply":"fix it"},
 {"threadId":"PRRT_b","commentId":22,"path":"b.go","line":3,"finding":"B is unused.","acceptedBy":"a-maintainer","reply":"fix it"}]
JSON
printf '{"fixed":["PRRT_a"],"unfixed":[{"threadId":"PRRT_b","reason":"B is exported and used by the tests"}]}' > "$tmp/report.json"

land() { : > "$GH_CALLS"; rm -f "$tmp/work/.resolve-threads"
         (cd "$tmp/work" && python3 "$S" --repo o/r --pr 7 --branch "$BRANCH" \
            --work-list "$tmp/work.json" --patch "${PATCH:-$tmp/fix.patch}" --report "${REPORT:-$tmp/report.json}" \
            --dispatched-by a-maintainer 2>&1); }

out=$(land); rc=$?

it "applies the patch and pushes one commit to the branch"
assert_status 0 "$rc"
assert_equals "1" "$(git -C "$ORIGIN" rev-list --count master.."$BRANCH")"

it "names the findings in the commit"
msg=$(git -C "$ORIGIN" log -1 --format=%B "$BRANCH")
assert_contains "$msg" "fix(review): address 1 accepted review finding"
assert_contains "$msg" "a.go:3 — A returns nothing."

it "replies in the fixed thread naming the commit"
sha=$(git -C "$ORIGIN" rev-parse --short "$BRANCH")
assert_contains "$(cat "$GH_CALLS")" "comments/11/replies -f body=Fixed in $sha"

# THE ORDERING. At the moment of the reply, origin already held the commit.
it "replies only AFTER the push landed"
assert_contains "$(grep 'comments/11/replies' "$GH_CALLS")" "@origin=$sha"

it "answers the unfixed thread with the reason, and leaves it open"
assert_contains "$(cat "$GH_CALLS")" "comments/22/replies -f body=Not addressed by the dispatch (B is exported and used by the tests). Left open."

it "says on the pull request that a token push starts no CI"
assert_contains "$(cat "$GH_CALLS")" "pr comment 7 --repo o/r --body Dispatch by @a-maintainer: $sha addresses 1 accepted finding, 1 left open."
assert_contains "$(cat "$GH_CALLS")" "CI and the review have NOT run on it"

it "hands only the fixed thread to the resolver"
assert_equals "PRRT_a" "$(cat "$tmp/work/.resolve-threads")"

it "resolves after replying, through resolve-review-threads.py"
assert_contains "$out" "resolved PRRT_a"
assert_equals "1" "$(grep -c resolveReviewThread "$GH_CALLS")"
reply_line=$(grep -n 'comments/11/replies' "$GH_CALLS" | cut -d: -f1)
resolve_line=$(grep -n resolveReviewThread "$GH_CALLS" | cut -d: -f1)
[ "$reply_line" -lt "$resolve_line" ] && pass || fail "resolve ran before the reply"

# A REPORTED FIX THE PATCH DOES NOT CONTAIN is a claim, and the file is the
# check: the model said b.go was fixed, and nothing in the patch touches b.go.
it "does not trust a fix the patch does not contain"
fresh_repo
printf '{"fixed":["PRRT_a","PRRT_b"],"unfixed":[]}' > "$tmp/report2.json"
out=$(REPORT="$tmp/report2.json" land)
assert_contains "$out" "unfixed  PRRT_b (b.go): reported fixed, but the patch does not touch \`b.go\`"
assert_equals "PRRT_a" "$(cat "$tmp/work/.resolve-threads")"

it "drops a reported thread the work list does not carry"
fresh_repo
printf '{"fixed":["PRRT_a","PRRT_human"],"unfixed":[]}' > "$tmp/report3.json"
out=$(REPORT="$tmp/report3.json" land)
assert_contains "$out" "DROPPED  PRRT_human"
assert_not_contains "$(cat "$GH_CALLS")" "PRRT_human"

# A STALE PATCH RESOLVES NOTHING. The branch moved: a.go changed under the fix.
it "refuses a patch that no longer applies, pushes nothing, resolves nothing, and says so"
fresh_repo
printf 'package a\n\n// moved\nfunc A() string { return "" }\n' > "$tmp/work/a.go"
git -C "$tmp/work" commit -qam "moved" && git -C "$tmp/work" push -q origin "$BRANCH"
before=$(git -C "$ORIGIN" rev-parse "$BRANCH")
out=$(land); rc=$?
assert_status 1 "$rc"
assert_equals "$before" "$(git -C "$ORIGIN" rev-parse "$BRANCH")"
assert_contains "$(cat "$GH_CALLS")" "pr comment 7 --repo o/r --body Dispatch by @a-maintainer: the patch no longer applies"
assert_not_contains "$(cat "$GH_CALLS")" "resolveReviewThread"
assert_not_contains "$(cat "$GH_CALLS")" "/replies"

it "with an empty patch and nothing fixed, commits nothing and says every finding is still open"
fresh_repo
: > "$tmp/empty.patch"
printf '{"fixed":[],"unfixed":[{"threadId":"PRRT_a","reason":"could not"}]}' > "$tmp/report4.json"
out=$(PATCH="$tmp/empty.patch" REPORT="$tmp/report4.json" land); rc=$?
assert_status 0 "$rc"
assert_equals "0" "$(git -C "$ORIGIN" rev-list --count master.."$BRANCH")"
assert_contains "$(cat "$GH_CALLS")" "nothing landed. Every accepted finding is still open"
assert_contains "$(cat "$GH_CALLS")" "\`b.go\`: not addressed by the fixing step"

it "with nothing accepted, writes nothing and says so"
fresh_repo
printf '[]' > "$tmp/none.json"
out=$(cd "$tmp/work" && : > "$GH_CALLS" && python3 "$S" --repo o/r --pr 7 --branch "$BRANCH" \
        --work-list "$tmp/none.json" --patch "$tmp/fix.patch" --report "$tmp/report.json" 2>&1); rc=$?
assert_status 0 "$rc"
assert_equals "0" "$(git -C "$ORIGIN" rev-list --count master.."$BRANCH")"
assert_contains "$(cat "$GH_CALLS")" "no finding is accepted, so nothing was written"

it "refuses, loudly, when an input is absent rather than empty"
out=$(cd "$tmp/work" && python3 "$S" --repo o/r --pr 7 --branch "$BRANCH" \
        --work-list "$tmp/work.json" --patch "$tmp/missing.patch" --report "$tmp/report.json" 2>&1); rc=$?
assert_status 1 "$rc"
assert_contains "$out" "ABSENT"

# THE RESOLVER'S REFUSAL HOLDS THROUGH THIS PATH TOO: hand it a thread a person
# authored and it refuses, whatever this program believed.
it "cannot resolve a person's thread even when the work list claims it"
fresh_repo
printf 'package b\n\nfunc B() int { return 2 }\n' > "$tmp/work/b.go"
printf 'package c\n' > "$tmp/work/c.go"
git -C "$tmp/work" add c.go; git -C "$tmp/work" diff --cached > "$tmp/c.patch"; git -C "$tmp/work" reset -q; rm "$tmp/work/c.go"; git -C "$tmp/work" checkout -q -- b.go
cat > "$tmp/work5.json" <<'JSON'
[{"threadId":"PRRT_human","commentId":33,"path":"c.go","line":1,"finding":"a person's remark","acceptedBy":"x","reply":"fix it"}]
JSON
printf '{"fixed":["PRRT_human"],"unfixed":[]}' > "$tmp/report5.json"
out=$(cd "$tmp/work" && : > "$GH_CALLS" && python3 "$S" --repo o/r --pr 7 --branch "$BRANCH" \
        --work-list "$tmp/work5.json" --patch "$tmp/c.patch" --report "$tmp/report5.json" 2>&1)
assert_contains "$out" "REFUSED  PRRT_human: authored by 'a-maintainer', not the review"
assert_not_contains "$(cat "$GH_CALLS")" "resolveReviewThread"

it "contains no model invocation of its own"
assert_not_contains "$(cat "$S")" "claude"

rm -rf "$tmp"
summary
