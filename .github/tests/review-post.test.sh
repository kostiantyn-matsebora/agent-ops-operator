#!/usr/bin/env bash
# The one command the coordinator posts through. `gh` is stubbed; what would
# have been sent is asserted from the recorded calls.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/review-post.py"
tmp=$(mktemp -d)
mkdir -p "$tmp/bin"
cat > "$tmp/bin/gh" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$GH_CALLS"
case "$*" in
  "pr view"*) echo "abc123" ;;
  *"path=bad.md"*) echo "line is not part of the diff" >&2; exit 1 ;;
esac
exit 0
STUB
chmod +x "$tmp/bin/gh"
export GH_CALLS="$tmp/calls" RESOLVE_LIST="$tmp/.resolve-threads"

doc='{"repo":"o/r","number":7,
 "findings":[{"path":"docs/a.md","line":3,"body":"**Claim:** x"},{"path":"bad.md","line":1,"body":"**Claim:** y"}],
 "replies":[{"commentId":55,"body":"Fixed in abc123."}],
 "resolve":["PRRT_kwDOTvBhKs6dAE5n"],
 "summary":"### Review\n1 new · 0 carried over · 1 resolved · 0 dismissed"}'

it "posts every finding on the head sha, every reply, records the resolve list, and posts one summary"
out=$(printf '%s' "$doc" | PATH="$tmp/bin:$PATH" python3 "$S" 2>"$tmp/err"); rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$tmp/calls")" "api repos/o/r/pulls/7/comments -f commit_id=abc123 -f path=docs/a.md -F line=3 -f side=RIGHT -f body=**Claim:** x"
assert_contains "$(cat "$tmp/calls")" "api repos/o/r/pulls/7/comments/55/replies -f body=Fixed in abc123."
assert_contains "$(cat "$tmp/calls")" "pr comment 7 -R o/r --body ### Review"
assert_contains "$(cat "$tmp/.resolve-threads")" "PRRT_kwDOTvBhKs6dAE5n"

it "reports what it did as JSON, a refused finding counted and named — never swallowed"
assert_contains "$out" '"inline": 1'
assert_contains "$out" '"inlineFailed": 1'
assert_contains "$out" '"summaryPosted": true'
assert_contains "$(cat "$tmp/err")" "bad.md:1 not posted"

it "reads the document from a file too"
printf '%s' "$doc" > "$tmp/doc.json"
: > "$tmp/calls"
PATH="$tmp/bin:$PATH" python3 "$S" "$tmp/doc.json" >/dev/null 2>&1
assert_contains "$(cat "$tmp/calls")" "pr comment 7"

it "a summary that could not be posted is a failure"
cat > "$tmp/bin/gh" <<'STUB'
#!/usr/bin/env bash
case "$*" in "pr view"*) echo abc ;; "pr comment"*) exit 1 ;; esac
STUB
printf '%s' "$doc" | PATH="$tmp/bin:$PATH" python3 "$S" >/dev/null 2>&1
assert_status 1 "$?"

rm -rf "$tmp"
summary
