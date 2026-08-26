#!/usr/bin/env bash
# The program that decides how many readers a review spawns, and what each sees.
#
# Grouping must agree with `components.sh`, so one case runs it for real
# against this repository; the rest use a fixed component list, because the
# shape being tested is the grouping and not the tree.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/review-queue.py"

tmp=$(mktemp -d)
printf '["signals/cron","platform/manager","platform/console"]' > "$tmp/components.json"

q() { python3 "$S" --components "$tmp/components.json" 2>/dev/null; }
groups() { python3 -c 'import json,sys;print(" ".join(g["group"]+":"+g["kind"]+":"+str(len(g["paths"])) for g in json.load(sys.stdin)))'; }

it "groups a component's file under the component"
assert_equals "signals/cron:component:1" "$(printf 'signals/cron/main.go\n' | q | groups)"

it "groups a nested path under its component, not its parent directory"
assert_equals "platform/manager:component:2" "$(printf 'platform/manager/internal/httpapi/a.go\nplatform/manager/api/v1alpha1/b.go\n' | q | groups)"

it "keeps two components apart"
assert_equals "platform/console:component:1 platform/manager:component:1" "$(printf 'platform/manager/x.go\nplatform/console/ui/y.ts\n' | q | groups)"

it "groups docs by top-level directory"
assert_equals "docs:directory:2" "$(printf 'docs/concepts.md\ndocs/guides/pipeline.md\n' | q | groups)"

it "a docs-only diff is exactly one entry"
assert_equals "1" "$(printf 'docs/a.md\ndocs/b.md\ndocs/_data/nav.yml\n' | q | python3 -c 'import json,sys;print(len(json.load(sys.stdin)))')"

it "groups a path in no component by its top-level directory"
assert_equals ".github:directory:1" "$(printf '.github/workflows/ci.yml\n' | q | groups)"

it "groups a root file under root"
assert_equals "root:directory:2" "$(printf 'README.md\nCLAUDE.md\n' | q | groups)"

it "ignores blank lines and a leading ./"
assert_equals "signals/cron:component:1" "$(printf '\n./signals/cron/x.go\n\n' | q | groups)"

it "takes paths as arguments too"
assert_equals "docs:directory:1 signals/cron:component:1" "$(python3 "$S" --components "$tmp/components.json" signals/cron/a.go docs/x.md 2>/dev/null | groups)"

it "carries the paths inside each group"
assert_contains "$(printf 'signals/cron/a.go\n' | q)" '"signals/cron/a.go"'

it "agrees with components.sh on the real tree"
out=$(printf 'signals/cron/main.go\nchart/values.yaml\n' | python3 "$S" --root "$ROOT" 2>/dev/null | groups)
assert_equals "chart:directory:1 signals/cron:component:1" "$out"

rm -rf "$tmp"
summary
