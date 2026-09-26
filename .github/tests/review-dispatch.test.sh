#!/usr/bin/env bash
# The shape of the dispatch workflow — the properties that make it safe to let a
# comment start a run that writes to a branch.
#
# Each of these is a line somebody could "simplify" away while every job still
# passed: the trigger set, which job holds `contents: write`, which job runs the
# model, and that they are not the same job.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
W="$ROOT/.github/workflows/review-dispatch.yml"

py() { python3 -c "
import sys, yaml
d = yaml.safe_load(open(sys.argv[1]))
$1
" "$W"; }

it "never triggers on pull_request_target — in this workflow or any other"
assert_equals "" "$(python3 -c '
import sys, yaml
for f in sys.argv[1:]:
    on = yaml.safe_load(open(f))[True]
    if "pull_request_target" in (on if isinstance(on, dict) else [on] if isinstance(on, str) else on): print(f)
' "$ROOT"/.github/workflows/*.yml)"

it "triggers on the two comment events, the label, the review's completion and a hand run, and nothing else"
assert_equals "issue_comment pull_request pull_request_review_comment workflow_dispatch workflow_run" "$(py 'print(" ".join(sorted(d[True])))')"

it "the pull_request trigger is the label event alone — never opened or synchronize, which would run on every push"
assert_equals "['labeled']" "$(py 'print(d[True]["pull_request"]["types"])')"

it "the workflow_run trigger names the review AND ci, and completes only"
assert_equals "['claude-review', 'ci'] ['completed']" "$(py 'print(d[True]["workflow_run"]["workflows"], d[True]["workflow_run"]["types"])')"

# BOTH LABEL CHECKS MUST SIT INSIDE THE SAME `pull_request &&` CLAUSE, not
# merely appear somewhere in the gate's `if` -- a bare top-level check would
# still contain the label name and still pass a plain assert_contains, while
# firing the gate on every event that carries that label, not only a
# `pull_request: labeled` one.
pr_clause() { py '
import re
m = re.search(r"\(github\.event_name == '"'"'pull_request'"'"' && \([^)]*\)\)", d["jobs"]["gate"]["if"])
print(m.group(0) if m else "NOT FOUND")
'; }

it "the label the gate prefilters on is the one the vocabulary file states, tied to the pull_request event"
label=$(python3 -c 'import json;print(json.load(open("'"$ROOT"'/.github/review-triage.json"))["approve_label"])')
assert_contains "$(pr_clause)" "github.event.label.name == '$label'"
assert_equals "conveyor:fix" "$label"

it "the keep-going label ALSO prefilters the pull_request trigger, tied to the SAME clause, so placing it re-triggers a round"
keep_going=$(python3 -c 'import json;print(json.load(open("'"$ROOT"'/.github/review-triage.json"))["keep_going_label"])')
assert_contains "$(pr_clause)" "github.event.label.name == '$keep_going'"
assert_equals "conveyor:keep-going" "$keep_going"

it "a review that did not complete successfully starts no round"
assert_contains "$(py 'print(d["jobs"]["gate"]["if"])')" "github.event.workflow_run.path == '.github/workflows/claude-review.yml' && github.event.workflow_run.conclusion == 'success'"

# THE TWO RUNS START A ROUND FOR OPPOSITE REASONS, so the prefilter names each
# workflow with the conclusion that matters for it. A green CI run starting a
# round would be a round over nothing, every push.
it "a ci run starts a round only when it FAILED"
assert_contains "$(py 'print(d["jobs"]["gate"]["if"])')" "github.event.workflow_run.path == '.github/workflows/ci.yml' && github.event.workflow_run.conclusion == 'failure'"
assert_not_contains "$(py 'print(d["jobs"]["gate"]["if"])')" "workflow_run.path == '.github/workflows/ci.yml' && github.event.workflow_run.conclusion == 'success'"

# MEASURED LIVE ON #233: `claude-review.yml` sets `run-name: "Review of #<n>"`,
# and a workflow_run event's own `.name` field reflects THAT per-run display
# title, never the workflow's static `name:` -- so a literal 'claude-review'
# here matched nothing, ever, and two rounds landed on #233 with a clean
# review completing after the second and no further round ever starting.
it "the gate matches workflow_run events on .path, never .name -- a run-name overrides .name silently"
assert_not_contains "$(py 'print(d["jobs"]["gate"]["if"])')" "workflow_run.name == 'claude-review'"
assert_not_contains "$(py 'print(d["jobs"]["gate"]["if"])')" "workflow_run.name == 'ci'"

it "the gate reads the vocabulary from the default branch on every event but a hand run"
assert_contains "$(py 'print(d["jobs"]["gate"]["steps"][0]["with"]["ref"])')" "github.event_name == 'workflow_dispatch' && github.ref || github.event.repository.default_branch"

# THE GATE IS ONE PROGRAM. Every decision it makes is `conveyor.gate`'s, tested over every
# combination of its facts in conveyor.test.py, and its I/O is tested in dispatch-gate.test.sh.
# What only THIS file can pin is that the workflow still calls it, with every input it reads.
it "the gate step runs dispatch-gate.py and nothing else: no decision is left in the workflow's shell"
gate=$(py 'print(d["jobs"]["gate"]["steps"][1]["run"])')
assert_equals "python3 .github/scripts/dispatch-gate.py" "$gate"

it "the gate is handed every input it reads, from the event and never from a string in the shell"
genv=$(py 'print(d["jobs"]["gate"]["steps"][1]["env"])')
for v in EVENT BODY ASSOCIATION SENDER PR REVIEW_TITLE WORKFLOW_RUN_PATH REVIEW_PR RUN_HEAD INPUT_MODE EVENT_LABEL GH_TOKEN; do
  assert_contains "$genv" "'$v'"
done
assert_contains "$genv" "workflow_run.path"
assert_not_contains "$genv" "workflow_run.name"

it "the gate's step id stays `who`, since the other jobs read their inputs from its outputs"
assert_equals "who" "$(py 'print(d["jobs"]["gate"]["steps"][1]["id"])')"

it "the gate declares every output the later jobs read, and the program writes each of them"
outs=$(py 'print(sorted(d["jobs"]["gate"]["outputs"]))')
for o in approver branch dispatcher head max_rounds mode pr since; do
  assert_contains "$outs" "'$o'"
  assert_contains "$(cat "$ROOT/.github/scripts/dispatch-gate.py")" "\"$o\""
done

it "the gate program never trusts a grant it did not re-check: it asks the machine's gate, on every path"
prog=$(cat "$ROOT/.github/scripts/dispatch-gate.py")
assert_contains "$prog" "conveyor.gate("
assert_not_contains "$prog" "run_label"
assert_not_contains "$prog" "conveyor:run"

it "the collect job holds the analysis token and the model job holds none — no secret reaches the fixing step but its own credential"
assert_contains "$(py 'print(d["jobs"]["collect"]["steps"][1]["env"])')" "SONAR_TOKEN"
fixjob=$(py 'print(d["jobs"]["fix"])')
assert_not_contains "$fixjob" "SONAR_TOKEN"
assert_not_contains "$fixjob" "secrets.GITHUB_TOKEN"
assert_contains "$fixjob" "secrets.CLAUDE_CODE_OAUTH_TOKEN"

# `actions: READ` IS NOT `actions: write`, AND THE DIFFERENCE IS THE WHOLE RULE.
# A dispatched run's check runs never reach the merge box (#131, gotchas.md), so
# nothing here may START a workflow. Reading a failed run's log is what makes a
# red check a work item, and it is granted to `collect` — the job with no model
# in it — and to no other.
it "no job may dispatch a workflow: only collect may read runs, and none may write them"
assert_equals "collect" "$(py 'print(" ".join(j for j,v in d["jobs"].items() if v["permissions"].get("actions")))')"
assert_equals "read" "$(py 'print(d["jobs"]["collect"]["permissions"]["actions"])')"
assert_not_contains "$(py 'print(d["jobs"]["land"])')" "workflow run"
assert_equals "" "$(py 'print(d["jobs"]["fix"]["permissions"].get("actions",""))')"

it "the push credential is read by the landing job alone, on a labelled pull request only, and the model's job cannot name it"
assert_equals "land" "$(py 'print(" ".join(j for j,v in d["jobs"].items() if "AUTOFIX_DEPLOY_KEY" in str(v)))')"
cred=$(py 'print([s for s in d["jobs"]["land"]["steps"] if s.get("id")=="cred"][0])')
assert_contains "$cred" "needs.gate.outputs.mode == 'all'"
assert_contains "$cred" "secrets.AUTOFIX_DEPLOY_KEY"

it "the landing program is told whether the push starts workflows, and only when the credential was configured"
assert_contains "$(py 'print(d["jobs"]["land"]["steps"][-1]["env"]["STARTS"])')" "steps.cred.outputs.starts == 'true' && '--push-starts-workflows'"

# THE THIRD SOURCE. A red required check holds the merge exactly as a thread
# does, so the work list is threads, then analysis issues, then checks — in that
# order, which is the order a person reads them in.
it "the work list merges three sources, in order, and only under the label"
collect=$(py 'print(d["jobs"]["collect"]["steps"][1]["run"])')
assert_contains "$collect" "accepted-findings.py"
assert_contains "$collect" "sonar-issues.py"
assert_contains "$collect" "failed-checks.py"
assert_contains "$collect" "threads + sonar_issues + check_items"
assert_contains "$collect" 'if [ "$MODE" = "all" ]'

it "the landing program is handed the checks, so its summary can account for them"
assert_contains "$(py 'print(d["jobs"]["land"]["steps"][-1]["run"])')" "--checks"

it "the fixer is told to reproduce a check before fixing it, and may run the job's command"
fixstep=$(py 'print([s for s in d["jobs"]["fix"]["steps"] if s.get("id")=="model"][0])')
assert_contains "$fixstep" "source: check"
assert_contains "$fixstep" "REPRODUCE IT FIRST"
assert_contains "$fixstep" "Bash(go:*)"

# THE BOUND IS READ FROM THE VOCABULARY FILE, NOT DECLARED IN THIS WORKFLOW.
# A workflow's env: block cannot read a file, so MAX_ROUNDS no longer exists
# as a static workflow constant — the gate reads max_rounds from
# .github/review-triage.json and passes it down as an output.
it "the bound is read from the vocabulary file's max_rounds, not a workflow constant"
assert_not_contains "$(py 'print(list(d.keys()))')" "'env'"
assert_contains "$(cat "$ROOT/.github/scripts/dispatch-gate.py")" 'vocab["max_rounds"]'
assert_contains "$(py 'print(d["jobs"]["gate"]["outputs"])')" "max_rounds"
assert_contains "$(py 'print(d["jobs"]["land"]["steps"][-1]["run"])')" '--max-rounds "${{ needs.gate.outputs.max_rounds }}"'
max_rounds=$(python3 -c 'import json;print(json.load(open("'"$ROOT"'/.github/review-triage.json"))["max_rounds"])')
assert_equals "5" "$max_rounds"

it "the fixing prompt states the fix-or-dispute contract and forbids touching the analysis service"
prompt=$(py 'print([s for s in d["jobs"]["fix"]["steps"] if "claude-code-action" in s.get("uses","")][0]["with"]["prompt"])')
assert_contains "$prompt" "FIX OR DISPUTE, NEVER SKIP"
assert_contains "$prompt" '"action": "disputed"'
assert_contains "$prompt" "you cannot reach the service"

it "the review run is named for its pull request, which is how a completion event names it"
assert_contains "$(python3 -c 'import yaml,sys;print(yaml.safe_load(open(sys.argv[1]))["run-name"])' "$ROOT/.github/workflows/claude-review.yml")" "Review of #"

it "grants nothing at the workflow level"
assert_equals "{}" "$(py 'print(d["permissions"])')"

it "runs the model in a job that holds contents: read"
assert_equals "read" "$(py 'print(d["jobs"]["fix"]["permissions"]["contents"])')"
assert_contains "$(py 'print([s.get("uses","") for s in d["jobs"]["fix"]["steps"]])')" "anthropics/claude-code-action"

it "holds contents: write only in the landing job"
assert_equals "land" "$(py 'print(" ".join(j for j,v in d["jobs"].items() if v["permissions"].get("contents")=="write"))')"

it "runs no model in the landing job"
assert_not_contains "$(py 'print(d["jobs"]["land"])')" "claude"

# THE GRANTED TOOLS, NOT THE PROSE AROUND THEM. This read the whole
# `claude_args` string, so a COMMENT containing "through" or a path holding
# `.github` failed it — a security assertion that fires on a word is one
# somebody deletes. It now parses the allowlist and asks what is actually
# granted, which is the property: no `gh`, no push, no commit, and no shell.
it "gives the model no gh, no git push, no git commit and no bare shell"
tools=$(py '
import re
args = [s for s in d["jobs"]["fix"]["steps"] if "claude-code-action" in s.get("uses","")][0]["with"]["claude_args"]
m = re.search(r"--allowedTools \"([^\"]*)\"", args)
print(",".join(t.strip() for t in m.group(1).split(",")))')
assert_not_contains "$tools" "Bash(gh"
assert_not_contains "$tools" "Bash(git push"
assert_not_contains "$tools" "Bash(git commit"
# A bare shell would make every entry beside it decoration.
assert_not_contains "$tools" "Bash(bash"
assert_not_contains "$tools" "Bash(sh:"
assert_not_contains "$tools" "Bash(python3:"

it "uploads the patch with hidden files included, and fails on nothing found"
up=$(py 'print([s["with"] for s in d["jobs"]["fix"]["steps"] if "upload-artifact" in s.get("uses","")][0])')
assert_contains "$up" "'include-hidden-files': True"
assert_contains "$up" "'if-no-files-found': 'error'"

it "gates the fixing job on something having been accepted"
assert_contains "$(py 'print(d["jobs"]["fix"]["if"])')" "needs.collect.outputs.accepted != '0'"

it "prefilters on the same dispatch form the vocabulary file states"
form=$(python3 -c 'import json;print(json.load(open("'"$ROOT"'/.github/review-triage.json"))["dispatch"][0])')
assert_contains "$(py 'print(d["jobs"]["gate"]["if"])')" "startsWith(github.event.comment.body, '$form')"

it "does not cancel a dispatch in progress"
assert_not_contains "$(py 'print(d["concurrency"])')" "cancel-in-progress"

# SILENCE AND REFUSAL ARE DIFFERENT FACTS. The step substituting an empty
# report for a missing one still writes report.json (so the lander always has
# a file), but rides a sentinel beside it, and the landing step turns that
# into --report-missing.
it "the missing-report substitution still happens, and a sentinel rides beside it into the landing step"
patch_step=$(py 'print([s for s in d["jobs"]["fix"]["steps"] if s.get("name")=="The patch, and the report"][0]["run"])')
assert_contains "$patch_step" "echo '{\"items\":[]}' > \"\$RUNNER_TEMP/dispatch/report.json\""
assert_contains "$patch_step" "report.no-report"
assert_contains "$(py 'print(d["jobs"]["land"]["steps"][-1]["run"])')" "--report-missing"

# ---------------------------------------------------------------------------
# THE FIXER'S SCRATCH MUST NOT LAND (#243). The cut used to be `git add -N .`,
# every untracked file included, and the model wrote helper scripts into the
# checkout because the prompt handed it `$WORK_LIST` -- an env name it cannot
# expand with no shell. Both halves are pinned: the prompt names real paths,
# and dispatch-patch.py decides what crosses.

it "the prompt names the work list and the report by their paths, never by an env var the model cannot expand"
assert_not_contains "$prompt" '$WORK_LIST'
assert_not_contains "$prompt" '$REPORT'
assert_contains "$prompt" "runner.temp }}/dispatch/work-list.json"
assert_contains "$prompt" "runner.temp }}/dispatch/report.json"

it "the prompt gives the model a scratch directory outside the checkout, and states the created-file rule"
assert_contains "$prompt" "runner.temp }}/dispatch/scratch/"
assert_contains "$prompt" '"created": ['
assert_contains "$prompt" "file it does not name is not landed"
assert_contains "$(py 'print([s for s in d["jobs"]["fix"]["steps"] if "claude-code-action" in s.get("uses","")][0]["with"]["claude_args"])')" '--add-dir "${{ runner.temp }}/dispatch"'

it "the patch is cut by dispatch-patch.py, restored from the branch the workflow came from, never by add -N ."
assert_not_contains "$fixjob" "git add -N ."
assert_contains "$patch_step" 'python3 "$RUNNER_TEMP/dispatch-patch.py"'
assert_contains "$patch_step" '--dropped "$RUNNER_TEMP/dispatch/dropped.json"'
restore=$(py 'print([s for s in d["jobs"]["fix"]["steps"] if s.get("name")=="The patch program, from the branch the workflow came from"][0])')
assert_contains "$restore" 'git show "origin/$SOURCE:.github/scripts/dispatch-patch.py"'
assert_contains "$restore" "github.event_name == 'workflow_dispatch' && github.ref_name || github.event.repository.default_branch"

it "the dropped list rides in the artifact and reaches the lander"
assert_contains "$(py 'print([s for s in d["jobs"]["fix"]["steps"] if "upload-artifact" in s.get("uses","")][0]["with"]["path"])')" "dispatch/dropped.json"
assert_contains "$(py 'print(d["jobs"]["land"]["steps"][-1]["run"])')" '--dropped "$RUNNER_TEMP/dispatch/dropped.json"'

# ---------------------------------------------------------------------------
# THE OTHER WORKFLOW A LABEL STARTS, AND THE ONE THAT CARRIES A STANDING
# INSTRUCTION FORWARD AT THE TWO LATER TRANSITIONS. It holds a token that
# starts a machine writing to this repository, so its shape is pinned in the
# same file as the fixing loop's: three jobs, each granted only what it uses.
R="$ROOT/.github/workflows/remote-implement.yml"
rpy() { python3 -c "
import sys, yaml
d = yaml.safe_load(open(sys.argv[1]))
$1
" "$R"; }

it "remote-implement triggers on a labelled issue, on ci's workflow_run completing, and on a manual dispatch, and nothing else"
assert_equals "issues workflow_dispatch workflow_run" "$(rpy 'print(" ".join(sorted(d[True])))')"
assert_equals "['labeled']" "$(rpy 'print(d[True]["issues"]["types"])')"
assert_equals "['ci']" "$(rpy 'print(d[True]["workflow_run"]["workflows"])')"
assert_equals "['completed']" "$(rpy 'print(d[True]["workflow_run"]["types"])')"

it "the manual dispatch takes a required pr input, and nothing else — recovery only, never a routine trigger"
assert_equals "['pr']" "$(rpy 'print(list(d[True]["workflow_dispatch"]["inputs"]))')"
assert_equals "True" "$(rpy 'print(d[True]["workflow_dispatch"]["inputs"]["pr"]["required"])')"

it "never triggers on pull_request or pull_request_target directly — the token those events carry has no write access here"
assert_not_contains "$(rpy 'print(d[True])')" "pull_request_target"
assert_not_contains "$(rpy 'print(list(d[True]))')" "pull_request"

it "remote-implement grants nothing at the top level, and each job only what it uses"
assert_equals "{}" "$(rpy 'print(d["permissions"])')"
assert_equals "archive fire open" "$(rpy 'print(" ".join(sorted(d["jobs"])))')"
# `contents: read` for the checkout, `issues: write` to comment and to remove
# the label from somebody who may not push. Nothing else — in particular no
# `contents: write`: this job starts a session, it never writes to the tree.
assert_equals "{'contents': 'read', 'issues': 'write'}" "$(rpy 'print(d["jobs"]["fire"]["permissions"])')"

it "remote-implement prefilters fire on EITHER the implement or the run label"
implement_label=$(python3 -c 'import json;print(json.load(open("'"$ROOT"'/.github/review-triage.json"))["implement_label"])')
run_label=$(python3 -c 'import json;print(json.load(open("'"$ROOT"'/.github/review-triage.json"))["run_label"])')
fire_if=$(rpy 'print(d["jobs"]["fire"]["if"])')
assert_contains "$fire_if" "github.event.label.name == '$implement_label'"
assert_contains "$fire_if" "github.event.label.name == '$run_label'"
archive_label=$(python3 -c 'import json;print(json.load(open("'"$ROOT"'/.github/review-triage.json"))["archive_label"])')
it "remote-implement fires on the ARCHIVE label too: the last station of the opsx lane has an actor"
assert_contains "$fire_if" "github.event.label.name == '$archive_label'"
assert_equals "conveyor:archive" "$archive_label"
assert_equals "conveyor:implement" "$implement_label"
assert_equals "conveyor:run" "$run_label"

it "the open job hands ci's conclusion to carry.py: only a green ci may mark a head mergeable"
assert_contains "$(rpy 'print([s for s in d["jobs"]["open"]["steps"] if s.get("id") == "carry"][0]["env"])')" "CI_CONCLUSION': '\${{ github.event.workflow_run.conclusion }}"
assert_contains "$(rpy 'print([s for s in d["jobs"]["open"]["steps"] if s.get("id") == "carry"][0]["run"])')" '--ci-conclusion "$CI_CONCLUSION"'

it "the fire endpoint and its token are not in the tree"
job=$(rpy 'print(d["jobs"]["fire"])')
assert_contains "$job" "vars.ROUTINE_FIRE_URL"
assert_contains "$job" "secrets.ROUTINE_FIRE_TOKEN"

# THE OPEN TRANSITION: a session's own pull request appears, unlabelled.
# If its originating issue still carries the standing instruction, carry
# it forward as the fix-station label ON THIS PULL REQUEST.
#
# Anchored on `ci` completing FOR a pull_request event, never on
# `pull_request` directly. A GITHUB_TOKEN issued to a pull_request-triggered
# run carries no write access in this repository at all, measured live (see
# the workflow's own header comment).
it "the open job fires on ci completing for a pull_request event, never pull_request directly"
open_if=$(rpy 'print(d["jobs"]["open"]["if"])')
assert_contains "$open_if" "github.event_name == 'workflow_run'"
assert_contains "$open_if" "github.event.workflow_run.event == 'pull_request'"
assert_not_contains "$open_if" "github.event.pull_request"

# 'success' ALONE WAS THE BUG (#51): `ci-green` needs `review-clean`, so `ci`
# concludes `failure` whenever the review posted findings -- exactly the pull
# request that most needs `conveyor:fix` carried onto it. The carry that
# starts the fixing loop must not wait on the green state the fixing loop
# exists to produce.
it "the open job's conclusion check accepts BOTH success and failure, never success alone"
assert_contains "$open_if" "github.event.workflow_run.conclusion == 'success'"
assert_contains "$open_if" "github.event.workflow_run.conclusion == 'failure'"

it "the open job resolves the pull request from the workflow_run's head sha, not from a pull_request event payload"
open_run=$(rpy 'print([s for s in d["jobs"]["open"]["steps"] if s.get("id") == "carry"][0]["run"])')
assert_contains "$(rpy 'print([s for s in d["jobs"]["open"]["steps"] if s.get("id") == "carry"][0]["env"])')" "HEAD_SHA': '\${{ github.event.workflow_run.head_sha }}"
assert_contains "$open_run" "commits/\$HEAD_SHA/pulls"
assert_contains "$open_run" "select(.state == \"open\")"

it "the open job's own no-pull-request message names the workflow_run's head sha, never \$GITHUB_SHA (that names the trusted checkout's commit, not this run's)"
assert_contains "$open_run" '${HEAD_SHA:0:7}'
assert_not_contains "$open_run" '${GITHUB_SHA:0:7}'

# A FAILED LOOKUP AND A GENUINELY EMPTY ONE ARE DIFFERENT FACTS.
#
# A bare `cmd | jq ... || true` reads a transient gh api failure as the
# ordinary "no pull request" case, silently. gh's own exit status must be
# checked before its output is, so a real failure is reported as one.
it "the open job's pull-request lookup checks gh's own exit status, never swallowing a real failure into the ordinary empty case"
assert_contains "$open_run" 'if ! api_out=$(gh api'
assert_contains "$open_run" "could not look up pull requests"
assert_not_contains "$open_run" '.number.*| head -1 || true'

# THE change/* GUARD AND Refs #<n> EXTRACTION LIVE IN ONE SHARED SCRIPT,
# carry-from-pr.sh, called by BOTH open and archive with the resolved pull
# request number and the station -- not duplicated in the workflow YAML.
it "the open job hands the resolved pull request off to carry.py, station fix"
assert_contains "$open_run" ".github/scripts/carry.py --repo \"\$GITHUB_REPOSITORY\" --pr \"\$pr\" --station fix"
assert_not_contains "$open_run" "headRepositoryOwner"
assert_not_contains "$open_run" "carry-grant.py"
assert_not_contains "$open_run" "carry-from-pr.sh"

it "the open job is granted issues: write for the marker comment, pull-requests: write since carry.py labels a PULL REQUEST here and issues: write alone 403s on that mutation, and actions: write to re-dispatch review-dispatch.yml after a real carry"
assert_equals "{'contents': 'read', 'issues': 'write', 'pull-requests': 'write', 'actions': 'write'}" "$(rpy 'print(d["jobs"]["open"]["permissions"])')"

# THE TRUSTED COPY, NOT THE PULL REQUEST'S OR THE MERGE COMMIT'S. A
# workflow_run job's default checkout already resolves the TRIGGERING
# workflow's repository ref, but pinning explicitly keeps the guarantee
# independent of that default — same pattern review-dispatch.yml uses for
# its own workflow_run jobs.
it "the open job's checkout is pinned to the default branch"
assert_equals "\${{ github.event.repository.default_branch }}" "$(rpy 'print(d["jobs"]["open"]["steps"][0]["with"]["ref"])')"

# THE ARCHIVE TRANSITION: a merge just landed on the default branch. Carry
# the standing instruction forward as the archive-station label ON THE
# ISSUE, since the pull request that carried the change is closed by the
# time this runs. Anchored on `ci` completing FOR a push event, since a
# merge to the default branch is what triggers that run.
it "the archive job fires on ci completing for a push event"
archive_if=$(rpy 'print(d["jobs"]["archive"]["if"])')
assert_contains "$archive_if" "github.event_name == 'workflow_run'"
assert_contains "$archive_if" "github.event.workflow_run.event == 'push'"
assert_contains "$archive_if" "github.event.workflow_run.conclusion == 'success'"

it "the archive job also checks the push landed on the default branch, not just any push-triggered ci run"
assert_contains "$archive_if" "github.event.workflow_run.head_branch == github.event.repository.default_branch"

it "the archive job resolves the merged pull request via a merged-pull-request search on the pushed commit"
archive_run=$(rpy 'print([s for s in d["jobs"]["archive"]["steps"] if s.get("id") == "carry"][0]["run"])')
assert_contains "$archive_run" "search/issues"
assert_contains "$archive_run" "is:pr is:merged \$HEAD_SHA"
assert_contains "$(rpy 'print([s for s in d["jobs"]["archive"]["steps"] if s.get("id") == "carry"][0]["env"])')" "github.event.workflow_run.head_sha"

it "the archive job's merged-pull-request search also checks gh's own exit status, same reason as the open job"
assert_contains "$archive_run" 'if ! pr=$(gh api'
assert_contains "$archive_run" "could not search for a merged pull request"

# THE SAME SHARED SCRIPT, station archive -- see the open job's test above
# for why this is not duplicated in the workflow YAML.
it "the archive job hands the resolved pull request off to carry.py, station archive"
assert_contains "$archive_run" ".github/scripts/carry.py --repo \"\$GITHUB_REPOSITORY\" --pr \"\$pr\" --station archive"
assert_not_contains "$archive_run" "headRepositoryOwner"
assert_not_contains "$archive_run" "carry-grant.py"
assert_not_contains "$archive_run" "carry-from-pr.sh"

# carry.py ITSELF: the change/* guard, the Refs and Closes extraction, and the stations it
# accepts, pinned once for whichever job calls it. Its decisions are the machine's.
it "carry.py refuses anything that is not a same-repo change/* branch, using isCrossRepository -- never an owner-string approximation"
carry=$(cat "$ROOT/.github/scripts/carry.py")
assert_contains "$carry" "isCrossRepository"
assert_contains "$carry" 'startswith("change/")'
assert_not_contains "$carry" "headRepositoryOwner"

it "carry.py accepts only the two stations, rather than defaulting to archive"
assert_contains "$carry" 'choices=["fix", "archive"]'

it "carry.py reads Refs and Closes from the body, and asks the machine rather than deciding"
assert_contains "$carry" '"Refs"'
assert_contains "$carry" '"Closes"'
assert_contains "$carry" "conveyor.carry_fix"
assert_contains "$carry" "conveyor.carry_archive"

it "the archive job is granted issues: write to carry the grant, plus pull-requests: read to find the merged pull request"
assert_equals "{'contents': 'read', 'issues': 'write', 'pull-requests': 'read'}" "$(rpy 'print(d["jobs"]["archive"]["permissions"])')"

it "the archive job's checkout is ALSO pinned to the default branch, same reason as open"
assert_equals "\${{ github.event.repository.default_branch }}" "$(rpy 'print(d["jobs"]["archive"]["steps"][0]["with"]["ref"])')"

# THE CARRIED ROUND RUNS, AND A DEAD ROUND IS REPORTED (#220). A carried
# `conveyor:fix` round is dispatched by `github-actions[bot]`, which the
# action refused by default before any model ran; and with `fix` failed,
# `land` was skipped and nothing reached the pull request for three days.
it "the fixing step names github-actions as the ONE bot that may start it -- never '*', never empty"
model=$(py 'print([s for s in d["jobs"]["fix"]["steps"] if s.get("id")=="model"][0]["with"]["allowed_bots"])')
assert_equals "github-actions" "$model"

it "land runs on a FAILED fix too, substitutes the empty patch and report, and passes --fix-failed"
assert_contains "$(py 'print(d["jobs"]["land"]["if"])')" "needs.fix.result == 'failure'"
assert_contains "$(py 'print([s.get("if","") for s in d["jobs"]["land"]["steps"]])')" "needs.fix.result == 'skipped' || needs.fix.result == 'failure'"
assert_contains "$(py 'print(d["jobs"]["land"]["steps"][-1]["env"]["FIX_FAILED"])')" "needs.fix.result == 'failure' && '--fix-failed'"
assert_contains "$(py 'print(d["jobs"]["land"]["steps"][-1]["run"])')" '$FIX_FAILED'

# `fix` HAS A BOUND, AND `cancelled` (what a bound produces) IS NOT `failure`.
# Measured on #243: the model step sat in_progress for 15+ hours with no job
# timeout at all, `land` never ran (none of its three matched fix.result
# values fire on a job with no result), and loop:running stayed posted over
# nothing. GitHub reports a `timeout-minutes` job as `cancelled`, never
# `failure` -- its own distinction, not this workflow's -- so `land` and the
# flag it passes both need the extra value, or the bound alone trades one
# silent hang for a different silent stop.
it "the fix job is bounded, so a hang becomes a terminal result land can act on"
assert_equals "30" "$(py 'print(d["jobs"]["fix"]["timeout-minutes"])')"

it "land also runs on a CANCELLED fix (a timeout, never a crash), and passes the distinct --fix-timed-out flag"
assert_contains "$(py 'print(d["jobs"]["land"]["if"])')" "needs.fix.result == 'cancelled'"
assert_contains "$(py 'print([s.get("if","") for s in d["jobs"]["land"]["steps"]])')" "needs.fix.result == 'failure' || needs.fix.result == 'cancelled'"
assert_contains "$(py 'print(d["jobs"]["land"]["steps"][-1]["env"]["FIX_TIMED_OUT"])')" "needs.fix.result == 'cancelled' && '--fix-timed-out'"
assert_contains "$(py 'print(d["jobs"]["land"]["steps"][-1]["run"])')" '$FIX_TIMED_OUT'

it "land restores conveyor-state.py, conveyor.py (which both import) AND review-not-clean.py beside the landing programs, and hands them to land-dispatch.py"
assert_contains "$(py 'print(d["jobs"]["land"])')" "for s in land-dispatch.py resolve-review-threads.py conveyor-state.py conveyor.py review-not-clean.py; do"
assert_contains "$(py 'print(d["jobs"]["land"]["steps"][-1]["run"])')" '--state-script "$RUNNER_TEMP/conveyor-state.py"'
assert_contains "$(py 'print(d["jobs"]["land"]["steps"][-1]["run"])')" '--thread-check-script "$RUNNER_TEMP/review-not-clean.py"'

it "the gate marks the loop and the station by EVENT when a round starts, through the machine's writer, and grants nothing by it"
assert_contains "$prog" 'apply_events(repo, int(pr), loop_event=d.loop_event)'
assert_contains "$prog" 'station_event=d.station_event'
assert_not_contains "$prog" "--loop running"

it "on a review completion that starts no round, the gate sends the refresh, which sends the machine an event"
assert_contains "$prog" "refresh-loop-state.py"
assert_contains "$prog" 'trigger.event == "review_completed"'

# MEASURED LIVE ON #233, the second occurrence of the same bug: a guard compared
# workflow_run.name to the literal 'claude-review', so it never ran. The path carries no per-run wording.
it "a completion is told apart by its workflow FILE's path, never its display name"
assert_contains "$prog" 'WORKFLOW_RUN_PATH'
assert_not_contains "$prog" "WORKFLOW_RUN_NAME"
assert_not_contains "$prog" "workflow_run.name"

summary
