#!/usr/bin/env bash
# The program that decides which rules a file reader reads. A reader inherits
# none, so a path the table gets wrong is a rule the review does not apply —
# and a rule the table forgets is one the review has stopped enforcing.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/review-rules.py"

r() { python3 "$S" "$@" | tr '\n' ' '; }

it "a manager file gets the doctrine: invariants, terminology, wiring, adapters — and no map, no lore"
out=$(r platform/manager/internal/httpapi/signals.go)
for f in invariants terminology wiring adapters; do assert_contains "$out" ".claude/rules/$f.md"; done
assert_not_contains "$out" "structure.md"
assert_not_contains "$out" "gotchas.md"

it "the repository map goes only to what derives the tree"
assert_contains "$(r .github/components.sh)" ".claude/rules/structure.md"
assert_contains "$(r signals/cron/Dockerfile)" ".claude/rules/structure.md"
assert_contains "$(r runtimes/ollama/go.mod)" ".claude/rules/structure.md"
assert_not_contains "$(r README.md)" "structure.md"

it "an ingest file adds the signal rules"
assert_contains "$(r platform/manager/internal/ingest/group.go)" ".claude/rules/signal-rules.md"
assert_not_contains "$(r platform/manager/internal/httpapi/signals.go)" "signal-rules"

it "a chart file gets the chart rule, and wiring"
out=$(r chart/values.yaml)
assert_contains "$out" ".claude/rules/chart.md"
assert_contains "$out" ".claude/rules/wiring.md"

it "a docs page gets documentation and the site's own context"
out=$(r docs/concepts.md)
assert_contains "$out" ".claude/rules/documentation.md"
assert_contains "$out" "docs/CLAUDE.md"

it "the theme gets the palette rule beside the console's"
assert_contains "$(r platform/console/ui/src/theme/tokens.ts)" ".claude/rules/palette-and-mark.md"
assert_contains "$(r platform/console/ui/src/theme/tokens.ts)" ".claude/rules/terminology.md"

it "every path a scoped rule's own frontmatter names routes to that rule"
for f in palette-and-mark signal-rules chart; do
  for pat in $(sed -n '2,/^---$/p' "$ROOT/.claude/rules/$f.md" | grep -oE '"[^"]+"' | tr -d '"'); do
    sample=$(printf '%s' "$pat" | sed 's#/\*\*#/x/y.go#; s#\*\*#x/y.go#')
    assert_contains "$(r "$sample")" ".claude/rules/$f.md"
  done
done

it "a root file gets the authoring rules"
assert_contains "$(r README.md)" ".claude/rules/authoring.md"

it "every path gets retired-vocabulary, and none gets a session rule"
for p in README.md chart/x.yaml docs/a.md platform/manager/a.go signals/cron/a.go .github/workflows/ci.yml; do
  out=$(r "$p")
  assert_contains "$out" ".claude/rules/retired-vocabulary.md"
  for f in build-test worktree-delivery session-naming publication visual-check answering gotchas; do
    assert_not_contains "$out" "$f.md"
  done
done

it "several paths are the union, each file once"
out=$(r platform/manager/a.go signals/cron/a.go)
assert_equals "1" "$(printf '%s' "$out" | tr ' ' '\n' | grep -c 'invariants.md')"
assert_contains "$out" "signal-rules.md"

it "--check passes on the real tree: every routed file exists and every rule is reachable"
python3 "$S" --check >/dev/null 2>&1
assert_status 0 "$?"

it "--check fails naming a rule no path routes to"
tmp=$(mktemp -d); mkdir -p "$tmp/.claude/rules" "$tmp/docs"
cp "$ROOT"/.claude/rules/*.md "$tmp/.claude/rules/"; cp "$ROOT/docs/CLAUDE.md" "$tmp/docs/"
echo '## Orphan' > "$tmp/.claude/rules/orphan.md"
err=$(python3 "$S" --check --root "$tmp" 2>&1 >/dev/null)
assert_status 1 "$?"
assert_contains "$err" "no path routes to .claude/rules/orphan.md"
rm -rf "$tmp"

summary
