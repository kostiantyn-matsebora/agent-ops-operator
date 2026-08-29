#!/usr/bin/env bash
# The program that decides what a dispatch writes to a branch.
#
# ITS REFUSALS ARE THE PRODUCT. Every thread it keeps becomes code on somebody's
# branch, so the interesting cases are the ones it must NOT keep: a person's own
# remark, a finding the review accepted for itself, an acceptance from a
# stranger, and a reply that merely sounds like agreement.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/accepted-findings.py"

tmp=$(mktemp -d); stub_gh "$tmp/bin"
export GH_CALLS="$tmp/calls" GH_FIXTURE="$tmp/threads.json"

# THE SHAPE GRAPHQL RETURNS: a bot is `claude` + `__typename: Bot`, and every
# comment carries its own `authorAssociation`. One thread per case.
cat > "$GH_FIXTURE" <<'JSON'
{"data":{"repository":{"pullRequest":{"reviewThreads":{
  "pageInfo":{"hasNextPage":false,"endCursor":null},
  "nodes":[
    {"id":"PRRT_accepted","isResolved":false,"isOutdated":false,"path":"a.go","line":12,
     "comments":{"nodes":[
       {"databaseId":101,"body":"This leaks the handle.","authorAssociation":"NONE","author":{"login":"claude","__typename":"Bot"}},
       {"databaseId":102,"body":"Fix it.","authorAssociation":"OWNER","author":{"login":"a-maintainer","__typename":"User"}}]}},
    {"id":"PRRT_argued","isResolved":false,"isOutdated":false,"path":"b.go","line":3,
     "comments":{"nodes":[
       {"databaseId":201,"body":"Off by one.","authorAssociation":"NONE","author":{"login":"claude","__typename":"Bot"}},
       {"databaseId":202,"body":"No, the slice is half-open here.","authorAssociation":"OWNER","author":{"login":"a-maintainer","__typename":"User"}}]}},
    {"id":"PRRT_unanswered","isResolved":false,"isOutdated":false,"path":"c.go","line":40,
     "comments":{"nodes":[
       {"databaseId":301,"body":"Unchecked error.","authorAssociation":"NONE","author":{"login":"claude","__typename":"Bot"}}]}},
    {"id":"PRRT_selfaccepted","isResolved":false,"isOutdated":false,"path":"d.go","line":7,
     "comments":{"nodes":[
       {"databaseId":401,"body":"Missing nil check.","authorAssociation":"NONE","author":{"login":"claude","__typename":"Bot"}},
       {"databaseId":402,"body":"fix it","authorAssociation":"NONE","author":{"login":"claude","__typename":"Bot"}}]}},
    {"id":"PRRT_human","isResolved":false,"isOutdated":false,"path":"e.go","line":9,
     "comments":{"nodes":[
       {"databaseId":501,"body":"I would rename this.","authorAssociation":"OWNER","author":{"login":"a-maintainer","__typename":"User"}},
       {"databaseId":502,"body":"fix it","authorAssociation":"OWNER","author":{"login":"a-maintainer","__typename":"User"}}]}},
    {"id":"PRRT_resolved","isResolved":true,"isOutdated":false,"path":"f.go","line":1,
     "comments":{"nodes":[
       {"databaseId":601,"body":"Dead code.","authorAssociation":"NONE","author":{"login":"claude","__typename":"Bot"}},
       {"databaseId":602,"body":"fix it","authorAssociation":"OWNER","author":{"login":"a-maintainer","__typename":"User"}}]}},
    {"id":"PRRT_stranger","isResolved":false,"isOutdated":false,"path":"g.go","line":5,
     "comments":{"nodes":[
       {"databaseId":701,"body":"Typo.","authorAssociation":"NONE","author":{"login":"claude","__typename":"Bot"}},
       {"databaseId":702,"body":"fix it","authorAssociation":"NONE","author":{"login":"passer-by","__typename":"User"}}]}},
    {"id":"PRRT_embedded","isResolved":false,"isOutdated":false,"path":"h.go","line":2,
     "comments":{"nodes":[
       {"databaseId":801,"body":"Wrong default.","authorAssociation":"NONE","author":{"login":"claude","__typename":"Bot"}},
       {"databaseId":802,"body":"fix it, and while you are there rewrite the config loader","authorAssociation":"OWNER","author":{"login":"a-maintainer","__typename":"User"}}]}},
    {"id":"PRRT_overruled","isResolved":false,"isOutdated":false,"path":"j.go","line":4,
     "comments":{"nodes":[
       {"databaseId":1001,"body":"Shadowed variable.","authorAssociation":"NONE","author":{"login":"claude","__typename":"Bot"}},
       {"databaseId":1002,"body":"fix it","authorAssociation":"NONE","author":{"login":"passer-by","__typename":"User"}},
       {"databaseId":1003,"body":"fix it","authorAssociation":"OWNER","author":{"login":"a-maintainer","__typename":"User"}}]}},
    {"id":"PRRT_impostor","isResolved":false,"isOutdated":false,"path":"i.go","line":8,
     "comments":{"nodes":[
       {"databaseId":901,"body":"Stale comment.","authorAssociation":"NONE","author":{"login":"claude","__typename":"Bot"}},
       {"databaseId":902,"body":"fix it","authorAssociation":"OWNER","author":{"login":"claude","__typename":"User"}}]}}
  ]}}}}}
