#!/usr/bin/env bash
# The shape of the review — the lines that make a per-file, build-gated
# review safe, each of which could be "simplified" away with every run still
# green.
#
# THE PLAN IS THE WORKFLOW AND THE ROLES ARE FILES. `claude-review.yml`
# builds the queue with a program that also decides which changed path is
# READ this run versus CARRIED from an earlier one; `read` builds the
# component first, then runs one `claude -p` PER FILE (blind) plus one per
# file with an open thread (the verdict pass), merges them with a program,
# and hands every reading to the `review-coordinator` in one job; a fourth
# resolves threads and runs no model. Every job that runs a model installs
# the CLI through one composite action and restores what it reads from the
# base branch first.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
W="$ROOT/.github/workflows/claude-review.yml"
A="$ROOT/.github/actions/claude-cli/action.yml"
REVIEWER="$ROOT/.claude/agents/file-reviewer.md"
VERDICT="$ROOT/.claude/agents/thread-verdict.md"
COORD="$ROOT/.claude/agents/review-coordinator.md"

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

it "can be dispatched with full, ignoring the coverage record, and quiet-reads is a workflow variable"
assert_contains "$(py 'print(" ".join(d.get("on", d.get(True))["workflow_dispatch"]["inputs"]))')" "full"
assert_contains "$q" "--quiet-reads"
assert_contains "$q" "full_flag"
assert_equals "1" "$(py 'print(d["env"]["REVIEW_QUIET_READS"])')"
assert_equals "4" "$(py 'print(d["env"]["REVIEW_READERS"])')"

it "every restore of tooling says so when the pull request's copy differed"
for j in queue read consolidate; do
  assert_contains "$(py "print(runs('$j'))")" '[ -z "$restored" ] || echo "::notice::'
done

it "the read matrix is the queue's read-path component list, one job each, and a failed reading fails alone"
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

it "the read job restores review-build.sh with its other tooling, and the roles are the two readers"
tooling=$(py 'print(runs("read"))')
assert_contains "$tooling" "review-build.sh"
assert_contains "$(py 'print(step("read","uses","./.github/actions/claude-cli")["with"]["restore"])')" ".claude/agents/file-reviewer.md"
assert_contains "$(py 'print(step("read","uses","./.github/actions/claude-cli")["with"]["restore"])')" ".claude/agents/thread-verdict.md"
assert_contains "$(py 'print(step("consolidate","uses","./.github/actions/claude-cli")["with"]["restore"])')" ".claude/agents/review-coordinator.md"
assert_contains "$(py 'print(step("read","uses","./.github/actions/claude-cli")["with"]["base-ref"])')" "needs.queue.outputs.base"

it "the action pins the version once, caches on it, and asserts what it installed"
assert_contains "$(av)" "."
assert_contains "$(cat "$A")" "actions/cache@"
assert_contains "$(cat "$A")" 'claude-cli-${{ inputs.version }}'
assert_contains "$(cat "$A")" 'claude --version'
assert_contains "$(cat "$A")" 'git checkout "$BASE" -- "$f"'
assert_equals "" "$(grep -rl --fixed-strings "$(av)" "$ROOT/.github" | grep -v actions/claude-cli || true)"

it "the build gate runs before any model, holds no credential, and skips reading on failure"
build=$(py 'print(step("read","id","build"))')
assert_contains "$build" "review-build.sh"
assert_not_contains "$build" "CLAUDE_CODE_OAUTH_TOKEN"
idx_build=$(py "print([i for i,s in enumerate(steps('read')) if s.get('id')=='build'][0])")
idx_read=$(py "print([i for i,s in enumerate(steps('read')) if s.get('name','').startswith('Read \${{')][0])")
[ "$idx_build" -lt "$idx_read" ] && pass || fail "the build must precede the reading"
assert_contains "$(py "print(steps('read')[$idx_read].get('if',''))")" "steps.build.outputs.built == 'true'"
unbuilt=$(py 'print(step("read","name","Record ${{ matrix.entry.group }} as unbuilt"))')
assert_contains "$unbuilt" "steps.build.outputs.built != 'true'"
assert_contains "$unbuilt" "unbuilt-reading.json"
assert_contains "$unbuilt" "--kind component"

