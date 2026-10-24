#!/usr/bin/env bash
# retired-vocabulary-guard.py never ran under any test: it was entirely
# absent from the coverage report, not merely thin. This drives the real
# scan against fixture files (via --path, so the real repository tree is
# never touched) using the project's OWN configured terms
# (.github/retired-vocabulary.json), and exercises: a live assertion of a
# retired name (fails), the same name recorded as removed (passes), the
# fenced-code exemption, the paragraph/window join, --show, --counts, and a
# clean tree.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
GUARD="$ROOT/.github/scripts/retired-vocabulary-guard.py"

tmp=$(mktemp -d)

it "a retired field asserted as a current claim is reported and fails"
cat > "$tmp/live.md" <<'MD'
A Channel or SignalSource sets spec.type to pick its adapter.
MD
out=$(python3 "$GUARD" --path "$tmp/live.md" 2>&1)
assert_status 1 "$?"
assert_contains "$out" "channel-spec-type"
assert_contains "$out" "spec.adapter"

it "the same name, recorded as removed in the neighbouring line, passes"
cat > "$tmp/record.md" <<'MD'
Channels used to carry spec.type.
It was removed once the adapter became the routing key.
MD
out=$(python3 "$GUARD" --path "$tmp/record.md" 2>&1)
assert_status 0 "$?"
assert_contains "$out" "clean (1 files)"

it "a fenced code block showing the retired field is exempt, even unrecorded"
cat > "$tmp/fenced.md" <<'MD'
See the migration example:

```yaml
spec.type: alertmanager
```

Nothing above this line mentions it again.
MD
out=$(python3 "$GUARD" --path "$tmp/fenced.md" 2>&1)
assert_status 0 "$?"

it "--show prints the offending line, --counts prints per-term totals only"
shown=$(python3 "$GUARD" --path "$tmp/live.md" --show 2>&1)
assert_contains "$shown" "spec.type to pick"
counts=$(python3 "$GUARD" --path "$tmp/live.md" --counts 2>&1)
assert_status 1 "$?"
assert_contains "$counts" "channel-spec-type: 1"
assert_contains "$counts" "total: 1"

it "--counts on a clean set exits 0 and prints total: 0"
counts0=$(python3 "$GUARD" --path "$tmp/record.md" --counts 2>&1)
assert_status 0 "$?"
assert_contains "$counts0" "total: 0"

it "several files scan together, and only the offending one is reported"
out=$(python3 "$GUARD" --path "$tmp/record.md" "$tmp/live.md" 2>&1)
assert_contains "$out" "$(basename "$tmp")/live.md"
assert_not_contains "$out" "$(basename "$tmp")/record.md:"

rm -rf "$tmp"
summary
