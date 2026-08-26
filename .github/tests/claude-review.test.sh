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

it "passes --agents exactly once — a second copy is the one that runs, and it is stale"
assert_equals "1" "$(py 'print(parts.count("--agents"))')"

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

it "excludes the developer-session rules by ** glob, and keeps the review's"
excl=$(py 'print(" ".join(json.loads(parts[parts.index("--settings")+1])["claudeMdExcludes"]))')
for f in build-test worktree-delivery session-naming publication visual-check answering; do
  assert_contains "$excl" "**/.claude/rules/$f.md"
done
for f in invariants terminology wiring retired-vocabulary adapters structure authoring documentation gotchas; do
  assert_not_contains "$excl" "$f.md"
done

it "hands a reviewer the component's whole review history, resolved threads included"
assert_contains "$prompt" "unresolved AND resolved"
assert_contains "$p" "A RESOLVED thread is history, not work"

it "excludes only rule files, never CLAUDE.md"
assert_not_contains "$excl" "CLAUDE.md"

it "every excluded rule exists — a glob that matches nothing is a rule silently kept"
for g in $excl; do [ -f "$ROOT/${g#**/}" ] && pass || fail "no such rule: $g"; done

it "still holds the two-job split: only reconcile has contents: write"
assert_equals "reconcile" "$(python3 -c '
import yaml,sys; d=yaml.safe_load(open(sys.argv[1]))
print(" ".join(j for j,v in d["jobs"].items() if v.get("permissions",{}).get("contents")=="write"))' "$W")"

summary
