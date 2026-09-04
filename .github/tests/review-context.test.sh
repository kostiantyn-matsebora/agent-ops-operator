#!/usr/bin/env bash
# review-context.py prints what each review model context WILL HOLD, and
# nothing in the suite ran it before this file: it never appeared in a
# coverage report, at any percentage, because no test ever imported or
# invoked it. This exercises both roles end to end, against the real
# repository (its ROOT is derived from `__file__`, not from a cwd), and both
# branches of `diff_size` — the successful `git diff` and the ref that fails.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
CTX="$ROOT/.github/scripts/review-context.py"

tmp=$(mktemp -d)
cat > "$tmp/input.json" <<'JSON'
{
  "repo": "o/r",
  "number": 5,
  "base": "master",
  "headSha": "abc1234",
  "queue": [
    {"group": "docs", "kind": "generic", "paths": ["docs/a.md"]},
    {"group": "signals/cron", "kind": "generic", "paths": ["signals/cron/main.go"]}
  ],
  "entries": [
    {"slug": "docs", "group": "docs", "chunk": "", "paths": ["docs/a.md"]},
    {"slug": "signals__cron", "group": "signals/cron", "chunk": "", "paths": ["signals/cron/main.go"]}
  ],
  "threads": [],
  "specPaths": [],
  "paths": ["docs/a.md", "signals/cron/main.go"]
}
JSON

it "the component role prints one row for the session plus one per file, with byte and token totals"
out=$(cd "$ROOT" && python3 "$CTX" component --input "$tmp/input.json" --group docs)
assert_contains "$out" "CONTEXTS FOR docs"
assert_contains "$out" "component session (runs the workflow)"
assert_contains "$out" "reader docs/a.md"
assert_contains "$out" "B "

it "with no --base it falls back to origin/<the input's base>, and a real ref makes diff_size succeed"
out=$(cd "$ROOT" && python3 "$CTX" component --input "$tmp/input.json" --group signals__cron)
assert_contains "$out" "CONTEXTS FOR signals/cron"
assert_contains "$out" "reader signals/cron/main.go"

it "an unresolvable --base makes diff_size fail closed at 0 bytes rather than crash"
out=$(cd "$ROOT" && python3 "$CTX" component --input "$tmp/input.json" --group docs --base does-not-exist-xyz 2>&1)
assert_status 0 "$?"
assert_contains "$out" "diff 0"

it "component without --group is refused"
python3 "$CTX" component --input "$tmp/input.json" >/dev/null 2>&1
assert_status 2 "$?"

it "the coordinator role prints one context line, sized from the assembled message"
mkdir -p "$tmp/readings"
echo '{"component":"docs","findings":[],"changedNames":[],"files":[],"threads":[]}' > "$tmp/readings/docs.json"
out=$(cd "$ROOT" && python3 "$CTX" coordinator --input "$tmp/input.json" --readings "$tmp/readings")
assert_contains "$out" "CONTEXT FOR THE COORDINATOR"
assert_contains "$out" "coordinator"
assert_contains "$out" "message (readings + threads)"

rm -rf "$tmp"
summary
