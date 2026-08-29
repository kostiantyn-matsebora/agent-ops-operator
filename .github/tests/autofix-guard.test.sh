#!/usr/bin/env bash
# The guard that refuses an archive while a pull request's fixing loop is open.
#
# TWO REFUSALS AND EVERY FAIL-OPEN. A guard that blocks work it cannot read gets
# disabled, so the cases where it must ALLOW — no gh, no pull request, no label,
# an API error — are as much the product as the two where it must refuse. `gh`
# is stubbed and answers by sub-command, so nothing here reaches GitHub.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/autofix-guard.py"

tmp=$(mktemp -d)
export GH_CALLS="$tmp/calls" FX="$tmp/fx"
mkdir -p "$tmp/bin" "$FX"
cat > "$tmp/bin/gh" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$GH_CALLS"
[ -z "${GH_DOWN:-}" ] || { echo "connection refused" >&2; exit 1; }
case "$*" in
  "pr view"*)                cat "$FX/pr.json" ;;
  "run list"*"--status "*)   s=$(printf '%s' "$*" | sed -n 's/.*--status \([a-z_]*\).*/\1/p'); f="$FX/runs-$s.json"; [ -f "$f" ] && cat "$f" || echo '[]' ;;
  "api graphql"*)            cat "$FX/threads.json" ;;
  "api repos/"*"/issues/"*"/comments"*) cat "${FX}/comments.json" 2>/dev/null || echo '[]' ;;
  "repo view"*)              echo '{"nameWithOwner":"o/r"}' ;;
  *) echo '{}' ;;
esac
STUB
chmod +x "$tmp/bin/gh"; export PATH="$tmp/bin:$PATH"

labelled() { printf '{"number":7,"headRefName":"change/thing","state":"OPEN","labels":[{"name":"autofix"}]}' > "$FX/pr.json"; }
unlabelled() { printf '{"number":7,"headRefName":"change/thing","state":"OPEN","labels":[]}' > "$FX/pr.json"; }
threads() { cat > "$FX/threads.json"; }
none_running() { rm -f "$FX"/runs-*.json; }
quiet_threads() { threads <<'JSON'
{"data":{"repository":{"pullRequest":{"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[
  {"id":"PRRT_a","isResolved":false,"path":"a.go","line":1,"comments":{"nodes":[
    {"body":"A finding.","author":{"login":"claude","__typename":"Bot"}}]}}]}}}}}
JSON
}
run() { : > "$GH_CALLS"; python3 "$S" --repo o/r --pr 7 "$@" 2>&1; }

labelled; none_running; quiet_threads
out=$(run); rc=$?
it "allows a labelled pull request with no round running and no dispute"
assert_status 0 "$rc"
assert_contains "$out" "no round is running and every dispute is answered"

unlabelled
out=$(run); rc=$?
it "allows, without asking further, a pull request that does not carry the label"
assert_status 0 "$rc"
assert_contains "$out" "does not carry \`autofix\`"
assert_not_contains "$(cat "$GH_CALLS")" "run list"

# REFUSAL ONE: a round is running — by branch, or by the number in the run's name.
labelled
printf '[{"databaseId":1,"displayTitle":"review-dispatch #7","headBranch":"change/thing","url":"https://example.com/runs/1"}]' > "$FX/runs-in_progress.json"
out=$(run); rc=$?
it "REFUSES while a dispatch run for the branch is in progress, naming it"
assert_status 1 "$rc"
assert_contains "$out" "a fixing round is still running on #7"
assert_contains "$out" "https://example.com/runs/1 (in_progress: review-dispatch #7)"

none_running
printf '[{"databaseId":2,"displayTitle":"review-dispatch after Review of #7","headBranch":"master","url":"https://example.com/runs/2"}]' > "$FX/runs-queued.json"
out=$(run); rc=$?
it "REFUSES while a review-completion round names this pull request, though it runs on the default branch"
assert_status 1 "$rc"
assert_contains "$out" "runs/2 (queued: review-dispatch after Review of #7)"

none_running
printf '[{"databaseId":3,"displayTitle":"review-dispatch after Review of #77","headBranch":"master","url":"u3"}]' > "$FX/runs-queued.json"
out=$(run); rc=$?
it "ignores a round on another pull request"
assert_status 0 "$rc"
none_running

