#!/usr/bin/env bash
# The project list the provisioning call carries, and the quality gate it is
# asked for separately.
#
# Every component is a project, and so is ONE thing that is not a component:
# the workflow's own scripts. The list is what ci.yml's "project must exist"
# assertion is measured against, so a name missing here fails a job later with
# a message pointing back at this script.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
S="$ROOT/.github/scripts/sonar-provision.sh"

tmp=$(mktemp -d)
# A curl that answers with what it was asked to send, keeping the raw body for
# the test to read — and never reaches SonarCloud.
mkdir -p "$tmp/bin"; export CURL_BODY="$tmp/body"
cat > "$tmp/bin/curl" <<'STUB'
#!/usr/bin/env bash
while [ $# -gt 0 ]; do
  case "$1" in --data) printf '%s' "$2" > "$CURL_BODY"; printf '%s' "$2"; exit 0 ;; esac
  shift
done
echo '{}'
STUB
chmod +x "$tmp/bin/curl"
export PATH="$tmp/bin:$PATH"

out=$(cd "$ROOT" && SONAR_TOKEN=t SONAR_ORG=org sh "$S" 2>&1); status=$?
components=$("$ROOT/.github/components.sh" images | jq -r '.[].component')
n_components=$(printf '%s\n' "$components" | wc -l | tr -d ' ')

it "runs, and prints one key per project"
assert_status 0 "$status"

it "carries every component components.sh lists"
missing=""
for c in $components; do
  case "$out" in *"_agent-ops-operator_$c"*) ;; *) missing="$missing $c" ;; esac
done
assert_equals "" "$missing"

it "carries the scripts unit, which is not a component"
assert_contains "$out" "org_agent-ops-operator_scripts"

it "carries exactly the components plus that one"
assert_equals "$((n_components + 1))" "$(printf '%s\n' "$out" | grep -c '_agent-ops-operator_')"

it "names it by the component pattern, inside the monorepo binding"
assert_equals "agentops-scripts" "$(jq -r '.projects[] | select(.projectKey | endswith("_scripts")) | .projectName' "$CURL_BODY")"

it "binds it to the same installation as every component"
assert_equals "1" "$(jq -r '[.projects[].installationKey] | unique | length' "$CURL_BODY")"

it "provisions only the named projects when given names — what CI asks for"
out=$(cd "$ROOT" && SONAR_TOKEN=t SONAR_ORG=org sh "$S" scripts 2>&1)
assert_equals "1" "$(jq -r '.projects | length' "$CURL_BODY")"

it "and names the right one"
assert_equals "org_agent-ops-operator_scripts" "$(jq -r '.projects[0].projectKey' "$CURL_BODY")"

it "a name that is not a project of this repository provisions nothing"
: > "$CURL_BODY"
(cd "$ROOT" && SONAR_TOKEN=t SONAR_ORG=org sh "$S" somebody-elses >/dev/null 2>&1); status=$?
assert_status 64 "$status"

it "and sends no request at all"
assert_equals "" "$(cat "$CURL_BODY")"


# ============================================================================
# THE GATE STAGE, which the runs above never reach: they pass no `--gate`, and
# that is the assertion rather than an omission. CI calls this script itself
# for a missing project, and the scan step fails a component's job on the
# gate's verdict — so provisioning a threshold has to be something a person
# asks for.
# ============================================================================

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
# A curl that DRAINS ITS STDIN, on demand. The script's two provisioning loops
# are `... | while read`, so the body inherits the pipe: a command in it that
# reads stdin swallows the rest of the list and the run provisions the first
# item only, reporting success. `sq` passes `</dev/null` to stop that, and this
# is what proves it does. The harness gives the SCRIPT /dev/null as stdin, so a
# drain outside a loop reads EOF at once and only the loops can be truncated —
# without that the regression shows up as a hang, which reads as a broken test
# rather than as the bug it is.
[ -n "${CURL_DRAINS_STDIN:-}" ] && cat >/dev/null
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
run() {  # run <fixtures-dir> [args...] — the gate scenarios pass --gate
  local tree; tree=$(make_tree)
  local fx="$1"; shift
  CURL_CALLS="$CALLS" CURL_FIXTURES="$fx" SONAR_TOKEN=t SONAR_ORG=o \
    sh "$tree/.github/scripts/sonar-provision.sh" "$@" > "$TMP/out" 2>&1 </dev/null && RC=0 || RC=$?
  OUT=$(cat "$TMP/out")
}

