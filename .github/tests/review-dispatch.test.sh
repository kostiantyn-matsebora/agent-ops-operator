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

it "the review-completion trigger names the review workflow and completes only"
assert_equals "['claude-review'] ['completed']" "$(py 'print(d[True]["workflow_run"]["workflows"], d[True]["workflow_run"]["types"])')"

it "the label the gate prefilters on is the one the vocabulary file states"
label=$(python3 -c 'import json;print(json.load(open("'"$ROOT"'/.github/review-triage.json"))["approve_label"])')
assert_contains "$(py 'print(d["jobs"]["gate"]["if"])')" "github.event.label.name == '$label'"

it "a review that did not complete successfully starts no round"
assert_contains "$(py 'print(d["jobs"]["gate"]["if"])')" "github.event.workflow_run.conclusion == 'success'"

it "the gate reads the vocabulary from the default branch on every event but a hand run"
assert_contains "$(py 'print(d["jobs"]["gate"]["steps"][0]["with"]["ref"])')" "github.event_name == 'workflow_dispatch' && github.ref || github.event.repository.default_branch"

it "a non-writer's label is removed, visibly, in the gate"
gate=$(py 'print(d["jobs"]["gate"]["steps"][1]["run"])')
assert_contains "$gate" "collaborators/\$1/permission"
assert_contains "$gate" "--remove-label"

it "the label is read at every round, so removing it stops the loop at the next boundary"
assert_contains "$gate" "grep -qx \"\$LABEL\""

it "the collect job holds the analysis token and the model job holds none — no secret reaches the fixing step but its own credential"
assert_contains "$(py 'print(d["jobs"]["collect"]["steps"][1]["env"])')" "SONAR_TOKEN"
fixjob=$(py 'print(d["jobs"]["fix"])')
assert_not_contains "$fixjob" "SONAR_TOKEN"
assert_not_contains "$fixjob" "secrets.GITHUB_TOKEN"
assert_contains "$fixjob" "secrets.CLAUDE_CODE_OAUTH_TOKEN"

it "no job may dispatch a workflow — a dispatched run's checks never reach the merge box, so the loop pushes instead"
assert_equals "" "$(py 'print(" ".join(j for j,v in d["jobs"].items() if v["permissions"].get("actions")))')"
assert_not_contains "$(py 'print(d["jobs"]["land"])')" "workflow run"

it "the push credential is read by the landing job alone, on a labelled pull request only, and the model's job cannot name it"
assert_equals "land" "$(py 'print(" ".join(j for j,v in d["jobs"].items() if "AUTOFIX_DEPLOY_KEY" in str(v)))')"
cred=$(py 'print([s for s in d["jobs"]["land"]["steps"] if s.get("id")=="cred"][0])')
assert_contains "$cred" "needs.gate.outputs.mode == 'all'"
assert_contains "$cred" "secrets.AUTOFIX_DEPLOY_KEY"

it "the landing program is told whether the push starts workflows, and only when the credential was configured"
assert_contains "$(py 'print(d["jobs"]["land"]["steps"][-1]["env"]["STARTS"])')" "steps.cred.outputs.starts == 'true' && '--push-starts-workflows'"

it "the bound is a workflow constant the landing job passes through"
assert_equals "3" "$(py 'print(d["env"]["MAX_ROUNDS"])')"
assert_contains "$(py 'print(d["jobs"]["land"]["steps"][-1]["run"])')" '--max-rounds "$MAX_ROUNDS"'

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

it "gives the model no gh, no git push, and no git commit"
tools=$(py 'print([s for s in d["jobs"]["fix"]["steps"] if "claude-code-action" in s.get("uses","")][0]["with"]["claude_args"])')
assert_not_contains "$tools" "gh"
assert_not_contains "$tools" "git push"
assert_not_contains "$tools" "git commit"

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

summary