it "review-build.sh derives the recipe per group, per the D1 table, and never runs go test"
b=$(cat "$ROOT/.github/scripts/review-build.sh")
assert_contains "$b" "go build ./... && go vet ./..."
assert_not_contains "$b" "go test"
assert_contains "$b" "node --test"
assert_contains "$b" "helm lint"
assert_contains "$b" "helm template"
assert_contains "$b" "npm ci"
assert_contains "$b" "unbuilt"

it "the group's toolchain (go/node/helm) is set up conditionally before the build runs"
recipe=$(py 'print(step("read","id","recipe"))')
assert_contains "$recipe" "go.mod"
idx_recipe=$(py "print([i for i,s in enumerate(steps('read')) if s.get('id')=='recipe'][0])")
[ "$idx_recipe" -lt "$idx_build" ] && pass || fail "the toolchain must be set up before the build runs"

it "the per-file reader loop is a shell xargs over REVIEW_READERS, blind, cached-prefix, one process per file"
r=$(py 'print(step("read","name","Read ${{ matrix.entry.group }}")["run"])')
assert_contains "$r" 'xargs -r -P "$REVIEW_READERS"'
assert_contains "$r" "reader-system"
assert_contains "$r" "reader-tools"
assert_contains "$r" '--append-system-prompt "$(cat readings/raw/system.md)"'
assert_contains "$r" '--json-schema "$FILE_READING_SCHEMA"'
assert_contains "$r" "review-trace.py"
assert_contains "$r" 'review-reading-check.py "readings/raw/read-$slug.json" --group "$GROUP" --kind file'
assert_not_contains "$r" "Agent("
assert_not_contains "$r" "review-component"

it "the verdict loop runs only over paths with an open thread, then the two are merged with --paths"
assert_contains "$r" "verdict-paths"
assert_contains "$r" "verdict-system"
assert_contains "$r" "verdict-tools"
assert_contains "$r" '--json-schema "$VERDICT_SCHEMA"'
assert_contains "$r" 'review-reading-check.py "readings/raw/verdict-$slug.json" --group "$GROUP" --kind verdict'
assert_contains "$r" 'review-reading-check.py readings/merge --group "$GROUP" --paths "$PATHS" --out readings/reading.json'

it "the file reading and verdict schemas carry the cross-review and detached-finding facts"
schema=$(py 'print(step("read","name","Read ${{ matrix.entry.group }}")["env"]["FILE_READING_SCHEMA"])')
for k in '"declares"' '"references"'; do assert_contains "$schema" "$k"; done
assert_not_contains "$schema" '"threads"'
vschema=$(py 'print(step("read","name","Read ${{ matrix.entry.group }}")["env"]["VERDICT_SCHEMA"])')
for k in '"verdict"' '"finding"' "fixed" "standing" "gone" "detached"; do assert_contains "$vschema" "$k"; done

it "the file reader's tools are read-only git plus the file tools"
tools=$(fm tools "$REVIEWER")
for t in "Bash(git diff:*)" "Bash(git ls-files:*)" "Bash(git cat-file:*)" "Read" "Grep" "Glob"; do assert_contains "$tools" "$t"; done

it "the verdict role reads a diff and the file, and holds no rule-reading tool it would need one for"
vtools=$(fm tools "$VERDICT")
assert_contains "$vtools" "Bash(git diff:*)"
assert_not_contains "$vtools" "Write"
assert_not_contains "$vtools" "Edit"

