#!/usr/bin/env bash
# The step that holds `contents: write`.
#
# ITS REFUSALS ARE THE PRODUCT, not its resolutions. Resolving a human's review
# thread hides a person's objection and reports it as handled — the one failure
# in this mechanism that destroys information rather than adding noise.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/resolve-review-threads.py"

tmp=$(mktemp -d); stub_gh "$tmp/bin"
export GH_CALLS="$tmp/calls" GH_FIXTURE="$tmp/threads.json"

# THE SHAPE GRAPHQL ACTUALLY RETURNS, which is not the shape REST does. A bot
# is `claude` + `__typename: Bot` here and `claude[bot]` there — and this fixture
# used to carry the REST spelling, so every test passed while production refused
# the review's own threads on every run. A fixture asserting the wrong world
# cannot fail.
cat > "$GH_FIXTURE" <<'JSON'
{"data":{"repository":{"pullRequest":{"reviewThreads":{
  "pageInfo":{"hasNextPage":false,"endCursor":null},
  "nodes":[
    {"id":"PRRT_mine","isResolved":false,"isOutdated":false,"path":"a.go",
     "comments":{"nodes":[{"author":{"login":"claude","__typename":"Bot"}}]}},
    {"id":"PRRT_human","isResolved":false,"isOutdated":false,"path":"b.go",
     "comments":{"nodes":[{"author":{"login":"a-maintainer","__typename":"User"}}]}},
    {"id":"PRRT_done","isResolved":true,"isOutdated":false,"path":"c.go",
     "comments":{"nodes":[{"author":{"login":"claude","__typename":"Bot"}}]}},
    {"id":"PRRT_stale","isResolved":false,"isOutdated":true,"path":"d.go",
     "comments":{"nodes":[{"author":{"login":"a-maintainer","__typename":"User"}}]}},
    {"id":"PRRT_impostor","isResolved":false,"isOutdated":false,"path":"e.go",
     "comments":{"nodes":[{"author":{"login":"claude","__typename":"User"}}]}}
  ]}}}}}
JSON

run() { : > "$GH_CALLS"; printf '%s\n' "$@" > "$tmp/list"
        python3 "$S" --repo o/r --pr 1 --file "$tmp/list" 2>&1; }

it "resolves a thread it authored"
out=$(run PRRT_mine)
assert_contains "$out" "resolved PRRT_mine"

it "actually sends the mutation"
assert_contains "$(cat "$GH_CALLS")" "resolveReviewThread"

it "REFUSES a thread authored by a person"
out=$(run PRRT_human)
assert_contains "$out" "PRRT_human: authored by"

it "sends no mutation when it refuses"
assert_not_contains "$(cat "$GH_CALLS")" "resolveReviewThread"

it "names who authored the thread it refused"
assert_contains "$out" "a-maintainer"

it "refuses a detached thread it did not author, and says it is outdated"
out=$(run PRRT_stale)
assert_contains "$out" "(outdated)"

it "leaves an already-resolved thread alone"
out=$(run PRRT_done)
assert_contains "$out" "already  PRRT_done"

it "refuses a thread that is not on the pull request"
out=$(run PRRT_ghost)
assert_contains "$out" "no such thread"

it "reports every author it saw, so a wrong identity is visible"
out=$(run PRRT_mine)
assert_contains "$out" "thread authors on this pull request: a-maintainer (person), claude"

it "resolves nothing under --dry-run"
: > "$GH_CALLS"; printf 'PRRT_mine\n' > "$tmp/list"
python3 "$S" --repo o/r --pr 1 --file "$tmp/list" --dry-run >/dev/null 2>&1
assert_not_contains "$(cat "$GH_CALLS")" "resolveReviewThread"

it "de-duplicates a thread listed twice"
out=$(run PRRT_mine PRRT_mine)
assert_equals "1" "$(printf '%s' "$out" | grep -c 'resolved PRRT_mine')"

it "does nothing at all when the list is empty"
: > "$tmp/list"
out=$(python3 "$S" --repo o/r --pr 1 --file "$tmp/list" 2>&1)
assert_contains "$out" "asked for no threads"

# AN EMPTY LIST AND AN ABSENT ONE ARE DIFFERENT FACTS. Reading them alike is
# what let a review whose artifact never uploaded print the healthy sentence.
it "refuses, loudly, when the list is absent rather than empty"
rm -f "$tmp/list"
out=$(python3 "$S" --repo o/r --pr 1 --file "$tmp/list" 2>&1) && rc=0 || rc=$?
assert_equals "1" "$rc"
assert_contains "$out" "ABSENT"
printf 'PRRT_mine\n' > "$tmp/list"

# THE REGRESSION. The allowlist is written the REST way because that is how a
# login reads on GitHub; the data arrives the GraphQL way. Both are normalised,
# so `claude[bot]` in config recognises `claude` on the wire.
it "recognises the review despite REST and GraphQL spelling a bot differently"
out=$(run PRRT_mine)
assert_contains "$out" "resolved PRRT_mine"

# A RECOGNISED NAME IS NOT A RECOGNISED ACCOUNT: `claude` is a login a person
# can hold, and normalising the spelling is what makes that reachable.
it "refuses a PERSON whose login matches the review's, on the account type"
out=$(run PRRT_impostor)
assert_contains "$out" "is a person, not the review app"

it "says which authors were people when it lists what it saw"
out=$(run PRRT_mine)
assert_contains "$out" "a-maintainer (person)"

it "honours a narrowed --authors, refusing what is no longer recognised"
printf 'PRRT_mine\n' > "$tmp/list"
out=$(python3 "$S" --repo o/r --pr 1 --file "$tmp/list" --authors "someone-else" 2>&1)
assert_contains "$out" "PRRT_mine: authored by"

rm -rf "$tmp"
summary
