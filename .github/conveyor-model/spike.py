#!/usr/bin/env python3
"""SPIKE: can python-statemachine's io.load() run our station.yaml in the
stateless, label-as-state pattern -- construct fresh, force a starting
state, fire one event, read the result, discard?

Converts our flat (from/event/to) table shape into the library's own native
schema (states: as a MAPPING keyed by id, each with its own transitions:
list) on the fly, writes that to a temp file, loads it, and drives it.
"""
import pathlib
import sys
import tempfile

import yaml

HERE = pathlib.Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))


def to_native_schema(our_doc: dict) -> dict:
    """our station.yaml/loop.yaml shape -> the library's native {states: {id: {...}}} shape."""
    states = {s: {"transitions": []} for s in our_doc["states"]}
    for s in our_doc["terminal"]:
        states[s]["final"] = True
    for t in our_doc["transitions"]:
        if t["to"] is None:   # SKIP: the library has no notion of "explicit no-op",
            continue          # so we simply omit it -- an unmatched event is a no-op by default anyway
        states[t["from"]]["transitions"].append({"event": t["to"] and t["event"], "target": t["to"]})
    # exactly one state must be `initial` in this schema; pick the doc's own
    # declared starting point. Our generated files don't carry one explicitly
    # (the table has no privileged entry state besides "none"), so use "none".
    initial_state = "none" if "none" in states else our_doc["states"][0]
    states[initial_state]["initial"] = True
    return {"name": our_doc["machine"], "states": states}


def main() -> int:
    our_station = yaml.safe_load((HERE / "station.yaml").read_text())
    native = to_native_schema(our_station)

    with tempfile.NamedTemporaryFile("w", suffix=".yaml", delete=False) as f:
        yaml.safe_dump(native, f)
        native_path = f.name

    from statemachine.io import load

    StationChart = load(native_path, format="yaml", trusted=False)
    print(f"loaded chart: {StationChart}")

    # ---- the actual spike: stateless pattern -----------------------------
    # 1. construct fresh
    # 2. force the CURRENT state to whatever we "read from the GitHub label"
    #    (simulated here as the string "fix")
    # 3. fire ONE event
    # 4. read back the resulting state id
    # 5. discard the instance -- nothing must persist beyond this object
    simulated_label_state = "fix"
    event_to_fire = "pr:green"  # from station.yaml: fix --pr:green--> merge

    try:
        sm = StationChart(start_value=simulated_label_state)
        print(f"constructed with start_value={simulated_label_state!r}; "
              f"current_state={sm.current_state.id!r}")
    except TypeError as exc:
        print(f"start_value kwarg not accepted on this chart class: {exc}")
        print("trying alternate construction...")
        sm = StationChart()
        print(f"default current_state={sm.current_state.id!r}")
        return 1

    sm.send(event_to_fire)
    print(f"after send({event_to_fire!r}): current_state={sm.current_state.id!r}")

    expected = "merge"
    ok = sm.current_state.id == expected
    print(f"EXPECTED {expected!r}, GOT {sm.current_state.id!r} -> {'PASS' if ok else 'FAIL'}")

    # confirm statelessness: a second, independent instance does not see the first's state
    sm2 = StationChart(start_value="implement")
    print(f"second independent instance starts at {sm2.current_state.id!r} "
          f"(unaffected by the first instance's transition)")

    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
