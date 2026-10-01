#!/usr/bin/env python3
"""Render workflows.yaml (+ loop.yaml + triggers.yaml) into Mermaid state diagrams.

STRUCTURE, verified against conveyor.py's real functions, not assumed:

  conveyor.run has no "invokes" -- that function never existed (see
  workflows.yaml's and triggers.yaml's own correction notes). It is a
  standing fact three SEPARATE decision functions independently re-check:
  fire() (on a label placement), carry_fix() (on a pull request's CI
  completing), carry_archive() (on a pull request merging). So each of
  conveyor.implement / .fix / .archive carries its OWN real entry
  transitions in workflows.yaml -- some from a direct label placement, some
  CARRIED from conveyor:run's standing grant -- and this renderer draws
  every one of them on that workflow's own diagram. There is no single
  "main" diagram any more: five independently-triggerable workflows, five
  diagrams, each complete on its own terms.

  `loop` is never triggered on its own -- only conveyor.fix's `running`
  state starts it -- so it is drawn ONLY as a nested composite state inside
  conveyor.fix's diagram, never as a diagram of its own.

  `station` is not a workflow -- it is STATE, a value the conveyor workflows
  read as a guard and write as an action. It gets no diagram.

MANUAL VS AUTOMATIC, drawn with UML's OWN vocabulary (not an invented one):
  - a SIGNAL event (event_type: signal in triggers.yaml) is a person's (or
    an external system's) own action -- drawn as a plain trigger name,
    `trigger [guard] / action`.
  - a CHANGE event (event_type: change) fires because a condition became
    true, with nobody acting at that moment -- carry_fix() and
    carry_archive() are exactly this: an unconditional GitHub event
    (CI finished, a PR merged) re-checks a standing grant. Drawn with UML's
    own `when(condition)` syntax instead of a bare trigger name, so the
    picture itself shows which edges are automatic.

GOTCHA, found live: Mermaid's stateDiagram-v2 parser treats a literal `:`
ANYWHERE in an edge label as the label delimiter, even inside a nested
composite state and even when quoted or HTML-entity-escaped -- tried
`&colon;`, `&#58;`, and a wrapping `"..."`, all still broke the parse.
conveyor.py's own event names are colon-namespaced (`fire:implement`,
`round:start`, `end:continue`), so every label is sanitized here, replacing
`:` with `.` for DISPLAY ONLY -- the YAML files keep the real event names.

GOTCHA, found live: two SIBLING composite states cannot reuse the same
substate names (idle/running/done), even though each sits in its own
`state X { ... }` block -- rejected as a syntax error regardless of nesting
depth. Composite substates are prefixed with their own label.

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


def build_trigger_index(triggers: list[dict]) -> dict[str, dict]:
    """event name -> its trigger entry, so a transition can ask "is this a
    signal or a change event" without repeating triggers.yaml's own data."""
    return {t["event"]: t for t in triggers}


def transition_label(t: dict, trigger_index: dict[str, dict]) -> str:
    """`trigger [guard] / action` for a signal event, or UML's
    `when(trigger) [guard] / action` for a change event -- the one visible
    difference between a person acting and a condition becoming true."""
    trig = trigger_index.get(t["event"])
    event_text = t["event"]
    if trig and trig.get("event_type") == "change":
        event_text = f"when({event_text})"
    label = event_text
    if t.get("guard"):
        label += f"  [{t['guard']}]"
    if t.get("action") and t["action"] != "none":
        label += f"  / {t['action']}"
    return mmd_safe(label)


def loop_pairs(loop: dict) -> dict[tuple[str, str], list[str]]:
    by_pair: dict[tuple[str, str], list[str]] = {}
    for t in loop["transitions"]:
        if t["to"] is not None:
            by_pair.setdefault((t["from"], t["to"]), []).append(t["event"])
    return by_pair


def render_loop_block(by_pair: dict, exceptions: list[tuple[str, str, str]], indent: str) -> list[str]:
    """The loop machine as a Mermaid composite-state BODY (no `state X {`
    wrapper -- the caller owns that). Every one of loop.yaml's real events
    is a SIGNAL in UML's sense -- none of them is "a condition became true
    with nobody acting"; a round starting, a check completing, a thread
    opening are all themselves the triggering occurrence -- so no when(...)
    wrapping applies inside this block."""
    def ev(a, b):
        return mmd_safe(", ".join(sorted(by_pair[(a, b)])))

    lines = [
        f"{indent}[*] --> none",
        f"{indent}none --> running_ : {ev('none', 'running')}",
        f"{indent}running_ --> waiting : {ev('running', 'waiting')}",
        f"{indent}waiting --> mergeable : {ev('waiting', 'mergeable')}",
    ]
    for frm, event, tgt in exceptions:
        frm_id = "running_" if frm == "running" else frm
        tgt_id = "running_" if tgt == "running" else tgt
        lines.append(f"{indent}{frm_id} --> {tgt_id} : {mmd_safe(event)}")
    lines.append(f"{indent}mergeable --> [*] : person merges")
    return lines


