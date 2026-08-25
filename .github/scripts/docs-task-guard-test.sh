#!/usr/bin/env bash
# THE LOCAL GATE AND THE CI CHECK MUST NOT DRIFT.
#
# The documentation rule is asserted twice on purpose: a PreToolUse hook refuses
# `openspec archive` in a session, and a CI check fails a pull request for
# everyone whose harness that is not. Two enforcement points asking one question
# is only safe while they give the same answer -- and "they call the same
# script" is a property that has to be TESTED, not asserted, because the next
# person to fix a bug in one of them will not know the other exists.
#
# So every fixture is judged by BOTH, and a disagreement fails here rather than
# on somebody's pull request.
set -euo pipefail

root="$(cd "$(dirname "$0")/../.." && pwd)"
guard="$root/.github/scripts/docs-task-guard.py"
hook="$root/.claude/hooks/require-docs-task.sh"
fixtures="$root/.github/tests/docs-task"

fail=0
for dir in "$fixtures"/*/; do
  name=$(basename "$dir")
  expected=$(cat "$dir/expected")

  # 1. THE GUARD, judging the file directly.
  got_guard=0
  python3 "$guard" --tasks "$dir/tasks.md" >/dev/null 2>&1 || got_guard=$?

  # 2. THE HOOK, judging the same file through an `openspec archive` command.
  #    It needs a project shaped like the real one, so the fixture is staged
  #    into a throwaway tree at the path the hook resolves.
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' RETURN
  mkdir -p "$tmp/openspec/changes/$name" "$tmp/.github/scripts"
  cp "$dir/tasks.md" "$tmp/openspec/changes/$name/tasks.md"
  cp "$guard" "$tmp/.github/scripts/"

  got_hook=0
  printf '{"tool_input":{"command":"openspec archive %s"}}' "$name" \
    | CLAUDE_PROJECT_DIR="$tmp" bash "$hook" >/dev/null 2>&1 || got_hook=$?
  # The hook signals refusal with 2 (the harness's "block") rather than 1.
  [ "$got_hook" -eq 2 ] && got_hook=1
  rm -rf "$tmp"; trap - RETURN

  if [ "$got_guard" != "$expected" ] || [ "$got_hook" != "$expected" ]; then
    echo "  FAILED   $name — expected $expected, guard said $got_guard, hook said $got_hook"
    fail=1
  else
    echo "  ok       $name (both said $expected)"
  fi
done

if [ "$fail" -ne 0 ]; then
  echo
  echo "The local gate and the CI check disagree, or one of them is wrong." >&2
  echo "They must call the same decision — see .github/scripts/docs-task-guard.py." >&2
  exit 1
fi
echo
echo "the gate and the check agree on every fixture"
