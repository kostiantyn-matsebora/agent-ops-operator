#!/usr/bin/env bash
# The tracking issue: opened once, advanced, closed.
#
# THE POINTER RULE IS THE THING UNDER TEST. An issue that restated the proposal
# would be a second source of truth, and nothing would ever tell a reader which
# of the two they were reading.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)

repo=$(make_repo)
cp "$ROOT/.github/scripts/opsx-issue.sh" "$repo/.github/scripts/"
chmod +x "$repo/.github/scripts/opsx-issue.sh"
make_change "$repo" demo-change "Retries keep the first failure"
cat >> "$repo/openspec/changes/demo-change/proposal.md" <<'MD'

A paragraph of rationale that MUST NOT be copied into the issue, because a
second copy is a second thing to keep true.
MD

tmp=$(mktemp -d); stub_gh "$tmp/bin"
export GH_CALLS="$tmp/calls"
S() { (cd "$repo" && GH_CALLS="$GH_CALLS" .github/scripts/opsx-issue.sh "$@" 2>&1); }

it "opens an issue and returns its number"
: > "$GH_CALLS"
assert_equals "101" "$(S open demo-change)"

it "writes the binding as a bare number, and nothing else"
assert_equals "101" "$(cat "$repo/openspec/changes/demo-change/.github-issue")"

it "titles it with the change name and the proposal's first sentence"
assert_contains "$(cat "$GH_CALLS")" "demo-change: Retries keep the first failure"

# THE FIRST SENTENCE, NOT THE FIRST LINE. The prose is hard-wrapped, so a
# line-scoped read cuts a title mid-clause — `...serviceaccounts that nothing`
# on a real proposal, which reads as a bug in the tool rather than as a title.
it "joins the wrapped paragraph rather than cutting at the line break"
assert_contains "$(cat "$GH_CALLS")" "demo-change: Retries keep the first failure rather than the last one"

it "stops at the first sentence and takes no more of the Why"
assert_not_contains "$(cat "$GH_CALLS")" "must never reach the title"

# WHERE TO READ IT, AS A LINK. A path is not a pointer to anybody who has not
# cloned this repository. With no `gh repo view` available the row says so
# rather than emitting a broken link.
it "carries a row for reading the change"
assert_contains "$(cat "$GH_CALLS")" "**Read it**"

# A BOUND IS NOT A SLICE. Cutting at a fixed offset ends a title inside a word —
# `...serviceaccounts that noth...` on a real proposal in this tree — which is
# the same defect at the other end of the same function.
# NOT truncating $GH_CALLS: the assertions below this point read the whole
# recording, and clearing it here would take demo-change's calls with it.
make_change "$repo" long-change \
  "The chart grants the manager permissions on serviceaccounts that nothing calls"
S open long-change >/dev/null
it "trims a long title back to a word, not to an offset"
assert_contains "$(cat "$GH_CALLS")" "serviceaccounts that..."

it "never ends a title inside a word"
assert_not_contains "$(cat "$GH_CALLS")" "that noth..."

# THE PHASE COMMAND IS WHAT REGENERATES THE BODY, so the link it writes only
# becomes correct if the flow actually calls it. It did not: apply.md and
# archive.md advanced the label with raw `gh issue edit`, which leaves the body
# — and its link to a branch that does not exist yet — untouched for the life of
# the issue. Dead code in a script nobody calls is invisible; this is the check
# that makes it visible.
# Matched as a COMMAND — start of line, inside a fenced block — not as a
# mention: both files now name `gh issue edit` in prose, to say not to use it.
it "is what the opsx commands call, rather than raw gh issue edit"
for f in "$ROOT/.claude/commands/opsx/apply.md" "$ROOT/.claude/commands/opsx/archive.md"; do
  assert_equals "0" "$(grep -cE '^[[:space:]]*gh issue (edit|comment|close)' "$f" || true)"
  assert_contains "$(cat "$f")" "opsx-issue.sh phase"
done

it "labels it as proposed"
assert_contains "$(cat "$GH_CALLS")" "opsx:proposed"

