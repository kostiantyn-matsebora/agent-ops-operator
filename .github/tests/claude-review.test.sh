#!/usr/bin/env bash
# The shape of the review — the lines that make a per-component review safe,
# each of which could be "simplified" away with every run still green.
#
# THE PLAN IS THE WORKFLOW AND THE ROLES ARE FILES. `claude-review.yml` builds
# the queue with a program, runs one `component-reviewer` job per changed
# component, hands every reading to the `review-coordinator` in one job, and
# resolves threads in a fourth that runs no model. Every job that runs a model
# installs the CLI through one composite action and restores what it reads
# from the base branch first. The tests here read the workflow, the action and
# the two role files, and assert the guard on each.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
W="$ROOT/.github/workflows/claude-review.yml"
A="$ROOT/.github/actions/claude-cli/action.yml"
REVIEWER="$ROOT/.claude/agents/file-reviewer.md"
COORD="$ROOT/.claude/agents/review-coordinator.md"
WF="$ROOT/.claude/workflows/review-component.js"

py() { python3 -c "
import sys, yaml, json, re
d = yaml.safe_load(open(sys.argv[1]))
jobs = d['jobs']
def steps(j): return jobs[j]['steps']
def runs(j): return '\n'.join(s.get('run','') for s in steps(j))
def uses(j): return [s.get('uses','') for s in steps(j)]
def step(j, key, val): return [s for s in steps(j) if s.get(key)==val][0]
def envkeys(j): return ' '.join(k for s in steps(j) for k in s.get('env',{}))
$1
" "$W"; }
av() { python3 -c 'import yaml,sys;print(yaml.safe_load(open(sys.argv[1]))["inputs"]["version"]["default"])' "$A"; }

# frontmatter <key> <file>
fm() { python3 -c "
import sys,re
t=open(sys.argv[2]).read(); m=re.match(r'---\n(.*?)\n---', t, re.S); fm=m.group(1)
for line in fm.splitlines():
    k,_,v=line.partition(':')
    if k.strip()==sys.argv[1]: print(v.strip())
" "$1" "$2"; }
body() { python3 -c "import sys,re; t=open(sys.argv[1]).read(); print(re.sub(r'^---\n.*?\n---\n', '', t, flags=re.S))" "$1"; }

it "is four jobs: queue, read, consolidate, reconcile — and nothing else"
assert_equals "queue read consolidate reconcile" "$(py 'print(" ".join(jobs))')"
assert_equals "queue" "$(py 'print(jobs["read"]["needs"])')"
assert_equals "queue read" "$(py 'print(" ".join(jobs["consolidate"]["needs"]))')"
assert_equals "queue consolidate" "$(py 'print(" ".join(jobs["reconcile"]["needs"]))')"

it "the privileged job runs the base branch's resolver, never the pull request's"
assert_contains "$(py 'print(step("reconcile","uses","actions/checkout@v4")["with"]["ref"])')" "needs.queue.outputs.base"

it "the queue runs no model: a program builds it — the base branch's program"
assert_not_contains "$(py 'print(runs("queue"))')" "claude -p"
assert_contains "$(py 'print(runs("queue"))')" "review-input.py"
assert_not_contains "$(py 'print(" ".join(uses("queue")))')" "claude-cli"
q=$(py 'print(step("queue","id","input")["run"])')
for f in .github/scripts/review-input.py .github/scripts/review-queue.py .github/components.sh; do assert_contains "$q" "$f"; done
assert_contains "$q" 'git checkout "$BASE" -- "$f"'
python3 -c 'import sys; s=sys.argv[1]; sys.exit(0 if s.index("git checkout") < s.index("python3 .github/scripts/review-input.py") else 1)' "$q" && pass || fail "the restore must precede the program it restores"

it "every restore of tooling says so when the pull request's copy differed"
for j in queue read consolidate; do
  assert_contains "$(py "print(runs('$j'))")" '[ -z "$restored" ] || echo "::notice::'
done

it "the read matrix is the queue's component list, one job each, and a failed reading fails alone"
assert_contains "$(py 'print(jobs["read"]["strategy"]["matrix"]["entry"])')" "fromJSON(needs.queue.outputs.groups)"
assert_equals "False" "$(py 'print(jobs["read"]["strategy"]["fail-fast"])')"
assert_equals "True" "$(py 'print(jobs["read"]["continue-on-error"])')"
assert_contains "$(py 'print(jobs["read"]["if"])')" "needs.queue.outputs.count != '0'"

it "every model job installs the CLI through the composite action, after restoring the action itself from the base"
for j in read consolidate; do
  assert_contains "$(py "print(' '.join(uses('$j')))")" "./.github/actions/claude-cli"
  idx_restore=$(py "print([i for i,s in enumerate(steps('$j')) if 'actions/claude-cli/action.yml' in s.get('run','')][0])")
  idx_uses=$(py "print([i for i,s in enumerate(steps('$j')) if s.get('uses','')=='./.github/actions/claude-cli'][0])")
  [ "$idx_restore" -lt "$idx_uses" ] && pass || fail "$j: the action must be restored before uses: resolves it"
done

it "each model job restores its own role from the base branch, through the action"
assert_contains "$(py 'print(step("read","uses","./.github/actions/claude-cli")["with"]["restore"])')" ".claude/agents/file-reviewer.md"
assert_contains "$(py 'print(step("read","uses","./.github/actions/claude-cli")["with"]["restore"])')" ".claude/workflows/review-component.js"
assert_contains "$(py 'print(step("consolidate","uses","./.github/actions/claude-cli")["with"]["restore"])')" ".claude/agents/review-coordinator.md"
assert_contains "$(py 'print(step("read","uses","./.github/actions/claude-cli")["with"]["base-ref"])')" "needs.queue.outputs.base"

it "the action pins the version once, caches on it, and asserts what it installed"
assert_contains "$(av)" "."
assert_contains "$(cat "$A")" "actions/cache@"
assert_contains "$(cat "$A")" 'claude-cli-${{ inputs.version }}'
assert_contains "$(cat "$A")" 'claude --version'
assert_contains "$(cat "$A")" 'git checkout "$BASE" -- "$f"'
assert_equals "" "$(grep -rl --fixed-strings "$(av)" "$ROOT/.github" | grep -v actions/claude-cli || true)"

it "the component session runs the saved workflow and may spawn only the file reader"
r=$(py 'print(runs("read"))')
assert_contains "$r" 'claude -p "$prompt"'
assert_contains "$r" '--json-schema "$READING_SCHEMA"'
assert_contains "$r" 'review-prompt.py reader'
assert_contains "$r" 'review-reading-check.py'
allowed=$(py 'print(re.search(r"--allowedTools \"([^\"]*)\"", runs("read")).group(1))')
assert_equals "Workflow,Agent(file-reviewer)" "$allowed"
assert_not_contains "$r" 'gh pr comment'
assert_not_contains "$r" '--agent '

it "the merged reading's schema carries the cross-review facts"
schema=$(py 'print(step("read","name","Read ${{ matrix.entry.group }}")["env"]["READING_SCHEMA"])')
for k in '"files"' '"declares"' '"references"' '"unread"' '"changedNames"'; do assert_contains "$schema" "$k"; done

it "the file reader's tools are read-only git plus the file tools"
tools=$(fm tools "$REVIEWER")
for t in "Bash(git diff:*)" "Bash(git ls-files:*)" "Bash(git cat-file:*)" "Read" "Grep" "Glob"; do assert_contains "$tools" "$t"; done

it "the saved workflow reads per file with the file reader, validates by schema, and merges"
s=$(cat "$WF")
assert_equals "export const meta = {" "$(grep -m1 '^export const meta' "$WF")"
assert_contains "$s" "name: 'review-component'"
assert_contains "$s" "pipeline(files"
assert_contains "$s" "agentType: 'file-reviewer'"
assert_contains "$s" "schema: FILE_READING"
assert_contains "$s" "required: ['path', 'findings', 'declares', 'references', 'threads']"
for k in changedNames files threads unread; do assert_contains "$s" "$k"; done
assert_contains "$s" "RULE FILES TO READ FOR THIS PATH"
assert_contains "$s" "resolved and unresolved alike"

it "what each context holds is the first thing a model job says — before anything is installed or restored"
for j in read consolidate; do
  assert_contains "$(py "print(runs('$j'))")" "review-context.py"
  idx_ctx=$(py "print([i for i,s in enumerate(steps('$j')) if 'review-context.py' in s.get('run','')][0])")
  idx_cli=$(py "print([i for i,s in enumerate(steps('$j')) if s.get('uses','')=='./.github/actions/claude-cli'][0])")
  idx_restore=$(py "print([i for i,s in enumerate(steps('$j')) if 'actions/claude-cli/action.yml' in s.get('run','')][0])")
  [ "$idx_ctx" -lt "$idx_restore" ] && [ "$idx_ctx" -lt "$idx_cli" ] && pass || fail "$j: the measurement must precede the restore and the install"
done

it "every model step leaves a trace in the log: turns, tokens, and each file reader's duration"
assert_contains "$(py 'print(runs("read"))')" "review-trace.py read-execution.jsonl --out reader-output.json"
assert_contains "$(py 'print(runs("read"))')" "--output-format stream-json --verbose --json-schema"
assert_contains "$(py 'print(runs("consolidate"))')" 'review-trace.py "$out"'
assert_contains "$(py 'print(json.dumps([s.get("with",{}) for s in steps("read")]))')" 'read-execution-${{ matrix.entry.slug }}'

it "the coordinator runs its role in consolidate, and holds the summary gate and the resolve list"
c=$(py 'print(runs("consolidate"))')
assert_contains "$c" ".github/scripts/mark-thread-resolved.sh"
assert_contains "$c" 'claude -p "$prompt" --agent review-coordinator'
assert_contains "$c" '--output-format stream-json'
assert_contains "$c" 'review-prompt.py coordinator'
assert_contains "$c" 'startswith("### Review")'
assert_contains "$c" 'touch "${{ github.workspace }}/.resolve-threads"'
assert_contains "$(py 'print(step("consolidate","name","The review actually ran")["if"])')" "always()"

it "consolidate runs whatever the readings concluded, and not on a dry run"
cif=$(py 'print(jobs["consolidate"]["if"])')
assert_contains "$cif" "always()"
assert_contains "$cif" "needs.queue.result == 'success'"
assert_contains "$cif" "inputs.dry_run"

it "the readings reach the coordinator as artifacts, missing ones included"
assert_contains "$(py 'print(json.dumps([s.get("with",{}) for s in steps("consolidate")]))')" '"pattern": "reading-*"'
assert_contains "$(py 'print(json.dumps([s.get("with",{}) for s in steps("read")]))')" 'reading-${{ matrix.entry.slug }}'
assert_contains "$(py 'print(json.dumps(step("consolidate","id","list")))')" "produced=true"

it "does not run a pull request's own edit of this file — unless a person dispatched that branch on purpose"
assert_contains "$(py 'print(runs("queue"))')" "workflow_edited=true"
assert_contains "$(py 'print(jobs["read"]["if"])')" "needs.queue.outputs.run == 'true'"
assert_contains "$(py 'print(step("queue","id","guard")["env"]["DISPATCHED"])')" "github.event_name == 'workflow_dispatch'"
assert_contains "$(py 'print(step("queue","id","guard")["run"])')" '[ "$DISPATCHED" = "true" ] ||'

it "can be dispatched by hand against a pull request, with a dry run"
assert_contains "$(py 'print(" ".join(d.get("on", d.get(True))["workflow_dispatch"]["inputs"]))')" "number"
assert_contains "$(py 'print(" ".join(d.get("on", d.get(True))["workflow_dispatch"]["inputs"]))')" "dry_run"
assert_contains "$(py 'print(d["concurrency"]["group"])')" "inputs.number"

it "runs the CLI itself with the credential as env, never the action"
assert_not_contains "$(py 'print(" ".join(uses("read")+uses("consolidate")))')" "claude-code-action"
assert_contains "$(py 'print(envkeys("read"))')" "CLAUDE_CODE_OAUTH_TOKEN"
assert_contains "$(py 'print(envkeys("consolidate"))')" "CLAUDE_CODE_OAUTH_TOKEN"

it "gives the reviewer no posting tool, and no way to spawn"
assert_not_contains "$tools" "gh"
assert_not_contains "$tools" "mcp__"
assert_not_contains "$tools" "mark-thread-resolved"
assert_not_contains "$tools" "Write"
assert_not_contains "$tools" "Edit"
assert_not_contains "$tools" "Agent"

it "tells the reviewer what is not available, so it stops probing"
p=$(body "$REVIEWER")
assert_contains "$p" "NOT AVAILABLE"
assert_contains "$p" "redirection"

it "asks the file reader for JSON with the fields the workflow merges and the coordinator cross-reviews"
assert_contains "$p" '"declares"'
assert_contains "$p" '"references"'
assert_contains "$p" '"threads"'
assert_contains "$p" '"findings"'

it "the file reader judges against the rules it is told to read, and inherits none"
assert_contains "$p" "RULE FILES TO READ"
assert_contains "$p" "Nothing else about this project's doctrine is in your context"

it "asks the reviewer for the four finding fields the inline comment is built from, with the caps"
assert_contains "$p" '"where"'
assert_contains "$p" '"fix"'
assert_contains "$p" 'AT MOST 15 WORDS, ONE CLAUSE'
assert_contains "$p" 'AT MOST 12 WORDS'

it "hands a reader its file's whole review history, resolved threads included"
assert_contains "$p" "A RESOLVED thread is history, not work"
assert_contains "$(cat "$WF")" "resolved and unresolved alike"

it "the coordinator is the only writer, and posts a finding as four labeled lines"
c=$(body "$COORD")
assert_contains "$c" '**Claim:**'
assert_contains "$c" '**Where:**'
assert_contains "$c" '**Rule:**'
assert_contains "$c" '**Fix:**'
assert_contains "$(fm tools "$COORD")" "gh pr comment"
assert_not_contains "$(fm tools "$COORD")" "Agent"
assert_not_contains "$(fm tools "$COORD")" "mcp__"
assert_contains "$c" "pulls/<PR NUMBER>/comments"

it "consolidates in the coordinator: cross-review from readings, reach outside, unreviewed, and the first finding names the guarded files"
assert_contains "$c" "git grep -l -F"
assert_contains "$c" "unreviewed: <group>"
assert_contains "$c" "unread: <path>"
assert_contains "$c" "THE CROSS-REVIEW FROM THE READINGS"
assert_contains "$c" "Do not open"
assert_contains "$c" "NO RULE FILE"
for f in ".claude/rules/" ".claude/workflows/" ".github/actions/claude-cli/" "review-prompt.py" "review-rules.py" "review-context.py"; do assert_contains "$c" "$f"; done

it "nothing names the deleted script or the retired component reader"
assert_equals "" "$(grep -l 'review-pr.js\|component-reviewer' "$W" "$A" "$REVIEWER" "$COORD" "$WF" 2>/dev/null || true)"
[ ! -e "$ROOT/.claude/workflows/review-pr.js" ] && pass || fail "the script must be gone"
[ ! -e "$ROOT/.claude/agents/component-reviewer.md" ] && pass || fail "the component reader must be gone"

it "no context inherits a rule file: every rule is excluded by one glob, and CLAUDE.md stays"
excl=$(py 'print(" ".join(json.loads(d["env"]["CLAUDE_SETTINGS"])["claudeMdExcludes"]))')
assert_equals "**/.claude/rules/*.md" "$excl"
assert_contains "$(py 'print(runs("read")+runs("consolidate"))')" '--settings "$CLAUDE_SETTINGS"'
assert_not_contains "$excl" "CLAUDE.md"

it "the rules a reader reads are routed by a program that reaches every review criterion"
python3 "$ROOT/.github/scripts/review-rules.py" --check >/dev/null 2>&1
assert_status 0 "$?"
assert_contains "$(py 'print(runs("read"))')" "review-rules.py"

it "still holds the privilege split: only reconcile has contents: write, and no model job does"
assert_equals "reconcile" "$(py 'print(" ".join(j for j,v in jobs.items() if v.get("permissions",{}).get("contents")=="write"))')"
assert_equals "consolidate" "$(py 'print(" ".join(j for j,v in jobs.items() if v.get("permissions",{}).get("pull-requests")=="write"))')"

summary
