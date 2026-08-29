#!/usr/bin/env bash
# The analysis organisation's quality gate is PROVISIONED, and this is what
# says the provisioning is idempotent.
#
# A GATE SET IN A DASHBOARD IS STATE NOBODY CAN READ FROM THE REPOSITORY. This
# script is the record of what every component's project is judged by, so the
# thing worth testing is not that it can create a gate once — it is that the
# second run creates NOTHING, and that a metric the gate already carries is
# updated in place instead of gaining a twin. A duplicate condition is how a
# threshold somebody deliberately relaxed gets tightened again by a re-run,
# silently.
#
# NO NETWORK. `curl` is stubbed from fixtures and `components.sh` is a stub of
# two components, so the suite reaches neither sonarcloud.io nor the real tree.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)

# --- a tree the script can be run from --------------------------------------
#
# `sonar-provision.sh` resolves the repository from its OWN path, so it is
# copied into a throwaway tree beside a `components.sh` that answers with two
# components. Fifteen real ones would make every assertion about call counts a
# statement about the current shape of this repository.
make_tree() {
  local dir; dir=$(mktemp -d)
  mkdir -p "$dir/.github/scripts"
  cp "$ROOT/.github/scripts/sonar-provision.sh" "$dir/.github/scripts/"
  cat > "$dir/.github/components.sh" <<'CS'
#!/usr/bin/env sh
echo '[{"component":"manager"},{"component":"console"}]'
CS
  chmod +x "$dir/.github/components.sh"
  echo "$dir"
}

# --- a `curl` that answers from fixtures ------------------------------------
#
# Honours the two shapes the script uses: `-sf ... | jq` for the monorepo call,
# and `-o <file> -w '%{http_code}'` for every quality-gate call. Records each
# invocation to $CURL_CALLS so a test can assert what WOULD have been sent.
stub_curl() {  # stub_curl <bindir>
  local bin="$1"; mkdir -p "$bin"
  cat > "$bin/curl" <<'STUB'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${CURL_CALLS:-/dev/null}"
out=""; url=""; want_code=""; prev=""
for a in "$@"; do
  [ "$prev" = "-o" ] && out="$a"
  [ "$prev" = "-w" ] && want_code=1
  case "$a" in https://*) url="$a" ;; esac
  prev="$a"
done
path=${url#*/api/}
# `-G` puts the data in the query string, but the stub reads the ARGS, so the
# url it is handed carries no query and the path needs no trimming.
fixture="${CURL_FIXTURES}/${path//\//_}.json"
# A path named in $CURL_403 answers 403 — the token lacking *Administer
# Quality Gates* is a case the script must report rather than crash on.
code=200
case "${CURL_403:-}" in "") ;; *) case "$path" in *${CURL_403}*) code=403 ;; esac ;; esac
body='{}'
[ -f "$fixture" ] && body=$(cat "$fixture")
[ "$code" = 403 ] && body='{"errors":[{"msg":"Insufficient privileges"}]}'
if [ -n "$out" ]; then printf '%s' "$body" > "$out"; else printf '%s' "$body"; fi
[ -n "$want_code" ] && printf '%s' "$code"
# -f is what the monorepo call uses: no body, nonzero exit, on an HTTP error.
if [ -z "$want_code" ] && [ "$code" != 200 ]; then exit 22; fi
exit 0
STUB
  chmod +x "$bin/curl"
  export PATH="$bin:$PATH"
}

# The built-in gate, as sonarcloud.io answers it (read 2026-08-29): six
# conditions, every one of them on NEW code.
SONAR_WAY='{"id":9,"name":"Sonar way","isBuiltIn":true,"conditions":[
  {"id":35,"metric":"new_security_rating","op":"GT","error":"1"},
  {"id":36,"metric":"new_reliability_rating","op":"GT","error":"1"},
  {"id":37,"metric":"new_maintainability_rating","op":"GT","error":"1"},
  {"id":38,"metric":"new_coverage","op":"LT","error":"80"},
  {"id":39,"metric":"new_duplicated_lines_density","op":"GT","error":"3"},
  {"id":40,"metric":"new_security_hotspots_reviewed","op":"LT","error":"100"}]}'

