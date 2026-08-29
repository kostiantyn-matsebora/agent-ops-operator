#!/usr/bin/env bash
# The program that turns the analysis service's open issues into work-list items.
#
# WHAT IS ASSERTED IS THE KEYING AND THE FLAG. A project key spelled differently
# from the one the scan action submits under reads a project that does not
# exist and reports it clean; an analysis of an OLDER commit reported as the
# head's would let a round declare the service satisfied on code it never saw.
# `curl` is stubbed, answering from fixtures by URL, so the suite touches no
# network and no token.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/sonar-issues.py"

tmp=$(mktemp -d)
export CURL_CALLS="$tmp/calls" FIXTURES="$tmp/fixtures"
mkdir -p "$tmp/bin" "$FIXTURES"

# A curl that records the URL it was asked and answers from a fixture chosen
# by the path and the project key.
cat > "$tmp/bin/curl" <<'STUB'
#!/usr/bin/env bash
url="${@: -1}"
printf '%s\n' "$*" >> "$CURL_CALLS"
case "$url" in
  *"/api/project_pull_requests/list?project="*)
    key=${url##*project=}
    f="$FIXTURES/prs-$key.json"; [ -f "$f" ] && cat "$f" || echo '{"pullRequests":[]}' ;;
  *"/api/issues/search?"*)
    key=$(printf '%s' "$url" | sed -n 's/.*componentKeys=\([^&]*\).*/\1/p')
    page=$(printf '%s' "$url" | sed -n 's/.*&p=\([0-9]*\).*/\1/p')
    f="$FIXTURES/issues-$key-p${page:-1}.json"; [ -f "$f" ] && cat "$f" || echo '{"total":0,"issues":[]}' ;;
  *) exit 22 ;;
esac
STUB
chmod +x "$tmp/bin/curl"; export PATH="$tmp/bin:$PATH"

# THE COMPONENT LIST, in the shape components.sh prints: the collector reads it
# rather than restating it.
cat > "$tmp/components.json" <<'JSON'
[{"component":"manager","context":"./platform/manager"},
 {"component":"signal-cron","context":"./signals/cron"},
 {"component":"console","context":"./platform/console"}]
JSON

# manager: analysed at the head. signal-cron: analysed at an OLDER commit.
# console: never analysed for this pull request.
cat > "$FIXTURES/prs-org_agent-ops-operator_manager.json" <<'JSON'
{"pullRequests":[{"key":"7","branch":"change/x","base":"master","status":{"qualityGateStatus":"ERROR"},
                  "analysisDate":"2026-08-29T10:00:00+0000","commit":{"sha":"abc1234abc1234"}}]}
JSON
cat > "$FIXTURES/prs-org_agent-ops-operator_signal-cron.json" <<'JSON'
{"pullRequests":[{"key":"7","branch":"change/x","base":"master","status":{"qualityGateStatus":"OK"},
                  "analysisDate":"2026-08-28T10:00:00+0000","commit":{"sha":"0ld0ld0ld0ld"}}]}
JSON
cat > "$FIXTURES/issues-org_agent-ops-operator_manager-p1.json" <<'JSON'
{"total":2,"issues":[
  {"key":"AZ1","rule":"go:S1234","severity":"MAJOR","type":"CODE_SMELL","component":"org_agent-ops-operator_manager:internal/x.go","line":12,"message":"Remove this unused variable."},
  {"key":"AZ2","rule":"go:S9999","severity":"MINOR","type":"BUG","component":"org_agent-ops-operator_manager:cmd/main.go","line":3,"message":"Handle the error."}]}
JSON
# signal-cron has issues too, but its analysis is stale: they must NOT be read.
cat > "$FIXTURES/issues-org_agent-ops-operator_signal-cron-p1.json" <<'JSON'
{"total":1,"issues":[{"key":"STALE","rule":"go:S1","severity":"MAJOR","type":"BUG","component":"org_agent-ops-operator_signal-cron:main.go","line":1,"message":"old"}]}
JSON

