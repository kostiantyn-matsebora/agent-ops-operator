#!/usr/bin/env sh
# Provisions the SonarCloud project for every component, INSIDE the monorepo
# binding of this repository — which is the only way a project decorates pull
# requests. Idempotent: a key that already exists is left alone by the server.
#
#   SONAR_TOKEN=<token with Create Projects> SONAR_ORG=<org key> \
#     .github/scripts/sonar-provision.sh            # every project
#     .github/scripts/sonar-provision.sh scripts    # only the named ones
#
# CI CALLS THE SECOND FORM ITSELF (.github/actions/sonar-scan) when the
# project it is about to analyse does not exist, so a new component's first
# run creates its project inside the monorepo binding — bound to the
# repository, decorating pull requests — rather than the scanner's own
# auto-provisioning creating one that is bound to nothing.
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
# ONE PROJECT IS NOT A COMPONENT: `scripts`, the workflow's own scripts under
# .github/, analysed by ci.yml's `scripts` job under the same key and name
# pattern. It is appended here rather than taught to components.sh, because a
# directory that program lists is a directory the release tags.
#
# THE GATE STAGE IS OPT-IN (`--gate`), AND UNDER THE PARAGRAPH ABOVE IT HAS TO
# BE. CI calls this script ITSELF for a project that does not exist, and the
# scan step now FAILS the component's job on the gate's verdict — so a script
# that always provisioned the gate would let a missing project turn the whole
# organisation's threshold on, from inside a job, with nobody asking. Creating
# a project and deciding what the tree is judged by are two acts, and only the
# first is automatic.
#
#   .github/scripts/sonar-provision.sh --gate     # and the gate as well
set -eu
: "${SONAR_TOKEN:?}" "${SONAR_ORG:?}"
cd "$(dirname "$0")/../.."
with_gate=false
_n=$#; _i=0
while [ "$_i" -lt "$_n" ]; do
  case "$1" in
    --gate) with_gate=true ;;
    -*) echo "sonar-provision: unknown option '$1' (only --gate)" >&2; exit 2 ;;
    *) set -- "$@" "$1" ;;
  esac
  shift; _i=$((_i + 1))
done

