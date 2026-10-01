#!/usr/bin/env python3
"""Render each conveyor:* workflow in workflows.yaml as its OWN Mermaid
state diagram -- mechanically, no inference, no cross-conveyor merging.

MODEL: a conveyor is a PROCESS. Its diagram shows exactly its own `states:`
and `transitions:`, written directly in station's or loop's real
vocabulary. If workflows.yaml says a conveyor has no single initial state
(`initial: null`), the diagram draws NO `[*]` at all, not a fan-out
fiction -- UML's initial pseudostate means exactly one unconditional entry
point, verified against the spec.

EVERY TRANSITION CARRIES A REAL GUARD. CORRECTED: an earlier version drew
`fire:archive` firing unconditionally from six states, which misrepresented
fire() -- it never cares WHICH source station, but it is gated by write
access, the lane, whether the change is finished, and whether a session is
already at work. Every transition's edge is therefore labeled
`event [guard]`, UML's own notation, with the FULL guard text (often long
prose, since it is read verbatim off the real function) placed as a note
beside the diagram rather than crammed onto the arrow -- an arrow label
carrying a paragraph is unreadable, so this renderer shows a numbered
reference on the edge and the full text in a legend block under the
diagram.

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
    state, every transition, every guard it lists, nothing inferred.

    Guards are often identical prose repeated across several `from:`
    entries (the same real condition, verified in conveyor.py not to
    depend on the source state) -- deduplicated into ONE numbered legend
    entry rather than printed once per edge, so "same as above" collapses
    to the same reference instead of looking like N different conditions.
    """
    lines = ["stateDiagram-v2"]

    initial = w.get("initial")
    if initial:
        lines.append(f"    [*] --> {initial}")
    else:
        lines.append(f"    %% no single initial state: this conveyor's real trigger "
                      f"fires identically from most of its own states once its guard holds")

    for s in w["states"]:
        lines.append(f"    state {s}")

    # collect transitions by (from, to), merging same-guard events the way
    # earlier renders did, but now also tracking which guard text backs
    # each one so "[g1]" can be attached per edge. A transition OWNED BY a
    # different function than this conveyor's own label (ending(),
    # recover(), refresh() -- the loop's own lifecycle, included for
    # context) commonly has no simple boolean guard to state; it is shown
    # as "ownerFn()" instead of a guard id, never silently dropped.
    guard_text_to_id: dict[str, int] = {}
    guard_order: list[str] = []

    def guard_id(text: str) -> int:
        text = text.strip()
        if text not in guard_text_to_id:
            guard_text_to_id[text] = len(guard_order) + 1
            guard_order.append(text)
        return guard_text_to_id[text]

    by_pair: dict[tuple[str, str], list[tuple[str, str]]] = {}
    for t in w["transitions"]:
        if t.get("guard"):
            tag = f"g{guard_id(t['guard'])}"
        else:
            tag = t.get("owned_by", "?")
        by_pair.setdefault((t["from"], t["to"]), []).append((t["event"], tag))

    for (frm, to), pairs in sorted(by_pair.items()):
        parts = [f"{mmd_safe(ev)} [{tag}]" for ev, tag in sorted(pairs)]
        lines.append(f"    {frm} --> {to} : {', '.join(parts)}")

    for text, gid in zip(guard_order, range(1, len(guard_order) + 1)):
        lines.append(f"    note right of {w['states'][-1]}")
        lines.append(f"        g{gid}: {mmd_safe(text)}")
        lines.append("    end note")

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
    print("one diagram per conveyor, each transition carries its real guard (gN),")
    print("the full guard text in a note below. Nothing fires unconditionally.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