# fixtures <dir> <qualitygates/list body> <get_by_project body>
fixtures() {
  local d="$1"; mkdir -p "$d"
  printf '%s' "$SONAR_WAY"                                   > "$d/qualitygates_show.json"
  printf '%s' "$2"                                           > "$d/qualitygates_list.json"
  printf '%s' "$3"                                           > "$d/qualitygates_get_by_project.json"
  printf '%s' '{"id":777,"name":"agentops"}'                 > "$d/qualitygates_create.json"
  printf '%s' '{"projects":[{"projectKey":"o_agent-ops-operator_manager"},{"projectKey":"o_agent-ops-operator_console"}]}' \
    > "$d/alm_integration_provision_monorepo_projects.json"
}

# The agentops gate as the `list` answer carries it — the six built-in
# conditions copied across, with $1 spliced in for the coverage condition.
agentops_gate() {  # agentops_gate <isDefault> [extra conditions json]
  local extra="${2:-}"
  printf '{"qualitygates":[{"id":777,"name":"agentops","isDefault":%s,"isBuiltIn":false,"conditions":[
    {"id":101,"metric":"new_security_rating","op":"GT","error":"1"},
    {"id":102,"metric":"new_reliability_rating","op":"GT","error":"1"},
    {"id":103,"metric":"new_maintainability_rating","op":"GT","error":"1"},
    {"id":104,"metric":"new_coverage","op":"LT","error":"80"},
    {"id":105,"metric":"new_duplicated_lines_density","op":"GT","error":"3"},
    {"id":106,"metric":"new_security_hotspots_reviewed","op":"LT","error":"100"}%s]}]}' "$1" "$extra"
}

# run <fixtures-dir> — the script, against the stubs. Output and exit code.
OUT=""; RC=0
run() {
  local tree; tree=$(make_tree)
  CURL_CALLS="$CALLS" CURL_FIXTURES="$1" SONAR_TOKEN=t SONAR_ORG=o \
    sh "$tree/.github/scripts/sonar-provision.sh" > "$TMP/out" 2>&1 && RC=0 || RC=$?
  OUT=$(cat "$TMP/out")
}

TMP=$(mktemp -d)
BIN="$TMP/bin"
stub_curl "$BIN"

# --- an organisation with no gate at all ------------------------------------

CALLS="$TMP/calls-fresh"; : > "$CALLS"
F="$TMP/fx-fresh"; fixtures "$F" '{"qualitygates":[{"id":9,"name":"Sonar way","isDefault":true,"isBuiltIn":true,"conditions":[]}]}' '{"qualityGate":{"id":"9","name":"Sonar way","default":true}}'
run "$F"

it "creates the gate when the organisation has none"
assert_contains "$OUT" "gate created: agentops"

it "adds every condition of the built-in gate, copied rather than listed here"
for m in new_security_rating new_reliability_rating new_maintainability_rating \
         new_coverage new_duplicated_lines_density new_security_hotspots_reviewed; do
  assert_contains "$OUT" "condition added: $m"
done

it "adds the overall coverage condition this repository is here for"
assert_contains "$OUT" "condition added: coverage LT 80"

it "makes it the organisation default"
assert_contains "$OUT" "gate set as the organisation default"

it "assigns every component's project explicitly, not by leaving it to the default"
assert_contains "$OUT" "assigned: o_agent-ops-operator_manager"
assert_contains "$OUT" "assigned: o_agent-ops-operator_console"

it "succeeds"
assert_equals "0" "$RC"

# --- the gate exists with the six new-code conditions and no coverage one ----

CALLS="$TMP/calls-six"; : > "$CALLS"
F="$TMP/fx-six"; fixtures "$F" "$(agentops_gate true)" '{"qualityGate":{"id":"777","name":"agentops","default":true}}'
run "$F"

it "creates no second gate when one is already there"
assert_contains "$OUT" "gate present: agentops"
assert_not_contains "$OUT" "gate created"