TMP=$(mktemp -d)
BIN="$TMP/bin"
stub_curl "$BIN"

# --- an organisation with no gate at all ------------------------------------

CALLS="$TMP/calls-fresh"; : > "$CALLS"
F="$TMP/fx-fresh"; fixtures "$F" '{"qualitygates":[{"id":9,"name":"Sonar way","isDefault":true,"isBuiltIn":true,"conditions":[]}]}' '{"qualityGate":{"id":"9","name":"Sonar way","default":true}}'
run "$F" --gate

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
run "$F" --gate

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
run "$F" --gate

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
run "$F" --gate

it "updates a condition whose threshold differs, in place"
assert_contains "$OUT" "condition updated: coverage LT 50 -> LT 80"

it "does NOT create a duplicate condition on that metric"
assert_not_contains "$(cat "$CALLS")" "create_condition"

# --- a token without the permission ------------------------------------------

CALLS="$TMP/calls-403"; : > "$CALLS"
F="$TMP/fx-403"; fixtures "$F" '{"qualitygates":[]}' '{}'
CURL_403=qualitygates run "$F" --gate
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
CURL_403=create_condition run "$F" --gate
unset CURL_403

it "fails when a condition cannot be written, rather than reporting a half gate"
assert_equals "1" "$RC"

it "got as far as the gate before failing, so the log says where it stopped"
assert_contains "$OUT" "gate present: agentops"

it "assigns no project after a condition failed"
assert_not_contains "$OUT" "assigned"

# --- WITHOUT --gate, the organisation's gate is not touched -----------------
#
# THE CASE THE FLAG EXISTS FOR, and the one CI exercises on every run: the scan
# step calls this script for a project that does not exist. If that call also
# provisioned the gate, a missing project would turn an 80% threshold on for
# the whole organisation from inside a job — and the verdict now fails the
# component's job, so it would block every component under it.

CALLS="$TMP/calls-nogate"; : > "$CALLS"
F="$TMP/fx-nogate"; fixtures "$F" '{"qualitygates":[]}' '{}'
run "$F"

it "provisions the projects with no --gate"
assert_contains "$OUT" "o_agent-ops-operator_manager"

it "makes NO quality-gate call at all"
assert_not_contains "$(cat "$CALLS")" "qualitygates"

it "succeeds"
assert_equals "0" "$RC"

it "refuses an option it does not know, rather than ignoring it"
run "$F" --gates
assert_equals "2" "$RC"
assert_contains "$OUT" "unknown option"

# --- a tool in the loop that reads stdin must not truncate the list ---------

CALLS="$TMP/calls-drain"; : > "$CALLS"
F="$TMP/fx-drain"; fixtures "$F" '{"qualitygates":[{"id":9,"name":"Sonar way","isDefault":true,"conditions":[]}]}' '{"qualityGate":{"name":"Sonar way"}}'
CURL_DRAINS_STDIN=1 run "$F" --gate
unset CURL_DRAINS_STDIN

it "assigns EVERY project even when the API call consumes stdin"
assert_contains "$OUT" "assigned: o_agent-ops-operator_manager"
assert_contains "$OUT" "assigned: o_agent-ops-operator_console"

it "writes every condition too, not just the first"
assert_equals "7" "$(printf '%s\n' "$OUT" | grep -c 'condition added')"

rm -rf "$tmp"
summary
