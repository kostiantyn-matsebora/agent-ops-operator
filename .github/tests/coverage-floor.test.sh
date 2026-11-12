#!/usr/bin/env bash
# coverage-floor.sh holds a statement-coverage floor on one package, read from
# a Go profile. It exists because the first version, inline in ci.yml, counted
# every line of the profile. Under -coverpkg=./... each test binary writes
# EVERY block, hit only by the binary that exercises it, so a package covered at
# 96% read 7% in CI and failed a PR whose code was fine. Driven here against
# fixture profiles shaped the way -coverpkg writes them.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
FLOOR="$ROOT/.github/scripts/coverage-floor.sh"

PKG='internal/state/nats/'
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT

# Three test binaries, each listing the same two nats blocks (10 + 10
# statements). Only the nats binary hits them.
cat > "$dir/dup.out" <<'P'
mode: set
m/internal/state/nats/a.go:1.1,5.2 10 1
m/internal/state/nats/b.go:1.1,5.2 10 1
m/internal/state/nats/a.go:1.1,5.2 10 0
m/internal/state/nats/b.go:1.1,5.2 10 0
m/internal/state/nats/a.go:1.1,5.2 10 0
m/internal/state/nats/b.go:1.1,5.2 10 0
m/internal/other/c.go:1.1,5.2 500 0
P

it "counts a block hit by one binary as covered, however many binaries list it"
out=$(bash "$FLOOR" "$dir/dup.out" "$PKG" 90 2>&1); status=$?
assert_status 0 "$status"
it "...reporting the deduplicated percentage"
assert_contains "$out" "100.0%"

cat > "$dir/miss.out" <<'P'
mode: set
m/internal/state/nats/a.go:1.1,5.2 10 1
m/internal/state/nats/b.go:1.1,5.2 90 0
m/internal/state/nats/a.go:1.1,5.2 10 0
m/internal/state/nats/b.go:1.1,5.2 90 0
P

it "fails when blocks are genuinely never hit"
out=$(bash "$FLOOR" "$dir/miss.out" "$PKG" 90 2>&1); status=$?
assert_status 1 "$status"
it "...naming the floor"
assert_contains "$out" "below the 90% floor"

it "fails when the package is absent from the profile"
out=$(bash "$FLOOR" "$dir/dup.out" "internal/nowhere/" 90 2>&1); status=$?
assert_status 1 "$status"

it "ignores packages other than the one named"
out=$(bash "$FLOOR" "$dir/dup.out" "internal/other/" 90 2>&1); status=$?
assert_status 1 "$status"

cat > "$dir/iso.out" <<'P'
mode: set
m/internal/state/nats/a.go:1.1,5.2 10 0
m/internal/other/c.go:1.1,5.2 500 1
P

it "passes a package that is covered, whatever the other packages hold"
out=$(bash "$FLOOR" "$dir/iso.out" "internal/other/" 90 2>&1); status=$?
assert_status 0 "$status"

it "reports an unreadable profile as unreadable, never as a missing package"
out=$(bash "$FLOOR" "$dir/does-not-exist.out" "$PKG" 90 2>&1); status=$?
assert_status 1 "$status"
assert_not_contains "$out" "package not found"
assert_contains "$out" "could not read"

it "reports a fragment no block matches as a missing package"
out=$(bash "$FLOOR" "$dir/iso.out" "internal/nowhere/" 90 2>&1); status=$?
assert_status 1 "$status"
assert_contains "$out" "package not found"

it "matches the fragment from a path boundary, not as a substring"
out=$(bash "$FLOOR" "$dir/iso.out" "nal/other/" 90 2>&1); status=$?
assert_status 1 "$status"
assert_contains "$out" "package not found"

it "names its usage when given the wrong number of arguments"
out=$(bash "$FLOOR" "$dir/iso.out" 2>&1); status=$?
assert_status 1 "$status"
assert_contains "$out" "usage: coverage-floor.sh"

it "matches the fragment up to a path boundary on the right too"
out=$(bash "$FLOOR" "$dir/iso.out" "internal/oth" 90 2>&1); status=$?
assert_status 1 "$status"
assert_contains "$out" "package not found"

it "treats a fragment given without its trailing slash as the same package"
out=$(bash "$FLOOR" "$dir/iso.out" "internal/other" 90 2>&1); status=$?
assert_status 0 "$status"

summary
