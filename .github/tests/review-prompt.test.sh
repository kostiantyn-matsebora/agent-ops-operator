#!/usr/bin/env bash
# review-prompt.py: the fixed system prefix every file reader of a job
# shares, the per-file and per-verdict prompts, and the coordinator's
# message — carried paths, coverage counts and the coverage-input mode.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
PROMPT="$ROOT/.github/scripts/review-prompt.py"

tmp=$(mktemp -d)

# A fixture input against the REAL checkout's rule routing — chart/values.yaml
# routes to chart.md, docs/foo.md does not.
cat > "$tmp/input.json" <<'EOF'
{
 "repo": "o/r", "number": 9, "base": "master", "head": "change/thing", "headSha": "abc1234",
 "paths": ["chart/values.yaml", "docs/foo.md", "signals/cron/main.go"],
 "queue": [
   {"group": "chart", "kind": "directory", "paths": ["chart/values.yaml"]},
   {"group": "docs", "kind": "directory", "paths": ["docs/foo.md"]},
   {"group": "signals/cron", "kind": "component", "paths": ["signals/cron/main.go"]}
 ],
 "since": {"chart/values.yaml": "origin/master", "docs/foo.md": "origin/master"},
 "quietAt": {"docs/foo.md": 2},
 "carried": [{"path": "signals/cron/main.go", "since": "deadbeef", "quiet": 3,
   "threads": [{"id": "PRRT_1", "path": "signals/cron/main.go", "line": 4, "isResolved": false, "isOutdated": false, "commentId": 1, "author": "bot", "body": "**Claim:** x"}]}],
 "coverageInvalidated": "",
 "threads": [
   {"id": "PRRT_open", "path": "chart/values.yaml", "line": 2, "isResolved": false, "isOutdated": false, "commentId": 2, "author": "bot", "body": "**Claim:** open one"},
   {"id": "PRRT_closed", "path": "chart/values.yaml", "line": 3, "isResolved": true, "isOutdated": false, "commentId": 3, "author": "bot", "body": "**Claim:** dismissed one"},
   {"id": "PRRT_1", "path": "signals/cron/main.go", "line": 4, "isResolved": false, "isOutdated": false, "commentId": 1, "author": "bot", "body": "**Claim:** x"}
 ],
 "specPaths": [],
 "entries": [
   {"group": "chart", "slug": "chart", "chunk": "", "paths": ["chart/values.yaml"]},
   {"group": "docs", "slug": "docs", "chunk": "", "paths": ["docs/foo.md"]}
 ]
}
EOF

it "reader-system is deterministic for one group"
a=$(python3 "$PROMPT" reader-system --input "$tmp/input.json" --group chart)
b=$(python3 "$PROMPT" reader-system --input "$tmp/input.json" --group chart)
assert_equals "$a" "$b"

it "reader-system for a chart path holds chart.md; for a docs path it does not"
assert_contains "$a" "chart.md"
c=$(python3 "$PROMPT" reader-system --input "$tmp/input.json" --group docs)
assert_not_contains "$c" ".claude/rules/chart.md"
assert_contains "$c" "documentation.md"

it "reader-system opens with the file-reviewer role's body"
assert_contains "$a" "FILE REVIEWER"

it "reader holds coordinates only — no thread, no finding, no rule text"
r=$(python3 "$PROMPT" reader --input "$tmp/input.json" --group chart --path chart/values.yaml)
assert_contains "$r" "FILE: chart/values.yaml"
assert_contains "$r" "SINCE: origin/master"
assert_not_contains "$r" "PRRT_"
assert_not_contains "$r" "chart.md"

it "reader-tools prints the file-reviewer role's tools: line"
t=$(python3 "$PROMPT" reader-tools)
assert_contains "$t" "Bash(git diff:*)"
assert_not_contains "$t" "Write"

it "verdict-paths names only paths with an unresolved thread"
vp=$(python3 "$PROMPT" verdict-paths --input "$tmp/input.json" --group chart)
assert_equals "chart/values.yaml" "$vp"

it "verdict holds only the unresolved threads on that file, never the resolved one"
v=$(python3 "$PROMPT" verdict --input "$tmp/input.json" --group chart --path chart/values.yaml)
assert_contains "$v" "PRRT_open"
assert_not_contains "$v" "PRRT_closed"

it "verdict is absent (exit 1) for a file with no open thread"
mkdir -p "$tmp/nothread"
python3 "$PROMPT" verdict --input "$tmp/input.json" --group docs --path docs/foo.md >/dev/null 2>&1
assert_status 1 "$?"

it "verdict-system is the thread-verdict role's body, and names no rule"
vs=$(python3 "$PROMPT" verdict-system)
assert_contains "$vs" "thread"
assert_not_contains "$vs" "invariants.md"

it "the coordinator's message holds a CARRIED PATHS block with the carried thread"
mkdir -p "$tmp/readings"
echo '{"component":"chart","findings":[],"changedNames":[],"files":[{"path":"chart/values.yaml","declares":[],"references":[]}],"threads":[],"unread":[]}' > "$tmp/readings/reading-chart.json"
echo '{"component":"docs","findings":[],"changedNames":[],"files":[{"path":"docs/foo.md","declares":[],"references":[]}],"threads":[],"unread":[]}' > "$tmp/readings/reading-docs.json"
co=$(python3 "$PROMPT" coordinator --input "$tmp/input.json" --readings "$tmp/readings" 2>/dev/null)
assert_contains "$co" "CARRIED PATHS"
assert_contains "$co" "signals/cron/main.go (read at deadbeef, quiet 3)"
assert_contains "$co" "PRRT_1"

it "the coordinator's message states the coverage counts"
assert_contains "$co" "COVERAGE: 2 of 3 changed file(s) read this run"
assert_contains "$co" "1 carried (quiet)"

it "an unbuilt component is distinguished from an unreviewed one"
echo '{"component":"signals/cron","unbuilt":"boom","findings":[],"changedNames":[],"files":[],"threads":[],"unread":["signals/cron/other.go"]}' > "$tmp/readings2-placeholder"
mkdir -p "$tmp/readings3"
cp "$tmp/readings/reading-chart.json" "$tmp/readings/reading-docs.json" "$tmp/readings3/"
co3=$(python3 "$PROMPT" coordinator --input "$tmp/input.json" --readings "$tmp/readings3" 2>"$tmp/err3")
assert_contains "$co3" '"reading": {'
assert_not_contains "$(cat "$tmp/err3")" "unreviewed: chart"

it "coordinator --coverage writes quietBefore per read path, from quietAt"
python3 "$PROMPT" coordinator --input "$tmp/input.json" --readings "$tmp/readings" --coverage "$tmp/coverage.json"
cov=$(cat "$tmp/coverage.json")
assert_contains "$cov" '"sha": "abc1234"'
assert_contains "$cov" '"docs/foo.md": {
   "quietBefore": 2
  }'
assert_contains "$cov" '"chart/values.yaml": {
   "quietBefore": 0
  }'

rm -rf "$tmp"
summary
