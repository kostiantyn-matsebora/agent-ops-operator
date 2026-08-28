#!/usr/bin/env bash
# The two programs that stand between the queue and the model: the one that
# builds the review's input (no model, no network here — `gh` is stubbed), and
# the one that assembles each role's delegation message from it.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
INPUT="$ROOT/.github/scripts/review-input.py"
PROMPT="$ROOT/.github/scripts/review-prompt.py"

tmp=$(mktemp -d)

# --- review-input.py, against a stubbed gh ----------------------------------
# A `gh` that answers the three calls the program makes, from fixtures.
mkdir -p "$tmp/bin"
cat > "$tmp/bin/gh" <<'STUB'
#!/usr/bin/env bash
case "$*" in
  "pr view"*)     echo '{"baseRefName":"master","headRefName":"change/thing"}' ;;
  "pr diff"*)     printf 'docs/a.md\nsignals/cron/main.go\nplatform/manager/x.go\n' ;;
  # TWO PAGES. The first says there is another and hands a cursor; the call
  # that carries the cursor gets the second. A caller that stops at one page
  # sees one thread and the test says so.
  "api graphql"*"after=CUR"*) echo '{"data":{"repository":{"pullRequest":{"reviewThreads":{"pageInfo":{"hasNextPage":false},"nodes":[{"id":"PRRT_2","isResolved":true,"isOutdated":false,"path":"signals/cron/main.go","line":1,"comments":{"nodes":[{"databaseId":8,"author":{"login":"bot"},"body":"**Claim:** y"}]}}]}}}}}' ;;
  "api graphql"*) echo '{"data":{"repository":{"pullRequest":{"reviewThreads":{"pageInfo":{"hasNextPage":true,"endCursor":"CUR"},"nodes":[{"id":"PRRT_1","isResolved":false,"isOutdated":false,"path":"docs/a.md","line":3,"comments":{"nodes":[{"databaseId":7,"author":{"login":"bot"},"body":"**Claim:** x"}]}}]}}}}}' ;;
esac
STUB
chmod +x "$tmp/bin/gh"

repo=$(make_repo)
mkdir -p "$repo/openspec/changes/thing/specs/cap" "$repo/docs" "$repo/signals/cron" "$repo/platform/manager"
echo spec > "$repo/openspec/changes/thing/specs/cap/spec.md"
git -C "$repo" add -A && git -C "$repo" commit -qm specs

it "builds the input from the three gh calls and the checkout, and emits the matrix"
out=$(cd "$repo" && PATH="$tmp/bin:$PATH" GITHUB_OUTPUT="$tmp/gho" python3 "$INPUT" --repo o/r --number 5 --out "$tmp/input.json" 2>&1)
assert_contains "$out" "3 component(s) from 3 path(s), 2 thread(s), 1 delta spec(s)"

it "walks every page of threads — the second page's thread is there"
assert_contains "$(cat "$tmp/input.json")" '"id": "PRRT_2"'
assert_contains "$(cat "$tmp/gho")" 'count=3'
assert_contains "$(cat "$tmp/gho")" 'base=master'
assert_contains "$(cat "$tmp/gho")" '{"group": "platform/manager", "slug": "platform__manager"}'

it "the threads are flattened to the shape the roles read"
assert_contains "$(cat "$tmp/input.json")" '"commentId": 7'
assert_contains "$(cat "$tmp/input.json")" '"author": "bot"'

it "the delta specs are the head change's, and only on a change/ branch"
assert_contains "$(cat "$tmp/input.json")" 'openspec/changes/thing/specs/cap/spec.md'

# --- review-prompt.py, over that input --------------------------------------

it "the component's workflow args carry one entry per file: its threads and the rules routed to its path — and no other component's files"
c=$(python3 "$PROMPT" component --input "$tmp/input.json" --group docs)
assert_contains "$c" '"component": "docs"'
assert_contains "$c" '"path": "docs/a.md"'
assert_contains "$c" '"id": "PRRT_1"'
assert_contains "$c" ".claude/rules/documentation.md"
assert_contains "$c" "docs/CLAUDE.md"
assert_contains "$c" "openspec/changes/thing/specs/cap/spec.md"
assert_not_contains "$c" "signals/cron"

it "a file's threads are its own, not the component's"
c=$(python3 "$PROMPT" component --input "$tmp/input.json" --group signals/cron)
assert_contains "$c" '"id": "PRRT_2"'
assert_not_contains "$c" '"id": "PRRT_1"'
assert_contains "$c" ".claude/rules/signal-rules.md"

it "the component session's instruction is to run the workflow with those args and read nothing"
r=$(python3 "$PROMPT" reader --input "$tmp/input.json" --group platform/manager)
assert_contains "$r" 'saved workflow `review-component`'
assert_contains "$r" '"component": "platform/manager"'
assert_contains "$r" "Do not read the diff"

it "an unknown component is refused"
python3 "$PROMPT" reader --input "$tmp/input.json" --group nope >/dev/null 2>&1
assert_status 1 "$?"

it "the coordinator's message holds one reading per queued component, null where none was produced"
mkdir -p "$tmp/readings/reading-docs" "$tmp/readings/reading-signals__cron"
echo '{"component":"docs","findings":[],"changedNames":["A"],"threads":[]}' > "$tmp/readings/reading-docs/reading.json"
echo '{"component":"signals/cron","findings":[],"changedNames":[],"threads":[]}' > "$tmp/readings/reading-signals__cron/reading.json"
c=$(python3 "$PROMPT" coordinator --input "$tmp/input.json" --readings "$tmp/readings" 2>"$tmp/err")
assert_contains "$c" '"group": "platform/manager",
  "reading": null'
assert_contains "$c" '"changedNames": [
    "A"
   ]'
assert_contains "$(cat "$tmp/err")" "unreviewed: platform/manager"
assert_contains "$c" "CHANGED PATHS:"
assert_contains "$c" "REVIEW THREADS:"

it "a readings directory that does not exist is every component unreviewed, not a crash"
c=$(python3 "$PROMPT" coordinator --input "$tmp/input.json" --readings "$tmp/nowhere" 2>"$tmp/err")
assert_contains "$(cat "$tmp/err")" "unreviewed: docs, platform/manager, signals/cron"

rm -rf "$tmp" "$repo"
summary
