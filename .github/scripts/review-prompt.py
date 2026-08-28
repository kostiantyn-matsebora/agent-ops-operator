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


def _load_rules_for():
    """`review-rules.py` by path — a hyphenated file name is not a module name."""
    import importlib.util
    p = pathlib.Path(__file__).resolve().parent / "review-rules.py"
    spec = importlib.util.spec_from_file_location("review_rules", p)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod.rules_for


def _entry(d: dict, slug: str) -> dict:
    """A matrix entry by slug — a component, or one chunk of it. The component
    it belongs to supplies `kind` and the sibling file names."""
    entry = next((e for e in d.get("entries", []) if e["slug"] == slug), None)
    if entry is None:
        raise SystemExit(f"no such entry in the queue: {slug}")
    comp = next(g for g in d["queue"] if g["group"] == entry["group"])
    return {**entry, "kind": comp["kind"], "all_paths": comp["paths"]}


def component_args(d: dict, slug: str) -> dict:
    """The `review-component` workflow's args for one matrix entry: one item
    per file in the entry with its threads and THE RULE FILES TO READ for its
    path — routed by `review-rules.py`, since no reader inherits any. The
    sibling names are the whole component's, so a reference to a file in
    another chunk is still recognisable."""
    rules_for = _load_rules_for()
    entry = _entry(d, slug)
    files = [{"path": p,
              "rules": rules_for(p),
              "threads": [t for t in d["threads"] if t["path"] == p]}
             for p in entry["paths"]]
    return {"repo": d["repo"], "number": d["number"], "base": f"origin/{d['base']}",
            "component": entry["group"], "kind": entry["kind"], "chunk": entry.get("chunk", ""),
            "siblings": [p for p in entry["all_paths"] if p not in entry["paths"]],
            "files": files, "specPaths": d["specPaths"]}


def reader(d: dict, slug: str) -> str:
    """The component session's whole instruction: run the saved workflow with
    these args and return its result verbatim. It reads nothing itself."""
    a = component_args(d, slug)
    return ("Run the saved workflow `review-component` with these args and nothing else:\n"
            + json.dumps(a) + "\n"
            "You have the Workflow tool and nothing else — no shell, no file tools; do not try them. "
            "Do not read the diff and do not review anything yourself; the workflow's file readers do. "
            "It runs in the background: do NOT answer, and do NOT produce structured output, until its "
            "completion notification arrives — an answer before that is a reading of nothing, and is "
            "discarded. When it returns, return its result JSON verbatim — every field it produced.\n")


def merge(readings: list[dict], component: str, expected_paths: list[str]) -> dict:
    """A component's chunk readings as one reading. A chunk with no reading
    leaves its files in `unread`, by name."""
    files = [f for r in readings for f in r.get("files", [])]
    read_paths = {f["path"] for f in files}
    unread = sorted(set(p for r in readings for p in r.get("unread", []))
                    | {p for p in expected_paths if p not in read_paths})
    return {"component": component,
            "findings": [x for r in readings for x in r.get("findings", [])],
            "changedNames": sorted({n for r in readings for n in r.get("changedNames", [])}),
            "files": files,
            "threads": [t for r in readings for t in r.get("threads", [])],
            "unread": unread}


def coordinator(d: dict, readings_dir: pathlib.Path) -> tuple[str, list[str]]:
    by_component: dict[str, list[dict]] = {}
    for f in sorted(readings_dir.glob("**/*.json")) if readings_dir.exists() else []:
        try:
            r = json.loads(f.read_text())
        except (OSError, json.JSONDecodeError):
            continue
        if isinstance(r, dict) and isinstance(r.get("component"), str):
            by_component.setdefault(r["component"], []).append(r)
    # A component read in chunks is handed to the coordinator as ONE reading;
    # a component none of whose chunks produced one is `null` — unreviewed.
    readings = [{"group": g["group"],
                 "reading": merge(by_component[g["group"]], g["group"], g["paths"]) if g["group"] in by_component else None}
                for g in d["queue"]]
    unreviewed = [r["group"] for r in readings if r["reading"] is None]
    lines = [header(d).rstrip("\n"),
             "CHANGED PATHS:", *("  " + p for p in d["paths"]),
             "REVIEW THREADS:", json.dumps(d["threads"], indent=1),
             "READINGS (one per component, null = unreviewed):", json.dumps(readings, indent=1)]
    return "\n".join(lines) + "\n", unreviewed


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("role", choices=["reader", "component", "coordinator"])
    ap.add_argument("--input", type=pathlib.Path, default=pathlib.Path("review-input.json"))
    ap.add_argument("--group", help="reader / component: the matrix entry's slug")
    ap.add_argument("--readings", type=pathlib.Path, default=pathlib.Path("readings"),
                    help="coordinator: the directory of validated readings")
    args = ap.parse_args()
    d = json.loads(args.input.read_text())

    if args.role in ("reader", "component"):
        if not args.group:
            ap.error(f"{args.role} needs --group")
        if args.role == "component":
            sys.stdout.write(json.dumps(component_args(d, args.group), indent=1) + "\n")
        else:
            sys.stdout.write(reader(d, args.group))
        return 0
    text, unreviewed = coordinator(d, args.readings)
    sys.stdout.write(text)
    if unreviewed:
        print(f"unreviewed: {', '.join(unreviewed)}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