it "what each context holds is the first thing the read job says — before anything is installed, built or restored"
idx_ctx=$(py "print([i for i,s in enumerate(steps('read')) if 'review-context.py' in s.get('run','')][0])")
idx_cli=$(py "print([i for i,s in enumerate(steps('read')) if s.get('uses','')=='./.github/actions/claude-cli'][0])")
idx_restore=$(py "print([i for i,s in enumerate(steps('read')) if 'actions/claude-cli/action.yml' in s.get('run','')][0])")
[ "$idx_ctx" -lt "$idx_restore" ] && [ "$idx_ctx" -lt "$idx_cli" ] && [ "$idx_ctx" -lt "$idx_build" ] && pass || fail "the measurement must precede the restore, the install and the build"
idx_ctx_c=$(py "print([i for i,s in enumerate(steps('consolidate')) if 'review-context.py' in s.get('run','')][0])")
idx_cli_c=$(py "print([i for i,s in enumerate(steps('consolidate')) if s.get('uses','')=='./.github/actions/claude-cli'][0])")
[ "$idx_ctx_c" -lt "$idx_cli_c" ] && pass || fail "consolidate: the measurement must precede the install"

it "the coverage input is written before the model runs, and handed to review-post.py after posting"
idx_cov=$(py "print([i for i,s in enumerate(steps('consolidate')) if 'coverage.json' in s.get('run','') and s.get('id')!='review'][0])")
idx_model=$(py "print([i for i,s in enumerate(steps('consolidate')) if s.get('id')=='review'][0])")
[ "$idx_cov" -lt "$idx_model" ] && pass || fail "the coverage input must be written before the model runs"
c=$(py 'print(runs("consolidate"))')
assert_contains "$c" "review-post.py post.json --coverage coverage.json"

it "the coordinator runs its role in consolidate, and holds the summary gate and the resolve list"
assert_contains "$c" ".github/scripts/mark-thread-resolved.sh"
assert_contains "$c" 'claude -p "$prompt" --agent review-coordinator'
assert_contains "$c" '--output-format stream-json'
assert_contains "$c" 'review-prompt.py coordinator --input review-input.json --readings readings'
assert_contains "$c" 'post-result.json'
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

it "gives the file reviewer no posting tool, and no way to spawn"
assert_not_contains "$tools" "gh"
assert_not_contains "$tools" "mcp__"
assert_not_contains "$tools" "mark-thread-resolved"
assert_not_contains "$tools" "Write"
assert_not_contains "$tools" "Edit"
assert_not_contains "$tools" "Agent"
assert_not_contains "$vtools" "Agent"

it "tells the file reviewer what is not available, so it stops probing"
p=$(body "$REVIEWER")
assert_contains "$p" "NOT AVAILABLE"
assert_contains "$p" "redirection"

it "asks the file reader for JSON with the fields the merge program reads, and no thread field"
assert_contains "$p" '"declares"'
assert_contains "$p" '"references"'
assert_contains "$p" '"findings"'
assert_not_contains "$p" '"threads":'

it "the model and its effort are chosen by the workflow, on every claude -p, and reach the subagents by inherit"
assert_equals "claude-sonnet-5" "$(py 'print(d["env"]["CLAUDE_MODEL"])')"
assert_equals "low" "$(py 'print(d["env"]["CLAUDE_EFFORT"])')"
for j in read consolidate; do
  assert_contains "$(py "print(runs('$j'))")" '--model "$CLAUDE_MODEL" --effort "$CLAUDE_EFFORT"'
done
assert_equals "inherit" "$(fm model "$REVIEWER")"
assert_equals "inherit" "$(fm model "$VERDICT")"
assert_equals "inherit" "$(fm model "$COORD")"

it "the file reader is told the cross-review lists are not optional, and is blind — no thread in its return"
assert_contains "$p" "THEY ARE NOT OPTIONAL"
assert_not_contains "$p" '"threads"'

it "the file reader judges against the rules already in its system prompt, and reads no thread"
assert_contains "$p" "SYSTEM PROMPT"
assert_contains "$p" "YOU JUDGE NO THREAD"

