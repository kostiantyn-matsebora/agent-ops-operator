#!/usr/bin/env bash
# pinned-runtime-guard.py asserts every chart-pinned runtime image corresponds
# to a git tag whose tree carries the runtime's report module. It exists
# because chart 15.0.0 pinned runtime-claude 0.9.0 for three weeks after the
# work-report contract landed, and every code test stayed green while no
# install showed run statistics. Driven here against throwaway repositories —
# never the real one — covering the pass, each failure class, and the exit
# codes CI acts on.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
GUARD="$ROOT/.github/scripts/pinned-runtime-guard.py"

# A fixture repository with the three runtime bundles' values files and the
# three report modules, tagged per component. Arguments name the versions to
# pin; the tags created always say 1.0.0.
make_runtime_repo() {  # make_runtime_repo <claude-pin> <copilot-pin> <ollama-pin>
  local dir; dir=$(mktemp -d)
  git -C "$dir" init -q -b master
  git -C "$dir" config user.email test@example.com
  git -C "$dir" config user.name "Test"
  mkdir -p "$dir/chart/charts/claude" "$dir/chart/charts/copilot" "$dir/chart/charts/ollama" \
           "$dir/runtimes/claude" "$dir/runtimes/copilot" "$dir/runtimes/ollama"
  echo "image: ghcr.io/example/agentops-runtime-claude:$1" > "$dir/chart/charts/claude/values.yaml"
  echo "image: ghcr.io/example/agentops-runtime-copilot:$2" > "$dir/chart/charts/copilot/values.yaml"
  echo "image: ghcr.io/example/agentops-runtime-ollama:$3" > "$dir/chart/charts/ollama/values.yaml"
  echo report > "$dir/runtimes/claude/report.js"
  echo report > "$dir/runtimes/copilot/report.js"
  echo report > "$dir/runtimes/ollama/report.go"
  git -C "$dir" add -A
  git -C "$dir" commit -qm "with report modules"
  git -C "$dir" tag runtime-claude-v1.0.0
  git -C "$dir" tag runtime-copilot-v1.0.0
  git -C "$dir" tag runtime-ollama-v1.0.0
  echo "$dir"
}

echo "  pinned-runtime-guard"

it "passes when every pin's tag exists and carries the report module"
repo=$(make_runtime_repo 1.0.0 1.0.0 1.0.0)
out=$(python3 "$GUARD" "$repo" 2>&1); status=$?
assert_status 0 "$status"
it "...and says so"
assert_contains "$out" "every pinned runtime carries the work-report contract"

it "fails when a pin names a tag that does not exist"
repo=$(make_runtime_repo 2.0.0 1.0.0 1.0.0)
out=$(python3 "$GUARD" "$repo" 2>&1); status=$?
assert_status 1 "$status"
it "...naming the missing tag and the order to follow"
assert_contains "$out" "runtime-claude-v2.0.0 does not exist — push the release tag first"

it "fails when the pinned tag's tree lacks the report module"
repo=$(make_runtime_repo 1.0.0 1.0.0 1.0.0)
git -C "$repo" rm -q runtimes/copilot/report.js
git -C "$repo" commit -qm "drop the report module"
git -C "$repo" tag runtime-copilot-v3.0.0
sed -i 's/:1.0.0/:3.0.0/' "$repo/chart/charts/copilot/values.yaml"
out=$(python3 "$GUARD" "$repo" 2>&1); status=$?
assert_status 1 "$status"
it "...naming the contract file the tag predates"
assert_contains "$out" "runtime-copilot-v3.0.0 lacks runtimes/copilot/report.js"

it "fails on a dev or branch pin — a chart pins release versions only"
repo=$(make_runtime_repo 1.0.0 dev-abc123 1.0.0)
out=$(python3 "$GUARD" "$repo" 2>&1); status=$?
assert_status 1 "$status"
it "...naming the rule"
assert_contains "$out" "a chart pins release versions only"

it "fails CLOSED on a missing values file"
repo=$(make_runtime_repo 1.0.0 1.0.0 1.0.0)
rm "$repo/chart/charts/ollama/values.yaml"
out=$(python3 "$GUARD" "$repo" 2>&1); status=$?
assert_status 1 "$status"
it "...reporting it unreadable"
assert_contains "$out" "chart/charts/ollama/values.yaml: unreadable"

it "fails when the image does not name the component at all"
repo=$(make_runtime_repo 1.0.0 1.0.0 1.0.0)
echo "image: ghcr.io/example/something-else:1.0.0" > "$repo/chart/charts/claude/values.yaml"
out=$(python3 "$GUARD" "$repo" 2>&1); status=$?
assert_status 1 "$status"
it "...naming the expected image"
assert_contains "$out" "does not name agentops-runtime-claude"

summary
