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

# The LAST `## ` section is the one the rule is about.
last_heading=$(grep -n '^## ' "$tasks" | tail -1)
[ -n "$last_heading" ] || exit 0
lineno=${last_heading%%:*}
title=${last_heading#*:}

if ! printf '%s' "$title" | grep -qi 'documentation'; then
  cat >&2 <<EOF
BLOCKED: '$name' has no documentation section, and it must be the LAST one.

Its final section is:
  $title

Every change ends with a dedicated documentation task covering BOTH halves,
listed separately because they are skipped independently:

  1. the reference docs   — docs/concepts.md, docs/contracts.md, a bundle page,
                            docs/CHANGELOG.md
  2. THE ADOPTER SITE     — the landing page, introduction.md,
                            getting-started.md, installation.md, docs/guides/*

Add it to $tasks, complete it, then archive.
See .claude/rules/documentation.md.
EOF
  exit 2
fi

undone=$(tail -n +"$lineno" "$tasks" | grep -c '^- \[ \]')
if [ "$undone" -gt 0 ]; then
  cat >&2 <<EOF
BLOCKED: '$name' has $undone unfinished documentation task(s).

$(tail -n +"$lineno" "$tasks" | grep '^- \[ \]' | sed 's/^/  /')

Archiving records the work as finished while the half a reader meets is not.
Finish these, or say explicitly that you are archiving with documentation
outstanding and why.
See .claude/rules/documentation.md.
EOF
  exit 2
fi

exit 0
