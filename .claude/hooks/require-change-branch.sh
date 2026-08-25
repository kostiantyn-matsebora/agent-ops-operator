#!/usr/bin/env bash
# REFUSE TO COMMIT A CHANGE'S WORK TO master FROM THE SHARED CHECKOUT.
#
# `.claude/rules/worktree-delivery.md` says every openspec change is implemented
# in its own worktree, on `change/<name>`, and lands as a pull request. This is
# the half a rule cannot enforce: the harness runs this, not the model, so it
# does not depend on the rule being in context at the moment it matters.
#
# WHY THE SHARED CHECKOUT IS THE SUBJECT. Several sessions run this repository at
# once. While they shared one working copy they shared one HEAD and one set of
# files, and it cost work twice — a branch collision on 2026-08-23 that had to be
# cherry-picked back, and on 2026-08-24 a `clean` that deleted another session's
# entire unstaged change directory. The old rule (commit straight to master,
# never branch) contained the first and not the second.
#
# HOW IT TELLS THE CASES APART, without guessing where a command will run.
# It inspects the MAIN checkout's own index. A commit made inside a worktree
# stages nothing here, so this finds nothing and says nothing — which is exactly
# right, and needs no parsing of `cd` out of a command line.
#
# FAILS OPEN on anything it cannot read — no jq, no git, an unreadable index, a
# detached HEAD. A hook that blocks work it does not understand gets disabled,
# and then it enforces nothing at all. The CI check asserting the same decision
# is what makes failing open safe.
#
# NOT A BAN ON COMMITTING HERE. A typo, a broken link or a one-line fix with no
# openspec change behind it stages nothing under openspec/changes/, so it passes.
set -u

command -v jq  >/dev/null 2>&1 || exit 0
command -v git >/dev/null 2>&1 || exit 0

payload=$(cat) || exit 0
cmd=$(printf '%s' "$payload" | jq -r '.tool_input.command // empty' 2>/dev/null) || exit 0
[ -n "$cmd" ] || exit 0

# `git commit` only. Not `git add`, not `git status` — the refusal belongs at the
# point the history would actually gain the commit.
printf '%s' "$cmd" | grep -qE '(^|[;&|[:space:]])git([[:space:]]+-[^[:space:]]+)*[[:space:]]+commit([[:space:]]|$)' || exit 0

root="${CLAUDE_PROJECT_DIR:-.}"
[ -d "$root/.git" ] || exit 0        # a worktree's .git is a FILE, so this is the main checkout only

branch=$(git -C "$root" rev-parse --abbrev-ref HEAD 2>/dev/null) || exit 0
default=$(git -C "$root" symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null)
default=${default#origin/}
[ -n "$default" ] || default=master
[ "$branch" = "$default" ] || exit 0

staged=$(git -C "$root" diff --cached --name-only 2>/dev/null) || exit 0
[ -n "$staged" ] || exit 0

# Which changes does this commit touch, and which of them already own a branch?
owned=""
while IFS= read -r name; do
  [ -n "$name" ] || continue
  if git -C "$root" show-ref --verify --quiet "refs/heads/change/$name"; then
    owned="$owned  $name -> change/$name
"
  fi
done <<EOF
$(printf '%s\n' "$staged" | sed -n 's|^openspec/changes/\([^/]*\)/.*|\1|p' | grep -v '^archive$' | sort -u)
EOF

[ -n "$owned" ] || exit 0

cat >&2 <<EOF
BLOCKED: this commit belongs to a change that already owns a branch, and you are
on '$branch' in the SHARED checkout.

$owned
Every openspec change is implemented in its own worktree and lands as a pull
request. Committing it here puts it on the default branch instead, and leaves
the branch to diverge from work nobody can see.

Work it in its worktree:

  git worktree list                                   # is one already open?
  git worktree add ../agent-ops-worktrees/<name> change/<name>
  cd ../agent-ops-worktrees/<name>

If this really is unrelated work that happens to touch that directory, unstage
the change's files and commit the rest.

See .claude/rules/worktree-delivery.md.
EOF
exit 2
