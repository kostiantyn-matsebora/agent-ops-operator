#!/usr/bin/env python3
"""Validate one reading of the review, and merge a directory of them into a
component's.

A READING THAT IS NOT IN THE STATED SHAPE IS A FAILED READING, BY NAME. Each
per-file or per-verdict `claude -p` process is asked for JSON; what it
returns is checked here, by a program, before anything consolidates it. A
reading that fails leaves no file, and a file it names is `unread` rather
than silently dropped.

THREE SHAPES, ONE PROGRAM:
  file      one file reader's return: {path, findings, declares, references}
  verdict   one thread-verdict pass's return: {threads: [{id, verdict, finding?}]}
  component the merged reading the coordinator reads: {component, findings,
            changedNames, files, threads, unread} — or, for a component the
            build step could not build, {component, unbuilt, findings: [],
            changedNames: [], files: [], threads: [], unread: [...]}

Single-envelope mode validates ONE of the above from the CLI's
`--output-format json` envelope (`structured_output`, or the first `{...}` in
`result` when the reader was not asked for structured output). Merge mode
(the first argument is a DIRECTORY) reads every already-validated
`file-*.json` and `verdict-*.json` in it and produces the component reading,
naming as `unread` every expected path with no valid file reading.
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


def _is_str_list(v) -> bool:
    return isinstance(v, list) and all(isinstance(n, str) for n in v)


def _finding_problems(f, prefix: str) -> list[str]:
    if not isinstance(f, dict):
        return [f"{prefix} is not an object"]
    out: list[str] = []
    for key, typ in (("path", str), ("line", int), ("claim", str)):
        if not isinstance(f.get(key), typ) or isinstance(f.get(key), bool):
            out.append(f"{prefix}.{key} missing or not {typ.__name__}")
    return out


def problems_file(reading: dict) -> list[str]:
    out: list[str] = []
    for key in ("path", "findings", "declares", "references"):
        if key not in reading:
            out.append(f"missing key: {key}")
    if out:
        return out
    if not isinstance(reading["path"], str) or not reading["path"]:
        out.append("path is not a non-empty string")
    if not isinstance(reading["findings"], list):
        out.append("findings is not a list")
    else:
        for i, f in enumerate(reading["findings"]):
            out.extend(_finding_problems(f, f"findings[{i}]"))
    for key in ("declares", "references"):
        if not _is_str_list(reading[key]):
            out.append(f"{key} is not a list of strings")
    return out


def problems_verdict(reading: dict) -> list[str]:
    if "threads" not in reading:
        return ["missing key: threads"]
    if not isinstance(reading["threads"], list):
        return ["threads is not a list"]
    out: list[str] = []
    for i, t in enumerate(reading["threads"]):
        if not isinstance(t, dict) or not isinstance(t.get("id"), str):
            out.append(f"threads[{i}] has no string id")
            continue
        v = t.get("verdict")
        if v not in VERDICTS:
            out.append(f"threads[{i}].verdict is not one of {sorted(VERDICTS)}: {v!r}")
            continue
        finding = t.get("finding")
        if v == "detached":
            if not isinstance(finding, dict):
                out.append(f"threads[{i}] is detached but carries no finding")
            else:
                out.extend(_finding_problems(finding, f"threads[{i}].finding"))
        elif finding is not None:
            out.append(f"threads[{i}] is {v!r} and must carry no finding")
    return out


def problems_component(reading: dict) -> list[str]:
    # THE UNBUILT SHAPE IS VALID ON ITS OWN, AND NEVER BESIDE CONTENT. It is
    # what review-build.sh writes directly, with no reader run at all.
    if "unbuilt" in reading:
        if not isinstance(reading["unbuilt"], str):
            return ["unbuilt is not a string"]
        nonempty = [k for k in ("findings", "changedNames", "files", "threads") if reading.get(k)]
        if nonempty:
            return [f"unbuilt is set alongside non-empty {nonempty}"]
        if "component" not in reading or not isinstance(reading["component"], str):
            return ["missing key: component"]
        return []
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
            out.extend(_finding_problems(f, f"findings[{i}]"))
    if not _is_str_list(reading["changedNames"]):
        out.append("changedNames is not a list of strings")
    if "files" in reading:
        if not isinstance(reading["files"], list):
            out.append("files is not a list")
        else:
            for i, f in enumerate(reading["files"]):
                if not isinstance(f, dict) or not isinstance(f.get("path"), str):
                    out.append(f"files[{i}] has no string path")
                for key in ("declares", "references"):
                    v = f.get(key) if isinstance(f, dict) else None
                    if not _is_str_list(v):
                        out.append(f"files[{i}].{key} is not a list of strings")
    if "unread" in reading and not _is_str_list(reading["unread"]):
        out.append("unread is not a list of strings")
    if not isinstance(reading["threads"], list):
        out.append("threads is not a list")
    else:
        for i, t in enumerate(reading["threads"]):
            if not isinstance(t, dict) or not isinstance(t.get("id"), str):
                out.append(f"threads[{i}] has no string id")
            elif t.get("verdict") not in VERDICTS:
                out.append(f"threads[{i}].verdict is not one of {sorted(VERDICTS)}: {t.get('verdict')!r}")
    return out


PROBLEMS = {"file": problems_file, "verdict": problems_verdict, "component": problems_component}


def merge_dir(directory: pathlib.Path, group: str, expected: list[str]) -> dict:
    """Every valid `file-*.json` and `verdict-*.json` in `directory`, into the
    component's reading. A `detached` verdict's `finding` joins `findings`
    exactly as a blind reader's own finding would — the coordinator dedups
    both the same way."""
    files: list[dict] = []
    findings: list[dict] = []
    changed_names: set[str] = set()
    threads: list[dict] = []
    read_paths: set[str] = set()
    for f in sorted(directory.glob("file-*.json")):
        try:
            r = json.loads(f.read_text())
        except (OSError, json.JSONDecodeError):
            continue
        if not isinstance(r, dict) or problems_file(r):
            continue
        files.append({"path": r["path"], "declares": r["declares"], "references": r["references"]})
        findings.extend(r["findings"])
        changed_names.update(r["declares"])
        read_paths.add(r["path"])
    for f in sorted(directory.glob("verdict-*.json")):
        try:
            r = json.loads(f.read_text())
        except (OSError, json.JSONDecodeError):
            continue
        if not isinstance(r, dict) or problems_verdict(r):
            continue
        for t in r["threads"]:
            threads.append({"id": t["id"], "verdict": t["verdict"]})
            if t["verdict"] == "detached":
                findings.append(t["finding"])
    unread = sorted(set(expected) - read_paths)
    return {"component": group, "findings": findings, "changedNames": sorted(changed_names),
            "files": files, "threads": threads, "unread": unread}


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("envelope", type=pathlib.Path, help="the CLI's json output, or a directory to merge")
    ap.add_argument("--group", help="the component this reading is for")
    ap.add_argument("--paths", default="", help="merge mode: comma-separated expected paths, for `unread`")
    ap.add_argument("--out", type=pathlib.Path, required=True, help="where to write the validated/merged reading")
    ap.add_argument("--kind", choices=["component", "file", "verdict"], default="component",
                    help="single-envelope mode: which shape to validate")
    ap.add_argument("--extract", action="store_true",
                    help="only extract the JSON object from the envelope (the coordinator's posting document); no reading validation")
    args = ap.parse_args()

    if args.extract:
        try:
            envelope = json.loads(args.envelope.read_text())
            doc = extract(envelope) if isinstance(envelope, dict) else None
        except (OSError, ValueError, json.JSONDecodeError) as e:
            print(f"::error::no posting document in the envelope: {e}", file=sys.stderr)
            return 1
        args.out.write_text(json.dumps(doc) + "\n")
        return 0

    if not args.group:
        ap.error("--group is required unless --extract")

    if args.envelope.is_dir():
        expected = [p for p in args.paths.split(",") if p]
        reading = merge_dir(args.envelope, args.group, expected)
        args.out.parent.mkdir(parents=True, exist_ok=True)
        args.out.write_text(json.dumps(reading, indent=1) + "\n")
        print(f"{args.group}: merged {len(reading['files'])} file(s), {len(reading['findings'])} finding(s), "
              f"{len(reading['threads'])} verdict(s), {len(reading['unread'])} unread")
        return 0

    try:
        envelope = json.loads(args.envelope.read_text())
        if not isinstance(envelope, dict):
            raise ValueError("envelope is not an object")
        reading = extract(envelope)
    except (OSError, ValueError, json.JSONDecodeError) as e:
        print(f"::error::reading for {args.group}: {e}", file=sys.stderr)
        return 1

    errs = PROBLEMS[args.kind](reading)
    if errs:
        for e in errs:
            print(f"::error::reading for {args.group}: {e}", file=sys.stderr)
        return 1

    if args.kind == "component" and reading["component"] != args.group:
        # The coordinator matches readings to the queue by this field.
        print(f"::warning::reading names component {reading['component']!r}; recorded as {args.group!r}", file=sys.stderr)
        reading["component"] = args.group

    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(json.dumps(reading, indent=1) + "\n")
    if args.kind == "component" and "unbuilt" in reading:
        print(f"{args.group}: unbuilt")
    elif args.kind == "file":
        print(f"{args.group}: {reading['path']}: {len(reading['findings'])} finding(s)")
    elif args.kind == "verdict":
        print(f"{args.group}: {len(reading['threads'])} verdict(s)")
    else:
        print(f"{args.group}: {len(reading['findings'])} finding(s), {len(reading['changedNames'])} changed name(s), {len(reading['threads'])} verdict(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
