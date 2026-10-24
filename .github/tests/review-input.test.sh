#!/usr/bin/env bash
# review-input.py: reads the pull request's own coverage markers and decides,
# per changed path, READ (from the base or from a delta) or CARRIED — the
# read-until-quiet, then carry rule. No network — `gh` is stubbed.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
INPUT="$ROOT/.github/scripts/review-input.py"

tmp=$(mktemp -d)
mkdir -p "$tmp/bin"

# --- a real repository, so the git comparisons are real -------------------
repo=$(mktemp -d)
git -C "$repo" init -q -b master
git -C "$repo" config user.email t@example.com
git -C "$repo" config user.name T
mkdir -p "$repo/docs" "$repo/signals/cron" "$repo/platform/manager" "$repo/.claude/rules"
echo base > "$repo/README.md"
echo "docs base" > "$repo/docs/a.md"
echo "cron base" > "$repo/signals/cron/main.go"
echo "mgr base" > "$repo/platform/manager/x.go"
echo "rule base" > "$repo/.claude/rules/foo.md"
git -C "$repo" add -A && git -C "$repo" commit -qm base

# C1: the point a previous review "read" docs/a.md and signals/cron/main.go.
echo "docs c1" > "$repo/docs/a.md"
echo "cron c1" > "$repo/signals/cron/main.go"
git -C "$repo" add -A && git -C "$repo" commit -qm c1
C1=$(git -C "$repo" rev-parse HEAD)

# HEAD: signals/cron/main.go changes again (since C1); docs/a.md does not;
# platform/manager/x.go changes but was never read before.
echo "cron c2" > "$repo/signals/cron/main.go"
echo "mgr c2" > "$repo/platform/manager/x.go"
git -C "$repo" add -A && git -C "$repo" commit -qm c2
git -C "$repo" update-ref refs/remotes/origin/master "$(git -C "$repo" rev-parse HEAD~2)"

PATHS_FILE="$tmp/paths"
printf 'docs/a.md\nsignals/cron/main.go\nplatform/manager/x.go\n' > "$PATHS_FILE"

stub_gh_with_comments() {  # stub_gh_with_comments <comments-file>
  local comments="$1"
  cat > "$tmp/bin/gh" <<STUB
#!/usr/bin/env bash
case "\$*" in
  "pr view"*)     echo '{"baseRefName":"master","headRefName":"change/thing","headRefOid":"deadbeef"}' ;;
  "pr diff"*)     cat "$PATHS_FILE" ;;
  "api repos/o/r/issues/5/comments --paginate -q .[].body") cat "$comments" 2>/dev/null ;;
  "api graphql"*) echo '{"data":{"repository":{"pullRequest":{"reviewThreads":{"pageInfo":{"hasNextPage":false},"nodes":[]}}}}}' ;;
esac
STUB
  chmod +x "$tmp/bin/gh"
}

marker() {  # marker <sha> <path:quiet> ...
  local sha="$1"; shift
  local paths="{"
  local first=true
  for pq in "$@"; do
    local p="${pq%%:*}" q="${pq##*:}"
    $first || paths="$paths,"
    first=false
    paths="$paths\"$p\":{\"quiet\":$q}"
  done
  paths="$paths}"
  echo "some text <!-- claude-review-coverage {\"sha\":\"$sha\",\"paths\":$paths} --> more text"
}

# --- two markers on the pull request: the NEWER one wins per path ---------
comments0="$tmp/comments0"
{
  marker "$C1" "docs/a.md:0" "signals/cron/main.go:0"
  marker "$C1" "docs/a.md:1" "signals/cron/main.go:0"
} > "$comments0"
stub_gh_with_comments "$comments0"

it "two coverage markers on the pull request: the newer one's quiet count wins"
out=$(cd "$repo" && PATH="$tmp/bin:$PATH" python3 "$INPUT" --repo o/r --number 5 --out "$tmp/input0.json" 2>"$tmp/err0")
assert_contains "$(cat "$tmp/err0")" "docs/a.md: carried (quiet 1)"

# --- scenario 1: read-until-quiet, per path -------------------------------
comments1="$tmp/comments1"
marker "$C1" "docs/a.md:1" "signals/cron/main.go:0" > "$comments1"
stub_gh_with_comments "$comments1"

it "docs/a.md is unchanged since C1 and quiet already 1 (>= K=1): CARRIED"
out=$(cd "$repo" && PATH="$tmp/bin:$PATH" GITHUB_OUTPUT="$tmp/gho1" python3 "$INPUT" --repo o/r --number 5 --out "$tmp/input1.json" 2>"$tmp/err1")
assert_contains "$(cat "$tmp/err1")" "docs/a.md: carried (quiet 1)"
d=$(cat "$tmp/input1.json")
assert_contains "$d" '"path": "docs/a.md"'
assert_contains "$d" "\"since\": \"$C1\""

