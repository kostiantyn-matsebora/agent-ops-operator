#!/usr/bin/env bash
# NO CHECK REPORTS THE REVIEW'S THREAD STATE. `review-clean` asks one question
# a check can answer -- did the review run for this head -- and the review's
# own `reconcile` job fails its run on nothing else. An open thread is branch
# protection's live, merge-time question. Measured on #220: the gate lived in
# `reconcile`, froze the run's conclusion at `failure`, a resolved thread
# changed nothing -- and the repair of re-running on the thread event does not
# exist, because `pull_request_review_thread` is not an Actions trigger.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
CI="$ROOT/.github/workflows/ci.yml"
REVIEW="$ROOT/.github/workflows/claude-review.yml"
py() { python3 -c "
import sys, yaml
d = yaml.safe_load(open(sys.argv[1]))
$2
" "$1"; }

it "review-clean asks only whether the review RAN: review-is-green.py, and no thread question"
steps=$(py "$CI" 'print([s.get("run","") for s in d["jobs"]["review-clean"]["steps"]])')
assert_contains "$steps" "review-is-green.py"
assert_not_contains "$steps" "review-not-clean.py"

it "review-clean may read actions, and write nothing"
assert_equals "{'contents': 'read', 'actions': 'read'}" "$(py "$CI" 'print(d["jobs"]["review-clean"]["permissions"])')"

it "review-clean is still what ci-green needs for the review"
assert_contains "$(py "$CI" 'print(d["jobs"]["ci-green"]["needs"])')" "review-clean"

it "the review's reconcile job no longer fails its run on an open thread"
assert_not_contains "$(py "$REVIEW" 'print([s.get("run","") for s in d["jobs"]["reconcile"]["steps"]])')" "review-not-clean.py"

it "review-not-clean.py is RUN by NO workflow job: a check must never carry the thread question"
runs=$(python3 - "$ROOT"/.github/workflows/*.yml <<'PY'
import sys, yaml
hits = []
for path in sys.argv[1:]:
    d = yaml.safe_load(open(path))
    for job, spec in (d.get("jobs") or {}).items():
        for step in spec.get("steps") or []:
            if "review-not-clean.py" in (step.get("run") or ""):
                hits.append(f"{path.rsplit('/',1)[-1]}:{job}")
print(" ".join(sorted(hits)))
PY
)
assert_equals "" "$runs"

it "no workflow listens on pull_request_review_thread: it is a webhook event, not an Actions trigger, and the file is refused"
triggers=$(python3 - "$ROOT"/.github/workflows/*.yml <<'PY'
import sys, yaml
hits = []
for path in sys.argv[1:]:
    on = yaml.safe_load(open(path)).get(True) or {}
    keys = on if isinstance(on, dict) else {k: None for k in (on if isinstance(on, list) else [on])}
    if "pull_request_review_thread" in keys:
        hits.append(path.rsplit("/", 1)[-1])
print(" ".join(hits))
PY
)
assert_equals "" "$triggers"

summary
