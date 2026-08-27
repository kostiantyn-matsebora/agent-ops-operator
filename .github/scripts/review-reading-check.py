#!/usr/bin/env python3
"""Validate one component reading of the review, and write it where the
coordinator will find it.

A READING THAT IS NOT IN THE STATED SHAPE IS A FAILED READING, BY NAME. The
reader is a model asked for JSON; what it returns is checked here, by a
program, before anything consolidates it — the validation the workflow
runtime's schema used to do for free when the readers were `agent()` calls.
A reading that fails leaves no file, the job is red under its component's
name, and the coordinator reports the component as unreviewed.

Input is the CLI's `--output-format json` envelope. With `--json-schema` the
reading is in `structured_output`; without it, the reader was asked to return
only the JSON, so the first `{...}` in `result` is taken. Both are accepted,
so a version of the CLI that drops one field does not silently fail every
review.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import sys

VERDICTS = {"fixed", "standing", "gone", "detached"}


def extract(envelope: dict) -> dict:
    """The reading inside the CLI's result envelope."""
    so = envelope.get("structured_output")
    if isinstance(so, dict):
        return so
    text = envelope.get("result")
    if not isinstance(text, str):
        raise ValueError("envelope has neither structured_output nor a text result")
    start = text.find("{")
    if start < 0:
        raise ValueError("result holds no JSON object — the reader returned prose")
    dec = json.JSONDecoder()
    # The first object that parses, scanning forward: a reader that wrote a
    # sentence with a brace in it before the JSON is still readable.
    while start >= 0:
        try:
            obj, _ = dec.raw_decode(text[start:])
            if isinstance(obj, dict):
                return obj
        except json.JSONDecodeError:
            pass
        start = text.find("{", start + 1)
    raise ValueError("result holds no parseable JSON object")


def problems(reading: dict) -> list[str]:
    out: list[str] = []
    for key in ("component", "findings", "changedNames", "threads"):
        if key not in reading:
            out.append(f"missing key: {key}")
    if out:
        return out
    if not isinstance(reading["component"], str) or not reading["component"]:
        out.append("component is not a non-empty string")
    if not isinstance(reading["findings"], list):
        out.append("findings is not a list")
    else:
        for i, f in enumerate(reading["findings"]):
            if not isinstance(f, dict):
                out.append(f"findings[{i}] is not an object")
                continue
            for key, typ in (("path", str), ("line", int), ("claim", str)):
                if not isinstance(f.get(key), typ) or isinstance(f.get(key), bool):
                    out.append(f"findings[{i}].{key} missing or not {typ.__name__}")
    if not isinstance(reading["changedNames"], list) or not all(isinstance(n, str) for n in reading["changedNames"]):
        out.append("changedNames is not a list of strings")
    if not isinstance(reading["threads"], list):
        out.append("threads is not a list")
    else:
        for i, t in enumerate(reading["threads"]):
            if not isinstance(t, dict) or not isinstance(t.get("id"), str):
                out.append(f"threads[{i}] has no string id")
            elif t.get("verdict") not in VERDICTS:
                out.append(f"threads[{i}].verdict is not one of {sorted(VERDICTS)}: {t.get('verdict')!r}")
    return out


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("envelope", type=pathlib.Path, help="the CLI's json output")
    ap.add_argument("--group", required=True, help="the component this reading is for")
    ap.add_argument("--out", type=pathlib.Path, required=True, help="where to write the validated reading")
    args = ap.parse_args()

    try:
        envelope = json.loads(args.envelope.read_text())
        if not isinstance(envelope, dict):
            raise ValueError("envelope is not an object")
        reading = extract(envelope)
    except (OSError, ValueError, json.JSONDecodeError) as e:
        print(f"::error::reading for {args.group}: {e}", file=sys.stderr)
        return 1

    errs = problems(reading)
    if errs:
        for e in errs:
            print(f"::error::reading for {args.group}: {e}", file=sys.stderr)
        return 1
    if reading["component"] != args.group:
        # The coordinator matches readings to the queue by this field.
        print(f"::warning::reading names component {reading['component']!r}; recorded as {args.group!r}", file=sys.stderr)
        reading["component"] = args.group

    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(json.dumps(reading, indent=1) + "\n")
    print(f"{args.group}: {len(reading['findings'])} finding(s), {len(reading['changedNames'])} changed name(s), {len(reading['threads'])} verdict(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
