#!/usr/bin/env bash
# The branch-wide Blocker/High backlog collector.
#
# WHAT IS ASSERTED: the component list comes from components.sh (plus
# scripts, same pattern sonar-issues.py uses), no pullRequest param ever
# reaches the service (the branch-wide backlog, not one pull request's),
# counts are tallied per softwareQuality from impacts[], and a 0-result
# Clean Code query is re-asked on the legacy severity scale rather than
# trusted as a clean backlog. `curl` is stubbed, answering from fixtures by
# URL, so the suite touches no network and no token.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/sonar-findings-baseline.py"

tmp=$(mktemp -d)
export CURL_CALLS="$tmp/calls" FIXTURES="$tmp/fixtures"
mkdir -p "$tmp/bin" "$FIXTURES"

# A curl that records the URL it was asked and answers from a fixture chosen
# by the project key and which severity scale was queried.
cat > "$tmp/bin/curl" <<'STUB'
#!/usr/bin/env bash
url="${@: -1}"
printf '%s\n' "$*" >> "$CURL_CALLS"
case "$url" in
  *"/api/issues/search?"*)
    key=$(printf '%s' "$url" | sed -n 's/.*componentKeys=\([^&]*\).*/\1/p')
    page=$(printf '%s' "$url" | sed -n 's/.*&p=\([0-9]*\).*/\1/p')
    case "$url" in
      *impactSeverities*) scale=cc ;;
      *severities=*)      scale=legacy ;;
      *)                  scale=cc ;;
    esac
    f="$FIXTURES/issues-$key-$scale-p${page:-1}.json"; [ -f "$f" ] && cat "$f" || echo '{"total":0,"issues":[]}' ;;
  *) exit 22 ;;
esac
STUB
chmod +x "$tmp/bin/curl"; export PATH="$tmp/bin:$PATH"

# THE COMPONENT LIST, in the shape components.sh prints — the collector reads
# it rather than restating it, same fixture pattern sonar-issues.test.sh uses.
cat > "$tmp/components.json" <<'JSON'
[{"component":"manager"},{"component":"signal-cron"},{"component":"console"}]
JSON

# manager: two Clean Code Blocker/High issues, one reliability, one carrying
# BOTH security and maintainability impacts on the same issue.
cat > "$FIXTURES/issues-org_agent-ops-operator_manager-cc-p1.json" <<'JSON'
{"total":2,"issues":[
  {"key":"AZ1","impacts":[{"softwareQuality":"RELIABILITY","severity":"HIGH"}]},
  {"key":"AZ2","impacts":[{"softwareQuality":"SECURITY","severity":"BLOCKER"},{"softwareQuality":"MAINTAINABILITY","severity":"HIGH"}]}]}
JSON

# signal-cron: no Clean Code issues, but the legacy scale carries some — the
# taxonomy mismatch this script exists to catch.
cat > "$FIXTURES/issues-org_agent-ops-operator_signal-cron-cc-p1.json" <<'JSON'
{"total":0,"issues":[]}
JSON
cat > "$FIXTURES/issues-org_agent-ops-operator_signal-cron-legacy-p1.json" <<'JSON'
{"total":3,"issues":[{"key":"L1"},{"key":"L2"},{"key":"L3"}]}
JSON

# console: clean on both scales.
cat > "$FIXTURES/issues-org_agent-ops-operator_console-cc-p1.json" <<'JSON'
{"total":0,"issues":[]}
JSON
cat > "$FIXTURES/issues-org_agent-ops-operator_console-legacy-p1.json" <<'JSON'
{"total":0,"issues":[]}
JSON

# scripts: not a components.sh entry — appended the same way
# sonar-provision.sh appends it. Clean on both scales.
cat > "$FIXTURES/issues-org_agent-ops-operator_scripts-cc-p1.json" <<'JSON'
{"total":0,"issues":[]}
JSON
cat > "$FIXTURES/issues-org_agent-ops-operator_scripts-legacy-p1.json" <<'JSON'
{"total":0,"issues":[]}
JSON

