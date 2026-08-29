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

# --mode all: THE LABELLED PULL REQUEST. Fix or dispute, a dispute is a marked
# reply that stays open, analysis issues are on the list, rounds are counted
# from the pull request, every ending is ONE summary naming the approver, and
# a landed round dispatches CI and the review.
export GH_COMMENTS="$tmp/comments.json"
printf '[]' > "$GH_COMMENTS"
cat > "$tmp/bin/gh" <<'STUB'
#!/usr/bin/env bash
# One call per LINE, a multi-line body flattened, so a test can read a whole
# comment with one grep.
call="${*//$'\n'/ }"
case "$*" in
  "api repos/"*"/replies"*)
    printf '%s @origin=%s\n' "$call" "$(git -C "$ORIGIN" rev-parse --short "$BRANCH" 2>/dev/null || echo none)" >> "$GH_CALLS" ;;
  *) printf '%s\n' "$call" >> "$GH_CALLS" ;;
esac
case "$*" in
  "api graphql"*) cat "$GH_FIXTURE" ;;
  "api repos/"*"/issues/"*"/comments --paginate") cat "$GH_COMMENTS" ;;
  "workflow run"*) [ -z "${GH_DISPATCH_FAILS:-}" ] || { echo "HTTP 422" >&2; exit 1; } ;;
esac
exit 0
STUB
cat > "$tmp/sonar.json" <<'JSON'
{"consulted":true,"stale":[],"projects":[],"issues":[]}
JSON
cat > "$tmp/work-all.json" <<'JSON'
[{"id":"PRRT_a","source":"review","threadId":"PRRT_a","commentId":11,"path":"a.go","line":3,"finding":"A returns nothing.","acceptedBy":"an-approver","reply":""},
 {"id":"PRRT_b","source":"review","threadId":"PRRT_b","commentId":22,"path":"b.go","line":3,"finding":"B is unused.","acceptedBy":"an-approver","reply":""},
 {"id":"sonar:AZ1","source":"sonar","key":"AZ1","rule":"go:S1234","severity":"MAJOR","path":"b.go","line":1,"message":"Remove the unused function."}]
JSON
printf '{"items":[{"id":"PRRT_a","action":"fixed","reason":""},{"id":"PRRT_b","action":"disputed","reason":"B is exported and used by the tests"},{"id":"sonar:AZ1","action":"disputed","reason":"the function is the package API"}]}' > "$tmp/report-all.json"

land_all() { : > "$GH_CALLS"; rm -f "$tmp/work/.resolve-threads"
             (cd "$tmp/work" && python3 "$S" --repo o/r --pr 7 --branch "$BRANCH" \
                --work-list "${WORK:-$tmp/work-all.json}" --patch "${PATCH:-$tmp/fix.patch}" --report "${REPORT:-$tmp/report-all.json}" \
                --dispatched-by github-actions --mode all --approver an-approver --since 2026-08-29T10:00:00Z \
                --max-rounds 3 --sonar "$tmp/sonar.json" ${STARTS---push-starts-workflows} 2>&1); }

fresh_repo
out=$(land_all); rc=$?
sha=$(git -C "$ORIGIN" rev-parse --short "$BRANCH")

it "labelled: fixes what the patch fixes and pushes one commit naming the round"
assert_status 0 "$rc"
assert_equals "1" "$(git -C "$ORIGIN" rev-list --count master.."$BRANCH")"
assert_contains "$(git -C "$ORIGIN" log -1 --format=%s "$BRANCH")" "fix(review): address 1 review finding (autofix round 1)"

it "labelled: a disputed thread gets a MARKED reply naming the approver, and is not resolved"
assert_contains "$(cat "$GH_CALLS")" "comments/22/replies -f body=<!-- autofix:disputed -->"
assert_contains "$(cat "$GH_CALLS")" "Disputed by the fixing step: B is exported and used by the tests"
assert_contains "$(cat "$GH_CALLS")" "Left open for @an-approver"
assert_equals "PRRT_a" "$(cat "$tmp/work/.resolve-threads")"

it "labelled: disputed analysis issues go in ONE pull request comment under the marker, naming the key, and nothing touches the service"
assert_equals "1" "$(grep -c 'The fixing step disputes 1 analysis issue' "$GH_CALLS")"
assert_contains "$(grep 'disputes 1 analysis issue' "$GH_CALLS")" "<!-- autofix:disputed -->"
assert_contains "$(cat "$GH_CALLS")" "\`AZ1\` (go:S1234, \`b.go:1\`): the function is the package API"
assert_not_contains "$(cat "$GH_CALLS")" "sonarcloud"

it "labelled: the landing comment carries the round marker, and dispatches nothing — the push itself is what starts CI and the review"
assert_contains "$(cat "$GH_CALLS")" "<!-- autofix:round 1 -->"
assert_not_contains "$(cat "$GH_CALLS")" "workflow run"
assert_contains "$out" "the push starts CI and the review, and the review's completion starts the next round"

it "labelled: a round that goes on posts NO summary — the summary is the ending's"
assert_not_contains "$(cat "$GH_CALLS")" "<!-- autofix:summary -->"
assert_not_contains "$(cat "$GH_CALLS")" "Push again"

