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

# --- CI's own mode: WHICH changes it judges, and when --------------------------
#
# The fixtures above ask "is this task list finished?" — one file, one answer,
# shared by the hook. What follows asks the question CI has to answer for itself:
# is this change being FINISHED by this pull request? Proposing is not finishing,
# and archiving is.

echo
r=$(mktemp -d)
git -C "$r" init -q -b master
git -C "$r" config user.email test@example.com
git -C "$r" config user.name Test
mkdir -p "$r/.github/scripts"; cp "$guard" "$r/.github/scripts/"
echo seed > "$r/README.md"; git -C "$r" add .; git -C "$r" commit -qm seed
base=$(git -C "$r" rev-parse HEAD)

plan() {  # plan <name> <implementation-state> <docs-state>
  local d="$r/openspec/changes/$1"; mkdir -p "$d"
  { printf '## 1. The work\n\n- [%s] 1.1 do the thing\n\n' "$2"
    printf '## 2. Documentation\n\n- [%s] 2.1 docs/concepts.md\n' "$3"; } > "$d/tasks.md"
}

ci() { (cd "$r" && python3 .github/scripts/docs-task-guard.py --range "$base..HEAD" --root . 2>&1); }

check() {  # check <label> <expected-status> <expected-substring>
  # `|| got=$?` in ONE statement: a non-zero status is what half these cases
  # assert, and under `set -e` a bare assignment from a failing command ends the
  # run — silently, three checks early.
  local label="$1" want="$2" needle="$3" got=0 out
  out=$(ci) || got=$?
  if [ "$got" = "$want" ] && printf '%s' "$out" | grep -q "$needle"; then
    echo "  ok       $label"
  else
    echo "  FAILED   $label — status $got (wanted $want), output:"; printf '%s\n' "$out" | sed 's/^/      /'
    fail=1
  fi
}

# A PROPOSAL: nothing implemented, nothing documented. This failed every
# /opsx:propose pull request until 2026-08-26 — #54 was the case.
plan proposed ' ' ' '
git -C "$r" add -A; git -C "$r" commit -qm "propose"
check "a proposal is pending, not failed" 0 "pending  proposed"

# THE WORK IS DONE AND THE DOCS ARE NOT. This is what the rule is for.
plan proposed 'x' ' '
git -C "$r" add -A; git -C "$r" commit -qm "implement"
check "a finished change with unticked docs FAILS" 1 "FAILED   proposed"

# STRUCTURE IS JUDGED WHATEVER THE PHASE: a task list whose last section is not
# documentation is malformed on the day it is written, and that is the cheapest
# day to say so.
mkdir -p "$r/openspec/changes/shapeless"
printf '## 1. Documentation\n\n- [x] 1.1 docs\n\n## 2. Something else\n\n- [ ] 2.1 x\n' \
  > "$r/openspec/changes/shapeless/tasks.md"
git -C "$r" add -A; git -C "$r" commit -qm "shapeless"
check "a proposal whose last section is not documentation FAILS" 1 "FAILED   shapeless"

# ARCHIVING IS THE CLAIM OF COMPLETION, and it is the case CI never saw: the live
# directory is gone from the diff, so the old scoping said "touches no openspec
# change" on the very pull request that archived one.
r2=$(mktemp -d)
git -C "$r2" init -q -b master
git -C "$r2" config user.email test@example.com; git -C "$r2" config user.name Test
mkdir -p "$r2/.github/scripts"; cp "$guard" "$r2/.github/scripts/"
echo seed > "$r2/README.md"; git -C "$r2" add .; git -C "$r2" commit -qm seed
base2=$(git -C "$r2" rev-parse HEAD)
mkdir -p "$r2/openspec/changes/archive/2026-08-26-landed"
printf '## 1. The work\n\n- [x] 1.1 done\n\n## 2. Documentation\n\n- [ ] 2.1 docs/concepts.md\n' \
  > "$r2/openspec/changes/archive/2026-08-26-landed/tasks.md"
git -C "$r2" add -A; git -C "$r2" commit -qm "archive it"
got=0
out=$( (cd "$r2" && python3 .github/scripts/docs-task-guard.py --range "$base2..HEAD" --root . 2>&1) ) || got=$?
if [ "$got" = 1 ] && printf '%s' "$out" | grep -q "FAILED   landed (archiving)"; then
  echo "  ok       an archiving pull request is judged at the archived path"
else
  echo "  FAILED   an archiving pull request is judged at the archived path — status $got"
  printf '%s\n' "$out" | sed 's/^/      /'; fail=1
fi

rm -rf "$r" "$r2"
[ "$fail" -eq 0 ] || { echo; echo "CI mode judged the wrong changes." >&2; exit 1; }
echo
echo "and CI judges a change it FINISHES, never one it merely touches"