API=https://sonarcloud.io/api
GATE=agentops
# `scripts` is the workflow's OWN tooling, never shipped -- no image, no
# chart, nothing `components.sh` or a release tag ever names. A finding
# Sonar's engine will not credit any code shape for (pythonsecurity:S2083/
# S8707 on a CLI --out path, four documented-pattern attempts made and
# recorded in sonar-ratings-baseline's tasks.md) should not gate a project
# with no artifact to gate. Assigned this EMPTY gate instead of `agentops`
# -- zero conditions, always green -- rather than disabling `ci-green`'s
# scan step entirely, so `scripts` keeps reporting real findings for a
# person to read, it just never blocks a merge on them.
GATE_UNENFORCED=agentops-unenforced
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
  # STDIN IS CLOSED ON EVERY CALL. Both loops below are `... | while read`, so
  # the loop body inherits the PIPE as its stdin — and a command in that body
  # that reads stdin eats the rest of the list, silently provisioning half of
  # it. curl does not read stdin today; `</dev/null` is what stops a later
  # `--data @-` or a swapped tool from making that a one-character bug nobody
  # would look for here.
  if [ "$_method" = GET ]; then
    _code=$(curl -s -o "$BODY" -w '%{http_code}' -u "$SONAR_TOKEN:" -G "$@" "$API/$_path" </dev/null)
  else
    _code=$(curl -s -o "$BODY" -w '%{http_code}' -u "$SONAR_TOKEN:" -X POST "$@" "$API/$_path" </dev/null)
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
# The whole list, then the arguments as a filter over it — so a name that is
# not a project this repository owns cannot be provisioned by typing it.
wanted='[]'; [ $# -eq 0 ] || wanted=$(printf '%s\n' "$@" | jq -R . | jq -sc .)
body=$(.github/components.sh images | jq -c --arg o "$SONAR_ORG" --arg i "$inst" --argjson w "$wanted" '{
  addAsFavorite: false, newCodeDefinitionType: "days", newCodeDefinitionValue: "30",
  organization: $o,
  projects: [(.[] | .component), "scripts"
             | select(($w | length) == 0 or IN($w[]))
             | {installationKey: $i,
                projectKey: ($o + "_agent-ops-operator_" + .),
                projectName: ("agentops-" + .)}]}')
[ "$(printf '%s' "$body" | jq '.projects | length')" -gt 0 ] || { echo "nothing to provision: $*" >&2; exit 64; }
curl -sf -u "$SONAR_TOKEN:" -X POST -H 'Content-Type: application/json' \
  "$API/alm_integration/provision_monorepo_projects" --data "$body" </dev/null \
  | jq -r '.projects[].projectKey'

# --- stage 2: the gate ------------------------------------------------------

if [ "$with_gate" != true ]; then
  exit 0
fi

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
# coverage and the three overall ratings (sonar-ratings-baseline). Copied
# rather than listed here so upstream's set can change without this script
# describing a gate contributors no longer see.
#
# 1.0-5.0, A=1 ... E=5 on all three -- `GT 2` fails worse than B, mirroring
# `coverage LT 80`'s own numeric-scale condition. Maintainability keeps its
# historical SQALE key; there is no separate "new" vs "overall" spelling to
# confuse it with, unlike `new_coverage` vs `coverage` above.
wanted=$(sq GET qualitygates/show "organization=$SONAR_ORG" "name=$BUILTIN" \
  | jq -r '.conditions[] | "\(.metric) \(.op) \(.error)"')
wanted="$wanted
coverage LT $COVERAGE_THRESHOLD
reliability_rating GT 2
security_rating GT 2
sqale_rating GT 2"

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

# --- stage 2b: the unenforced gate, for `scripts` alone ---------------------
#
# Created once and never synced with conditions afterwards -- its whole
# point is to carry NONE, ever, so there is nothing here to keep in step
# with `agentops` or the built-in gate the way stage 2 does above.
unenforced_id=$(printf '%s' "$gates" | jq -r --arg n "$GATE_UNENFORCED" \
  'first(.qualitygates[] | select(.name == $n) | .id) // empty')
if [ -z "$unenforced_id" ]; then
  unenforced_id=$(sq POST qualitygates/create "organization=$SONAR_ORG" "name=$GATE_UNENFORCED" | jq -r '.id')
  echo "gate created: $GATE_UNENFORCED ($unenforced_id)"
else
  echo "gate present: $GATE_UNENFORCED ($unenforced_id)"
fi

# EVERY PROJECT IS ASSIGNED EXPLICITLY, not left to the default. The default
# only catches projects nobody has assigned, so a project moved onto another
# gate by hand would keep it and report green against conditions this
# repository never chose.
# EVERY project, never the `$@` filter: the filter says which projects to
# CREATE, and a gate assigned to the one component somebody happened to name
# would leave the rest judged by whatever they were on. `scripts` is
# assigned separately, to `GATE_UNENFORCED` rather than `agentops` --
# see the constant's own comment for why.
.github/components.sh images | jq -r --arg o "$SONAR_ORG" \
  '(.[] | .component) | $o + "_agent-ops-operator_" + .' | while read -r key; do
  current=$(sq GET qualitygates/get_by_project "organization=$SONAR_ORG" "project=$key" \
    | jq -r '.qualityGate.name // empty')
  if [ "$current" = "$GATE" ]; then
    echo "  assigned already: $key"
  else
    sq POST qualitygates/select "organization=$SONAR_ORG" "gateId=$gate_id" "projectKey=$key" >/dev/null
    echo "  assigned: $key"
  fi
done

scripts_key="${SONAR_ORG}_agent-ops-operator_scripts"
scripts_current=$(sq GET qualitygates/get_by_project "organization=$SONAR_ORG" "project=$scripts_key" \
  | jq -r '.qualityGate.name // empty')
if [ "$scripts_current" = "$GATE_UNENFORCED" ]; then
  echo "  assigned already: $scripts_key"
else
  sq POST qualitygates/select "organization=$SONAR_ORG" "gateId=$unenforced_id" "projectKey=$scripts_key" >/dev/null
  echo "  assigned: $scripts_key -> $GATE_UNENFORCED"
fi
