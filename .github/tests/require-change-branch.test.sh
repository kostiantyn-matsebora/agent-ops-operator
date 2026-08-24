#!/usr/bin/env bash
# The hook that refuses a change's commit on the shared checkout.
#
# THE TWO DIRECTIONS MATTER EQUALLY. Refusing too little makes it decorative;
# refusing too much makes it the thing somebody disables, and then it enforces
# nothing at all.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
HOOK="$ROOT/.claude/hooks/require-change-branch.sh"

run_hook() {  # run_hook <repo> <command>
  printf '{"tool_input":{"command":"%s"}}' "$2" \
    | CLAUDE_PROJECT_DIR="$1" bash "$HOOK" 2>&1
}
status_of() { printf '{"tool_input":{"command":"%s"}}' "$2" \
    | CLAUDE_PROJECT_DIR="$1" bash "$HOOK" >/dev/null 2>&1; echo $?; }

repo=$(make_repo)
make_change "$repo" demo-change
git -C "$repo" add openspec/changes/demo-change >/dev/null

it "allows a change's commit while no branch owns it"
assert_status 0 "$(status_of "$repo" "git commit -m x")"

git -C "$repo" branch change/demo-change

it "refuses once the change owns a branch"
assert_status 2 "$(status_of "$repo" "git commit -m x")"

it "names the change and its branch in the refusal"
assert_contains "$(run_hook "$repo" "git commit -m x")" "demo-change -> change/demo-change"

it "points at the worktree rather than just saying no"
assert_contains "$(run_hook "$repo" "git commit -m x")" "git worktree add"

it "allows unrelated work even while the branch exists"
git -C "$repo" reset -q; echo hi > "$repo/TYPO.md"; git -C "$repo" add TYPO.md
assert_status 0 "$(status_of "$repo" "git commit -m 'docs: typo'")"

it "ignores commands that are not a commit"
git -C "$repo" reset -q; git -C "$repo" add openspec/changes/demo-change
assert_status 0 "$(status_of "$repo" "git status")"

it "ignores a commit with nothing staged"
git -C "$repo" reset -q
assert_status 0 "$(status_of "$repo" "git commit -m x")"

it "ignores archived changes, whose directory shape is the same"
mkdir -p "$repo/openspec/changes/archive/old-thing"
echo x > "$repo/openspec/changes/archive/old-thing/tasks.md"
git -C "$repo" add openspec/changes/archive >/dev/null
git -C "$repo" branch change/archive 2>/dev/null
assert_status 0 "$(status_of "$repo" "git commit -m x")"

it "says nothing on a branch that is not the default"
git -C "$repo" reset -q; git -C "$repo" add openspec/changes/demo-change
git -C "$repo" checkout -q -b change/demo-change-work
assert_status 0 "$(status_of "$repo" "git commit -m x")"
git -C "$repo" checkout -q master

it "FAILS OPEN when the project directory is not a repository"
assert_status 0 "$(status_of "/nonexistent-$$" "git commit -m x")"

rm -rf "$repo"
summary
