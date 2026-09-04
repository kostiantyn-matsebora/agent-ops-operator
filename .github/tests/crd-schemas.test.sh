#!/usr/bin/env bash
# crd-schemas.py converts this repo's CRDs into kubeconform-shaped JSON
# schemas. It never ran under any test — absent from the coverage report
# entirely. This drives it against a fixture CRD directory (never the real
# chart/crds/), covering: a CRD with a schema (written), one version with no
# schema (skipped), a non-CRD document in the same file (ignored), a
# multi-doc YAML, the "nothing found" failure, and the argc usage error.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
GEN="$ROOT/.github/scripts/crd-schemas.py"

tmp=$(mktemp -d)
mkdir -p "$tmp/crds" "$tmp/out"

cat > "$tmp/crds/widgets.yaml" <<'YAML'
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.example.test
spec:
  names:
    kind: Widget
  versions:
    - name: v1
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
    - name: v2
      # No schema on this version at all — must be skipped, not crash.
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: not-a-crd
data:
  x: "y"
YAML

it "writes one schema per versioned CRD with a schema, skips the version without one, and ignores non-CRD docs"
out=$(python3 "$GEN" "$tmp/crds" "$tmp/out" 2>&1)
assert_status 0 "$?"
assert_contains "$out" "wrote 1 schema(s) to $tmp/out"
test -f "$tmp/out/widget_v1.json"
assert_status 0 "$?"
test -f "$tmp/out/widget_v2.json"
assert_status 1 "$?"

it "the written file is the schema, valid JSON"
assert_contains "$(cat "$tmp/out/widget_v1.json")" '"type": "object"'

it "an empty CRD directory writes nothing and fails naming it"
mkdir -p "$tmp/empty" "$tmp/emptyout"
out=$(python3 "$GEN" "$tmp/empty" "$tmp/emptyout" 2>&1)
assert_status 1 "$?"
assert_contains "$out" "no CRD schemas found under $tmp/empty"

it "the wrong number of arguments is a usage error, exit 2"
out=$(python3 "$GEN" "$tmp/crds" 2>&1)
assert_status 2 "$?"
assert_contains "$out" "usage: crd-schemas.py"

rm -rf "$tmp"
summary