JSON

run() { : > "$GH_CALLS"; rm -f "$tmp/out.json"
        python3 "$S" --repo o/r --pr 1 --out "$tmp/out.json" "$@" 2>&1; }
ids() { python3 -c 'import json,sys;print(" ".join(i["threadId"] for i in json.load(open(sys.argv[1]))))' "$tmp/out.json"; }

out=$(run)

it "keeps the findings a maintainer accepted, and only those"
assert_equals "PRRT_accepted PRRT_overruled" "$(ids)"

# THE REGRESSION THE FIRST REVIEW OF THIS PROGRAM FOUND: it returned on the
# first matching reply, so a stranger's `fix it` permanently vetoed the
# maintainer's underneath it.
it "lets a maintainer's acceptance stand after a stranger's invalid one"
assert_contains "$out" "ACCEPTED PRRT_overruled (j.go:4) by a-maintainer"

it "carries the finding, where it points, and the person's words"
assert_contains "$(cat "$tmp/out.json")" '"finding": "This leaks the handle."'
assert_contains "$(cat "$tmp/out.json")" '"path": "a.go"'
assert_contains "$(cat "$tmp/out.json")" '"reply": "Fix it."'

it "matches the vocabulary case-insensitively, ignoring a trailing full stop"
assert_contains "$out" "ACCEPTED PRRT_accepted"

it "leaves a finding a person argued with alone"
assert_contains "$out" "PRRT_argued (b.go:3): not accepted"

it "leaves an unanswered finding alone"
assert_contains "$out" "PRRT_unanswered (c.go:40): not accepted"

# THE REFUSAL THAT MATTERS MOST: a bot that can accept its own findings writes
# to the branch unattended.
it "REFUSES an acceptance written by the review itself"
assert_contains "$out" "PRRT_selfaccepted (d.go:7): acceptance written by the review itself"

it "refuses a PERSON whose login is the review's, when the review is a Bot"
assert_contains "$out" "PRRT_impostor (i.go:8): acceptance written by the review itself ('claude')"

it "ignores a thread whose first comment is a person's, whatever the reply says"
assert_contains "$out" "PRRT_human (e.go:9): first comment by 'a-maintainer', not the review"

it "ignores a thread a person already resolved"
assert_contains "$out" "PRRT_resolved (f.go:1): already resolved"

it "refuses an acceptance from an account that cannot push here"
assert_contains "$out" "PRRT_stranger (g.go:5): acceptance by 'passer-by' (NONE), who cannot push here"

# A reply that contains the phrase inside a longer sentence would let the
# thread carry an instruction past the list.
it "does not read an accept phrase embedded in a longer sentence"
assert_contains "$out" "PRRT_embedded (h.go:2): not accepted"

it "reports every skip rather than skipping silently"
assert_equals "8" "$(printf '%s\n' "$out" | grep -c '  skipped  ')"

it "writes the list even when nothing was accepted, so absent and empty differ"
printf '{"data":{"repository":{"pullRequest":{"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[]}}}}}' > "$GH_FIXTURE.empty"
GH_FIXTURE="$GH_FIXTURE.empty" out=$(run)
assert_equals "[]" "$(tr -d '\n' < "$tmp/out.json")"
assert_contains "$out" "0 accepted finding(s)"

it "never reads the dispatch comment: no argument carries a body"
help=$(python3 "$S" --help 2>&1)
assert_not_contains "$help" "--body"
assert_not_contains "$help" "--comment"

