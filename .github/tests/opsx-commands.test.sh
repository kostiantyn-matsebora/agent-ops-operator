#!/usr/bin/env bash
# The opsx command and skill files are openspec-GENERATED scaffolding that this
# repository EDITS: the tracking-issue flow, the worktree rule, the role
# dispatch and the cross-review all live as marked steps inside them. An
# `openspec` update regenerates the scaffolding, and a regeneration that drops
# a marked step retires that flow SILENTLY — the commands keep working, minus
# the one thing this repository added. This test makes that loud: re-apply the
# marked steps, then update the CLI.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
CMDS="$ROOT/.claude/commands/opsx"

it "the apply command carries the repo steps: working copy, dispatch, cross-review"
f=$(cat "$CMDS/apply.md")
assert_contains "$f" "Work in the change's own working copy"
assert_contains "$f" "Dispatch each named section to its role agent"
assert_contains "$f" "Cross-role review before the pull request"

it "the apply skill mirrors the same three"
f=$(cat "$ROOT/.claude/skills/openspec-apply-change/SKILL.md")
assert_contains "$f" "Work in the change's own working copy"
assert_contains "$f" "Dispatch each named section to its role agent"
assert_contains "$f" "Cross-role review before the pull request"

it "the propose command carries the tracking issue and the contract role"
f=$(cat "$CMDS/propose.md")
assert_contains "$f" "Open the tracking issue"
assert_contains "$f" "Contract-shaped artifacts are drafted by the contract role"

it "the update command carries the contract role"
assert_contains "$(cat "$CMDS/update.md")" "Contract-shaped revisions go through the contract role"

it "the archive command carries its repo step"
assert_contains "$(cat "$CMDS/archive.md")" "Close the tracking issue"

it "every role agent the apply command names exists on disk"
names=$(grep -oE '[a-z]+-(architect|developer|engineer|specialist)' "$CMDS/apply.md" | sort -u)
[ -n "$names" ] && pass || fail "no role agent names parsed from apply.md"
for a in $names; do
  [ -f "$ROOT/.claude/agents/$a.md" ] && pass || fail "apply.md names $a but .claude/agents/$a.md is missing"
done

it "the tasks rule naming role agents is in the openspec config"
assert_contains "$(cat "$ROOT/openspec/config.yaml")" "EACH IMPLEMENTATION SECTION NAMES THE ROLE AGENT"

summary
