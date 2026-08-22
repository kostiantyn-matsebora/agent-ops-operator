#!/usr/bin/env python3
"""Convert this repo's CRDs into JSON schemas kubeconform can validate against.

Rendering the chart proves it produces YAML. It does not prove the custom
resources in it are VALID — kubeconform validates what it has a schema for and
SKIPS what it does not, so without this every Pipeline, Channel and MCPToolset
the chart ships would pass by being unrecognised. That is the failure mode this
exists to remove: a green check that validated the Deployments and shrugged at
the CRs.

Output names follow kubeconform's own convention, so the workflow points at
    <dir>/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json
"""
import json
import pathlib
import sys

import yaml


def main(src: str, dest: str) -> int:
    out = pathlib.Path(dest)
    out.mkdir(parents=True, exist_ok=True)
    written = 0
    for path in sorted(pathlib.Path(src).glob("*.yaml")):
        for doc in yaml.safe_load_all(path.read_text()):
            if not doc or doc.get("kind") != "CustomResourceDefinition":
                continue
            kind = doc["spec"]["names"]["kind"]
            for version in doc["spec"].get("versions", []):
                schema = (version.get("schema") or {}).get("openAPIV3Schema")
                if not schema:
                    continue
                name = f"{kind.lower()}_{version['name'].lower()}.json"
                (out / name).write_text(json.dumps(schema, indent=2))
                written += 1
    if written == 0:
        print(f"no CRD schemas found under {src}", file=sys.stderr)
        return 1
    print(f"wrote {written} schema(s) to {dest}")
    return 0


if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("usage: crd-schemas.py <crd-dir> <out-dir>", file=sys.stderr)
        raise SystemExit(2)
    raise SystemExit(main(sys.argv[1], sys.argv[2]))