it "reads the vocabulary from the file rather than restating it"
printf '{"accept":["ship it"],"dispatch":["/go"]}' > "$tmp/vocab.json"
out=$(run --vocabulary "$tmp/vocab.json")
assert_equals "" "$(ids)"
assert_contains "$out" "accept vocabulary: ship it"

it "sends nothing but the thread query"
assert_not_contains "$(cat "$GH_CALLS")" "mutation"

# --mode all: THE LABELLED PULL REQUEST. Every open finding of the review's is
# accepted by the label; a thread a previous round disputed waits for the person.
cat > "$GH_FIXTURE.all" <<'JSON'
{"data":{"repository":{"pullRequest":{"reviewThreads":{
  "pageInfo":{"hasNextPage":false,"endCursor":null},
  "nodes":[
    {"id":"PRRT_unanswered","isResolved":false,"isOutdated":false,"path":"c.go","line":40,
     "comments":{"nodes":[
       {"databaseId":301,"body":"Unchecked error.","authorAssociation":"NONE","author":{"login":"claude","__typename":"Bot"}}]}},
    {"id":"PRRT_argued","isResolved":false,"isOutdated":false,"path":"b.go","line":3,
     "comments":{"nodes":[
       {"databaseId":201,"body":"Off by one.","authorAssociation":"NONE","author":{"login":"claude","__typename":"Bot"}},
       {"databaseId":202,"body":"No, the slice is half-open here.","authorAssociation":"OWNER","author":{"login":"a-maintainer","__typename":"User"}}]}},
    {"id":"PRRT_disputed","isResolved":false,"isOutdated":false,"path":"d.go","line":7,
     "comments":{"nodes":[
       {"databaseId":401,"body":"Missing nil check.","authorAssociation":"NONE","author":{"login":"claude","__typename":"Bot"}},
       {"databaseId":402,"body":"<!-- autofix:disputed -->\nDisputed by the fixing step: the pointer cannot be nil here.","authorAssociation":"NONE","author":{"login":"github-actions","__typename":"Bot"}}]}},
    {"id":"PRRT_human","isResolved":false,"isOutdated":false,"path":"e.go","line":9,
     "comments":{"nodes":[
       {"databaseId":501,"body":"I would rename this.","authorAssociation":"OWNER","author":{"login":"a-maintainer","__typename":"User"}}]}},
    {"id":"PRRT_resolved","isResolved":true,"isOutdated":false,"path":"f.go","line":1,
     "comments":{"nodes":[
       {"databaseId":601,"body":"Dead code.","authorAssociation":"NONE","author":{"login":"claude","__typename":"Bot"}}]}}
  ]}}}}}
JSON

GH_FIXTURE="$GH_FIXTURE.all" out=$(run --mode all --approver an-approver)

it "in --mode all, lists every open finding of the review's — unanswered and argued alike — on the strength of the label"
assert_equals "PRRT_unanswered PRRT_argued" "$(ids)"
assert_contains "$out" "mode: all — every open finding is accepted by the label (placed by an-approver)"

it "in --mode all, each item carries id, source: review and the approver, with no reply"
assert_contains "$(cat "$tmp/out.json")" '"id": "PRRT_unanswered"'
assert_contains "$(cat "$tmp/out.json")" '"source": "review"'
assert_contains "$(cat "$tmp/out.json")" '"acceptedBy": "an-approver"'
assert_contains "$(cat "$tmp/out.json")" '"reply": ""'

it "in --mode all, skips a thread a previous round disputed, counting it as awaiting the person"
assert_contains "$out" "PRRT_disputed (d.go:7): disputed by a previous round, awaiting the person"

it "in --mode all, still ignores a person's thread and a resolved one"
assert_contains "$out" "PRRT_human (e.go:9): first comment by 'a-maintainer', not the review"
assert_contains "$out" "PRRT_resolved (f.go:1): already resolved"

it "in the default mode, the same fixture yields nothing — the label is not read here"
GH_FIXTURE="$GH_FIXTURE.all" out=$(run)
assert_equals "" "$(ids)"

it "in every mode an item carries id and source, so the lander keys on one field"
GH_FIXTURE="$tmp/threads.json"; out=$(run)
assert_contains "$(cat "$tmp/out.json")" '"id": "PRRT_accepted"'
assert_contains "$(cat "$tmp/out.json")" '"source": "review"'

rm -rf "$tmp"
summary
