#!/usr/bin/env python3
"""Group a pull request's changed paths into the review's work queue.

ONE ENTRY PER CHANGED COMPONENT. The review spawns one reader per entry, so
this program decides how many contexts a review costs and what each of them
sees. It is a program rather than a prompt instruction because the answer has
to be the same one `.github/components.sh` gives CI: a component is a directory
that publishes an image or is its own Go module, derived from the tree, never
listed.

Paths outside every component -- `docs/`, `.github/`, `openspec/`, `.claude/`,
`chart/` -- group by their top-level directory, and files at the root
(`README.md`, `CLAUDE.md`) form one `root` entry. A docs-only push is
therefore one reader, not zero: prose is reviewed here too.

Reads paths as arguments, or on stdin one per line when none are given
(`gh pr diff --name-only`), and writes a JSON list of `{group, kind, paths}` --
`kind` is `component` or `directory`.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import subprocess
import sys

HERE = pathlib.Path(__file__).resolve().parent
COMPONENTS_SH = HERE.parent / "components.sh"


def component_dirs(root: pathlib.Path) -> list[str]:
    """Every component context, as `components.sh` derives it, without `./`.
    Sorted longest first so a nested match wins over a parent's."""
    out = subprocess.run([str(COMPONENTS_SH), "images"], cwd=root,
                         capture_output=True, text=True, check=True).stdout
    dirs = {c["context"].removeprefix("./").strip("/") for c in json.loads(out)}
    return sorted(dirs, key=len, reverse=True)


def group_paths(paths: list[str], components: list[str]) -> list[dict]:
    groups: dict[str, dict] = {}
    for raw in paths:
        path = raw.strip().removeprefix("./")
        if not path:
            continue
        key, kind = None, None
        for comp in components:
            if path == comp or path.startswith(comp + "/"):
                key, kind = comp, "component"
                break
        if key is None:
            head, sep, _ = path.partition("/")
            key, kind = (head, "directory") if sep else ("root", "directory")
        entry = groups.setdefault(key, {"group": key, "kind": kind, "paths": []})
        entry["paths"].append(path)
    return sorted(groups.values(), key=lambda g: g["group"])


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--root", type=pathlib.Path, default=HERE.parent.parent,
                    help="repository root components.sh runs in")
    ap.add_argument("--components", type=pathlib.Path,
                    help="a JSON list of component directories, instead of running components.sh")
    ap.add_argument("paths", nargs="*", help="changed paths; stdin when absent")
    args = ap.parse_args()

    if args.components:
        components = sorted(json.loads(args.components.read_text()), key=len, reverse=True)
    else:
        components = component_dirs(args.root)

    paths = args.paths or sys.stdin.read().splitlines()
    queue = group_paths(paths, components)
    json.dump(queue, sys.stdout, indent=2)
    print()
    print(f"{len(queue)} group(s) from {sum(len(g['paths']) for g in queue)} path(s)", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