it "asks the reviewer for the four finding fields the inline comment is built from, with the caps"
assert_contains "$p" '"where"'
assert_contains "$p" '"fix"'
assert_contains "$p" 'AT MOST 15 WORDS, ONE CLAUSE'
assert_contains "$p" 'AT MOST 12 WORDS'

it "the verdict role judges fixed/standing/gone/detached, and requires finding only on detached"
vp=$(body "$VERDICT")
assert_contains "$vp" "fixed"
assert_contains "$vp" "standing"
assert_contains "$vp" "gone"
assert_contains "$vp" "detached"
assert_contains "$vp" "REQUIRED on \`detached\`"

it "the coordinator is the only writer, and posts a finding as four labeled lines"
c2=$(body "$COORD")
assert_contains "$c2" '**Claim:**'
assert_contains "$c2" '**Where:**'
assert_contains "$c2" '**Rule:**'
assert_contains "$c2" '**Fix:**'
assert_not_contains "$(fm tools "$COORD")" "review-post.py"
assert_not_contains "$(fm tools "$COORD")" "gh "
assert_not_contains "$(fm tools "$COORD")" "git diff"
assert_not_contains "$(fm tools "$COORD")" "Agent"
assert_not_contains "$(fm tools "$COORD")" "mcp__"
assert_contains "$c2" "RETURN THE POSTING DOCUMENT"
assert_contains "$c2" "HEAD SHA"
assert_contains "$c2" "YOU DO NOT VERIFY A FINDING, AND YOU DO NOT READ THE DIFF"

it "the coordinator dedups a blind finding against the threads it already holds — carried, dismissed, fixed"
assert_contains "$c2" "DEDUP AGAINST THE THREADS"
assert_contains "$c2" "carried over"
assert_contains "$c2" "dismissed"
assert_contains "$c2" "the fix did not hold"

it "a carried path's threads are left standing, and the coordinator states so rather than judging them"
assert_contains "$c2" "CARRIED PATHS"
assert_contains "$c2" "STANDING by construction"

it "the summary states the third coverage line and the unbuilt row"
assert_contains "$c2" "read: N of M changed files"
assert_contains "$c2" "K carried"
assert_contains "$c2" "unbuilt"

it "the coordinator posts nothing itself: the job runs review-post.py on its structured answer, and the gate reads that result"
callowed=$(py 'print(re.search(r"--allowedTools \"([^\"]*)\"", runs("consolidate")).group(1))')
assert_equals "Read,Grep,Glob,Bash(git grep:*)" "$callowed"
assert_contains "$c" '--json-schema "$POST_SCHEMA"'
assert_contains "$c" 'summaryPosted'
assert_contains "$(py 'print(step("consolidate","id","review")["env"]["POST_SCHEMA"])')" summary

it "consolidates in the coordinator: cross-review from readings, reach outside, unreviewed, and the first finding names the guarded files"
assert_contains "$c2" "git grep -l -F"
assert_contains "$c2" "unreviewed: <group>"
assert_contains "$c2" "unread: <path>"
assert_contains "$c2" "THE CROSS-REVIEW FROM THE READINGS"
assert_contains "$c2" "Do not open"
assert_contains "$c2" "NO RULE FILE"
for f in ".claude/rules/" "review-prompt.py" "review-rules.py" "review-context.py" "review-build.sh"; do assert_contains "$c2" "$f"; done

it "nothing names the deleted queue-reader workflow or the retired component reader"
assert_equals "" "$(grep -l 'review-component\.js\|component-reviewer' "$W" "$A" "$REVIEWER" "$COORD" "$VERDICT" 2>/dev/null || true)"
[ ! -e "$ROOT/.claude/workflows/review-component.js" ] && pass || fail "the script must be gone"
[ ! -e "$ROOT/.claude/agents/component-reviewer.md" ] && pass || fail "the component reader must be gone"
[ ! -d "$ROOT/.claude/workflows" ] && pass || fail "the workflows directory carried only this script"

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
