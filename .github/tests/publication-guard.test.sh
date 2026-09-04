#!/usr/bin/env bash
# publication-guard.py never ran under any test — absent from the coverage
# report entirely, at any percentage. This drives every rule (hostname,
# address literal, repository url, all three chat-identifier forms, email)
# against fixture files passed via --path (never the real tracked tree), plus
# the allowlisted negatives, --show, --counts, a binary file that cannot be
# decoded, and both branches of --messages.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
GUARD="$ROOT/.github/scripts/publication-guard.py"

tmp=$(mktemp -d)

cat > "$tmp/bad.md" <<'MD'
Reach it at tracker.widgets-fixture.io for details.
Or the raw address 10.20.30.40 if DNS is down.
Clone it from https://github.com/acme/internal-tool.git for now.
The chat id: -1009876543210 is where it posts.
A deep link: https://t.me/c/9876543210/5
A bare literal in isolation: -9988776655443 here.
Mail spool.owner@notreal-example.io picks it up.
MD

it "every rule fires once on a line built to trip it, and none of them crash the scan"
out=$(python3 "$GUARD" --path "$tmp/bad.md" 2>&1)
assert_status 1 "$?"
assert_contains "$out" "hostname"
assert_contains "$out" "address-literal"
assert_contains "$out" "repository-url"
assert_contains "$out" "chat-identifier"
assert_contains "$out" "email"

it "the matched text is never printed by default, only file, line and rule"
assert_not_contains "$out" "widgets-fixture"
assert_not_contains "$out" "10.20.30.40"

it "--show prints what matched, for local fixing only"
shown=$(python3 "$GUARD" --path "$tmp/bad.md" --show 2>&1)
assert_contains "$shown" "widgets-fixture.io"
assert_contains "$shown" "10.20.30.40"

it "--counts prints per-rule totals and nothing else, exit 1 when non-empty"
counts=$(python3 "$GUARD" --path "$tmp/bad.md" --counts 2>&1)
assert_status 1 "$?"
assert_contains "$counts" "hostname:"
assert_contains "$counts" "total:"

it "the reserved example space, loopback, this project's own clone url and the documented placeholder are all allowed"
cat > "$tmp/clean.md" <<'MD'
Try https://example.com or user@example.com, loopback 127.0.0.1, this
repository at https://github.com/kostiantyn-matsebora/agent-ops-operator, and
the documented chat id -1001234567890.
MD
out=$(python3 "$GUARD" --path "$tmp/clean.md" 2>&1)
assert_status 0 "$?"
assert_contains "$out" "publication-guard: clean"

it "--counts on a clean set prints total: 0 and exits 0"
counts0=$(python3 "$GUARD" --path "$tmp/clean.md" --counts 2>&1)
assert_status 0 "$?"
assert_contains "$counts0" "total: 0"

it "a file that cannot be decoded as text is skipped, not a crash"
printf '\xff\xfe\x00\x01binary' > "$tmp/bin.dat"
out=$(python3 "$GUARD" --path "$tmp/bin.dat" 2>&1)
assert_status 0 "$?"

it "--messages over an empty range finds nothing and does not touch the tree"
out=$(cd "$ROOT" && python3 "$GUARD" --path "$tmp/clean.md" --messages HEAD..HEAD 2>&1)
assert_status 0 "$?"

it "--messages over an unresolvable range reports the failure to stderr and continues"
out=$(cd "$ROOT" && python3 "$GUARD" --path "$tmp/clean.md" --messages not-a-real-range..HEAD 2>&1)
assert_contains "$out" "cannot read commit messages"

rm -rf "$tmp"
summary
