#!/usr/bin/env python3
"""Render each conveyor:* workflow in workflows.desired.yaml as its OWN
Mermaid state diagram -- mechanically, no inference, no cross-conveyor
merging.

MODEL: a conveyor is a PROCESS. Its diagram shows exactly its own `states:`
mapping, written directly in station's or loop's real vocabulary. STATES OWN
THEIR TRANSITIONS: `states:` is keyed by state id, each holding its own
`initial`/`final` flag and its own `transitions:` list -- not a flat list
with a separate name array. If no state in a workflow is marked
`initial: true`, the diagram draws NO single `[*]` entry, not a fan-out
fiction -- UML's initial pseudostate means exactly one unconditional entry
point, verified against the spec.

EVERY EDGE IS AN EVENT, LABELED `event [guard] / owner`, THE GUARD INLINE --
not a numbered reference to a separate note. An edge is the TRANSITION --
its trigger, its guard and who owns it belong ON it, not beside it.

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


def edge_label(ev: str, guard: str | None, owned_by: str | None) -> str:
    """event [guard] / owner -- every part the edge actually has, inline."""
    label = mmd_safe(ev)
    if guard:
        label += f" [{mmd_safe(guard)}]"
    if owned_by:
        label += f" / {mmd_safe(owned_by)}"
    return label


def mmd_id(name: str) -> str:
    """A state ID, unlike a label, cannot carry a literal `.` either --
    `conveyor.fix` as a node name is invalid syntax, not merely ambiguous."""
    return name.replace(".", "_").replace(":", "_")


def render_invoked_submachine(dep_name: str, dep: dict, at_state: str) -> list[str]:
    """A state's `invokes:` names another REAL workflow that runs AS that
    state's own internal behavior -- drawn as UML composite CONTAINMENT, the
    same shape as a reference composite state: a named container holding its
    own [*] entry point leading straight into the invoked workflow, named
    once. The invoked workflow's OWN states/transitions are not flattened in
    here -- that workflow already has its own diagram; this is a pointer
    into it, not a duplicate of it."""
    # The alias is scoped to the PARENT state, not just the invoked workflow's
    # name -- two sibling states can both invoke the same workflow (e.g.
    # conveyor.fix invoked from both implemented_with_issues and merge_failed),
    # and Mermaid treats two composites declaring the identical alias as one
    # shared node, merging their boxes visually. Verified live.
    dep_id = f"{mmd_id(at_state)}__{mmd_id(dep_name)}"
    return [
        f"    state {at_state} {{",
        f"        [*] --> {dep_id}",
        f"        state \"{dep_name}\" as {dep_id}",
        "    }",
    ]


def render_workflow(name: str, w: dict, all_workflows: dict | None = None) -> str:
    """One conveyor, drawn exactly as workflows.desired.yaml declares it -- every
    state, every transition, every guard it lists, nothing inferred.

    COLLAPSE, mechanical not guessed: when every state in this workflow
    produces the IDENTICAL set of (event, guard, owned_by, to) outcomes --
    checked by comparing the real data, never assumed from a count or a
    name -- that is a true "fires from any state" fact (conveyor.py's own
    tables are built this way), and drawing N copies of the same edge is
    noise Mermaid's auto-layout cannot cope with (verified live: a 6-state,
    2-event fan-in rendered as an unreadable tangle). It collapses to ONE
    edge per (event, guard, owned_by, to) from a single synthetic `*` node
    instead. A workflow whose states do NOT all agree is left fully literal
    -- collapsing a real difference would hide it.
    """
    states: dict = w["states"]
    state_ids = list(states.keys())

    lines = [
        "stateDiagram-v2",
        "    direction LR",
        "    classDef stationState fill:#e8d5b5,stroke:#8a6d3b,color:#4a3b1f",
        "    classDef loopState fill:#c9e4de,stroke:#2f6b5e,color:#1a3b33",
    ]
    state_class = "stationState" if w["acts_on"] == "station" else "loopState"
    initial_states = [sid for sid, s in states.items() if s.get("initial")]
    initial = initial_states[0] if initial_states else None

    # outcomes[state] = frozenset of (event, guard, owned_by, to) -- the
    # complete, real effect of being in that state, for the "do all states
    # agree" check below.
    outcomes: dict[str, set[tuple[str, str, str, str]]] = {
        sid: {(t["event"], t.get("guard") or "", t.get("owned_by") or "", t["to"])
              for t in s.get("transitions", [])}
        for sid, s in states.items()
    }
    non_empty = {s: o for s, o in outcomes.items() if o}
    all_same = len(non_empty) > 1 and len(set(map(frozenset, non_empty.values()))) == 1

    # EVERY diagram gets a real [*] start and, where a state truly has no
    # outgoing transition, a real [*] end -- a comment standing in for a
    # missing pseudostate is not valid syntax in that position either
    # (confirmed live).
    has_outgoing = {sid for sid, s in states.items() if s.get("transitions")}
    has_incoming = {t["to"] for s in states.values() for t in s.get("transitions", [])}
    final_states = [sid for sid in state_ids if sid in has_incoming and sid not in has_outgoing]

    if all_same:
        entries = ["any_state"]
    elif initial_states:
        entries = initial_states
    else:
        entries = [state_ids[0]]
    entry = entries[0]
    idx = lines.index("    direction LR") + 1
    for offset, sid in enumerate(entries):
        lines.insert(idx + offset, f"    [*] --> {sid}")
    note_idx = idx + len(entries)
    lines.insert(note_idx, f"    note left of {entry}")
    lines.insert(note_idx + 1, f"        {name}  --  subject: {w['subject']}")
    lines.insert(note_idx + 2, "    end note")

    if all_same:
        shared = next(iter(non_empty.values()))
        lines.append('    state "*" as any_state')
        for sid in state_ids:
            lines.append(f"    state {sid}")
        by_to: dict[str, list[tuple[str, str, str]]] = {}
        for ev, guard, owned_by, to in shared:
            by_to.setdefault(to, []).append((ev, guard, owned_by))
        for to, triples in sorted(by_to.items()):
            for ev, guard, owned_by in sorted(triples):
                lines.append(f"    any_state --> {to} : {edge_label(ev, guard, owned_by)}")
        covered = set(non_empty.keys())
        excluded = [sid for sid in state_ids if sid not in covered]
        if excluded:
            lines.append(f"    note right of any_state")
            lines.append(f"        excludes: {', '.join(excluded)}")
            lines.append(f"    end note")
    else:
        for sid in state_ids:
            lines.append(f"    state {sid}")
        for sid, s in states.items():
            for t in s.get("transitions", []):
                lines.append(f"    {sid} --> {t['to']} : "
                             f"{edge_label(t['event'], t.get('guard'), t.get('owned_by'))}")

    for sid in final_states:
        if not all_same or sid != entry:
            lines.append(f"    {sid} --> [*]")

    colored = list(state_ids) + (["any_state"] if all_same else [])
    lines.append(f"    class {', '.join(colored)} {state_class}")

    # `invokes:` ON A STATE NAMES A REAL SUB-WORKFLOW -- drawn as a nested
    # composite, never left in prose a program cannot follow. An `invokes:`
    # naming a workflow that does not exist is a real inconsistency --
    # surfaced as a visible error marker on the diagram, never silently
    # skipped.
    for sid, s in states.items():
        dep_name = s.get("invokes")
        if not dep_name:
            continue
        dep = (all_workflows or {}).get(dep_name)
        if dep is None:
            lines.append(f"    %% ERROR: {sid}.invokes: {dep_name} -- no such workflow")
            continue
        lines += render_invoked_submachine(dep_name, dep, sid)

    return "\n".join(lines)


def main() -> int:
    workflows = load("workflows.desired.yaml")["workflows"]

    out_dir = HERE / "mermaid"
    out_dir.mkdir(exist_ok=True)
    for old in out_dir.glob("*.mmd"):
        old.unlink()

    for name, w in workflows.items():
        safe = name.replace(".", "_").replace(":", "_")
        (out_dir / f"{safe}.mmd").write_text(render_workflow(name, w, all_workflows=workflows) + "\n")

    print(f"wrote {len(list(out_dir.glob('*.mmd')))} mermaid files to {out_dir}")
    print("one diagram per conveyor, every edge is event [guard] / owner inline.")

    return 0


if __name__ == "__main__":
    sys.exit(main())
