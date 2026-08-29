#!/usr/bin/env sh
# Provisions the SonarCloud project for every component, INSIDE the monorepo
# binding of this repository — which is the only way a project decorates pull
# requests — and then the quality gate every one of those projects is held to.
# Idempotent throughout: a project key that already exists is left alone by the
# server, and the gate stage writes only what a lookup says is missing or wrong.
#
#   SONAR_TOKEN=<user token> SONAR_ORG=<org key> \
#     .github/scripts/sonar-provision.sh
#
# THE TOKEN NEEDS TWO PERMISSIONS, AND EACH STAGE FAILS NAMING ITS OWN:
# *Create Projects* for the projects, *Administer Quality Gates* for the gate.
# A token carrying only the first provisions the projects and then stops on a
# 403 saying which permission it lacks — a legible half, rather than a silent
# one that leaves the organisation judged by the built-in gate forever.
#
# THIS IS THE WEB APP'S OWN CALL, NOT A DOCUMENTED API. SonarCloud's public
# api/projects/create makes a project bound to NO repository; the scanner's
# auto-provisioning does the same, which is why ci.yml refuses a missing
# project instead of letting it be created. The monorepo wizard posts exactly
# this JSON to alm_integration/provision_monorepo_projects (read out of the
# bundle on 2026-08-29: newCodeDefinitionType is the lowercase enum, and the
# installation key sits inside each project entry). If it stops working, the
# wizard's "Import JSON" accepts the same projects array.
#
# THE GATE STAGE IS THE PUBLIC API, unlike the call above it. Its conditions
# are COPIED FROM THE BUILT-IN `Sonar way` BY LOOKUP rather than hard-coded, so
# a re-run tracks whatever upstream currently ships and the only condition this
# repository states is the one it is adding: OVERALL `coverage LT 80`. The
# built-in gate is read-only and judges NEW code alone, which is what let a
# component sit at 27% and one at 79% pass identically.
#
# RESPONSE SHAPES, read off sonarcloud.io on 2026-08-29 against a public
# organisation: `list` answers {qualitygates:[{id,name,isDefault,isBuiltIn,
# conditions:[{id,metric,op,error}]}]}, `show` the same for one gate, and
# `get_by_project` {qualityGate:{id,name,default}}.
set -eu
: "${SONAR_TOKEN:?}" "${SONAR_ORG:?}"
cd "$(dirname "$0")/../.."

API=https://sonarcloud.io/api
GATE=agentops
BUILTIN='Sonar way'
COVERAGE_THRESHOLD=80

BODY=$(mktemp)
trap 'rm -f "$BODY"' EXIT INT TERM

# sq <GET|POST> <path> [<key>=<value> ...] — the answer on stdout.
#
# NOT `curl -sf`: -f prints nothing and reports one exit code for every HTTP
# error, and the whole point here is to tell a 403 (the token lacks the
# permission, and which one) from anything else. So the status is read with -w
# and the body kept, and a failure names both.
sq() {
  _method=$1; _path=$2; shift 2
  _n=$#; _i=0
  while [ "$_i" -lt "$_n" ]; do
    set -- "$@" --data-urlencode "$1"; shift; _i=$((_i + 1))
  done
  if [ "$_method" = GET ]; then
    _code=$(curl -s -o "$BODY" -w '%{http_code}' -u "$SONAR_TOKEN:" -G "$@" "$API/$_path")
  else
    _code=$(curl -s -o "$BODY" -w '%{http_code}' -u "$SONAR_TOKEN:" -X POST "$@" "$API/$_path")
  fi
  case "$_code" in
    2*) cat "$BODY"; return 0 ;;
    403)
      echo "sonar-provision: 403 on $_path — the token lacks *Administer Quality Gates*." >&2
      echo "  Grant it in the organisation's Administration > Permissions, or use a token" >&2
      echo "  belonging to a member who holds it. The projects stage needs *Create Projects*." >&2
      return 1 ;;
    *)
      echo "sonar-provision: HTTP $_code on $_path" >&2
      sed -n '1,5p' "$BODY" >&2
      return 1 ;;
  esac
}

# --- stage 1: the projects --------------------------------------------------

inst="${SONAR_ORG}/agent-ops-operator|1324376362"
components=$(.github/components.sh images)
body=$(printf '%s' "$components" | jq -c --arg o "$SONAR_ORG" --arg i "$inst" '{
  addAsFavorite: false, newCodeDefinitionType: "days", newCodeDefinitionValue: "30",
  organization: $o,
  projects: [.[] | {installationKey: $i,
                    projectKey: ($o + "_agent-ops-operator_" + .component),
                    projectName: ("agentops-" + .component)}]}')