# REFUSAL TWO: a dispute nobody answered.
threads <<'JSON'
{"data":{"repository":{"pullRequest":{"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[
  {"id":"PRRT_disputed","isResolved":false,"path":"a.go","line":12,"comments":{"nodes":[
    {"body":"A finding.","author":{"login":"claude","__typename":"Bot"}},
    {"body":"<!-- autofix:disputed -->\nDisputed by the fixing step: the slice is half-open.","author":{"login":"github-actions","__typename":"Bot"}}]}},
  {"id":"PRRT_answered","isResolved":false,"path":"b.go","line":3,"comments":{"nodes":[
    {"body":"A finding.","author":{"login":"claude","__typename":"Bot"}},
    {"body":"<!-- autofix:disputed -->\nDisputed by the fixing step: not a bug.","author":{"login":"github-actions","__typename":"Bot"}},
    {"body":"Agreed, dismissing.","author":{"login":"a-maintainer","__typename":"User"}}]}},
  {"id":"PRRT_botreply","isResolved":false,"path":"c.go","line":3,"comments":{"nodes":[
    {"body":"A finding.","author":{"login":"claude","__typename":"Bot"}},
    {"body":"<!-- autofix:disputed -->\nDisputed.","author":{"login":"github-actions","__typename":"Bot"}},
    {"body":"Fixed in abc1234.","author":{"login":"github-actions","__typename":"Bot"}}]}},
  {"id":"PRRT_resolved","isResolved":true,"path":"d.go","line":3,"comments":{"nodes":[
    {"body":"A finding.","author":{"login":"claude","__typename":"Bot"}},
    {"body":"<!-- autofix:disputed -->\nDisputed.","author":{"login":"github-actions","__typename":"Bot"}}]}}
]}}}}}
JSON
out=$(run); rc=$?
it "REFUSES over a disputed thread with no later comment by a person, naming the thread"
assert_status 1 "$rc"
assert_contains "$out" "the fixing step disputed a finding and no person has answered"
assert_contains "$out" "thread PRRT_disputed (a.go:12)"

it "counts a person's later reply as an answer, and a bot's as none"
assert_not_contains "$out" "PRRT_answered"
assert_contains "$out" "thread PRRT_botreply (c.go:3)"

it "ignores a disputed thread a person resolved"
assert_not_contains "$out" "PRRT_resolved"

quiet_threads
printf '[{"body":"<!-- autofix:disputed -->\\nThe fixing step disputes 1 analysis issue","user":{"login":"github-actions[bot]","type":"Bot"}}]' > "$FX/comments.json"
out=$(run); rc=$?
it "REFUSES over an unanswered dispute of analysis issues, which lives in a pull request comment"
assert_status 1 "$rc"
assert_contains "$out" "a pull request comment disputing analysis issues"

printf '[{"body":"<!-- autofix:disputed -->\\nThe fixing step disputes 1 analysis issue","user":{"login":"github-actions[bot]","type":"Bot"}},{"body":"Marked as false positive.","user":{"login":"a-maintainer","type":"User"}}]' > "$FX/comments.json"
out=$(run); rc=$?
it "allows once a person has answered the analysis dispute"
assert_status 0 "$rc"
rm -f "$FX/comments.json"

# FAIL-OPEN, every way.
it "allows when gh cannot be reached, and says why"
out=$(GH_DOWN=1 run); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "fail-open"

it "allows when there is no pull request for the branch"
printf 'null' > "$FX/pr.json"
out=$(run); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "no pull request to read"

it "allows a closed pull request"
printf '{"number":7,"headRefName":"change/thing","state":"MERGED","labels":[{"name":"autofix"}]}' > "$FX/pr.json"
out=$(run); rc=$?
assert_status 0 "$rc"

it "allows when gh is not installed at all"
mkdir -p "$tmp/nogh"; ln -s "$(command -v python3)" "$tmp/nogh/python3"
out=$(PATH="$tmp/nogh" python3 "$S" --repo o/r --pr 7 2>&1); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "no gh on PATH"

it "reads the label and the marker from the vocabulary file rather than restating them"
labelled; none_running; quiet_threads
printf '{"approve_label":"other-label","dispute_marker":"<!-- x -->"}' > "$tmp/vocab.json"
out=$(run --vocabulary "$tmp/vocab.json")
assert_contains "$out" "does not carry \`other-label\`"

rm -rf "$tmp"
summary
