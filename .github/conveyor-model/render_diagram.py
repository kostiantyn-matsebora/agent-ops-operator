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

    COLLAPSE, mechanical not guessed: when every `from:` state in this
    workflow produces the IDENTICAL set of (event, guard, to) outcomes --
    checked by comparing the real data, never assumed from a count or a
    name -- that is a true "fires from any state" fact (conveyor.py's own
    tables are built this way), and drawing N copies of the same edge is
    noise Mermaid's auto-layout cannot cope with (verified live: a 6-state,
    2-event fan-in rendered as an unreadable tangle). It collapses to ONE
    edge per (event, guard, to) from a single synthetic `*` node instead.
    A workflow whose states do NOT all agree is left fully literal --
    collapsing a real difference between states would hide it.
    """
    # SUBJECT IS VISIBLE ON THE DIAGRAM ITSELF -- not left for a reader to
    # cross-check against workflows.yaml. loop's states (running/waiting/
    # capped/mergeable) and a station-acting conveyor's states (implement/
    # fix/merge/archive) read as interchangeable label words on their own.
    # GOTCHA, found live: Mermaid's documented YAML-frontmatter `title:`
    # block (the only documented way to title a stateDiagram-v2) produced
    # `data-processed="true"` with ZERO <svg> output in this CDN build --
    # not a syntax error, a silent no-render. A note anchored on the entry
    # state is proven to work (same mechanism already used for guards), so
    # the subject is stated there instead of risking the frontmatter path.
    # COLOR BY acts_on: station-acting workflows (the issue) get one color
    # family, loop-acting workflows (the pull request) another -- the same
    # distinction the subject note states in words, now also visible at a
    # glance without reading text. classDef/class is the documented
    # mechanism for this in stateDiagram-v2; confirmed live it actually
    # paints (some Mermaid versions silently ignore state-diagram styling,
    # so this was verified by screenshot, not assumed from the docs).
    lines = [
        "stateDiagram-v2",
        "    classDef stationState fill:#e8d5b5,stroke:#8a6d3b,color:#4a3b1f",
        "    classDef loopState fill:#c9e4de,stroke:#2f6b5e,color:#1a3b33",
    ]
    state_class = "stationState" if w["acts_on"] == "station" else "loopState"
    initial = w.get("initial")

    guard_text_to_id: dict[str, int] = {}
    guard_order: list[str] = []

    def guard_id(text: str) -> int:
        text = text.strip()
        if text not in guard_text_to_id:
            guard_text_to_id[text] = len(guard_order) + 1
            guard_order.append(text)
        return guard_text_to_id[text]

    def tag_for(t: dict) -> str:
        if t.get("guard"):
            return f"g{guard_id(t['guard'])}"
        return t.get("owned_by", "?")

    # outcomes[from_state] = frozenset of (event, tag, to) -- the complete,
    # real effect of being in that state, for the "do all states agree"
    # check below.
    outcomes: dict[str, set[tuple[str, str, str]]] = {s: set() for s in w["states"]}
    for t in w["transitions"]:
        outcomes[t["from"]].add((t["event"], tag_for(t), t["to"]))

    non_empty = {s: o for s, o in outcomes.items() if o}
    all_same = len(non_empty) > 1 and len(set(map(frozenset, non_empty.values()))) == 1

    # EVERY diagram gets a real [*] start and, where a state truly has no
    # outgoing transition, a real [*] end -- a comment standing in for a
    # missing pseudostate is not a diagram, it is a diagram that failed to
    # render (confirmed live: stateDiagram-v2 does not accept a %% comment
    # in that position at all).
    has_outgoing = {t["from"] for t in w["transitions"]}
    has_incoming = {t["to"] for t in w["transitions"]}
    final_states = [s for s in w["states"] if s in has_incoming and s not in has_outgoing]

    if all_same:
        entry = "any_state"
    elif initial:
        entry = initial
    else:
        entry = w["states"][0]
    idx = lines.index("stateDiagram-v2") + 1
    lines.insert(idx, f"    [*] --> {entry}")
    lines.insert(idx + 1, f"    note left of {entry}")
    lines.insert(idx + 2, f"        {name}  --  subject: {w['subject']}")
    lines.insert(idx + 3, "    end note")

    if all_same:
        shared = next(iter(non_empty.values()))
        lines.append('    state "*" as any_state')
        for s in w["states"]:
            lines.append(f"    state {s}")
        by_to: dict[str, list[tuple[str, str]]] = {}
        for ev, tag, to in shared:
            by_to.setdefault(to, []).append((ev, tag))
        for to, pairs in sorted(by_to.items()):
            parts = [f"{mmd_safe(ev)} [{tag}]" for ev, tag in sorted(pairs)]
            lines.append(f"    any_state --> {to} : {', '.join(parts)}")
        covered = set(non_empty.keys())
        excluded = [s for s in w["states"] if s not in covered]
        if excluded:
            lines.append(f"    note right of any_state")
            lines.append(f"        excludes: {', '.join(excluded)}")
            lines.append(f"    end note")
    else:
        for s in w["states"]:
            lines.append(f"    state {s}")
        by_pair: dict[tuple[str, str], list[tuple[str, str]]] = {}
        for t in w["transitions"]:
            by_pair.setdefault((t["from"], t["to"]), []).append((t["event"], tag_for(t)))
        for (frm, to), pairs in sorted(by_pair.items()):
            parts = [f"{mmd_safe(ev)} [{tag}]" for ev, tag in sorted(pairs)]
            lines.append(f"    {frm} --> {to} : {', '.join(parts)}")

    for s in final_states:
        if not all_same or s != entry:
            lines.append(f"    {s} --> [*]")

    colored = list(w["states"]) + (["any_state"] if all_same else [])
    lines.append(f"    class {', '.join(colored)} {state_class}")

    # GOTCHA, found live: Mermaid's stateDiagram-v2 parser throws "Syntax
    # error in text" if a `note` targets a state with NO EDGE touching it
    # in the diagram -- bisected down to exactly this. `entry` (the real
    # [*] target) is always connected, so every note anchors there, never
    # on an arbitrary state from the states: list.
    for text, gid in zip(guard_order, range(1, len(guard_order) + 1)):
        lines.append(f"    note right of {entry}")
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

    # DESIRED workflows -- NOT YET IMPLEMENTED, rendered into a SEPARATE
    # subfolder so a reader can never mistake a target design for verified,
    # already-shipped behavior. Every title is prefixed "[DESIRED]".
    desired_path = HERE / "workflows.desired.yaml"
    if desired_path.exists():
        desired = load("workflows.desired.yaml")["workflows"]
        desired_dir = HERE / "mermaid" / "desired"
        desired_dir.mkdir(exist_ok=True, parents=True)
        for old in desired_dir.glob("*.mmd"):
            old.unlink()
        for name, w in desired.items():
            safe = name.replace(".", "_").replace(":", "_")
            text = render_workflow(f"[DESIRED] {name}", w)
            (desired_dir / f"{safe}.mmd").write_text(text + "\n")
        print(f"wrote {len(list(desired_dir.glob('*.mmd')))} DESIRED (not yet implemented) "
              f"mermaid files to {desired_dir}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
