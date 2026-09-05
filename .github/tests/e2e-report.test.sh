#!/usr/bin/env bash
# The program that turns an e2e pack's `go test -json` events into the two
# reports the workflow appends to the Actions run summary.
#
# WHAT IS ASSERTED IS THE LEVEL SPLIT AND THE ZERO-EVENTS FALLBACK. `summary`
# must stay short even when a test's full output is available — a full-tier
# report reading everything a smoke-tier one does would be the failure this
# change exists to avoid. And a build failure (no test2json event ever
# emitted) must render an explanation rather than crash the report step,
# since that step runs unconditionally.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/e2e-report.py"
tmp=$(mktemp -d)

cat > "$tmp/events.jsonl" <<'EOF'
{"Time":"2026-09-05T10:00:00Z","Action":"run","Test":"TestAlpha"}
{"Time":"2026-09-05T10:00:00.1Z","Action":"output","Test":"TestAlpha","Output":"=== RUN   TestAlpha\n"}
{"Time":"2026-09-05T10:00:05Z","Action":"pass","Test":"TestAlpha","Elapsed":5}
{"Time":"2026-09-05T10:00:05.1Z","Action":"run","Test":"TestBeta"}
{"Time":"2026-09-05T10:00:05.2Z","Action":"output","Test":"TestBeta","Output":"    beta_test.go:42: nonce mismatch: want abc got xyz\n"}
{"Time":"2026-09-05T10:00:07Z","Action":"fail","Test":"TestBeta","Elapsed":1.8}
{"Time":"2026-09-05T10:00:07.1Z","Action":"run","Test":"TestGamma"}
{"Time":"2026-09-05T10:00:07.2Z","Action":"skip","Test":"TestGamma","Elapsed":0}
{"Time":"2026-09-05T10:02:10Z","Action":"pass","Test":"","Elapsed":130}
EOF

it "summary level: counts, elapsed, and the failed/skipped names, WITHOUT per-test detail"
out=$(python3 "$S" --events "$tmp/events.jsonl" --level summary)
status=$?
assert_status 0 "$status"
assert_contains "$out" "1 passed, 1 failed, 1 skipped"
assert_contains "$out" "of 3 test(s)"
assert_contains "$out" "2m10s"
assert_contains "$out" '`TestBeta`'
assert_contains "$out" '`TestGamma`'
assert_not_contains "$out" "nonce mismatch"
assert_not_contains "$out" "| Test | Status | Elapsed |"

it "full level: every test's name, status and elapsed, plus a failure's captured output"
out=$(python3 "$S" --events "$tmp/events.jsonl" --level full)
assert_contains "$out" "| \`TestAlpha\` | PASS | 5s |"
assert_contains "$out" "| \`TestBeta\` | FAIL | 1.8s |"
assert_contains "$out" "| \`TestGamma\` | SKIP | 0s |"
assert_contains "$out" "nonce mismatch: want abc got xyz"

it "a package-level event (no Test field) is not counted as a test"
assert_not_contains "$out" '| `` |'

it "zero parseable events: says so, never crashes, exits 0"
: > "$tmp/empty.jsonl"
out=$(python3 "$S" --events "$tmp/empty.jsonl" --level summary)
status=$?
assert_status 0 "$status"
assert_contains "$out" "no test results were parsed"

it "a file that is not test2json JSON (a build failure) is treated the same as empty"
printf 'go: cannot find module providing package foo\n' > "$tmp/garbage.jsonl"
out=$(python3 "$S" --events "$tmp/garbage.jsonl" --level full)
status=$?
assert_status 0 "$status"
assert_contains "$out" "no test results were parsed"

it "a missing events file behaves like an empty one rather than erroring"
out=$(python3 "$S" --events "$tmp/does-not-exist.jsonl" --level summary)
status=$?
assert_status 0 "$status"
assert_contains "$out" "no test results were parsed"

rm -rf "$tmp"
summary