run() { : > "$CURL_CALLS"; rm -f "$tmp/out.json"
        SONAR_TOKEN=t python3 "$S" --organization org --pr 7 --head abc1234abc1234deadbeef \
          --components "$tmp/components.json" --api http://sonar.test --out "$tmp/out.json" "$@" 2>&1; }
field() { python3 -c 'import json,sys;d=json.load(open(sys.argv[1]));print(eval(sys.argv[2]))' "$tmp/out.json" "$1"; }

out=$(run); rc=$?

it "derives the project key exactly as the scan action does: <org>_agent-ops-operator_<component>"
assert_status 0 "$rc"
assert_contains "$(cat "$CURL_CALLS")" "project=org_agent-ops-operator_manager"
assert_contains "$(cat "$CURL_CALLS")" "componentKeys=org_agent-ops-operator_manager&pullRequest=7&resolved=false"

it "keys each issue sonar:<key> with source, rule, file and line — the file anchored at the component's directory"
assert_equals "sonar:AZ1 sonar:AZ2" "$(field '" ".join(i["id"] for i in d["issues"])')"
assert_equals "sonar" "$(field 'd["issues"][0]["source"]')"
assert_equals "go:S1234" "$(field 'd["issues"][0]["rule"]')"
assert_equals "platform/manager/internal/x.go" "$(field 'd["issues"][0]["path"]')"
assert_equals "12" "$(field 'd["issues"][0]["line"]')"

it "reads issues only from a project analysed at the head sha"
assert_not_contains "$(cat "$tmp/out.json")" "STALE"
assert_not_contains "$(cat "$CURL_CALLS")" "componentKeys=org_agent-ops-operator_signal-cron"

it "names the stale project, and the one never analysed, per project"
assert_equals "analysed stale absent" "$(field '" ".join(p["status"] for p in d["projects"])')"
assert_equals "signal-cron" "$(field '" ".join(d["stale"])')"
assert_contains "$out" "stale    signal-cron — analysed at 0ld0ld0, head is abc1234"

it "reports the analysis as consulted when at least one project is analysed at the head"
assert_equals "True" "$(field 'd["consulted"]')"
assert_contains "$out" "analysis consulted: 2 open issue(s)"

it "carries the quality gate's verdict per project, without acting on it"
assert_equals "ERROR" "$(field 'd["projects"][0]["qualityGate"]')"

# THE FLAG, NOT AN EMPTY LIST: no project analysed at this head.
it "reports NOT consulted when nothing is analysed at the head, with an empty list and the flag"
out=$(run --head fffffff); rc=$?
assert_status 0 "$rc"
assert_equals "False" "$(field 'd["consulted"]')"
assert_equals "0" "$(field 'len(d["issues"])')"
assert_contains "$out" "analysis NOT consulted"

it "asks nothing of the service without a token, and says so"
out=$(: > "$CURL_CALLS"; SONAR_TOKEN= python3 "$S" --organization org --pr 7 --head abc --components "$tmp/components.json" --out "$tmp/out.json" 2>&1)
assert_equals "" "$(cat "$CURL_CALLS")"
assert_contains "$out" "SONAR_TOKEN is not set"
assert_equals "False" "$(field 'd["consulted"]')"

it "an unreachable service is an error for that project, never a clean project"
cat > "$FIXTURES/prs-org_agent-ops-operator_console.json" <<'JSON'
not json
JSON
out=$(run)
assert_equals "error" "$(field 'd["projects"][2]["status"]')"
rm "$FIXTURES/prs-org_agent-ops-operator_console.json"

it "pages through a project's issues"
cat > "$FIXTURES/issues-org_agent-ops-operator_manager-p1.json" <<'JSON'
{"total":501,"issues":[{"key":"P1","rule":"r","severity":"MAJOR","type":"BUG","component":"org_agent-ops-operator_manager:a.go","line":1,"message":"one"}]}
JSON
cat > "$FIXTURES/issues-org_agent-ops-operator_manager-p2.json" <<'JSON'
{"total":501,"issues":[{"key":"P2","rule":"r","severity":"MAJOR","type":"BUG","component":"org_agent-ops-operator_manager:b.go","line":1,"message":"two"}]}
JSON
out=$(run)
assert_equals "sonar:P1 sonar:P2" "$(field '" ".join(i["id"] for i in d["issues"])')"

# THE TOKEN THAT LISTS CAN ALSO MARK. This program must never carry the call.
it "exposes no call that changes an issue's state in the service"
assert_not_contains "$(cat "$S")" "issues/do_transition"
assert_not_contains "$(cat "$S")" "issues/set_"
assert_not_contains "$(cat "$S")" "issues/bulk_change"
assert_not_contains "$(cat "$S")" "-X POST"

rm -rf "$tmp"
summary
