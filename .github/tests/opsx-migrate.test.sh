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

# READS, BUT NEVER WRITES. A dry-run that asked GitHub nothing could not report
# "adopted" against "would open", which is the distinction it exists to make —
# and getting that wrong once nearly opened a duplicate issue for a change a
# concurrent session had already tracked.
it "and writes nothing through gh under --dry-run"
assert_equals "0" "$(grep -cE 'issue (create|edit|comment|close)' "$GH_CALLS" || true)"

it "but does ask GitHub whether an issue already exists"
assert_contains "$(cat "$GH_CALLS")" "issue list"

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

# --- adoption: the sidecar is a CACHE, GitHub is the source of truth ---------
#
# The sidecar is committed on the change's OWN branch, so from the default branch
# it does not exist — and a fresh clone, a cleaned tree or a concurrent session
# holding the change all present the same blank. Without asking GitHub, this
# script opens a SECOND issue for a change that already has one, which is what
# nearly happened on 2026-08-25.

repo2=$(make_repo)
cp "$ROOT/.github/scripts/opsx-migrate.sh" "$ROOT/.github/scripts/opsx-issue.sh" "$repo2/.github/scripts/"
chmod +x "$repo2/.github/scripts/"*.sh
cp "$tmp/bin/openspec" "$tmp/bin/openspec2" 2>/dev/null || true
for c in alpha-change alpha-change-two; do make_change "$repo2" "$c"; done
export GH_ISSUES="$tmp/issues.json"
cat > "$GH_ISSUES" <<'JSON'
[
  {"number":77,"title":"alpha-change: something a proposal said","labels":[{"name":"opsx:applying"}],"body":"points at openspec/changes/alpha-change/"},
  {"number":78,"title":"a reporter's own words","labels":[{"name":"opsx:proposed"}],"body":"| **Change** | `openspec/changes/gamma-change/` |"},
  {"number":79,"title":"alpha-change-two: unrelated","labels":[{"name":"enhancement"}],"body":"no label of ours"}
]
JSON
M2() { (cd "$repo2" && PATH="$tmp/bin:$PATH" BASE=master GH_CALLS="$GH_CALLS" \
        GH_ISSUES="$GH_ISSUES" .github/scripts/opsx-migrate.sh "$@" 2>&1); }

# ORDER MATTERS HERE: the dry-run assertions run BEFORE the real one, because a
# real run writes sidecars and every later lookup is then answered locally.
it "reports an adoption under --dry-run without writing anything"
out=$(M2 --dry-run)
assert_contains "$out" "issue:#77 (adopted)"
assert_equals "" "$(cat "$repo2/openspec/changes/alpha-change/.github-issue" 2>/dev/null)"

it "does not hand one change another's issue on a name prefix"
assert_not_contains "$out" "alpha-change-two                   branch:would issue:#77 (adopted)"

it "ignores an issue that carries no opsx label"
assert_not_contains "$out" "issue:#79"

it "warns about the PROMOTED case, which no lookup can see"
assert_contains "$out" "PROMOTED"

# A FAILED LOOKUP IS NOT "NO MATCH". An expired token, a rate limit or a dropped
# connection would otherwise fall straight through to opening an issue — the
# duplicate this whole lookup exists to prevent, arriving through a different
# door and with no dry-run in between to catch it.
it "ABORTS when the issue list cannot be read, rather than opening one"
: > "$GH_CALLS"
out=$(GH_LIST_FAILS=1 M2 2>&1) && rc=0 || rc=$?
assert_status 1 "$rc"
assert_contains "$out" "FATAL"

it "and opens nothing at all when the lookup failed"
assert_not_contains "$(cat "$GH_CALLS")" "issue create"

# ONE FETCH PER RUN, not per change: the list is repository-wide and identical
# every time, and each extra call is another chance for a transient failure.
it "reads the issue list once for the whole run, not once per change"
: > "$GH_CALLS"
M2 --dry-run >/dev/null
assert_equals "1" "$(grep -c 'issue list' "$GH_CALLS" || true)"

it "ADOPTS an existing tracking issue rather than opening a second one"
: > "$GH_CALLS"
assert_contains "$(M2)" "issue:#77 (adopted)"

it "writes the adopted number back, so the next run needs no lookup"
assert_equals "77" "$(cat "$repo2/openspec/changes/alpha-change/.github-issue")"

it "opens exactly one issue: for the change that has none, never for the adopted one"
assert_equals "1" "$(grep -c 'issue create' "$GH_CALLS" || true)"

it "picks up a change added after the first run"
make_change "$repo" delta-change
: > "$GH_CALLS"; M >/dev/null
assert_equals "1" "$(grep -c 'issue create' "$GH_CALLS")"

rm -rf "$repo" "$tmp"
summary
