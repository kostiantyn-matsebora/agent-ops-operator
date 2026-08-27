#!/usr/bin/env python3
"""Assemble the delegation message for one role of the review.

`reader`       — one component's message: the coordinates, its paths, EVERY
                 thread on those paths (resolved and unresolved alike — the
                 reader's memory of its previous review), the delta specs.
`coordinator`  — the whole review: changed paths, every thread, and one
                 reading per queued component, `null` where the reader's job
                 produced none. A missing reading is a named gap, never a
                 dropped component.

The message is the same one the review's script built when the readers were
`agent()` calls; only where it is assembled moved. The ROLE — what to do with
the message — is the agent definition the job restored from the base branch.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import sys


def header(d: dict) -> str:
    return f"REPO: {d['repo']}\nPR NUMBER: {d['number']}\nBASE REF: origin/{d['base']}\n"


def reader(d: dict, group: str) -> str:
    entry = next((g for g in d["queue"] if g["group"] == group), None)
    if entry is None:
        raise SystemExit(f"no such component in the queue: {group}")
    own = [t for t in d["threads"] if t["path"] in entry["paths"]]
    lines = [header(d).rstrip("\n"),
             f"COMPONENT: {entry['group']} ({entry['kind']})",
             "PATHS:", *("  " + p for p in entry["paths"]),
             "THREADS ON THESE PATHS (your previous review — resolved and unresolved alike):",
             json.dumps(own, indent=1) if own else "  none",
             "DELTA SPECS OF THE CHANGE:",
             *(("  " + p for p in d["specPaths"]) if d["specPaths"] else ("  none",))]
    return "\n".join(lines) + "\n"


def coordinator(d: dict, readings_dir: pathlib.Path) -> tuple[str, list[str]]:
    by_component: dict[str, dict] = {}
    for f in sorted(readings_dir.glob("**/*.json")) if readings_dir.exists() else []:
        try:
            r = json.loads(f.read_text())
        except (OSError, json.JSONDecodeError):
            continue
        if isinstance(r, dict) and isinstance(r.get("component"), str):
            by_component[r["component"]] = r
    readings = [{"group": g["group"], "reading": by_component.get(g["group"])} for g in d["queue"]]
    unreviewed = [r["group"] for r in readings if r["reading"] is None]
    lines = [header(d).rstrip("\n"),
             "CHANGED PATHS:", *("  " + p for p in d["paths"]),
             "REVIEW THREADS:", json.dumps(d["threads"], indent=1),
             "READINGS (one per component, null = unreviewed):", json.dumps(readings, indent=1)]
    return "\n".join(lines) + "\n", unreviewed


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("role", choices=["reader", "coordinator"])
    ap.add_argument("--input", type=pathlib.Path, default=pathlib.Path("review-input.json"))
    ap.add_argument("--group", help="reader: the component")
    ap.add_argument("--readings", type=pathlib.Path, default=pathlib.Path("readings"),
                    help="coordinator: the directory of validated readings")
    args = ap.parse_args()
    d = json.loads(args.input.read_text())

    if args.role == "reader":
        if not args.group:
            ap.error("reader needs --group")
        sys.stdout.write(reader(d, args.group))
        return 0
    text, unreviewed = coordinator(d, args.readings)
    sys.stdout.write(text)
    if unreviewed:
        print(f"unreviewed: {', '.join(unreviewed)}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
