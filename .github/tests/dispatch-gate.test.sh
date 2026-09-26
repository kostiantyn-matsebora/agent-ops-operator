#!/usr/bin/env bash
# THE DISPATCH GATE, END TO END OVER A STUBBED gh. dispatch-gate.py maps an event onto the
# machine's trigger, gathers the facts, asks `conveyor.gate` and acts on the answer. The
# machine's own table of every combination is in conveyor.test.py, so this suite covers what
# only the adapter can get wrong: resolving the pull request, reading who placed the label and
# when, counting rounds since, the outputs the later jobs read, and the effects (refusal
# comment, stripped label, state events, refresh).
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)

tmp=$(mktemp -d)
export GH_CALLS="$tmp/calls" FX="$tmp/fx" GITHUB_OUTPUT="$tmp/out" GITHUB_REPOSITORY=o/r
mkdir -p "$tmp/bin" "$FX" "$tmp/repo/.github/scripts"
cat > "$tmp/bin/gh" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "${*//$'\n'/ }" >> "$GH_CALLS"
jq_of() { local f="$1"; shift; local expr=""; while [ $# -gt 0 ]; do [ "$1" = "--jq" ] && expr="$2"; shift; done
  if [ -n "$expr" ]; then jq -r "$expr" < "$f"; else cat "$f"; fi; }
num() { printf '%s' "$*" | sed -n 's#.*repos/o/r/issues/\([0-9]*\)/.*#\1#p'; }
case "$*" in
  "pr view "*"--json isCrossRepository"*) cat "$FX/pr.json" ;;
  "issue view "*"--json labels --jq"*) tr ' ' '\n' < "$FX/labels-$3" ;;
  "issue view "*"--json labels"*) python3 -c 'import json,sys;print(json.dumps({"labels":[{"name":x} for x in open(sys.argv[1]).read().split()]}))' "$FX/labels-$3" ;;
  "api repos/o/r/issues/"*"/timeline"*) cat "$FX/timeline-$(num "$@").json" 2>/dev/null || echo '[]' ;;
  "api repos/o/r/collaborators/"*"/permission"*) l=$(printf '%s' "$*" | sed 's#.*collaborators/\([^/]*\)/.*#\1#'); cat "$FX/perm-$l" 2>/dev/null || echo none ;;
  "api repos/o/r/issues/"*"/comments"*) jq_of "$FX/comments-$(num "$@").json" "$@" 2>/dev/null || echo '[]' ;;
  "api --method GET repos/o/r/commits/"*"/pulls"*) cat "$FX/head-prs" 2>/dev/null ;;
esac
exit 0
STUB
chmod +x "$tmp/bin/gh"; export PATH="$tmp/bin:$PATH"
for f in dispatch-gate.py conveyor.py conveyor_io.py conveyor-state.py; do cp "$ROOT/.github/scripts/$f" "$tmp/repo/.github/scripts/"; done
cp "$ROOT/.github/review-triage.json" "$tmp/repo/.github/"
cat > "$tmp/repo/.github/scripts/refresh-loop-state.py" <<'PY'
import sys
open(__import__("os").environ["GH_CALLS"], "a").write("REFRESH " + " ".join(sys.argv[1:]) + "\n")
PY

