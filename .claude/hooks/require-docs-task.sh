#!/usr/bin/env bash
# REFUSE TO ARCHIVE A CHANGE WHOSE DOCUMENTATION TASK IS NOT DONE.
#
# `.claude/rules/documentation.md` says every change ends with a dedicated
# documentation section covering BOTH the reference docs and the adopter site,
# and `.claude/rules/change-tests.md` says the two sections before it are unit
# tests and e2e tests, ticked. One gate, three sections, one script below.
# A rule stated in prose is followed until the evening someone is tired, and the
# cost lands on a reader weeks later meeting a page describing behaviour that no
# longer exists.
#
# ARCHIVING IS THE RIGHT GATE. It is the point of no return — the deltas fold
# into the specs and the change stops being a thing anyone revisits. Blocking
# earlier would fight normal work; blocking later is impossible.
#
# WHY A HOOK AND NOT A RULE. The harness runs this, not the model, so it does
# not depend on the model having the rule in context at the moment it matters.
# That is the opposite of the session-title hook deleted on 2026-08-23, which
# lost because it competed with Claude Code for the terminal title. Nothing else
# writes this decision, so there is nothing to lose to.
#
# FAILS OPEN on anything it cannot read — a store-backed change, a path it
# cannot resolve, no tasks file. A hook that blocks work it does not understand
# gets disabled, and then it enforces nothing at all.
#
# IT JUDGES THE TREE THE COMMAND RUNS IN, NOT THE ONE THE SESSION STARTED IN.
# `$CLAUDE_PROJECT_DIR` is the MAIN checkout, and every change is archived from
# its own worktree — so reading that path judged a tasks file belonging to a
# different commit of the same change. It read `master`'s copy, where the change
# is untouched and every task is open, and refused an archive whose work was
# finished. It fails CLOSED on a file it can read and should not be reading,
# which is the one failure mode the paragraph above is meant to exclude.
#
# The sibling `require-change-branch.sh` is NOT wrong in the same way and must
# not be "fixed" to match: its job is to refuse a commit in the SHARED checkout,
# so `$CLAUDE_PROJECT_DIR` is exactly the tree it means, and it exits early when
# that tree is a worktree.
set -u

command -v jq >/dev/null 2>&1 || exit 0
payload=$(cat) || exit 0
cmd=$(printf '%s' "$payload" | jq -r '.tool_input.command // empty' 2>/dev/null) || exit 0
[ -n "$cmd" ] || exit 0

# `openspec archive <name>` — and only that. `--store` is not handled, so a
# store-backed archive falls through and is allowed.
case "$cmd" in
  *openspec*archive*) ;;
  *) exit 0 ;;
esac
case "$cmd" in *--store*) exit 0 ;; esac

name=$(printf '%s' "$cmd" | sed -n 's/.*openspec[[:space:]]\+archive[[:space:]]\+\([A-Za-z0-9._-]\+\).*/\1/p')
[ -n "$name" ] || exit 0

# WHERE THE COMMAND WILL ACTUALLY RUN, in the order that answer gets more
# reliable. A leading `cd` wins over the payload's cwd, because that is the
# directory the shell is in by the time `openspec archive` executes.
cd_prefix=$(printf '%s' "$cmd" |
  sed -n 's/^[[:space:]]*cd[[:space:]]\{1,\}\([^&;|]*\).*/\1/p' |
  sed 's/[[:space:]]*$//' | tr -d "\"'")
payload_cwd=$(printf '%s' "$payload" | jq -r '.cwd // empty' 2>/dev/null)

# The first candidate that HOLDS THIS CHANGE wins. Falling back to the session's
# own checkout keeps the gate asking its question when nothing else resolves —
# which is what it did before it knew about worktrees.
root=""
for candidate in "$cd_prefix" "$payload_cwd" "${CLAUDE_PROJECT_DIR:-.}"; do
  [ -n "$candidate" ] || continue
  [ -d "$candidate" ] || continue
  top=$(git -C "$candidate" rev-parse --show-toplevel 2>/dev/null) || top="$candidate"
  if [ -r "$top/openspec/changes/$name/tasks.md" ]; then
    root="$top"
    break
  fi
done
[ -n "$root" ] || exit 0
tasks="$root/openspec/changes/$name/tasks.md"

# THE DECISION IS NOT MADE HERE. `.github/scripts/docs-task-guard.py` is the one
# implementation, and CI calls the same script — so the local gate and the check
# cannot drift into two answers to one question. This hook decides only WHEN to
# ask, and fails open when it cannot ask at all.
guard="$root/.github/scripts/docs-task-guard.py"
command -v python3 >/dev/null 2>&1 || exit 0
[ -r "$guard" ] || exit 0

if ! message=$(python3 "$guard" --tasks "$tasks" 2>&1); then
  cat >&2 <<EOF
BLOCKED: '$name' cannot be archived yet.

$message

Archiving is the point of no return — the deltas fold into the specs and nobody
revisits the change. Finish these, or say explicitly that you are archiving with
tests or documentation outstanding and why.
See .claude/rules/documentation.md and .claude/rules/change-tests.md.
EOF
  exit 2
fi

# THE SECOND QUESTION: IS THE PULL REQUEST'S FIXING LOOP STILL OPEN. A change
# whose pull request carries the `autofix` label may have a round about to land
# a commit, or a dispute the fixing step posted that no person has answered.
# `.github/scripts/autofix-guard.py` reads that off the pull request of the
# worktree's branch, fails open on everything it cannot read (no gh, no pull
# request, no label), and CI's docs-task job asks the same script.
autofix="$root/.github/scripts/autofix-guard.py"
[ -r "$autofix" ] || exit 0
if message=$(cd "$root" && python3 "$autofix" 2>&1); then
  exit 0
fi

cat >&2 <<EOF
BLOCKED: '$name' cannot be archived while its automatic fixing loop is open.

$message

Wait for the round to land, or answer the dispute in its thread (reply, or
resolve it to dismiss). See .claude/rules/worktree-delivery.md.
EOF
exit 2
