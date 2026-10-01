#!/usr/bin/env python3
"""Render each conveyor:* workflow in workflows.yaml as its OWN Mermaid
state diagram -- mechanically, no inference, no cross-conveyor merging.

MODEL: a conveyor is a PROCESS. Its diagram shows exactly its own `states:`
and `transitions:`, written directly in station's or loop's real
vocabulary -- never a separate invented idle/running/done, never a
rendering decision this script makes on its own. If workflows.yaml says a
conveyor has no single initial state (`initial: null`, several real
transitions each a genuine independent entry, per UML's own rule that an
initial pseudostate means EXACTLY ONE unconditional entry point -- verified
against the spec, not assumed), the diagram draws NO `[*]` at all, not a
fan-out fiction.

station.yaml / loop.yaml themselves are NOT rendered as one shared diagram
here. Five conveyors sharing one machine (station or loop) each tell their
OWN story about it; merging all five into one picture was tried and
rejected as unreadable noise with no single owner.

GOTCHA, found live: Mermaid's stateDiagram-v2 parser treats a literal `:`
ANYWHERE in an edge label as the label delimiter, even quoted or
HTML-entity-escaped. conveyor.py's own event names are colon-namespaced, so
every label is sanitized here, replacing `:` with `.` for DISPLAY ONLY --
the YAML files keep the real event names.

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


def mmd_safe(label: str) -> str:
    """Mermaid's stateDiagram-v2 breaks on a literal `:` anywhere in a label
    (see module docstring) -- never emit one un-sanitized."""
    return label.replace(":", ".")


def render_workflow(name: str, w: dict) -> str:
    """One conveyor, drawn exactly as workflows.yaml declares it -- every
    state it lists, every transition it lists, nothing inferred."""
    lines = ["stateDiagram-v2"]

    initial = w.get("initial")
    if initial:
        lines.append(f"    [*] --> {initial}")
    else:
        lines.append(f"    %% no single initial state: this conveyor's real trigger "
                      f"fires identically from most of its own states (see its own note:)")

    for s in w["states"]:
        lines.append(f"    state {s}")

    by_pair: dict[tuple[str, str], list[str]] = {}
    for t in w["transitions"]:
        by_pair.setdefault((t["from"], t["to"]), []).append(t["event"])

    for (frm, to), events in sorted(by_pair.items()):
        label = ", ".join(mmd_safe(e) for e in sorted(events))
        lines.append(f"    {frm} --> {to} : {label}")

    return "\n".join(lines)


def main() -> int:
    workflows = load("workflows.yaml")["workflows"]

    out_dir = HERE / "mermaid"
    out_dir.mkdir(exist_ok=True)
    for old in out_dir.glob("*.mmd"):
        old.unlink()

    for name, w in workflows.items():
        safe = name.replace(".", "_").replace(":", "_")
        (out_dir / f"{safe}.mmd").write_text(render_workflow(name, w) + "\n")

    print(f"wrote {len(list(out_dir.glob('*.mmd')))} mermaid files to {out_dir}")
    print("one diagram per conveyor -- each a direct, mechanical render of its own")
    print("states:/transitions: block in workflows.yaml, nothing inferred or merged.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
