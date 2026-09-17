#!/usr/bin/env bash
# A person's comment on a loop-driven pull request re-runs the one ci job that
# reads the conversation. What it listens to, who counts, what it may do, and
# that it is a re-run and never a check of its own (#131).
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
W="$ROOT/.github/workflows/dispute-answered.yml"
py() { python3 -c "
import sys, yaml
d = yaml.safe_load(open(sys.argv[1]))
$1
" "$W"; }

it "fires on a created comment -- on the pull request or in a review thread -- and on nothing else"
assert_equals "issue_comment pull_request_review_comment" "$(py 'print(" ".join(sorted(d[True])))')"
assert_equals "['created'] ['created']" "$(py 'print(d[True]["issue_comment"]["types"], d[True]["pull_request_review_comment"]["types"])')"

it "a bot's comment, a dispatch, and a comment on a plain issue all skip the job"
cond=$(py 'print(d["jobs"]["rerun"]["if"])')
assert_contains "$cond" "github.event.comment.user.type != 'Bot'"
assert_contains "$cond" "!startsWith(github.event.comment.body, '/fix-accepted')"
assert_contains "$cond" "github.event.issue.pull_request"

it "is one job holding actions: write, reading contents and pull requests, and nothing more"
assert_equals "rerun" "$(py 'print(" ".join(d["jobs"]))')"
assert_equals "{'contents': 'read', 'pull-requests': 'read', 'actions': 'write'}" "$(py 'print(d["jobs"]["rerun"]["permissions"])')"
assert_equals "{}" "$(py 'print(d["permissions"])')"

it "checks out the default branch, never the pull request's tree"
assert_contains "$(py 'print(d["jobs"]["rerun"]["steps"][0]["with"]["ref"])')" "github.event.repository.default_branch"

it "re-runs docs-task only on a pull request carrying the approve label, read from the vocabulary, and runs no model"
job=$(py 'print(d["jobs"]["rerun"])')
assert_contains "$job" "rerun-ci-job.py"
assert_contains "$job" "--job docs-task"
assert_contains "$job" '["approve_label"]'
assert_not_contains "$job" "claude"
assert_not_contains "$job" "workflow run"

it "serialises per pull request without cancelling"
assert_contains "$(py 'print(d["concurrency"]["group"])')" "github.event.issue.number || github.event.pull_request.number"
assert_equals "False" "$(py 'print(d["concurrency"]["cancel-in-progress"])')"

it "docs-task is the ci job it names, and that job is what runs the loop's archive guard"
ci=$(python3 -c 'import yaml;d=yaml.safe_load(open("'"$ROOT"'/.github/workflows/ci.yml"));print([s.get("run","") for s in d["jobs"]["docs-task"]["steps"]])')
assert_contains "$ci" "autofix-guard.py"

summary
