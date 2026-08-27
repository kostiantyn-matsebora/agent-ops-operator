#!/usr/bin/env bash
# The shape of the review — the lines that make a per-component review safe,
# each of which could be "simplified" away with every run still green.
#
# THE PLAN IS A SCRIPT AND THE ROLES ARE FILES. `.claude/workflows/review-pr.js`
# runs one `component-reviewer` per component and hands every reading to the
# `review-coordinator`; the workflow file only starts it, and restores all
# three from the base branch first. The tests here read the workflow file, the
# two role files and the script, and assert the guard on each.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
W="$ROOT/.github/workflows/claude-review.yml"
REVIEWER="$ROOT/.claude/agents/component-reviewer.md"
COORD="$ROOT/.claude/agents/review-coordinator.md"
SCRIPT="$ROOT/.claude/workflows/review-pr.js"

py() { python3 -c "
import sys, yaml, json, shlex
d = yaml.safe_load(open(sys.argv[1]))
steps = d['jobs']['review']['steps']
step = [s for s in steps if 'claude-code-action' in s.get('uses','')][0]
parts = shlex.split(step['with']['claude_args'])
allowed = parts[parts.index('--allowedTools')+1]
prompt = step['with']['prompt']
restore = [s for s in steps if 'base branch' in s.get('name','')]
$1
" "$W"; }

# frontmatter <key> <file>
fm() { python3 -c "
import sys,re
t=open(sys.argv[2]).read(); m=re.match(r'---\n(.*?)\n---', t, re.S); fm=m.group(1)
for line in fm.splitlines():
    k,_,v=line.partition(':')
    if k.strip()==sys.argv[1]: print(v.strip())
" "$1" "$2"; }
body() { python3 -c "import sys,re; t=open(sys.argv[1]).read(); print(re.sub(r'^---\n.*?\n---\n', '', t, flags=re.S))" "$1"; }

it "defines no reviewer inline: the roles are files"
assert_equals "0" "$(py 'print(parts.count("--agents"))')"
[ -f "$REVIEWER" ] && pass || fail "missing $REVIEWER"
[ -f "$COORD" ] && pass || fail "missing $COORD"
[ -f "$SCRIPT" ] && pass || fail "missing $SCRIPT"

it "restores the script and both roles from the base branch before the action runs"
assert_equals "1" "$(py 'print(len(restore))')"
run=$(py 'print(restore[0]["run"])')
assert_contains "$run" 'github.base_ref'
for f in .claude/workflows/review-pr.js .claude/agents/component-reviewer.md .claude/agents/review-coordinator.md; do
  assert_contains "$run" "$f"
done
assert_contains "$run" 'git checkout "$base" -- "$f"'
idx_restore=$(py 'print([i for i,s in enumerate(steps) if s in restore][0])')
idx_action=$(py 'print(steps.index(step))')
[ "$idx_restore" -lt "$idx_action" ] && pass || fail "restore step must precede the action"

it "lets the main context run the workflow and spawn only the two roles"
allowed=$(py 'print(allowed)')
assert_contains "$allowed" "Workflow"
assert_contains "$allowed" "Agent(component-reviewer)"
assert_contains "$allowed" "Agent(review-coordinator)"
assert_not_contains "$(printf '%s' "$allowed" | sed 's/Agent(component-reviewer)//; s/Agent(review-coordinator)//')" "Agent"

it "starts the saved workflow with the pull request's coordinates, and nothing else"
prompt=$(py 'print(prompt)')
assert_contains "$prompt" 'review-pr'
assert_contains "$prompt" '"repo": "${{ github.repository }}"'
assert_contains "$prompt" '"number": ${{ github.event.pull_request.number }}'
assert_contains "$prompt" '"base": "origin/${{ github.base_ref }}"'
assert_contains "$prompt" 'Do not post'
assert_not_contains "$prompt" 'Agent'

it "no longer keeps a checklist by hand"
assert_equals "False" "$(py 'print(step["with"].get("track_progress", False))')"

it "gives the reviewer no posting tool, and no way to spawn"
tools=$(fm tools "$REVIEWER")
assert_not_contains "$tools" "gh"
assert_not_contains "$tools" "mcp__"
assert_not_contains "$tools" "mark-thread-resolved"
assert_not_contains "$tools" "Write"
assert_not_contains "$tools" "Edit"
assert_not_contains "$tools" "Agent"

it "asks the reviewer for JSON with the three fields the coordinator reads"
p=$(body "$REVIEWER")
assert_contains "$p" '"changedNames"'
assert_contains "$p" '"threads"'
assert_contains "$p" '"findings"'

it "asks the reviewer for the four finding fields the inline comment is built from, with the caps"
assert_contains "$p" '"where"'
assert_contains "$p" '"fix"'
assert_contains "$p" 'AT MOST 15 WORDS, ONE CLAUSE'
assert_contains "$p" 'AT MOST 12 WORDS'

it "hands a reviewer the component's whole review history, resolved threads included"
assert_contains "$p" "A RESOLVED thread is history, not work"
assert_contains "$(cat "$SCRIPT")" "resolved and unresolved alike"

it "the coordinator is the only writer, and posts a finding as four labeled lines"
c=$(body "$COORD")
assert_contains "$c" '**Claim:**'
assert_contains "$c" '**Where:**'
assert_contains "$c" '**Rule:**'
assert_contains "$c" '**Fix:**'
assert_contains "$(fm tools "$COORD")" "gh pr comment"
assert_not_contains "$(fm tools "$COORD")" "Agent"

it "consolidates in the coordinator: reach, unreviewed, and the first finding"
assert_contains "$c" "git grep -l -F"
assert_contains "$c" "unreviewed: <group>"
assert_contains "$c" ".claude/rules/"
assert_contains "$c" ".claude/workflows/"

it "the script runs the queue, then one reviewer per component, then the coordinator — and validates each return"
s=$(cat "$SCRIPT")
assert_equals "export const meta = {" "$(grep -m1 '^export const meta' "$SCRIPT")"
assert_contains "$s" "name: 'review-pr'"
assert_contains "$s" "review-queue.py"
assert_contains "$s" "pipeline(q.queue"
assert_contains "$s" "agentType: 'component-reviewer'"
assert_contains "$s" "agentType: 'review-coordinator'"
assert_contains "$s" "schema: FINDINGS"
assert_contains "$s" "schema: OUTCOME"
assert_contains "$s" "required: ['component', 'findings', 'changedNames', 'threads']"

it "the script's dry run stops before the coordinator, so nothing is posted"
assert_contains "$s" "if (dryRun) return"
dry=$(python3 -c "import sys; s=open(sys.argv[1]).read(); print(s.index('if (dryRun) return') < s.index(\"agentType: 'review-coordinator'\"))" "$SCRIPT")
assert_equals "True" "$dry"

it "excludes the developer-session rules by ** glob, and keeps the review's"
excl=$(py 'print(" ".join(json.loads(parts[parts.index("--settings")+1])["claudeMdExcludes"]))')
for f in build-test worktree-delivery session-naming publication visual-check answering; do
  assert_contains "$excl" "**/.claude/rules/$f.md"
done
for f in invariants terminology wiring retired-vocabulary adapters structure authoring documentation gotchas; do
  assert_not_contains "$excl" "$f.md"
done

it "excludes only rule files, never CLAUDE.md"
assert_not_contains "$excl" "CLAUDE.md"

it "every excluded rule exists — a glob that matches nothing is a rule silently kept"
for g in $excl; do [ -f "$ROOT/${g#**/}" ] && pass || fail "no such rule: $g"; done

it "still holds the two-job split: only reconcile has contents: write"
assert_equals "reconcile" "$(python3 -c '
import yaml,sys; d=yaml.safe_load(open(sys.argv[1]))
print(" ".join(j for j,v in d["jobs"].items() if v.get("permissions",{}).get("contents")=="write"))' "$W")"

summary
