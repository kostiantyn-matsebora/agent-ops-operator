#!/usr/bin/env bash
# THE REVIEW'S VERDICT IS READ LIVE, IN CI, AND NOWHERE ELSE. `review-clean`
# asks two questions of the moment -- did the review run, is any of its threads
# still open -- and the review's own `reconcile` job asks neither of the run's
# conclusion. Measured on #220: the gate lived in `reconcile`, froze the run's
# conclusion at `failure`, and a resolved thread changed nothing.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
CI="$ROOT/.github/workflows/ci.yml"
REVIEW="$ROOT/.github/workflows/claude-review.yml"
py() { python3 -c "
import sys, yaml
d = yaml.safe_load(open(sys.argv[1]))
$2
" "$1"; }

it "review-clean runs review-is-green.py THEN review-not-clean.py, against the pull request, live"
steps=$(py "$CI" 'print([s.get("run","") for s in d["jobs"]["review-clean"]["steps"]])')
assert_contains "$steps" "review-is-green.py"
assert_contains "$steps" "review-not-clean.py"
assert_contains "$steps" 'github.event.pull_request.number'
green=$(py "$CI" 'print([i for i,s in enumerate(d["jobs"]["review-clean"]["steps"]) if "review-is-green" in s.get("run","")][0])')
open=$(py "$CI" 'print([i for i,s in enumerate(d["jobs"]["review-clean"]["steps"]) if "review-not-clean" in s.get("run","")][0])')
[ "$green" -lt "$open" ] && pass || fail "review-is-green must run before review-not-clean"

it "review-clean may read pull requests and actions, and write nothing"
assert_equals "{'contents': 'read', 'actions': 'read', 'pull-requests': 'read'}" "$(py "$CI" 'print(d["jobs"]["review-clean"]["permissions"])')"

it "review-clean is still what ci-green needs for the review"
assert_contains "$(py "$CI" 'print(d["jobs"]["ci-green"]["needs"])')" "review-clean"

it "the review's reconcile job no longer fails its run on an open thread"
assert_not_contains "$(py "$REVIEW" 'print([s.get("run","") for s in d["jobs"]["reconcile"]["steps"]])')" "review-not-clean.py"

it "review-not-clean.py is RUN by exactly one workflow job, ci's review-clean -- a comment naming it elsewhere is not a run"
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
assert_equals "ci.yml:review-clean" "$runs"

summary
