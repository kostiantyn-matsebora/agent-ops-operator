#!/usr/bin/env bash
# THE SWEEP HEARS THE ANSWER NO EVENT CARRIES: a disputed thread RESOLVED. It lists
# the pull requests whose loop waits, re-reads their disputes the way the archive
# guard does, and starts a round where none is unanswered. Everything else is left.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/conveyor-sweep.py"
W="$ROOT/.github/workflows/conveyor-sweep.yml"

tmp=$(mktemp -d)
export GH_CALLS="$tmp/calls" FX="$tmp/fx"
mkdir -p "$tmp/bin" "$FX"
cat > "$tmp/bin/gh" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "${*//$'\n'/ }" >> "$GH_CALLS"
num() { printf '%s' "$*" | sed -n 's#.*issues/\([0-9]*\)/.*#\1#p'; }
case "$*" in
  "pr list "*"conveyor:keep-going"*) cat "$FX/keep" 2>/dev/null || echo '[]' ;;
  "pr list "*) cat "$FX/prs" 2>/dev/null || echo '[]' ;;
  "api graphql"*) n=$(printf '%s' "$*" | sed -n 's/.*number=\([0-9]*\).*/\1/p'); cat "$FX/threads-$n" 2>/dev/null || echo '{"data":{"repository":{"pullRequest":{"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[]}}}}}' ;;
  "api repos/o/r/issues/"*"/comments"*) cat "$FX/comments-$(num "$@")" 2>/dev/null || echo '[]' ;;
  "workflow run"*) [ -z "${DISPATCH_FAILS:-}" ] || { echo "HTTP 422" >&2; exit 1; } ;;
esac
exit 0
STUB
chmod +x "$tmp/bin/gh"; export PATH="$tmp/bin:$PATH"