it "points at the change rather than copying it"
assert_contains "$(cat "$GH_CALLS")" "openspec/changes/demo-change/"

it "does NOT copy the proposal's prose into the issue"
assert_not_contains "$(cat "$GH_CALLS")" "second thing to keep true"

it "is idempotent: a second open returns the same number"
: > "$GH_CALLS"
assert_equals "101" "$(S open demo-change)"

it "and opens no second issue"
assert_not_contains "$(cat "$GH_CALLS")" "issue create"

it "reads the binding back"
assert_equals "101" "$(S number demo-change)"

it "advances the phase, removing the labels it is leaving"
: > "$GH_CALLS"; S phase demo-change review >/dev/null
calls=$(cat "$GH_CALLS")
assert_contains "$calls" "--add-label opsx:review"

it "removes the phase it came from"
assert_contains "$calls" "--remove-label opsx:proposed"

it "comments once per transition, not per task"
assert_equals "1" "$(printf '%s\n' "$calls" | grep -c 'issue comment')"

it "refuses a phase that is not one of the four"
S phase demo-change halfway >/dev/null 2>&1
assert_status 64 "$?"

it "closes the issue"
: > "$GH_CALLS"; S close demo-change >/dev/null
assert_contains "$(cat "$GH_CALLS")" "issue close 101"

it "PROMOTES a filed issue instead of opening one beside it"
make_change "$repo" promoted-change "Something a stranger reported"
: > "$GH_CALLS"
assert_equals "77" "$(S open promoted-change --promote 77)"

it "opens no issue of its own when promoting"
assert_not_contains "$(cat "$GH_CALLS")" "issue create"

it "binds the promoted issue's number"
assert_equals "77" "$(cat "$repo/openspec/changes/promoted-change/.github-issue")"

it "refuses a change that does not exist"
S open no-such-change >/dev/null 2>&1
assert_status 1 "$?"

# NO ARCHIVE DIRECTORY AT ALL is the state of a repository that has never
# archived anything, and `find` exits 1 on a missing path. Under `pipefail` that
# became the assignment's status and `errexit` aborted the lookup before its own
# fallback — the same shape that killed ci.yml's openspec gate. Exercised
# through a BARE substitution, because every caller in the script embeds it in a
# larger word and that masks the abort as an empty string.
it "does not abort when there is no archive directory at all"
rm -rf "$repo/openspec/changes/archive"
out=$(cd "$repo" && bash -c '
  set -euo pipefail
  . .github/scripts/opsx-issue.sh number demo-change >/dev/null
  change_dir no-such-change >/dev/null
  echo RETURNED' 2>/dev/null || true)
assert_equals "RETURNED" "$out"

# THE ARCHIVE PATH IS WHERE `close` ACTUALLY RUNS. `openspec archive` moves the
# change — sidecar and all — before the command that closes its issue is called,
# and resolving only the live path made that command fail at exactly that point.
it "still finds the binding after the change is archived"
mkdir -p "$repo/openspec/changes/archive"
mv "$repo/openspec/changes/demo-change" \
   "$repo/openspec/changes/archive/2026-08-25-demo-change"
assert_equals "101" "$(S number demo-change)"

it "closes an archived change's issue"
: > "$GH_CALLS"; S close demo-change >/dev/null
assert_contains "$(cat "$GH_CALLS")" "issue close 101"

it "advances an archived change's phase label"
: > "$GH_CALLS"; S phase demo-change archived >/dev/null
assert_contains "$(cat "$GH_CALLS")" "opsx:archived"

# A BARE SUFFIX MATCH WOULD HAND ONE CHANGE ANOTHER'S ISSUE. `*-auth` matches
# `2026-01-01-oauth`, so the date prefix is part of the pattern.
it "does not mistake a longer archived name for this one"
mkdir -p "$repo/openspec/changes/archive/2026-08-25-not-demo-change"
echo 999 > "$repo/openspec/changes/archive/2026-08-25-not-demo-change/.github-issue"
assert_equals "101" "$(S number demo-change)"

rm -rf "$repo" "$tmp"
summary