FIX=conveyor:fix
pr() {  # pr <state> <labels...>   (the pull request: Refs #51, same repo, branch change/thing)
  local state="$1"; shift
  python3 - "$state" "${BODY:-Refs #51}" "${CROSS:-false}" "$@" > "$FX/pr.json" <<'PY'
import json, sys
state, body, cross, *labels = sys.argv[1:]
print(json.dumps({"state": state, "body": body, "isCrossRepository": cross == "true", "headRefName": "change/thing",
                  "headRefOid": "abc1234def", "labels": [{"name": n} for n in labels]}))
PY
}
timeline() {  # timeline <n> <label> <login> [when]
  python3 - "$2" "$3" "${4:-2026-09-25T10:00:00Z}" > "$FX/timeline-$1.json" <<'PY'
import json, sys
print(json.dumps([{"event": "labeled", "label": {"name": sys.argv[1]}, "actor": {"login": sys.argv[2]}, "created_at": sys.argv[3]}]))
PY
}
comments() {  # comments <n> <body>@<when> ...
  local n="$1"; shift
  python3 - "$@" > "$FX/comments-$n.json" <<'PY'
import json, sys
out = []
for a in sys.argv[1:]:
    body, when = a.rsplit("@", 1)
    out.append({"body": body.replace("\\n", "\n"), "created_at": when})
print(json.dumps(out))
PY
}
perm() { printf '%s' "$2" > "$FX/perm-$1"; }
issue_has() { printf '%s' "$2" > "$FX/labels-$1"; }
rounds() {  # rounds <n>: n round markers after the placement
  local n="$1" args=() i
  for i in $(seq 1 "$n"); do args+=("<!-- conveyor:round $i -->@2026-09-25T1$i:00:00Z"); done
  comments 220 "${args[@]}"
}
reset() {
  rm -f "${FX:?}"/*; : > "$GH_CALLS"; : > "$GITHUB_OUTPUT"; unset BODY CROSS
  for v in EVENT BODY ASSOCIATION SENDER PR REVIEW_TITLE REVIEW_PR RUN_HEAD WORKFLOW_RUN_PATH INPUT_MODE EVENT_LABEL; do unset "$v"; done
  issue_has 51 "conveyor:run opsx:review"; timeline 51 conveyor:run maintainer; perm maintainer write
  echo '[]' > "$FX/comments-220.json"; echo '[]' > "$FX/comments-51.json"
}
gate() { : > "$GH_CALLS"; : > "$GITHUB_OUTPUT"; (cd "$tmp/repo" && python3 .github/scripts/dispatch-gate.py 2>&1); }
output() { grep "^$1=" "$GITHUB_OUTPUT" | tail -1 | cut -d= -f2-; }
called() { grep -c -- "$1" "$GH_CALLS"; }
refused() { grep -c "^issue comment 220 .*refused" "$GH_CALLS"; }

carried() { pr OPEN "$FIX"; timeline 220 "$FIX" 'github-actions[bot]'; comments 220 "<!-- carry-grant:fix -->\nplaced by the workflow, carrying @maintainer's standing instruction@2026-09-25T10:00:01Z"; }
by_hand() { pr OPEN "$FIX"; timeline 220 "$FIX" maintainer; perm maintainer write; }

# ==== comments ================================================================================

it "a dispatch comment from a member starts the threads mode, and needs no label"
reset; pr OPEN; export EVENT=issue_comment PR=220 SENDER=maintainer ASSOCIATION=MEMBER BODY="/fix-accepted"
out=$(gate); rc=$?
assert_status 0 "$rc"
assert_equals "threads" "$(output mode)"
assert_equals "220" "$(output pr)"

it "the form is the whole comment, trimmed, trailing punctuation dropped, any case"
reset; pr OPEN; export EVENT=issue_comment PR=220 SENDER=maintainer ASSOCIATION=OWNER BODY="  /FIX-ACCEPTED!  "
gate >/dev/null; assert_equals "threads" "$(output mode)"

it "any other comment is refused as not a dispatch, and starts nothing"
reset; pr OPEN; export EVENT=issue_comment PR=220 SENDER=maintainer ASSOCIATION=MEMBER BODY="please /fix-accepted soon"
out=$(gate); rc=$?
assert_status 1 "$rc"
assert_equals "1" "$(refused)"
assert_not_contains "$(cat "$GITHUB_OUTPUT")" "mode=threads"

it "a dispatch from a person without write access is refused, naming what they have"
reset; pr OPEN; export EVENT=issue_comment PR=220 SENDER=stranger ASSOCIATION=NONE BODY="/fix-accepted"
out=$(gate); rc=$?
assert_status 1 "$rc"
assert_contains "$(grep '^issue comment 220' "$GH_CALLS")" "@stranger"
assert_contains "$(grep '^issue comment 220' "$GH_CALLS")" "write access"

it "a review-thread comment is the same trigger as a pull request comment"
reset; pr OPEN; export EVENT=pull_request_review_comment PR=220 SENDER=maintainer ASSOCIATION=COLLABORATOR BODY="/fix-accepted"
gate >/dev/null; assert_equals "threads" "$(output mode)"

it "a dispatch from a fork's pull request is refused, whoever asks"
reset; CROSS=true pr OPEN; export EVENT=issue_comment PR=220 SENDER=maintainer ASSOCIATION=OWNER BODY="/fix-accepted"
out=$(gate); rc=$?
assert_status 1 "$rc"
assert_contains "$(grep '^issue comment 220' "$GH_CALLS")" "fork"

# ==== a label placed ==========================================================================

it "a writer placing the fix label starts a round over everything, approved by them, since the placement"
reset; by_hand; export EVENT=pull_request PR=220 SENDER=maintainer EVENT_LABEL=$FIX
out=$(gate); rc=$?
assert_status 0 "$rc"
assert_equals "all" "$(output mode)"
assert_equals "maintainer" "$(output approver)"
assert_equals "2026-09-25T10:00:00Z" "$(output since)"
assert_equals "change/thing" "$(output branch)"
assert_equals "abc1234def" "$(output head)"
assert_equals "5" "$(output max_rounds)"

it "a round's start is recorded by EVENT, on the pull request and on the issue"
assert_contains "$(cat "$GH_CALLS")" "issue edit 220 --repo o/r --add-label loop:running"
assert_contains "$(cat "$GH_CALLS")" "issue edit 51 --repo o/r --add-label station:fix"

it "a reader placing the fix label is refused and the label removed, visibly"
reset; pr OPEN "$FIX"; timeline 220 "$FIX" reader; perm reader read; export EVENT=pull_request PR=220 SENDER=reader EVENT_LABEL=$FIX
out=$(gate); rc=$?
assert_status 1 "$rc"
assert_equals "1" "$(called 'pr edit 220 --repo o/r --remove-label conveyor:fix')"
assert_equals "1" "$(refused)"
assert_not_contains "$(cat "$GITHUB_OUTPUT")" "mode=all"

it "the workflow's own bot placing the fix label is accepted while conveyor:run stands for a writer"
reset; carried; export EVENT=pull_request PR=220 SENDER='github-actions[bot]' EVENT_LABEL=$FIX
out=$(gate); rc=$?
assert_status 0 "$rc"
assert_equals "all" "$(output mode)"

it "the approver of a carried label is read from the carry's own marker, never from the bot's login"
assert_equals "maintainer" "$(output approver)"

it "the marker is matched by its sentence shape: a later @mention in the comment is not the approver"
reset; carried; comments 220 "<!-- carry-grant:fix -->\nplaced, carrying @maintainer's standing instruction. cc @someone-else@2026-09-25T10:00:01Z"
export EVENT=pull_request PR=220 SENDER='github-actions[bot]' EVENT_LABEL=$FIX
gate >/dev/null; assert_equals "maintainer" "$(output approver)"

it "the bot's fix label with NO standing grant is refused and removed: it re-checks, it never trusts"
reset; carried; issue_has 51 "opsx:review"; export EVENT=pull_request PR=220 SENDER='github-actions[bot]' EVENT_LABEL=$FIX
out=$(gate); rc=$?
assert_status 1 "$rc"
assert_equals "1" "$(called 'pr edit 220 --repo o/r --remove-label conveyor:fix')"

it "the bot's fix label on a Closes pull request stands on conveyor:archive alone: the archive PR's rounds run"
reset; carried; BODY="Closes #51" pr OPEN "$FIX"; issue_has 51 "conveyor:archive opsx:archived"; timeline 51 conveyor:archive maintainer
export EVENT=pull_request PR=220 SENDER='github-actions[bot]' EVENT_LABEL=$FIX
out=$(gate); rc=$?
assert_status 0 "$rc"
assert_equals "all" "$(output mode)"

it "the same conveyor:archive does NOT stand for a Refs pull request: the implement loop needs conveyor:run"
reset; carried; issue_has 51 "conveyor:archive"; timeline 51 conveyor:archive maintainer
export EVENT=pull_request PR=220 SENDER='github-actions[bot]' EVENT_LABEL=$FIX
gate >/dev/null; assert_equals "1" "$(called 'pr edit 220 --repo o/r --remove-label conveyor:fix')"

it "the bot placing keep-going is refused: a grant of more rounds is a person's"
reset; carried; export EVENT=pull_request PR=220 SENDER='github-actions[bot]' EVENT_LABEL=conveyor:keep-going
gate >/dev/null; assert_equals "1" "$(called 'pr edit 220 --repo o/r --remove-label conveyor:keep-going')"

it "a grant whose placer has lost write access no longer stands"
reset; carried; perm maintainer read; export EVENT=pull_request PR=220 SENDER='github-actions[bot]' EVENT_LABEL=$FIX
gate >/dev/null; assert_equals "1" "$(called 'pr edit 220 --repo o/r --remove-label conveyor:fix')"

# ==== completions =============================================================================

it "a review completion names its pull request by the run's title and starts a round on a labelled pull request"
reset; by_hand; export EVENT=workflow_run WORKFLOW_RUN_PATH=.github/workflows/claude-review.yml REVIEW_TITLE="Review of #220"
out=$(gate); rc=$?
assert_status 0 "$rc"
assert_equals "220" "$(output pr)"
assert_equals "all" "$(output mode)"

it "a completion with no title finds its pull request in the payload"
reset; by_hand; export EVENT=workflow_run WORKFLOW_RUN_PATH=.github/workflows/claude-review.yml REVIEW_PR=220
gate >/dev/null; assert_equals "220" "$(output pr)"

it "with neither, it finds the one open pull request whose head is the run's sha"
reset; by_hand; echo 220 > "$FX/head-prs"; export EVENT=workflow_run WORKFLOW_RUN_PATH=.github/workflows/ci.yml RUN_HEAD=abc1234def
gate >/dev/null; assert_equals "220" "$(output pr)"

it "a sha that heads TWO open pull requests names none: nothing starts, and it says so"
reset; by_hand; printf '220\n221\n' > "$FX/head-prs"; export EVENT=workflow_run WORKFLOW_RUN_PATH=.github/workflows/ci.yml RUN_HEAD=abc1234def
out=$(gate); assert_equals "none" "$(output mode)"; assert_contains "$out" "2 open pull requests"

it "a completion that names no pull request at all starts nothing and fails nothing"
reset; export EVENT=workflow_run WORKFLOW_RUN_PATH=.github/workflows/ci.yml REVIEW_TITLE="unrelated"
out=$(gate); rc=$?
assert_status 0 "$rc"; assert_equals "none" "$(output mode)"

it "a completion on a pull request WITHOUT the fix label starts nothing"
reset; pr OPEN; export EVENT=workflow_run WORKFLOW_RUN_PATH=.github/workflows/claude-review.yml REVIEW_TITLE="Review of #220"
gate >/dev/null; assert_equals "none" "$(output mode)"

it "a review completion that starts no round hands the loop to the refresh, which sends the machine an event"
assert_contains "$(cat "$GH_CALLS")" "REFRESH --repo o/r --pr 220"

it "a CI completion starts no refresh: only a review can have opened a thread"
reset; pr OPEN; export EVENT=workflow_run WORKFLOW_RUN_PATH=.github/workflows/ci.yml REVIEW_TITLE="ci #220"
gate >/dev/null; assert_equals "0" "$(called REFRESH)"

it "a completion on a CARRIED label is RE-CHECKED: removing conveyor:run stops a loop already running (#254)"
reset; carried; issue_has 51 "opsx:review"; export EVENT=workflow_run WORKFLOW_RUN_PATH=.github/workflows/claude-review.yml REVIEW_TITLE="Review of #220"
out=$(gate); rc=$?
assert_status 1 "$rc"
assert_equals "1" "$(called 'pr edit 220 --repo o/r --remove-label conveyor:fix')"
assert_contains "$(cat "$GH_CALLS")" "issue edit 220 --repo o/r --add-label loop:stalled"
assert_not_contains "$(cat "$GITHUB_OUTPUT")" "mode=all"

it "a completion on a carried label whose grant STANDS runs the round, approved by the person behind it"
reset; carried; export EVENT=workflow_run WORKFLOW_RUN_PATH=.github/workflows/claude-review.yml REVIEW_TITLE="Review of #220"
gate >/dev/null; assert_equals "all" "$(output mode)"; assert_equals "maintainer" "$(output approver)"

it "a completion on a closed pull request starts nothing"
reset; pr CLOSED "$FIX"; export EVENT=workflow_run WORKFLOW_RUN_PATH=.github/workflows/claude-review.yml REVIEW_TITLE="Review of #220"
gate >/dev/null; assert_equals "none" "$(output mode)"

# ==== the bound ===============================================================================

it "at the bound a round does NOT start: the cap is enforced BEFORE the round, and the loop says capped"
reset; by_hand; rounds 5; export EVENT=workflow_run WORKFLOW_RUN_PATH=.github/workflows/claude-review.yml REVIEW_TITLE="Review of #220"
out=$(gate); rc=$?
assert_status 0 "$rc"
assert_equals "none" "$(output mode)"
assert_contains "$(cat "$GH_CALLS")" "issue edit 220 --repo o/r --add-label loop:capped"

it "one round under the bound still starts"
reset; by_hand; rounds 4; export EVENT=workflow_run WORKFLOW_RUN_PATH=.github/workflows/claude-review.yml REVIEW_TITLE="Review of #220"
gate >/dev/null; assert_equals "all" "$(output mode)"

it "rounds from BEFORE the label was placed do not count: removing and re-adding starts the count afresh"
reset; by_hand; timeline 220 "$FIX" maintainer 2026-09-26T09:00:00Z; rounds 5
export EVENT=workflow_run WORKFLOW_RUN_PATH=.github/workflows/claude-review.yml REVIEW_TITLE="Review of #220"
gate >/dev/null; assert_equals "all" "$(output mode)"

it "conveyor:keep-going lifts the bound for a round"
reset; pr OPEN "$FIX" conveyor:keep-going; timeline 220 "$FIX" maintainer; rounds 5
export EVENT=workflow_run WORKFLOW_RUN_PATH=.github/workflows/claude-review.yml REVIEW_TITLE="Review of #220"
gate >/dev/null; assert_equals "all" "$(output mode)"

it "a hand dispatch of a round obeys the same bound"
reset; by_hand; rounds 5; export EVENT=workflow_dispatch PR=220 SENDER=maintainer INPUT_MODE=all
gate >/dev/null; assert_equals "none" "$(output mode)"

# ==== a hand run ==============================================================================

it "a hand run in threads mode needs no label and starts the accepted findings only"
reset; pr OPEN; export EVENT=workflow_dispatch PR=220 SENDER=maintainer INPUT_MODE=threads
gate >/dev/null; assert_equals "threads" "$(output mode)"

it "a hand run in all mode on a labelled pull request runs a round, approved by whoever placed the label"
reset; by_hand; export EVENT=workflow_dispatch PR=220 SENDER=maintainer INPUT_MODE=all
gate >/dev/null; assert_equals "all" "$(output mode)"; assert_equals "maintainer" "$(output approver)"

it "the workflow's own dispatch is a bot's and is RE-CHECKED: no standing grant, no round"
reset; carried; issue_has 51 "opsx:review"; export EVENT=workflow_dispatch PR=220 SENDER='github-actions[bot]' INPUT_MODE=all
out=$(gate); rc=$?
assert_status 1 "$rc"; assert_not_contains "$(cat "$GITHUB_OUTPUT")" "mode=all"

it "the workflow's own dispatch with the grant standing runs the round"
reset; carried; export EVENT=workflow_dispatch PR=220 SENDER='github-actions[bot]' INPUT_MODE=all
gate >/dev/null; assert_equals "all" "$(output mode)"

# ==== the odd cases ===========================================================================

it "a fix label the timeline shows nobody placing starts nothing, and says so"
reset; pr OPEN "$FIX"; export EVENT=workflow_dispatch PR=220 SENDER=maintainer INPUT_MODE=all
out=$(gate); assert_equals "none" "$(output mode)"; assert_contains "$out" "nobody placing it"

it "an event the workflow does not know starts nothing"
reset; export EVENT=push; out=$(gate); rc=$?
assert_status 0 "$rc"; assert_equals "none" "$(output mode)"

it "a pull request with no tracking issue runs on its own placement: a hand-labelled loop needs no grant"
reset; BODY="no issue here" pr OPEN "$FIX"; timeline 220 "$FIX" maintainer; export EVENT=pull_request PR=220 SENDER=maintainer EVENT_LABEL=$FIX
gate >/dev/null; assert_equals "all" "$(output mode)"
assert_equals "0" "$(called 'issue edit 51')"

it "the dispatcher is always an output, so the later jobs can name who asked"
assert_equals "maintainer" "$(output dispatcher)"

rm -rf "${tmp:?}"
summary