MARK='<!-- conveyor:disputed -->'
BOT='{"login":"github-actions","__typename":"Bot"}'
PERSON='{"login":"maintainer","__typename":"User"}'
# THE REST SHAPE, which the issue-comments endpoint answers in: `user.login`, `user.type`.
RBOT='{"login":"github-actions","type":"Bot"}'
RPERSON='{"login":"maintainer","type":"User"}'
threads() {  # threads <pr> <resolved:true|false> <answered:yes|no>
  local answer=""
  [ "$3" = yes ] && answer=",{\"body\":\"it is real\",\"author\":$PERSON}"
  printf '{"data":{"repository":{"pullRequest":{"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[{"id":"PRRT_x","isResolved":%s,"comments":{"nodes":[{"body":"finding","author":%s},{"body":"%s\\nDisputed by the fixing step: no.","author":%s}%s]}}]}}}}}' \
    "$2" "$BOT" "$MARK" "$BOT" "$answer" > "$FX/threads-$1"
}
prs() { python3 -c 'import json,sys;print(json.dumps([{"number":int(n)} for n in sys.argv[1:]]))' "$@" > "$FX/prs"; }
reset() { rm -f "${FX:?}"/*; : > "$GH_CALLS"; }
run() { python3 "$S" --repo o/r "$@" 2>&1; }
resumed() { grep -c '^workflow run remote-implement.yml --repo o/r -f testing_pr=' "$GH_CALLS"; }
dispatched() { grep -c '^workflow run review-dispatch.yml --repo o/r -f pr=' "$GH_CALLS"; }

it "lists only the pull requests carrying BOTH the fix label and loop:waiting"
reset; prs; out=$(run); rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$GH_CALLS")" "pr list --repo o/r --state open --limit 200 --label conveyor:fix --label loop:waiting"
assert_contains "$out" "nothing to resume"

it "a waiting pull request whose disputed thread was RESOLVED gets a round: the answer no event carries"
reset; prs 220; threads 220 true no
out=$(run); assert_status 0 "$?"
assert_equals "1" "$(dispatched)"
assert_contains "$(cat "$GH_CALLS")" "workflow run review-dispatch.yml --repo o/r -f pr=220 -f mode=all"
assert_contains "$out" "#220: every dispute is answered or resolved; started a round"

it "a waiting pull request whose dispute a person ANSWERED gets a round too"
reset; prs 220; threads 220 false yes
run >/dev/null; assert_equals "1" "$(dispatched)"

it "a dispute still unanswered and unresolved is left waiting, and nothing starts"
reset; prs 220; threads 220 false no
out=$(run); assert_status 0 "$?"
assert_equals "0" "$(dispatched)"
assert_contains "$out" "#220: still waiting on 1 unanswered dispute(s): thread PRRT_x"

it "a dispute of a check or an analysis issue lives in a pull request comment, and holds the wait until a person comments after it"
reset; prs 220; threads 220 true no
printf '[{"body":"%s\\nThe fixing step disputes 1 failed check","user":{"login":"github-actions","type":"Bot"}}]' "$MARK" > "$FX/comments-220"
out=$(run); assert_equals "0" "$(dispatched)"; assert_contains "$out" "a pull request comment disputing"
printf '[{"body":"%s\\nThe fixing step disputes 1 failed check","user":{"login":"github-actions","type":"Bot"}},{"body":"re-run it","user":{"login":"maintainer","type":"User"}}]' "$MARK" > "$FX/comments-220"
reset_calls() { : > "$GH_CALLS"; }; reset_calls
run >/dev/null; assert_equals "1" "$(dispatched)"

it "sweeps every waiting pull request, and each is judged on its own"
reset; prs 220 221; threads 220 true no; threads 221 false no
out=$(run); assert_equals "1" "$(dispatched)"
assert_contains "$(cat "$GH_CALLS")" "pr=220"; assert_not_contains "$(cat "$GH_CALLS")" "pr=221"

it "--dry-run says what would start and starts nothing"
reset; prs 220; threads 220 true no
out=$(run --dry-run); assert_equals "0" "$(dispatched)"; assert_contains "$out" "a round would start (dry run)"

it "a dispatch that fails is a notice, never a failed run, and the next sweep tries again"
reset; prs 220; threads 220 true no
out=$(DISPATCH_FAILS=1 run); rc=$?
assert_status 0 "$rc"; assert_contains "$out" "could not be started"

it "the dispute reading is conveyor_io's one walk, shared with the archive guard"
assert_contains "$(cat "$S")" "conveyor_io.unanswered_disputes"
assert_not_contains "$(cat "$S")" "def unanswered_after_marker"
assert_not_contains "$(cat "$S")" "query(\$owner:String!"

# ---- the workflow's shape
py() { python3 -c "
import sys, yaml
d = yaml.safe_load(open(sys.argv[1]))
$1
" "$W"; }
it "runs on a schedule and by hand, and on nothing else -- a thread resolution has no event to listen for"
assert_equals "schedule workflow_dispatch" "$(py 'print(" ".join(sorted(d[True])))')"

it "grants nothing at the top, and the one job reads pull requests and writes only actions, for the dispatch"
assert_equals "{}" "$(py 'print(d["permissions"])')"
assert_equals "{'contents': 'read', 'pull-requests': 'read', 'actions': 'write'}" "$(py 'print(d["jobs"]["sweep"]["permissions"])')"

it "checks out the default branch, never a pull request's tree, and runs the sweep program"
assert_contains "$(py 'print(d["jobs"]["sweep"]["steps"][0]["with"]["ref"])')" "default_branch"
assert_contains "$(py 'print(d["jobs"]["sweep"]["steps"][-1]["run"])')" "conveyor-sweep.py"

CONFLICT='<!-- conveyor:conflict -->'
comments() {  # comments <pr> <json array>
  printf '%s' "$2" > "$FX/comments-$1"
}

it "a pull request carrying conveyor:keep-going asks the testing station to resume, through remote-implement.yml's testing_pr input"
reset; echo '[{"number":9,"headRefName":"change/x"}]' > "$FX/keep"
out=$(run); assert_status 0 "$?"
assert_equals "1" "$(resumed)"

it "a keep-going on a branch that is not change/* is left alone"
reset; echo '[{"number":9,"headRefName":"feature/x"}]' > "$FX/keep"
out=$(run); assert_equals "0" "$(resumed)"

it "a keep-going dispatch that fails is a notice and exit 0, tried again by the next sweep"
reset; echo '[{"number":9,"headRefName":"change/x"}]' > "$FX/keep"
out=$(DISPATCH_FAILS=1 run); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "::notice::"

it "a dry run says it would resume the station and starts nothing"
reset; echo '[{"number":9,"headRefName":"change/x"}]' > "$FX/keep"
out=$(run --dry-run); assert_equals "0" "$(resumed)"
assert_contains "$out" "dry run"

it "a waiting loop that asked about a merge conflict stays waiting until a PERSON has answered"
reset; prs 220; threads 220 true yes
comments 220 "[{\"body\":\"$CONFLICT\\nThis branch conflicts.\",\"user\":$RBOT}]"
out=$(run); assert_equals "0" "$(dispatched)"
assert_contains "$out" "merge conflict"

it "a person's comment after the conflict question starts the round"
reset; prs 220; threads 220 true yes
comments 220 "[{\"body\":\"$CONFLICT\\nThis branch conflicts.\",\"user\":$RBOT},{\"body\":\"take theirs\",\"user\":$RPERSON}]"
out=$(run); assert_equals "1" "$(dispatched)"

rm -rf "$tmp"
summary
