#!/usr/bin/env python3
"""Render station.yaml, loop.yaml and workflows.yaml into Mermaid state diagrams.

Mermaid's `stateDiagram-v2` already gives correct UML notation (filled-circle
start, bullseye end, `trigger [guard] / action` labels) with no hand-rolled
layout math -- GitHub, the Artifact viewer and most editors render it
natively. One file per machine, two variants for station/loop:

  mermaid/station.mmd, loop.mmd                the FULL literal table --
      every (state, event) pair, for diffing against the engine's own
      rendering later. Dense by design: these tables are TOTAL.
  mermaid/station.scoped.mmd, loop.scoped.mmd  forward path + the few named
      exceptions worth a reader's attention (the loop stalling, the restart)
      -- the rest of the total table is implementation completeness, not
      domain flow, and is left out of this one on purpose.
  mermaid/workflow_<name>.mmd                  each of the 5 conveyor:*
      workflows, drawn in full -- small enough that literal is also readable.

Usage:
    python3 render_diagram.py          # writes into ./mermaid/
"""
from __future__ import annotations

import pathlib
import sys

import yaml

HERE = pathlib.Path(__file__).resolve().parent


def load(name: str) -> dict:
    return yaml.safe_load((HERE / name).read_text())


def mermaid_for_machine(doc: dict) -> str:
    """station.yaml / loop.yaml shape -> the FULL literal mermaid diagram.

    SKIP transitions (`to: null`) are deliberately NOT drawn as edges --
    they are the reason this diagram is dense (nearly every state answers
    nearly every event), and the full table is already visible as the
    source YAML. What IS preserved here is the real-to-real edges only.
    """
    lines = ["stateDiagram-v2"]
    terminal = set(doc.get("terminal", []))
    for s in terminal:
        lines.append(f"    {s} --> [*]")
    initial = "none" if "none" in doc["states"] else doc["states"][0]
    lines.append(f"    [*] --> {initial}")

    by_pair: dict[tuple[str, str], list[str]] = {}
    for t in doc["transitions"]:
        if t["to"] is not None and t["from"] != t["to"]:
            by_pair.setdefault((t["from"], t["to"]), []).append(t["event"])

    for (frm, to), events in sorted(by_pair.items()):
        label = ", ".join(sorted(events))
        lines.append(f"    {frm} --> {to} : {label}")

    return "\n".join(lines)


def mermaid_scoped(doc: dict, path: list[str], exceptions: list[tuple[str, str, str]]) -> str:
    """The readable view: the real forward sequence plus a short, hand-picked
    list of exceptions -- NOT the total table. See render_diagram.py's
    module docstring and docs/python-statemachine-schema.md for why the
    full table is excluded here on purpose."""
    by_pair = {(t["from"], t["to"]): t["event"] for t in doc["transitions"] if t["to"] is not None}
    lines = ["stateDiagram-v2", f"    [*] --> {path[0]}"]
    terminal = set(doc.get("terminal", []))
    for a, b in zip(path, path[1:]):
        ev = by_pair.get((a, b), "")
        lines.append(f"    {a} --> {b} : {ev}")
    for s in path:
        if s in terminal:
            lines.append(f"    {s} --> [*]")
    for frm, ev, tgt in exceptions:
        lines.append(f"    {frm} --> {tgt} : {ev}")
    return "\n".join(lines)


def mermaid_for_workflow(name: str, wf: dict) -> str:
    lines = ["stateDiagram-v2", "    [*] --> " + wf["initial"]]
    for t in wf["transitions"]:
        label = t["event"]
        if t.get("guard"):
            label += f"  [{t['guard']}]"
        if t.get("action") and t["action"] != "none":
            label += f"  / {t['action']}"
        lines.append(f"    {t['from']} --> {t['to']} : {label}")
    # a state is terminal only if something transitions INTO it and nothing
    # transitions OUT of it -- never assumed from a naming convention.
    targets_only = {s for s in wf["states"]
                    if any(t["to"] == s for t in wf["transitions"])
                    and not any(t["from"] == s for t in wf["transitions"])}
    for s in targets_only:
        lines.append(f"    {s} --> [*]")
    return "\n".join(lines)


def main() -> int:
    station = load("station.yaml")
    loop = load("loop.yaml")
    workflows = load("workflows.yaml")["workflows"]

    out_dir = HERE / "mermaid"
    out_dir.mkdir(exist_ok=True)

    (out_dir / "station.mmd").write_text(mermaid_for_machine(station) + "\n")
    (out_dir / "loop.mmd").write_text(mermaid_for_machine(loop) + "\n")

    (out_dir / "station.scoped.mmd").write_text(mermaid_scoped(
        station,
        path=["none", "implement", "fix", "merge", "archive", "done"],
        exceptions=[("fix", "end:stalled (loop)", "stalled"), ("stalled", "run re-placed", "implement")],
    ) + "\n")
    (out_dir / "loop.scoped.mmd").write_text(mermaid_scoped(
        loop,
        path=["none", "running", "waiting", "mergeable"],
        exceptions=[("running", "end:capped", "capped"), ("running", "end:stalled", "stalled")],
    ) + "\n")

    for name, wf in workflows.items():
        safe = name.replace(".", "_")
        (out_dir / f"workflow_{safe}.mmd").write_text(mermaid_for_workflow(name, wf) + "\n")

    print(f"wrote {len(list(out_dir.glob('*.mmd')))} mermaid files to {out_dir}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
