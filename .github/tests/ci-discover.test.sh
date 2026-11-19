#!/usr/bin/env bash
# ci-discover.sh decides which components CI builds. Driven with changed-path
# lists; the inventory comes from the real repository.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
export ENVTEST_MODULE=platform/manager
dir=$(mktemp -d); trap 'rm -rf "$dir"' EXIT

run() { printf '%s\n' "$@" > "$dir/c"; bash "$ROOT/.github/scripts/ci-discover.sh" "$dir/c"; }

it "an openspec-only change builds no module and no image"
out=$(run openspec/changes/x/tasks.md openspec/specs/y/spec.md)
assert_contains "$out" "modules=[]"
it "...and runs no component job"
assert_contains "$out" "images=[]"
for j in operator chart docs site system scripts; do
  it "...$j stays off for openspec"; assert_contains "$out" "$j=
"; done
it "...the flags are empty, not true"
assert_not_contains "$out" "=true"

it "a change inside one signal adapter builds only that image"
out=$(run signals/cron/main.go)
assert_contains "$out" "signal-cron"
it "...and not another"
assert_not_contains "$out" "signal-alertmanager"

it "a docs-only change runs the docs and site jobs but no image"
out=$(run docs/concepts.md)
assert_contains "$out" "docs=true"
it "...with no image"
assert_contains "$out" "images=[]"

it "the shared Dockerfile rebuilds everything"
out=$(run .github/docker/go-module.Dockerfile)
assert_contains "$out" "operator=true"
it "...as does an unknown change set"
out=$(bash "$ROOT/.github/scripts/ci-discover.sh")
assert_contains "$out" "system=true"
it "...as does an empty one"
out=$(run)
assert_contains "$out" "chart=true"

summary
