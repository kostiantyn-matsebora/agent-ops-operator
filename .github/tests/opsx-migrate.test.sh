#!/usr/bin/env bash
# Moving the changes already in flight onto the flow.
#
# IT RUNS ONCE, OVER ELEVEN THINGS, AND THEN NEVER AGAIN. That is exactly the
# shape nobody tests by hand and nobody notices going wrong — and the one
# mistake that cannot be undone is resetting a branch somebody was working on.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)

repo=$(make_repo)
cp "$ROOT/.github/scripts/opsx-migrate.sh" "$ROOT/.github/scripts/opsx-issue.sh" "$repo/.github/scripts/"
chmod +x "$repo/.github/scripts/"*.sh
for c in alpha-change beta-change gamma-change; do make_change "$repo" "$c"; done

tmp=$(mktemp -d); stub_gh "$tmp/bin"
export GH_CALLS="$tmp/calls"; : > "$GH_CALLS"

# `openspec list --json` without openspec: the tree is the answer either way.
cat > "$tmp/bin/openspec" <<'STUB'
#!/usr/bin/env bash
printf '{"changes":['
first=1
for d in openspec/changes/*/; do
  n=$(basename "$d"); [ "$n" = "archive" ] && continue
  [ $first -eq 1 ] || printf ','; first=0
  printf '{"name":"%s"}' "$n"
done
printf ']}'
STUB
chmod +x "$tmp/bin/openspec"

M() { (cd "$repo" && GH_CALLS="$GH_CALLS" BASE=master .github/scripts/opsx-migrate.sh "$@" 2>&1); }

it "names every active change"
out=$(M --dry-run)
assert_equals "3" "$(printf '%s\n' "$out" | grep -c 'change$\|change ')"

it "creates nothing under --dry-run"
assert_equals "" "$(git -C "$repo" branch --list 'change/*')"

it "and calls gh not at all"
assert_equals "" "$(cat "$GH_CALLS")"

it "reports what it would do"
assert_contains "$out" "3 branch(es) and 3 issue(s) would be created"

it "creates one branch per change for real"
: > "$GH_CALLS"; M >/dev/null
assert_equals "3" "$(git -C "$repo" branch --list 'change/*' | wc -l | tr -d ' ')"

it "creates one issue per change"
assert_equals "3" "$(grep -c 'issue create' "$GH_CALLS")"

it "binds every one"
assert_equals "3" "$(ls "$repo"/openspec/changes/*/.github-issue 2>/dev/null | wc -l | tr -d ' ')"

it "is idempotent: a second run opens no issue"
: > "$GH_CALLS"; M >/dev/null
assert_not_contains "$(cat "$GH_CALLS")" "issue create"

it "and creates no second branch"
assert_equals "3" "$(git -C "$repo" branch --list 'change/*' | wc -l | tr -d ' ')"

it "NEVER resets a branch that has moved on — that would discard live work"
git -C "$repo" checkout -q change/alpha-change
echo work > "$repo/work.txt"; git -C "$repo" add work.txt
git -C "$repo" commit -qm "work in progress"
head=$(git -C "$repo" rev-parse change/alpha-change)
git -C "$repo" checkout -q master
M >/dev/null
assert_equals "$head" "$(git -C "$repo" rev-parse change/alpha-change)"

it "picks up a change added after the first run"
make_change "$repo" delta-change
: > "$GH_CALLS"; M >/dev/null
assert_equals "1" "$(grep -c 'issue create' "$GH_CALLS")"

rm -rf "$repo" "$tmp"
summary
