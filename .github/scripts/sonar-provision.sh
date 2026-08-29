#!/usr/bin/env sh
# Provisions the SonarCloud project for every component, INSIDE the monorepo
# binding of this repository — which is the only way a project decorates pull
# requests. Idempotent: a key that already exists is left alone by the server.
#
#   SONAR_TOKEN=<user token with Create Projects> SONAR_ORG=<org key> \
#     .github/scripts/sonar-provision.sh
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
set -eu
: "${SONAR_TOKEN:?}" "${SONAR_ORG:?}"
cd "$(dirname "$0")/../.."
inst="${SONAR_ORG}/agent-ops-operator|1324376362"
body=$(.github/components.sh images | jq -c --arg o "$SONAR_ORG" --arg i "$inst" '{
  addAsFavorite: false, newCodeDefinitionType: "days", newCodeDefinitionValue: "30",
  organization: $o,
  projects: [(.[] | .component), "scripts"
             | {installationKey: $i,
                projectKey: ($o + "_agent-ops-operator_" + .),
                projectName: ("agentops-" + .)}]}')
curl -sf -u "$SONAR_TOKEN:" -X POST -H 'Content-Type: application/json' \
  https://sonarcloud.io/api/alm_integration/provision_monorepo_projects --data "$body" \
  | jq -r '.projects[].projectKey'
