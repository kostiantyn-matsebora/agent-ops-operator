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

it "titles it with the change name and the proposal's headline"
assert_contains "$(cat "$GH_CALLS")" "demo-change: Retries keep the first failure"

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

rm -rf "$repo" "$tmp"
summary