# ENDING: the cap. Two rounds already on the pull request since the label.
it "labelled: counts rounds from its own marked comments since the label, and at the cap posts ONE summary mentioning the approver instead of re-triggering"
fresh_repo
printf '[{"body":"<!-- autofix:round 1 -->\\nround one","created_at":"2026-08-29T11:00:00Z"},{"body":"<!-- autofix:round 2 -->\\nround two","created_at":"2026-08-29T12:00:00Z"},{"body":"<!-- autofix:round 9 -->\\nfrom before the label","created_at":"2026-08-28T12:00:00Z"}]' > "$GH_COMMENTS"
out=$(land_all); rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$GH_CALLS")" "<!-- autofix:round 3 -->"
assert_not_contains "$(cat "$GH_CALLS")" "workflow run"
assert_equals "1" "$(grep -c '<!-- autofix:summary -->' "$GH_CALLS")"
summary_line=$(grep '<!-- autofix:summary -->' "$GH_CALLS")
assert_contains "$summary_line" "round cap reached** — @an-approver"
assert_contains "$summary_line" "Rounds used: 3 of 3"
assert_contains "$summary_line" "Disputed (2)"
printf '[]' > "$GH_COMMENTS"

# ENDING: disputes only.
it "labelled: with everything disputed, commits nothing, posts the disputes, and ONE summary saying so"
fresh_repo
printf '{"items":[{"id":"PRRT_a","action":"disputed","reason":"A is fine"},{"id":"PRRT_b","action":"disputed","reason":"so is B"},{"id":"sonar:AZ1","action":"disputed","reason":"API"}]}' > "$tmp/report-disp.json"
: > "$tmp/empty.patch"
out=$(PATCH="$tmp/empty.patch" REPORT="$tmp/report-disp.json" land_all); rc=$?
assert_status 0 "$rc"
assert_equals "0" "$(git -C "$ORIGIN" rev-list --count master.."$BRANCH")"
assert_equals "2" "$(grep -c 'replies -f body=<!-- autofix:disputed -->' "$GH_CALLS")"
assert_equals "1" "$(grep -c '<!-- autofix:summary -->' "$GH_CALLS")"
assert_contains "$(grep 'autofix:summary' "$GH_CALLS")" "disputes only** — @an-approver"
assert_not_contains "$(cat "$GH_CALLS")" "workflow run"
assert_not_contains "$(cat "$GH_CALLS")" "resolveReviewThread"

# ENDING: clean.
it "labelled: with nothing to do, posts ONE summary saying the pull request is clean"
fresh_repo
printf '[]' > "$tmp/none.json"
out=$(WORK="$tmp/none.json" land_all); rc=$?
assert_status 0 "$rc"
assert_equals "1" "$(grep -c '<!-- autofix:summary -->' "$GH_CALLS")"
assert_contains "$(grep 'autofix:summary' "$GH_CALLS")" "clean** — @an-approver"
assert_contains "$(grep 'autofix:summary' "$GH_CALLS")" "Rounds used: 0 of 3"

it "labelled: says when the analysis was not consulted"
printf '{"consulted":false,"stale":["manager"],"projects":[],"issues":[]}' > "$tmp/sonar.json"
out=$(WORK="$tmp/none.json" land_all)
assert_contains "$(grep 'autofix:summary' "$GH_CALLS")" "The analysis service was NOT consulted this round"
assert_contains "$(grep 'autofix:summary' "$GH_CALLS")" "(stale for manager)"
printf '{"consulted":true,"stale":[],"projects":[],"issues":[]}' > "$tmp/sonar.json"

# ENDING: stale patch.
it "labelled: a stale patch lands nothing, resolves nothing, and ends the loop with ONE summary"
fresh_repo
printf 'package a\n\n// moved\nfunc A() string { return "" }\n' > "$tmp/work/a.go"
git -C "$tmp/work" commit -qam "moved" && git -C "$tmp/work" push -q origin "$BRANCH"
before=$(git -C "$ORIGIN" rev-parse "$BRANCH")
out=$(land_all); rc=$?
assert_status 1 "$rc"
assert_equals "$before" "$(git -C "$ORIGIN" rev-parse "$BRANCH")"
assert_equals "1" "$(grep -c '<!-- autofix:summary -->' "$GH_CALLS")"
assert_contains "$(grep 'autofix:summary' "$GH_CALLS")" "stale patch** — @an-approver"
assert_not_contains "$(cat "$GH_CALLS")" "/replies"
assert_not_contains "$(cat "$GH_CALLS")" "workflow run"

it "labelled: an item the report neither fixed nor disputed is DISPUTED as unaddressed, never dropped"
fresh_repo
printf '{"items":[{"id":"PRRT_a","action":"fixed","reason":""}]}' > "$tmp/report-partial.json"
out=$(REPORT="$tmp/report-partial.json" land_all)
assert_contains "$out" "disputed PRRT_b (b.go): not addressed by the fixing step"
assert_contains "$(cat "$GH_CALLS")" "comments/22/replies -f body=<!-- autofix:disputed -->"

# THE TOKEN PUSHED IT. Without the push credential the workflow does not pass
# --push-starts-workflows, and a landed round cannot be followed by another.
it "labelled: without the push credential, lands the round and ends the loop with ONE summary naming the missing secret"
fresh_repo
out=$(STARTS="" land_all); rc=$?
assert_status 1 "$rc"
assert_equals "1" "$(git -C "$ORIGIN" rev-list --count master.."$BRANCH")"
assert_equals "1" "$(grep -c '<!-- autofix:summary -->' "$GH_CALLS")"
assert_contains "$(grep 'autofix:summary' "$GH_CALLS")" "could not start the next round** — @an-approver"
assert_contains "$(grep 'autofix:summary' "$GH_CALLS")" "AUTOFIX_DEPLOY_KEY"
assert_contains "$(grep 'autofix:summary' "$GH_CALLS")" "Push again"

it "never dispatches a workflow: a dispatched run's checks never reach the merge box"
assert_not_contains "$(cat "$S")" "workflow run"

it "contains no model invocation of its own"
assert_not_contains "$(cat "$S")" "claude"

rm -rf "$tmp"
summary
