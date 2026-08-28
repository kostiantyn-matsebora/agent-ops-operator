#!/usr/bin/env python3
"""What each model context in a review job WILL HOLD, measured before the
model runs and printed at the top of the job log.

A context's cost is paid on every turn, so its size is the number that
explains a slow job — and it was invisible: the coordinator's 76 k tokens a
turn were found by downloading an execution artifact and counting, once. This
prints it for every context, every run, from the files themselves:

  per FILE READER:   the role file, CLAUDE.md, the delegation message, the
                     rule files routed to its path, its diff, the file itself
  the COMPONENT SESSION: CLAUDE.md and the workflow instruction (it reads nothing)
  the COORDINATOR:   its role file, CLAUDE.md, the message (readings + threads)

Bytes are exact; tokens are bytes / 4, an estimate stated as one. What a
reader goes on to `Read` beyond this (a sibling it chooses to look at) is not
here — this is the floor a context starts from, not a ceiling.

    review-context.py component --input review-input.json --group <g> [--base origin/master]
    review-context.py coordinator --input review-input.json --readings <dir>
"""
from __future__ import annotations

import argparse
import importlib.util
import json
import pathlib
import subprocess
import sys

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parents[1]


def _prompt_module():
    spec = importlib.util.spec_from_file_location("review_prompt", HERE / "review-prompt.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def size(path: str) -> int:
    p = ROOT / path
    return p.stat().st_size if p.is_file() else 0


def diff_size(base: str, path: str) -> int:
    try:
        out = subprocess.run(["git", "diff", "-M", f"{base}...HEAD", "--", path],
                             capture_output=True, text=True, check=True, cwd=ROOT).stdout
        return len(out.encode())
    except subprocess.CalledProcessError:
        return 0


def tok(b: int) -> str:
    return f"~{b // 4 / 1000:.1f}k"


def row(name: str, parts: dict[str, int]) -> str:
    total = sum(parts.values())
    detail = " + ".join(f"{k} {v:,}" for k, v in parts.items())
    return f"{name:<44} {total:>8,} B {tok(total):>8}   {detail}"


def component(d: dict, group: str, base: str) -> list[str]:
    pm = _prompt_module()
    a = pm.component_args(d, group)
    claude_md = size("CLAUDE.md")
    role = size(".claude/agents/file-reviewer.md")
    lines = [f"CONTEXTS FOR {a['component']}{' chunk ' + a['chunk'] if a.get('chunk') else ''} ({len(a['files'])} file(s) over two queue readers; a queue reader's context is its rules once plus each file it reads in turn — per-file cost below). Bytes exact; tokens ≈ bytes/4.",
             row("component session (runs the workflow)", {"CLAUDE.md": claude_md, "instruction": len(pm.reader(d, group).encode())})]
    for f in a["files"]:
        msg = len(json.dumps({k: v for k, v in f.items() if k != "rules"}).encode()) + 400
        parts = {"role": role, "CLAUDE.md": claude_md, "message": msg,
                 "rules": sum(size(r) for r in f["rules"]),
                 "diff": diff_size(base, f["path"]), "file": size(f["path"])}
        lines.append(row(f"  reader {f['path']}", parts))
    return lines


def coordinator(d: dict, readings: pathlib.Path) -> list[str]:
    pm = _prompt_module()
    text, _ = pm.coordinator(d, readings)
    parts = {"role": size(".claude/agents/review-coordinator.md"), "CLAUDE.md": size("CLAUDE.md"),
             "message (readings + threads)": len(text.encode())}
    return ["CONTEXT FOR THE COORDINATOR. Bytes exact; tokens ≈ bytes/4.", row("coordinator", parts)]


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("role", choices=["component", "coordinator"])
    ap.add_argument("--input", type=pathlib.Path, default=pathlib.Path("review-input.json"))
    ap.add_argument("--group")
    ap.add_argument("--base", default=None)
    ap.add_argument("--readings", type=pathlib.Path, default=pathlib.Path("readings"))
    args = ap.parse_args()
    d = json.loads(args.input.read_text())
    if args.role == "component":
        if not args.group:
            ap.error("component needs --group")
        lines = component(d, args.group, args.base or f"origin/{d['base']}")
    else:
        lines = coordinator(d, args.readings)
    print("\n".join(lines))
    return 0


if __name__ == "__main__":
    sys.exit(main())
