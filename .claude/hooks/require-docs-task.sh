#!/usr/bin/env bash
# REFUSE TO ARCHIVE A CHANGE WHOSE DOCUMENTATION TASK IS NOT DONE.
#
# `.claude/rules/documentation.md` says every change ends with a dedicated
# documentation section covering BOTH the reference docs and the adopter site.
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

root="${CLAUDE_PROJECT_DIR:-.}"
tasks="$root/openspec/changes/$name/tasks.md"
[ -r "$tasks" ] || exit 0

# THE DECISION IS NOT MADE HERE. `.github/scripts/docs-task-guard.py` is the one
# implementation, and CI calls the same script — so the local gate and the check
# cannot drift into two answers to one question. This hook decides only WHEN to
# ask, and fails open when it cannot ask at all.
guard="$root/.github/scripts/docs-task-guard.py"
command -v python3 >/dev/null 2>&1 || exit 0
[ -r "$guard" ] || exit 0

if message=$(python3 "$guard" --tasks "$tasks" 2>&1); then
  exit 0
fi

cat >&2 <<EOF
BLOCKED: '$name' cannot be archived yet.

$message

Archiving is the point of no return — the deltas fold into the specs and nobody
revisits the change. Finish these, or say explicitly that you are archiving with
documentation outstanding and why.
See .claude/rules/documentation.md.
EOF
exit 2
