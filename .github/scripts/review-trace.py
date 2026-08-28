#!/usr/bin/env python3
"""What a model step in the review actually did — from its stream-json
execution record, printed in the job log where the job ran.

The read jobs used to record one line: the validator's. When a one-file
component took eight minutes, nothing in the log said where: the session's
own turns, the workflow's wait, or the file reader. This prints all three:

  session   turns, wall-clock, API time, tokens in/out and cached
  workflow  each agent the saved workflow ran — label, state, duration
  result    the final result's structured_output (or text) → --out

    review-trace.py exec.jsonl --out reader-output.json
"""
from __future__ import annotations

import argparse
import json
import pathlib
import sys


def load(path: pathlib.Path) -> list[dict]:
    out = []
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            out.append(json.loads(line))
        except json.JSONDecodeError:
            continue
    return out


def agents(events: list[dict]) -> dict[str, dict]:
    """Every workflow agent, by label: its last seen node plus, measured from
    the progress events, WHEN it started and when its state first read done —
    the node itself carries no end time."""
    seen: dict[str, dict] = {}
    for e in events:
        if e.get("subtype") != "task_progress":
            continue
        now = (e.get("usage") or {}).get("duration_ms")
        for a in e.get("workflow_progress") or []:
            if a.get("type") != "workflow_agent":
                continue
            key = f"{a.get('label')}#{a.get('index')}"
            cur = seen.setdefault(key, {"node": a, "first_ms": now, "done_ms": None})
            cur["node"] = a
            if a.get("state") == "done" and cur["done_ms"] is None:
                cur["done_ms"] = now
            if cur["first_ms"] is None:
                cur["first_ms"] = now
    return seen


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("execution", type=pathlib.Path)
    ap.add_argument("--out", type=pathlib.Path, help="write the final result envelope here")
    args = ap.parse_args()
    ev = load(args.execution)
    results = [e for e in ev if e.get("type") == "result"]
    turns = sum(1 for e in ev if e.get("type") == "assistant")
    tools: dict[str, int] = {}
    for e in ev:
        if e.get("type") == "assistant":
            for c in e.get("message", {}).get("content", []):
                if c.get("type") == "tool_use":
                    tools[c["name"]] = tools.get(c["name"], 0) + 1
    denied = sum(1 for e in ev if e.get("subtype") == "permission_denied")

    print("SESSION")
    if results:
        r = results[-1]
        u = r.get("usage") or {}
        print(f"  turns {r.get('num_turns')} (assistant messages {turns}); wall {(r.get('duration_ms') or 0)/1000:.0f}s; "
              f"api {(r.get('duration_api_ms') or 0)/1000:.0f}s; results {len(results)}")
        print(f"  tokens in {u.get('input_tokens')} · cache read {u.get('cache_read_input_tokens')} · "
              f"cache write {u.get('cache_creation_input_tokens')} · out {u.get('output_tokens')}")
    else:
        print(f"  NO RESULT EVENT — the session ended without a result; assistant messages {turns}")
    print(f"  tool calls {tools}; permission denials {denied}")

    ag = agents(ev)
    if ag:
        print("WORKFLOW AGENTS (start = seconds after the workflow began; dur = until its state first read done)")
        starts = [c["node"].get("startedAt") for c in ag.values() if c["node"].get("startedAt")]
        t0 = min(starts) if starts else 0
        for key, c in ag.items():
            a = c["node"]
            s = a.get("startedAt")
            start_ms = (s - t0) if (s and t0) else c["first_ms"]
            start = f"+{start_ms/1000:.0f}s" if start_ms is not None else "?"
            dur = f"{(c['done_ms'] - start_ms)/1000:.0f}s" if (c["done_ms"] is not None and start_ms is not None) else "?"
            print(f"  {a.get('label')!s:<50} {a.get('state')!s:<9} start {start:>6}  dur {dur:>5}")

    if args.out:
        if results:
            args.out.write_text(json.dumps(results[-1]) + "\n")
        else:
            args.out.write_text("{}\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