run() {
  : > "$CURL_CALLS"; rm -f "$tmp/out.json"
  # cd'd into $tmp: --out and --components resolve under it, and
  # validated_path (pythonsecurity:S2083/S8707's own remediation) refuses
  # anything that doesn't -- see the dedicated test below for the refusal
  # itself.
  (cd "$tmp" && SONAR_TOKEN=t python3 "$S" --organization org --components components.json \
    --api http://sonar.test --out out.json "$@" 2>&1)
  return $?
}
field() {
  local expr="$1"
  python3 -c 'import json,sys;d=json.load(open(sys.argv[1]));print(eval(sys.argv[2]))' "$tmp/out.json" "$expr"
  return $?
}

out=$(run); rc=$?

it "runs and writes a JSON file"
assert_status 0 "$rc"

it "writes no organisation identifier to the output file -- the docstring's own guarantee"
assert_not_contains "$(cat "$tmp/out.json")" "organization"

it "derives the project key exactly as sonar-issues.py and sonar-provision.sh do"
assert_contains "$(cat "$CURL_CALLS")" "componentKeys=org_agent-ops-operator_manager"

it "never carries a pullRequest param -- the branch-wide backlog, not one pull request's"
assert_not_contains "$(cat "$CURL_CALLS")" "pullRequest"

it "carries every component components.sh lists, plus scripts -- not a component"
assert_equals "manager signal-cron console scripts" "$(field '" ".join(c["component"] for c in d["components"])')"

it "tallies Blocker/High counts per softwareQuality, from impacts[]"
assert_equals "1" "$(field 'd["components"][0]["counts"]["RELIABILITY"]')"
assert_equals "1" "$(field 'd["components"][0]["counts"]["SECURITY"]')"
assert_equals "1" "$(field 'd["components"][0]["counts"]["MAINTAINABILITY"]')"
assert_equals "2" "$(field 'd["components"][0]["total"]')"
assert_contains "$out" "manager"

it "a component with only Clean Code impacts reports cleanly, no mismatch"
assert_equals "False" "$(field 'd["components"][2]["taxonomyMismatch"]')"
console_line=$(printf '%s\n' "$out" | grep '^  console')
assert_not_contains "$console_line" "TAXONOMY"

it "a 0-result Clean Code query is re-asked on the legacy severity scale"
assert_contains "$(cat "$CURL_CALLS")" "componentKeys=org_agent-ops-operator_signal-cron"
assert_contains "$(cat "$CURL_CALLS")" "severities=BLOCKER%2CCRITICAL"

it "and reports BOTH counts when they disagree, rather than trusting the new one"
assert_equals "0" "$(field 'd["components"][1]["total"]')"
assert_equals "3" "$(field 'd["components"][1]["legacyCount"]')"
assert_equals "True" "$(field 'd["components"][1]["taxonomyMismatch"]')"
assert_contains "$out" "TAXONOMY MISMATCH"

it "refuses an --out path that resolves outside the working directory"
out=$(cd "$tmp" && SONAR_TOKEN=t python3 "$S" --organization org --components components.json \
  --api http://sonar.test --out ../../../../etc/passwd 2>&1); rc=$?
assert_status 1 "$rc"
assert_contains "$out" "outside the working directory"

it "asks nothing of the service without a token, and fails rather than reporting an empty baseline"
out=$(: > "$CURL_CALLS"; SONAR_TOKEN= python3 "$S" --organization org --components "$tmp/components.json" --out "$tmp/out.json" 2>&1); rc=$?
assert_status 1 "$rc"
assert_equals "" "$(cat "$CURL_CALLS")"
assert_contains "$out" "SONAR_TOKEN is not set"

it "exposes no call that changes an issue's state in the service"
assert_not_contains "$(cat "$S")" "issues/do_transition"
assert_not_contains "$(cat "$S")" "issues/set_"
assert_not_contains "$(cat "$S")" "issues/bulk_change"
assert_not_contains "$(cat "$S")" "-X POST"

rm -rf "$tmp"
summary
