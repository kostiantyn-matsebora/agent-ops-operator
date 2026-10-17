#!/usr/bin/env python3
"""A pinned runtime image must carry the work-report contract.

The manager draws a run's model/tool-call statistics — the `model.call` and
`tool.call` hops behind the console's Runs tables — only from what the RUNTIME
reports on completion (`internal/httpapi/workreport.go`). Every half of that
path is tested in isolation, so nothing failed when the chart pinned
runtime-claude 0.9.0 (built 2026-09-05) while the report module landed
2026-09-26: three weeks of installs whose Runs showed no statistics, with CI
green throughout.

For every runtime bundle's pinned image this asserts, against the repository's
OWN tags (publishing is a git tag):

  * the pin is a release version, never a dev or branch tag;
  * the git tag `<component>-v<version>` exists;
  * that tag's tree carries the runtime's report module.

Fails CLOSED on anything unreadable. The order it enforces: push the
component's release tag FIRST, bump the chart pin after.
"""

import re
import subprocess
import sys
from pathlib import Path

import yaml

# component, the values file pinning its image, the contract files its tagged
# tree must carry. A new runtime bundle adds a row here in the change that
# ships it.
RUNTIMES = [
    ("runtime-claude", "chart/charts/claude/values.yaml", ("runtimes/claude/report.js",)),
    ("runtime-copilot", "chart/charts/copilot/values.yaml", ("runtimes/copilot/report.js",)),
    ("runtime-ollama", "chart/charts/ollama/values.yaml", ("runtimes/ollama/report.go",)),
]

RELEASE = re.compile(r"^\d+\.\d+\.\d+$")


def git_ok(root: Path, *args: str) -> bool:
    return subprocess.run(["git", *args], cwd=root, capture_output=True).returncode == 0


def check(root: Path, component: str, values: str, contract: tuple) -> list[str]:
    try:
        data = yaml.safe_load((root / values).read_text())
    except OSError as e:
        return [f"{values}: unreadable ({e})"]
    image = str((data or {}).get("image", ""))
    if f"agentops-{component}:" not in image:
        return [f"{values}: image does not name agentops-{component}: {image!r}"]
    version = image.rsplit(":", 1)[1]
    if not RELEASE.fullmatch(version):
        return [f"{values}: pins {version!r} — a chart pins release versions only"]
    tag = f"{component}-v{version}"
    if not git_ok(root, "rev-parse", "-q", "--verify", f"refs/tags/{tag}"):
        return [f"{values}: pins {version} but tag {tag} does not exist — push the release tag first"]
    return [
        f"{values}: {tag} lacks {path} — the pinned image predates the work-report contract"
        for path in contract
        if not git_ok(root, "cat-file", "-e", f"{tag}:{path}")
    ]


def main() -> int:
    root = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(__file__).resolve().parents[2]
    failures = []
    for component, values, contract in RUNTIMES:
        bad = check(root, component, values, contract)
        failures.extend(bad)
        print(f"  {'FAIL' if bad else 'ok'}\t{component}")
    for line in failures:
        print(line, file=sys.stderr)
    if failures:
        return 1
    print("every pinned runtime carries the work-report contract")
    return 0


if __name__ == "__main__":
    sys.exit(main())