it "signals/cron/main.go changed since C1: READ from C1 (a delta, not the base)"
assert_contains "$(cat "$tmp/err1")" "signals/cron/main.go: read (since $C1, changed)"
assert_contains "$d" "\"signals/cron/main.go\": \"$C1\""

it "platform/manager/x.go has no marker: READ from the base"
assert_contains "$(cat "$tmp/err1")" "platform/manager/x.go: read (since origin/master, new)"
assert_contains "$d" '"platform/manager/x.go": "origin/master"'

it "the matrix has no job for a component whose every path is carried, only for read paths"
assert_contains "$(cat "$tmp/gho1")" '"paths": ["signals/cron/main.go"]'
assert_not_contains "$(cat "$tmp/gho1")" '"paths": ["docs/a.md"]'

it "the carried list names the path, its sha and its quiet count"
assert_contains "$d" '"carried": [
  {
   "path": "docs/a.md",
   "since": "'"$C1"'",
   "quiet": 1,'

# --- scenario 2: quiet below the threshold keeps reading ------------------
comments2="$tmp/comments2"
marker "$C1" "docs/a.md:0" "signals/cron/main.go:0" > "$comments2"
stub_gh_with_comments "$comments2"

it "unchanged, but quiet 0 < K=1: still READ, not carried"
out=$(cd "$repo" && PATH="$tmp/bin:$PATH" python3 "$INPUT" --repo o/r --number 5 --out "$tmp/input2.json" 2>"$tmp/err2")
assert_contains "$(cat "$tmp/err2")" "docs/a.md: read (since $C1, quiet 0 < 1)"

it "--quiet-reads 2 keeps a quiet-1 path read rather than carried"
comments2b="$tmp/comments2b"
marker "$C1" "docs/a.md:1" "signals/cron/main.go:0" > "$comments2b"
stub_gh_with_comments "$comments2b"
out=$(cd "$repo" && PATH="$tmp/bin:$PATH" python3 "$INPUT" --repo o/r --number 5 --quiet-reads 2 --out "$tmp/input2b.json" 2>"$tmp/err2b")
assert_contains "$(cat "$tmp/err2b")" "docs/a.md: read (since $C1, quiet 1 < 2)"

# --- scenario 3: a rebase invalidates the recorded sha for that path ------
comments3="$tmp/comments3"
marker "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef" "docs/a.md:5" > "$comments3"
stub_gh_with_comments "$comments3"

it "the recorded sha is not an ancestor of head: READ from the base, named rebased"
out=$(cd "$repo" && PATH="$tmp/bin:$PATH" python3 "$INPUT" --repo o/r --number 5 --out "$tmp/input3.json" 2>"$tmp/err3")
assert_contains "$(cat "$tmp/err3")" "docs/a.md: read (since origin/master, rebased)"

# --- scenario 4: a rule file change invalidates the WHOLE record ----------
echo "rule changed" > "$repo/.claude/rules/foo.md"
git -C "$repo" add -A && git -C "$repo" commit -qm "change a rule"

comments4="$tmp/comments4"
marker "$C1" "docs/a.md:1" "signals/cron/main.go:1" > "$comments4"
stub_gh_with_comments "$comments4"

it "a rule file changed since a recorded sha: EVERY path is read, named in the reason"
out=$(cd "$repo" && PATH="$tmp/bin:$PATH" python3 "$INPUT" --repo o/r --number 5 --out "$tmp/input4.json" 2>"$tmp/err4")
assert_contains "$(cat "$tmp/err4")" "docs/a.md: read (since origin/master, a rule file or the change's delta specs changed since a path was read)"
assert_contains "$(cat "$tmp/err4")" "signals/cron/main.go: read (since origin/master,"
assert_contains "$(cat "$tmp/input4.json")" '"coverageInvalidated": "a rule file or the change'"'"'s delta specs changed since a path was read"'

# --- scenario 5: --full ignores every marker -------------------------------
it "--full reads every path from the base, whatever the record says"
out=$(cd "$repo" && PATH="$tmp/bin:$PATH" python3 "$INPUT" --repo o/r --number 5 --full --out "$tmp/input5.json" 2>"$tmp/err5")
assert_contains "$(cat "$tmp/err5")" "docs/a.md: read (since origin/master, a full review was requested)"
assert_contains "$(cat "$tmp/input5.json")" '"coverageInvalidated": "a full review was requested"'

rm -rf "$tmp" "$repo"
summary
