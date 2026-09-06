#!/usr/bin/env python3
"""What each model context in a review job WILL HOLD, measured before the
model runs and printed at the top of the job log.

A context's cost is paid on every turn, so its size is the number that
explains a slow job. This prints it for every context, every run, from the
files themselves:

  the SHARED PREFIX every file reader of the job holds: the role, the routed
    rules, the delta specs — read from cache by every process after the first
  ONE FILE READER: the shared prefix (size only) plus its own per-file prompt,
    diff and file — for the entry's LARGEST file, the one that costs most
  ONE VERDICT PASS: for the file with the most open threads, if any
  the COORDINATOR: its role file, CLAUDE.md, the message (readings + threads)

Bytes are exact; tokens are bytes / 4, an estimate stated as one.

    review-context.py component --input review-input.json --group <g> [--base origin/master]
    review-context.py coordinator --input review-input.json --readings <dir>
"""
from __future__ import annotations

import argparse
import json
import pathlib
import subprocess
import sys

HERE = pathlib.Path(__file__).resolve().parent
ROOT = HERE.parents[1]


def _prompt_module():
    import importlib.util
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
    entry = pm._entry(d, group)
    prefix = pm.reader_system(d, group)
    prefix_bytes = len(prefix.encode())
    lines = [f"CONTEXTS FOR {entry['group']}{' chunk ' + entry['chunk'] if entry.get('chunk') else ''} "
             f"({len(entry['paths'])} file(s), each its own process; shared system prefix below is read from cache after the first). "
             "Bytes exact; tokens ≈ bytes/4.",
             f"{'shared system prefix (role + routed rules + delta specs)':<44} {prefix_bytes:>8,} B {tok(prefix_bytes):>8}"]
    if entry["paths"]:
        largest = max(entry["paths"], key=lambda p: size(p))
        msg = pm.reader(d, group, largest)
        since = d.get("since", {}).get(largest) or base
        parts = {"per-file prompt": len(msg.encode()), "diff": diff_size(since, largest), "file": size(largest)}
        lines.append(row(f"  reader (largest file) {largest}", parts))
        vp = pm.verdict_paths(d, group)
        if vp:
            busiest = max(vp, key=lambda p: sum(1 for t in d["threads"] if t["path"] == p and not t["isResolved"]))
            vmsg = pm.verdict(d, group, busiest)
            if vmsg is not None:
                vsince = d.get("since", {}).get(busiest) or base
                vparts = {"verdict message": len(vmsg.encode()), "diff": diff_size(vsince, busiest), "file": size(busiest)}
                lines.append(row(f"  verdict (most threads) {busiest}", vparts))
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
