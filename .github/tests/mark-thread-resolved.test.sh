#!/usr/bin/env bash
# The one thing that crosses the privilege boundary.
#
# It is three lines of shell, which is exactly why it is worth pinning: a
# malformed line here is REFUSED later by the resolver and reads as the guard
# working, when it was the caller that was wrong.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/mark-thread-resolved.sh"

tmp=$(mktemp -d); export RESOLVE_LIST="$tmp/list"

it "records a thread id"
bash "$S" PRRT_abc123 >/dev/null 2>&1
assert_equals "PRRT_abc123" "$(cat "$RESOLVE_LIST")"

it "appends rather than replacing"
bash "$S" PRRT_def456 >/dev/null 2>&1
assert_equals "2" "$(wc -l < "$RESOLVE_LIST" | tr -d ' ')"

it "records several in one call"
: > "$RESOLVE_LIST"; bash "$S" PRRT_a PRRT_b PRRT_c >/dev/null 2>&1
assert_equals "3" "$(wc -l < "$RESOLVE_LIST" | tr -d ' ')"

it "refuses an id with characters no node id has"
bash "$S" 'PRRT_x; rm -rf /' >/dev/null 2>&1
assert_status 65 "$?"

it "refuses being called with nothing"
bash "$S" >/dev/null 2>&1
assert_status 64 "$?"

it "writes nothing when it refuses"
: > "$RESOLVE_LIST"; bash "$S" 'bad id here' >/dev/null 2>&1
assert_equals "" "$(cat "$RESOLVE_LIST")"

rm -rf "$tmp"
summary
