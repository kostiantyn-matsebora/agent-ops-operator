#!/usr/bin/env bash
# The shape of the recovery workflow -- separate from review-dispatch.yml on
# purpose, so it shares no concurrency group with the run it audits.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
W="$ROOT/.github/workflows/recover-loop-state.yml"

py() { python3 -c "
import sys, yaml
d = yaml.safe_load(open(sys.argv[1]))
$1
" "$W"; }

it "triggers only on review-dispatch's own completion"
assert_equals "['review-dispatch'] ['completed']" "$(py 'print(d[True]["workflow_run"]["workflows"], d[True]["workflow_run"]["types"])')"

it "shares no concurrency group with review-dispatch.yml -- it must never be cancelled by, or race, the run it audits"
assert_equals "None" "$(py 'print(d.get("concurrency"))')"

it "only acts on a CANCELLED run naming a same-repo pull request"
assert_contains "$(py 'print(d["jobs"]["recover"]["if"])')" "conclusion == 'cancelled'"
assert_contains "$(py 'print(d["jobs"]["recover"]["if"])')" "pull_requests[0].number"

it "checks the completed run's own job count before doing anything else"
steps=$(py 'print([s.get("name","") for s in d["jobs"]["recover"]["steps"]])')
assert_contains "$steps" "Confirm the superseded run reported zero jobs"

it "the vocabulary and script are read from the DEFAULT branch, never the pull request's own copy"
assert_contains "$(py 'print(d["jobs"]["recover"]["steps"][1]["with"]["ref"])')" "github.event.repository.default_branch"

it "only writes the label when the confirmed job count is zero"
gate=$(py 'print([s.get("if","") for s in d["jobs"]["recover"]["steps"]])')
assert_contains "$gate" "steps.jobs.outputs.count == "

it "passes the run's own conclusion and job count to the script, rather than letting it re-derive them"
last=$(py 'print(d["jobs"]["recover"]["steps"][-1]["run"])')
assert_contains "$last" "--only-if-superseded"
assert_contains "$last" "--run-conclusion"
assert_contains "$last" "--run-job-count"

it "holds only the permissions it needs: no write beyond labels, no model"
assert_equals "{'contents': 'read', 'pull-requests': 'write', 'actions': 'read'}" \
  "$(py 'print(d["jobs"]["recover"]["permissions"])')"

summary
