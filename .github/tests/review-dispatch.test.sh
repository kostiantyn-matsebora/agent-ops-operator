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

it "the label the gate prefilters on is the one the vocabulary file states"
label=$(python3 -c 'import json;print(json.load(open("'"$ROOT"'/.github/review-triage.json"))["approve_label"])')
assert_contains "$(py 'print(d["jobs"]["gate"]["if"])')" "github.event.label.name == '$label'"
assert_equals "conveyor:fix" "$label"

it "the keep-going label ALSO prefilters the pull_request trigger, so placing it re-triggers a round"
keep_going=$(python3 -c 'import json;print(json.load(open("'"$ROOT"'/.github/review-triage.json"))["keep_going_label"])')
assert_contains "$(py 'print(d["jobs"]["gate"]["if"])')" "github.event.label.name == '$keep_going'"
assert_equals "conveyor:keep-going" "$keep_going"

it "a review that did not complete successfully starts no round"
assert_contains "$(py 'print(d["jobs"]["gate"]["if"])')" "github.event.workflow_run.name == 'claude-review' && github.event.workflow_run.conclusion == 'success'"

# THE TWO RUNS START A ROUND FOR OPPOSITE REASONS, so the prefilter names each
# workflow with the conclusion that matters for it. A green CI run starting a
# round would be a round over nothing, every push.
it "a ci run starts a round only when it FAILED"
assert_contains "$(py 'print(d["jobs"]["gate"]["if"])')" "github.event.workflow_run.name == 'ci' && github.event.workflow_run.conclusion == 'failure'"
assert_not_contains "$(py 'print(d["jobs"]["gate"]["if"])')" "workflow_run.name == 'ci' && github.event.workflow_run.conclusion == 'success'"

it "the gate reads the vocabulary from the default branch on every event but a hand run"
assert_contains "$(py 'print(d["jobs"]["gate"]["steps"][0]["with"]["ref"])')" "github.event_name == 'workflow_dispatch' && github.ref || github.event.repository.default_branch"

it "a non-writer's label is removed, visibly, in the gate"
gate=$(py 'print(d["jobs"]["gate"]["steps"][1]["run"])')
assert_contains "$gate" "collaborators/\$1/permission"
assert_contains "$gate" "--remove-label"

it "the label is read at every round, so removing it stops the loop at the next boundary"
assert_contains "$gate" "grep -qx \"\$LABEL\""

# A PROGRAM MAY CARRY A GRANT FORWARD OR CONSUME ONE. IT MAY NEVER MINT ONE.
# A label placed by github-actions[bot] (carry-grant.py's own actor) is
# RE-CHECKED against the issue it claims to carry from — never trusted
# because a workflow placed it.
it "a label carried by github-actions[bot] is re-checked against the originating issue's standing instruction"
assert_contains "$gate" 'SENDER" = "github-actions[bot]"'
assert_contains "$gate" "Refs #"
assert_contains "$gate" "RUN_LABEL"
assert_contains "$gate" "STANDING"

it "a carried label whose originating instruction is gone is refused exactly as a non-writer's is"
assert_contains "$gate" "no longer there"
assert_contains "$gate" "--remove-label \"\$THIS_LABEL\""

it "the pull_request trigger's label event is checked against ITSELF, not hardcoded to the fix label — a keep-going event is never refused for not being conveyor:fix"
assert_contains "$gate" 'THIS_LABEL="$EVENT_LABEL"'

# A CARRIED LABEL'S TIMELINE ACTOR IS THE BOT, NEVER THE APPROVER -- measured
# live: carry-grant.py places the label under this workflow's own token, so
# the timeline records github-actions[bot] as the actor. label_placement()
# must fall back to the real approver carry-grant.py's own marker comment
# already names, or every carried round reports "approved by
# github-actions[bot]" instead of the person whose grant it actually was.
it "label_placement falls back to the carry-grant marker comment's named approver when the timeline actor is the bot"
assert_contains "$gate" 'actor" = "github-actions[bot]"'
assert_contains "$gate" "carry-grant:fix"
assert_contains "$gate" "grep -oE '@[A-Za-z0-9_.-]+'"

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
assert_contains "$gate" 'json.load(open(".github/review-triage.json"))["max_rounds"]'
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

it "remote-implement triggers on a labelled issue and on ci's workflow_run completing, and nothing else"
assert_equals "issues workflow_run" "$(rpy 'print(" ".join(sorted(d[True])))')"
assert_equals "['labeled']" "$(rpy 'print(d[True]["issues"]["types"])')"
assert_equals "['ci']" "$(rpy 'print(d[True]["workflow_run"]["workflows"])')"
assert_equals "['completed']" "$(rpy 'print(d[True]["workflow_run"]["types"])')"

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
assert_equals "conveyor:implement" "$implement_label"
assert_equals "conveyor:run" "$run_label"

