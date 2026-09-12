#!/usr/bin/env bash
# A change's tracking issue closes at ARCHIVE, never at merge.
#
# THE REFUSALS ARE THE PRODUCT. Closing a change's issue on the proposal that
# opened it is the failure this guard exists for, and it is silent: GitHub does
# it on merge, in a thread nobody is reading.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
guard="$ROOT/.github/scripts/pr-closes-guard.py"

repo=$(make_repo)
tmp=$(mktemp -d)
base=$(git -C "$repo" rev-parse HEAD)

# A change in flight, with its binding — the shape a proposal's branch carries.
mkdir -p "$repo/openspec/changes/coordinated-agents"
echo 53 > "$repo/openspec/changes/coordinated-agents/.github-issue"
echo "a plan" > "$repo/openspec/changes/coordinated-agents/proposal.md"
git -C "$repo" add -A; git -C "$repo" commit -qm "propose"

body() { printf '%s\n' "$1" > "$tmp/body"; }
G() { (cd "$repo" && python3 "$guard" --body-file "$tmp/body" --range "$base..HEAD" --root . 2>&1); }
status() { local rc=0; G >/dev/null 2>&1 || rc=$?; echo "$rc"; }

it "REFUSES a proposal that would close its own change's tracking issue"
body "What this does.

Closes #53"
assert_equals "1" "$(status)"
assert_contains "$(G)" "REFUSED  #53 tracks coordinated-agents"

it "names the form to use instead, rather than only refusing"
assert_contains "$(G)" "Refs #"

it "accepts Refs, which is what the template now writes"
body "What this does.

Refs #53"
assert_equals "0" "$(status)"

it "accepts a closing keyword for an issue no change tracks"
body "Closes #31"
assert_equals "0" "$(status)"

it "catches every closing keyword GitHub honours, not just Closes"
for word in closes Closed FIX fixes resolved "Resolves:"; do
  body "$word #53"
  assert_equals "1" "$(status)"
done

# THE ONE CASE WHERE CLOSING IS CORRECT: the archive really does end the issue's
# life, and /opsx:archive closes it for exactly that reason.
it "ACCEPTS closing the issue of a change this pull request archives"
mkdir -p "$repo/openspec/changes/archive/2026-08-26-coordinated-agents"
git -C "$repo" mv openspec/changes/coordinated-agents/.github-issue \
                  openspec/changes/archive/2026-08-26-coordinated-agents/.github-issue
git -C "$repo" mv openspec/changes/coordinated-agents/proposal.md \
                  openspec/changes/archive/2026-08-26-coordinated-agents/proposal.md
git -C "$repo" commit -qm "archive it"
body "Closes #53"
assert_equals "0" "$(status)"
assert_contains "$(G)" "which this pull request archives"

# FAILS OPEN on what it cannot read, exactly as the other gates do.
it "says nothing when there is no body to read"
rm -f "$tmp/body"
assert_equals "0" "$(status)"

# WHAT THE BASE GAINED AFTER THE BRANCH POINT IS NOT THE PULL REQUEST'S. #174
# was refused for not closing the issues of two changes that master archived
# days after its branch was cut: the workflow diffed the event's frozen base
# sha against the MERGE commit, which holds master as it is now. The range the
# workflow hands over is the three-dot form, and this is the fixture it must
# stay correct on: a branch that archives nothing, over a base that did.
it "does not charge a pull request with an archive its base gained after the branch point"
head=$(git -C "$repo" rev-parse HEAD)
git -C "$repo" checkout -q -b topic "$base"
echo "unrelated" > "$repo/NOTES.md"
git -C "$repo" add -A; git -C "$repo" commit -qm "a topic commit"
body "Refs #53"
# master (the original HEAD) archived coordinated-agents; the topic did not.
rc=0; (cd "$repo" && python3 "$guard" --body-file "$tmp/body" --range "$head...HEAD" --root .) >"$tmp/out" 2>&1 || rc=$?
assert_equals "0" "$rc"
assert_not_contains "$(cat "$tmp/out")" "ARCHIVES a change"
it "and the two-dot form against the merge commit is the shape that did"
git -C "$repo" merge -q --no-edit "$head" 2>/dev/null || git -C "$repo" merge -q --no-edit -m "merge" "$head"
rc=0; (cd "$repo" && python3 "$guard" --body-file "$tmp/body" --range "$base..HEAD" --root .) >"$tmp/out" 2>&1 || rc=$?
assert_equals "1" "$rc"

# The workflow is the only caller, and the range is what it must not regress on.
it "ci.yml diffs the live base branch three-dot to the pull request's head, never github.sha"
job=$(awk '/^  pr-closes:/,/^  conformance:/' "$ROOT/.github/workflows/ci.yml")
assert_contains "$job" '--range "origin/$BASE_REF...$HEAD_SHA"'
assert_contains "$job" 'HEAD_SHA: ${{ github.event.pull_request.head.sha }}'
assert_not_contains "$job" 'GITHUB_SHA'

# THE GUARD'S OWN REFUSAL MUST REACH A PERSON, NOT ONLY A JOB LOG. #211 sat
# refused for hours because nobody opened the check: `conveyor-labels`
# archived on its branch, #204 stayed open, and the only trace was a red X.
it "ci.yml posts the guard's own failure text as a marker-deduped pull request comment, and still fails the check"
assert_contains "$job" 'pull-requests: write'
assert_contains "$job" 'marker="<!-- pr-closes-guard -->"'
assert_contains "$job" 'gh pr comment "$PR_NUMBER"'
assert_contains "$job" 'grep -qF "$marker"'
assert_contains "$job" 'exit 1'

rm -rf "$repo" "$tmp"
summary
