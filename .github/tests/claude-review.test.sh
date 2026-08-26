#!/usr/bin/env bash
# The shape of the review workflow — the lines that make a per-component review
# safe, each of which could be "simplified" away with every run still green.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
W="$ROOT/.github/workflows/claude-review.yml"

py() { python3 -c "
import sys, yaml, json, shlex
d = yaml.safe_load(open(sys.argv[1]))
step = [s for s in d['jobs']['review']['steps'] if 'claude-code-action' in s.get('uses','')][0]
parts = shlex.split(step['with']['claude_args'])
allowed = parts[parts.index('--allowedTools')+1]
agents = json.loads(parts[parts.index('--agents')+1])
reviewer = agents['component-reviewer']
$1
" "$W"; }

it "defines exactly one reviewer, inline, in this file"
assert_equals "component-reviewer" "$(py 'print(" ".join(agents))')"

it "does not ship a checked-in reviewer the branch could rewrite"
[ ! -e "$ROOT/.claude/agents" ] && pass || fail ".claude/agents/ exists"

it "lets the main context spawn only that reviewer"
assert_contains "$(py 'print(allowed)')" "Agent(component-reviewer)"
assert_not_contains "$(py 'print(allowed.replace("Agent(component-reviewer)",""))')" "Agent"

it "gives the reviewer no posting tool"
tools=$(py 'print(" ".join(reviewer["tools"]))')
assert_not_contains "$tools" "gh"
assert_not_contains "$tools" "mcp__"
assert_not_contains "$tools" "mark-thread-resolved"
assert_not_contains "$tools" "Write"
assert_not_contains "$tools" "Edit"

it "does not let the reviewer spawn"
assert_not_contains "$tools" "Agent"

it "bounds the reviewer"
assert_equals "True" "$(py 'print(isinstance(reviewer.get("maxTurns"), int))')"

it "asks the reviewer for JSON with the three fields the consolidator reads"
p=$(py 'print(reviewer["prompt"])')
prompt=$(py 'print(step["with"]["prompt"])')
assert_contains "$p" '"changedNames"'
assert_contains "$p" '"threads"'
assert_contains "$p" '"findings"'

it "asks the reviewer for the four finding fields the inline comment is built from, with the caps"
assert_contains "$p" '"where"'
assert_contains "$p" '"fix"'
assert_contains "$p" 'AT MOST 15 WORDS, ONE CLAUSE'
assert_contains "$p" 'AT MOST 12 WORDS'

it "posts a finding as four labeled lines, not prose"
assert_contains "$prompt" '**Claim:**'
assert_contains "$prompt" '**Where:**'
assert_contains "$prompt" '**Rule:**'
assert_contains "$prompt" '**Fix:**'

it "keeps the reviewer definition free of apostrophes, which end the shell argument"
assert_equals "0" "$(py 'print(json.dumps(reviewer).count(chr(39)))')"

it "consolidates in the main context: queue, reach, and the first finding"
assert_contains "$prompt" "review-queue.py"
assert_contains "$prompt" "git grep -l -F"
assert_contains "$prompt" "unreviewed: <group>"
assert_contains "$prompt" ".claude/rules/"

it "still holds the two-job split: only reconcile has contents: write"
assert_equals "reconcile" "$(python3 -c '
import yaml,sys; d=yaml.safe_load(open(sys.argv[1]))
print(" ".join(j for j,v in d["jobs"].items() if v.get("permissions",{}).get("contents")=="write"))' "$W")"

summary
