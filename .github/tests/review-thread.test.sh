#!/usr/bin/env bash
# The thread-event workflow: what it listens to, what it may do, and that it
# runs the trusted copy. A re-run, never a check of its own (#131).
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
W="$ROOT/.github/workflows/review-thread.yml"
py() { python3 -c "
import sys, yaml
d = yaml.safe_load(open(sys.argv[1]))
$1
" "$W"; }

it "fires on a review thread being resolved or unresolved, and on nothing else"
assert_equals "['pull_request_review_thread']" "$(py 'print(list(d[True]))')"
assert_equals "['resolved', 'unresolved']" "$(py 'print(d[True]["pull_request_review_thread"]["types"])')"

it "is one job holding actions: write and contents: read, and nothing more"
assert_equals "rerun" "$(py 'print(" ".join(d["jobs"]))')"
assert_equals "{'contents': 'read', 'actions': 'write'}" "$(py 'print(d["jobs"]["rerun"]["permissions"])')"
assert_equals "{}" "$(py 'print(d["permissions"])')"

it "checks out the default branch, never the pull request's tree"
assert_contains "$(py 'print(d["jobs"]["rerun"]["steps"][0]["with"]["ref"])')" "github.event.repository.default_branch"

it "runs rerun-review-clean.py on the pull request's head, and no model"
job=$(py 'print(d["jobs"]["rerun"])')
assert_contains "$job" "rerun-review-clean.py"
assert_contains "$job" "github.event.pull_request.head.sha"
assert_not_contains "$job" "claude"
assert_not_contains "$job" "workflow run"

it "serialises per pull request without cancelling: a second thread event waits for the first"
assert_contains "$(py 'print(d["concurrency"]["group"])')" "github.event.pull_request.number"
assert_equals "False" "$(py 'print(d["concurrency"]["cancel-in-progress"])')"

summary