LOOP_EXCEPTIONS = [("running", "end:capped", "capped"), ("running", "end:stalled", "stalled")]


def mermaid_for_workflow(name: str, wf: dict, trigger_index: dict[str, dict], loop: dict | None = None) -> str:
    """One workflow's own complete diagram -- every real entry transition
    workflows.yaml declares, whether it is a direct (signal) label
    placement or a carried (change) grant re-check. `loop` is passed only
    for conveyor.fix."""
    lines = ["stateDiagram-v2", "    [*] --> " + wf["initial"]]

    for t in wf["transitions"]:
        if loop is not None and t["from"] == "running" and t["event"].startswith("loop."):
            continue   # drawn as the edge OUT of the nested composite state instead
        lines.append(f"    {t['from']} --> {t['to']} : {transition_label(t, trigger_index)}")

    if loop is not None:
        lines.append("    state running {")
        lines += render_loop_block(loop_pairs(loop), LOOP_EXCEPTIONS, "        ")
        lines.append("    }")
        done_edge = next(t for t in wf["transitions"] if t["from"] == "running" and t["event"].startswith("loop."))
        lines.append(f"    running --> done : {transition_label(done_edge, trigger_index)}")

    targets_only = {s for s in wf["states"]
                    if any(t["to"] == s for t in wf["transitions"])
                    and not any(t["from"] == s for t in wf["transitions"])}
    for s in targets_only:
        lines.append(f"    {s} --> [*]")

    return "\n".join(lines)


def carried_entries(wf: dict) -> list[dict]:
    """The subset of a workflow's OWN idle->running transitions that fire
    because conveyor:run's grant was read (a `run.label_placed` entry, or
    one whose event name marks it as carried onto/from the pull request or
    issue) -- as opposed to a person placing THAT workflow's own label
    directly. Derived from the real event names workflows.yaml already
    declares, never hardcoded per workflow, so a new carried path is picked
    up automatically."""
    out = []
    for t in wf["transitions"]:
        if t["from"] != "idle":
            continue
        ev = t["event"]
        if ev.startswith("run.") or "carried" in ev:
            out.append(t)
    return out


def mermaid_for_run(run: dict, siblings: dict[str, dict], trigger_index: dict[str, dict]) -> str:
    """conveyor.run's OWN diagram, PLUS the real relationship the audit
    found: while running, its standing grant is what each sibling's CARRIED
    entry (never its direct-label entry, which lives on that sibling's own
    diagram) reads. Each sibling is drawn as a plain referenced state --
    its own standalone diagram already shows its full internals -- with the
    edge into it labeled by the REAL carried transition workflows.yaml
    declares, not a guard this renderer invents."""
    lines = ["stateDiagram-v2", "    [*] --> idle"]
    for t in run["transitions"]:
        lines.append(f"    {t['from']} --> {t['to']} : {transition_label(t, trigger_index)}")

    for name, wf in siblings.items():
        label = name.replace("conveyor.", "")
        for t in carried_entries(wf):
            lines.append(f"    running --> {label} : {transition_label(t, trigger_index)}")
        lines.append(f"    {label} --> running : done")
        lines.append(f"    note right of {label}")
        lines.append(f"        see {name}'s own diagram for its full states")
        lines.append(f"    end note")

    return "\n".join(lines)


def main() -> int:
    loop = load("loop.yaml")
    workflows = load("workflows.yaml")["workflows"]
    triggers = load("triggers.yaml")["triggers"]
    trigger_index = build_trigger_index(triggers)

    out_dir = HERE / "mermaid"
    out_dir.mkdir(exist_ok=True)
    for old in out_dir.glob("*.mmd"):
        old.unlink()

    for name, wf in workflows.items():
        safe = name.replace(".", "_")
        l = loop if name == "conveyor.fix" else None
        (out_dir / f"{safe}.mmd").write_text(mermaid_for_workflow(name, wf, trigger_index, loop=l) + "\n")

    siblings = {k: v for k, v in workflows.items()
                if k in ("conveyor.implement", "conveyor.fix", "conveyor.archive")}
    (out_dir / "conveyor_run_and_siblings.mmd").write_text(
        mermaid_for_run(workflows["conveyor.run"], siblings, trigger_index) + "\n")

    print(f"wrote {len(list(out_dir.glob('*.mmd')))} mermaid files to {out_dir}")
    print("every workflow's standalone diagram shows ALL its real entries: signal (a")
    print("person's label) and change (when(...), a carried grant re-checked on a")
    print("GitHub event). conveyor_run_and_siblings.mmd shows ONLY the carried edges,")
    print("each one the real transition workflows.yaml declares -- not an invented guard.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
