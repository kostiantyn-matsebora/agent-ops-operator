#!/usr/bin/env bash
# serviceaccount-guard.py fails a render carrying a ServiceAccount that is
# neither bound, authenticating, nor the floor. Never exercised by any test —
# it was absent from every coverage report entirely. This drives every
# classification: bound, authenticating, named-but-not-presented (both the
# pod-level and the account-level automount=false forms), the floor
# exemption, a CronJob's nested jobTemplate pod spec, and the clean case.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
GUARD="$ROOT/.github/scripts/serviceaccount-guard.py"

tmp=$(mktemp -d)

it "a bound account, an authenticating one, and the floor are all explained: clean exit"
cat > "$tmp/clean.yaml" <<'YAML'
apiVersion: v1
kind: ServiceAccount
metadata:
  name: agentops-bound
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: agentops-authenticates
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: agentops-runtime
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: bind
subjects:
  - kind: ServiceAccount
    name: agentops-bound
    namespace: agent-ops
roleRef:
  kind: ClusterRole
  name: some-role
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: agentops-adapter-x
spec:
  template:
    spec:
      serviceAccountName: agentops-authenticates
      containers: []
YAML
out=$(python3 "$GUARD" "$tmp/clean.yaml" clean 2>&1)
assert_status 0 "$?"
assert_contains "$out" "clean: 3 ServiceAccount(s), 1 bound, 1 authenticating"

it "an account nothing binds and nothing authenticates as is UNEXPLAINED, and the build fails"
cat > "$tmp/unexplained.yaml" <<'YAML'
apiVersion: v1
kind: ServiceAccount
metadata:
  name: agentops-orphan
YAML
out=$(python3 "$GUARD" "$tmp/unexplained.yaml" orphan 2>&1)
assert_status 1 "$?"
assert_contains "$out" "UNEXPLAINED"
assert_contains "$out" "agentops-orphan"

it "a pod naming an account but declining automount is NAMED BUT NEVER PRESENTED, reporting the pod"
cat > "$tmp/loose-pod.yaml" <<'YAML'
apiVersion: v1
kind: ServiceAccount
metadata:
  name: agentops-loose
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: agentops-runtime-loose
spec:
  template:
    spec:
      serviceAccountName: agentops-loose
      automountServiceAccountToken: false
      containers: []
YAML
out=$(python3 "$GUARD" "$tmp/loose-pod.yaml" loose 2>&1)
assert_status 1 "$?"
assert_contains "$out" "NAMED BUT NEVER PRESENTED"
assert_contains "$out" "agentops-loose (named by agentops-runtime-loose)"

it "the account itself declining automount is the same verdict, even if the pod says nothing"
cat > "$tmp/loose-account.yaml" <<'YAML'
apiVersion: v1
kind: ServiceAccount
metadata:
  name: agentops-loose2
automountServiceAccountToken: false
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: agentops-ds
spec:
  template:
    spec:
      serviceAccountName: agentops-loose2
      containers: []
YAML
out=$(python3 "$GUARD" "$tmp/loose-account.yaml" loose2 2>&1)
assert_status 1 "$?"
assert_contains "$out" "agentops-loose2 (named by agentops-ds)"

it "a CronJob's account is read off its nested jobTemplate pod spec, not spec.template"
cat > "$tmp/cronjob.yaml" <<'YAML'
apiVersion: v1
kind: ServiceAccount
metadata:
  name: agentops-housekeeping
---
apiVersion: batch/v1
kind: CronJob
metadata:
  name: agentops-housekeeping
spec:
  jobTemplate:
    spec:
      template:
        spec:
          serviceAccountName: agentops-housekeeping
          containers: []
YAML
out=$(python3 "$GUARD" "$tmp/cronjob.yaml" cron 2>&1)
assert_status 0 "$?"
assert_contains "$out" "1 authenticating"

it "the label defaults to the path when none is given"
out=$(python3 "$GUARD" "$tmp/clean.yaml" 2>&1)
assert_contains "$out" "$tmp/clean.yaml: 3 ServiceAccount(s)"

rm -rf "$tmp"
summary
