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

it "triggers on the two comment events and a hand run, and nothing else"
assert_equals "issue_comment pull_request_review_comment workflow_dispatch" "$(py 'print(" ".join(sorted(d[True])))')"

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