curl -sf -u "$SONAR_TOKEN:" -X POST -H 'Content-Type: application/json' \
  "$API/alm_integration/provision_monorepo_projects" --data "$body" \
  | jq -r '.projects[].projectKey'

# --- stage 2: the gate ------------------------------------------------------

gates=$(sq GET qualitygates/list "organization=$SONAR_ORG")
gate_id=$(printf '%s' "$gates" | jq -r --arg n "$GATE" \
  'first(.qualitygates[] | select(.name == $n) | .id) // empty')

if [ -z "$gate_id" ]; then
  gate_id=$(sq POST qualitygates/create "organization=$SONAR_ORG" "name=$GATE" | jq -r '.id')
  echo "gate created: $GATE ($gate_id)"
  existing=''
  is_default=false
else
  echo "gate present: $GATE ($gate_id)"
  # The conditions ride in the `list` answer, so the gate's current state costs
  # no second read. Kept as "<metric> <condition id> <op> <error>".
  existing=$(printf '%s' "$gates" | jq -r --arg n "$GATE" \
    '.qualitygates[] | select(.name == $n) | .conditions[]? | "\(.metric) \(.id) \(.op) \(.error)"')
  is_default=$(printf '%s' "$gates" | jq -r --arg n "$GATE" \
    'first(.qualitygates[] | select(.name == $n) | .isDefault) // false')
fi

# WANTED = every condition of the built-in gate, verbatim, plus overall
# coverage. Copied rather than listed here so upstream's set can change without
# this script describing a gate contributors no longer see.
wanted=$(sq GET qualitygates/show "organization=$SONAR_ORG" "name=$BUILTIN" \
  | jq -r '.conditions[] | "\(.metric) \(.op) \(.error)"')
wanted="$wanted
coverage LT $COVERAGE_THRESHOLD"

printf '%s\n' "$wanted" | while read -r metric op error; do
  [ -n "$metric" ] || continue
  have=$(printf '%s\n' "$existing" | awk -v m="$metric" '$1 == m {print; exit}')
  if [ -z "$have" ]; then
    sq POST qualitygates/create_condition "organization=$SONAR_ORG" "gateId=$gate_id" \
      "metric=$metric" "op=$op" "error=$error" >/dev/null
    echo "  condition added: $metric $op $error"
    continue
  fi
  # ONE CONDITION PER METRIC, UPDATED IN PLACE. A second create_condition on a
  # metric the gate already carries is accepted by the server and leaves the
  # gate with two conditions on it, which is how a re-run would tighten a
  # threshold somebody deliberately relaxed and never say so.
  cond_id=$(printf '%s' "$have" | awk '{print $2}')
  cond_op=$(printf '%s' "$have" | awk '{print $3}')
  cond_error=$(printf '%s' "$have" | awk '{print $4}')
  if [ "$cond_op" = "$op" ] && [ "$cond_error" = "$error" ]; then
    echo "  condition present: $metric $op $error"
  else
    sq POST qualitygates/update_condition "organization=$SONAR_ORG" "id=$cond_id" \
      "metric=$metric" "op=$op" "error=$error" >/dev/null
    echo "  condition updated: $metric $cond_op $cond_error -> $op $error"
  fi
done

if [ "$is_default" = true ]; then
  echo "gate is already the organisation default"
else
  sq POST qualitygates/set_as_default "organization=$SONAR_ORG" "id=$gate_id" >/dev/null
  echo "gate set as the organisation default"
fi

# EVERY PROJECT IS ASSIGNED EXPLICITLY, not left to the default. The default
# only catches projects nobody has assigned, so a project moved onto another
# gate by hand would keep it and report green against conditions this
# repository never chose.
printf '%s' "$components" | jq -r --arg o "$SONAR_ORG" \
  '.[] | $o + "_agent-ops-operator_" + .component' | while read -r key; do
  current=$(sq GET qualitygates/get_by_project "organization=$SONAR_ORG" "project=$key" \
    | jq -r '.qualityGate.name // empty')
  if [ "$current" = "$GATE" ]; then
    echo "  assigned already: $key"
  else
    sq POST qualitygates/select "organization=$SONAR_ORG" "gateId=$gate_id" "projectKey=$key" >/dev/null
    echo "  assigned: $key"
  fi
done