it "the fire endpoint and its token are not in the tree"
job=$(rpy 'print(d["jobs"]["fire"])')
assert_contains "$job" "vars.ROUTINE_FIRE_URL"
assert_contains "$job" "secrets.ROUTINE_FIRE_TOKEN"

# THE OPEN TRANSITION: a session's own pull request appears, unlabelled; if
# its originating issue still carries the standing instruction, carry it
# forward as the fix-station label ON THIS PULL REQUEST. Anchored on `ci`
# completing FOR a pull_request event, never on `pull_request` directly — a
# GITHUB_TOKEN issued to a pull_request-triggered run carries no write
# access in this repository at all, measured live (see the workflow's own
# header comment).
it "the open job fires on ci completing for a pull_request event, never pull_request directly"
open_if=$(rpy 'print(d["jobs"]["open"]["if"])')
assert_contains "$open_if" "github.event_name == 'workflow_run'"
assert_contains "$open_if" "github.event.workflow_run.event == 'pull_request'"
assert_contains "$open_if" "github.event.workflow_run.conclusion == 'success'"
assert_not_contains "$open_if" "github.event.pull_request"

it "the open job resolves the pull request from the workflow_run's head sha, not from a pull_request event payload"
open_run=$(rpy 'print(d["jobs"]["open"]["steps"][-1]["run"])')
assert_contains "$open_run" 'head_sha="${{ github.event.workflow_run.head_sha }}"'
assert_contains "$open_run" "commits/\$head_sha/pulls"
assert_contains "$open_run" "select(.state == \"open\")"

it "the open job's own no-pull-request message names the workflow_run's head sha, never \$GITHUB_SHA (that names the trusted checkout's commit, not this run's)"
assert_contains "$open_run" '${head_sha:0:7}'
assert_not_contains "$open_run" '${GITHUB_SHA:0:7}'

# THE change/* GUARD AND Refs #<n> EXTRACTION LIVE IN ONE SHARED SCRIPT,
# carry-from-pr.sh, called by BOTH open and archive with the resolved pull
# request number and the station -- not duplicated in the workflow YAML.
it "the open job hands the resolved pull request off to carry-from-pr.sh, station fix"
assert_contains "$open_run" ".github/scripts/carry-from-pr.sh \"\$pr\" fix"
assert_not_contains "$open_run" "headRepositoryOwner"
assert_not_contains "$open_run" "carry-grant.py"

it "the open job is granted issues: write for the marker comment, plus pull-requests: write since carry-grant.py labels a PULL REQUEST here and issues: write alone 403s on that mutation"
assert_equals "{'contents': 'read', 'issues': 'write', 'pull-requests': 'write'}" "$(rpy 'print(d["jobs"]["open"]["permissions"])')"

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
archive_run=$(rpy 'print(d["jobs"]["archive"]["steps"][-1]["run"])')
assert_contains "$archive_run" "search/issues"
assert_contains "$archive_run" "is:pr is:merged"
assert_contains "$archive_run" "github.event.workflow_run.head_sha"

# THE SAME SHARED SCRIPT, station archive -- see the open job's test above
# for why this is not duplicated in the workflow YAML.
it "the archive job hands the resolved pull request off to carry-from-pr.sh, station archive"
assert_contains "$archive_run" ".github/scripts/carry-from-pr.sh \"\$pr\" archive"
assert_not_contains "$archive_run" "headRepositoryOwner"
assert_not_contains "$archive_run" "carry-grant.py"

# carry-from-pr.sh ITSELF: the change/* guard, the Refs #<n> extraction, and
# routing to carry-grant.py by station -- pinned once, for whichever job
# calls it.
it "carry-from-pr.sh refuses anything that is not a same-repo change/* branch, using isCrossRepository -- never an owner-string approximation"
carry_from_pr=$(cat "$ROOT/.github/scripts/carry-from-pr.sh")
assert_contains "$carry_from_pr" "isCrossRepository"
assert_contains "$carry_from_pr" "change/*"
assert_not_contains "$carry_from_pr" "headRepositoryOwner"

it "carry-from-pr.sh refuses any station other than fix or archive, rather than defaulting to archive"
assert_contains "$carry_from_pr" 'fix|archive'

it "carry-from-pr.sh extracts Refs #<n> and calls carry-grant.py with --pr only for station fix"
assert_contains "$carry_from_pr" "Refs #"
assert_contains "$carry_from_pr" "carry-grant.py"
assert_contains "$carry_from_pr" '--station fix --pr "$pr"'
assert_contains "$carry_from_pr" "--station archive"

it "the archive job is granted issues: write to carry the grant, plus pull-requests: read to find the merged pull request"
assert_equals "{'contents': 'read', 'issues': 'write', 'pull-requests': 'read'}" "$(rpy 'print(d["jobs"]["archive"]["permissions"])')"

it "the archive job's checkout is ALSO pinned to the default branch, same reason as open"
assert_equals "\${{ github.event.repository.default_branch }}" "$(rpy 'print(d["jobs"]["archive"]["steps"][0]["with"]["ref"])')"

summary
