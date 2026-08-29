#!/usr/bin/env bash
# The project list the provisioning call carries.
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

rm -rf "$tmp"
summary