it "adds ONLY the coverage condition, leaving the six it already carries"
assert_contains "$OUT" "condition added: coverage LT 80"
assert_equals "1" "$(printf '%s\n' "$OUT" | grep -c 'condition added')"

it "reports the six it left alone rather than saying nothing about them"
assert_equals "6" "$(printf '%s\n' "$OUT" | grep -c 'condition present')"

it "does not set a gate that is already the default"
assert_contains "$OUT" "gate is already the organisation default"

it "makes no set_as_default call at all"
assert_not_contains "$(cat "$CALLS")" "set_as_default"

# --- a gate that is already complete, default and assigned ------------------

CALLS="$TMP/calls-done"; : > "$CALLS"
F="$TMP/fx-done"
fixtures "$F" "$(agentops_gate true ',{"id":107,"metric":"coverage","op":"LT","error":"80"}')" \
  '{"qualityGate":{"id":"777","name":"agentops","default":false}}'
run "$F"

it "creates nothing on a second run"
assert_not_contains "$(cat "$CALLS")" "qualitygates/create"

it "adds no condition on a second run"
assert_not_contains "$OUT" "condition added"

it "updates no condition on a second run"
assert_not_contains "$OUT" "condition updated"

it "reassigns nothing on a second run"
assert_contains "$OUT" "assigned already: o_agent-ops-operator_manager"
assert_not_contains "$(cat "$CALLS")" "qualitygates/select"

it "still succeeds"
assert_equals "0" "$RC"

# --- a threshold that drifted ------------------------------------------------
#
# THE CASE THE WHOLE update_condition BRANCH EXISTS FOR. create_condition on a
# metric the gate already carries is ACCEPTED by the server, so without this
# the gate would end up with two coverage conditions and the stricter one would
# win a decision nobody made.

CALLS="$TMP/calls-drift"; : > "$CALLS"
F="$TMP/fx-drift"
fixtures "$F" "$(agentops_gate true ',{"id":107,"metric":"coverage","op":"LT","error":"50"}')" \
  '{"qualityGate":{"id":"777","name":"agentops","default":true}}'
run "$F"

it "updates a condition whose threshold differs, in place"
assert_contains "$OUT" "condition updated: coverage LT 50 -> LT 80"

it "does NOT create a duplicate condition on that metric"
assert_not_contains "$(cat "$CALLS")" "create_condition"

# --- a token without the permission ------------------------------------------

CALLS="$TMP/calls-403"; : > "$CALLS"
F="$TMP/fx-403"; fixtures "$F" '{"qualitygates":[]}' '{}'
CURL_403=qualitygates run "$F"
unset CURL_403

it "fails when the token cannot administer quality gates"
assert_equals "1" "$RC"

it "names the permission rather than reporting an HTTP code"
assert_contains "$OUT" "Administer Quality Gates"

it "says where to grant it"
assert_contains "$OUT" "Permissions"

it "still provisioned the projects first, so the half that worked is legible"
assert_contains "$OUT" "o_agent-ops-operator_manager"

# --- a failure PART WAY THROUGH the conditions ------------------------------
#
# THE CONDITION LOOP IS A PIPELINE, SO IT RUNS IN A SUBSHELL, and a `set -e`
# abort inside one is exactly the kind of failure a script swallows and reports
# success for. A half-written gate reported as provisioned is worse than no
# gate: the next run finds a gate present and adds only what is missing, so the
# damage would be invisible on every run after the first.

CALLS="$TMP/calls-mid"; : > "$CALLS"
F="$TMP/fx-mid"; fixtures "$F" "$(agentops_gate true)" '{"qualityGate":{"name":"agentops"}}'
CURL_403=create_condition run "$F"
unset CURL_403

it "fails when a condition cannot be written, rather than reporting a half gate"
assert_equals "1" "$RC"

it "got as far as the gate before failing, so the log says where it stopped"
assert_contains "$OUT" "gate present: agentops"

it "assigns no project after a condition failed"
assert_not_contains "$OUT" "assigned"

summary
