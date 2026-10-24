#!/usr/bin/env bash
# The build gate: derives the recipe per group from a fake tree and runs it,
# writing the unbuilt reading on failure. No network, no real repository.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/review-build.sh"

tmp=$(mktemp -d)

it "a directory with no recipe prints no build and exits 0, built=true"
mkdir -p "$tmp/docs"
out=$(GITHUB_OUTPUT="$tmp/gho" bash "$S" docs --root "$tmp" 2>&1); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "docs: no build"
assert_contains "$(cat "$tmp/gho")" "built=true"

it "a go module that builds is reported built, built=true"
mkdir -p "$tmp/goodgo"
echo "module good" > "$tmp/goodgo/go.mod"
printf 'package main\nfunc main(){}\n' > "$tmp/goodgo/main.go"
: > "$tmp/gho2"
out=$(GITHUB_OUTPUT="$tmp/gho2" bash "$S" goodgo --root "$tmp" 2>&1); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "goodgo: built"
assert_contains "$(cat "$tmp/gho2")" "built=true"

it "a go module with a syntax error yields built=false and a reading naming the component"
mkdir -p "$tmp/badgo"
echo "module bad" > "$tmp/badgo/go.mod"
echo "not go code {{{" > "$tmp/badgo/main.go"
: > "$tmp/gho3"
GITHUB_OUTPUT="$tmp/gho3" bash "$S" badgo --out "$tmp/reading.json" --paths "badgo/main.go" --root "$tmp" >/dev/null 2>"$tmp/err"; rc=$?
assert_status 0 "$rc"
assert_contains "$(cat "$tmp/gho3")" "built=false"
assert_contains "$(cat "$tmp/err")" "badgo: build failed"
r=$(cat "$tmp/reading.json")
assert_contains "$r" '"component": "badgo"'
assert_contains "$r" '"unbuilt"'
assert_contains "$r" '"unread": [
  "badgo/main.go"
 ]'
assert_contains "$r" '"findings": []'

it "chart is built with helm lint and template"
mkdir -p "$tmp/chart/templates"
cat > "$tmp/chart/Chart.yaml" <<'EOF'
apiVersion: v2
name: fake
version: 0.1.0
EOF
: > "$tmp/gho4"
out=$(GITHUB_OUTPUT="$tmp/gho4" bash "$S" chart --root "$tmp" 2>&1); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "chart: built (helm lint"
assert_contains "$(cat "$tmp/gho4")" "built=true"

it "a node runtime is built with node --test"
mkdir -p "$tmp/runtimes/claude"
cat > "$tmp/runtimes/claude/x.test.js" <<'EOF'
const { test } = require('node:test');
test('ok', () => {});
EOF
: > "$tmp/gho5"
out=$(GITHUB_OUTPUT="$tmp/gho5" bash "$S" runtimes/claude --root "$tmp" 2>&1); rc=$?
assert_status 0 "$rc"
assert_contains "$out" "runtimes/claude: built (node --test)"
assert_contains "$(cat "$tmp/gho5")" "built=true"

rm -rf "$tmp"
summary
