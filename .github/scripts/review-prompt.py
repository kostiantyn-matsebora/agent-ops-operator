#!/usr/bin/env python3
"""Assemble the delegation message and fixed context for one role of the
review.

`reader-system` — the FIXED, byte-identical context for every file reader of
                  one component's job: the file-reviewer role's body, then
                  the full text of every rule file routed to the component's
                  paths (the union), then the change's delta specs. Emitted
                  once per job to a file; every per-file `claude -p` reads it
                  as `--append-system-prompt`, so the API's prompt cache pays
                  it once and serves it to every reader after the first.
`reader`          — one file's prompt: its path, `since`, the base ref, the
                    component, and the names of the component's other
                    changed paths (carried ones included).
`reader-tools`    — the file-reviewer role's `tools:` line, for
                    `--allowedTools`.
`verdict-system`  — the thread-verdict role's body. No rules: judging a
                    thread is not judging the diff against doctrine.
`verdict`         — one file's unresolved threads, its path and `since`.
`verdict-tools`   — the thread-verdict role's `tools:` line.
`coordinator`     — the whole review's message: changed paths, every thread,
                    carried paths with their standing threads, the
                    invalidation reason, and one reading per component that
                    had a read path this run (`null` where its job produced
                    nothing).
`coordinator --coverage <file>` — instead of the message, writes the
                    coverage input `review-post.py` needs: every path this
                    run actually read, with its quiet count BEFORE this run.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import sys


def header(d: dict) -> str:
    return (f"REPO: {d['repo']}\nPR NUMBER: {d['number']}\nBASE REF: origin/{d['base']}\n"
            f"HEAD SHA: {d.get('headSha') or 'unknown'}\n")


def _load_module(name: str):
    """A hyphenated script file by path — it is not a module name."""
    import importlib.util
    p = pathlib.Path(__file__).resolve().parent / f"{name}.py"
    spec = importlib.util.spec_from_file_location(name.replace("-", "_"), p)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def _role_body(path: pathlib.Path) -> str:
    """A role file's body, frontmatter stripped."""
    text = path.read_text()
    if text.startswith("---\n"):
        end = text.find("\n---\n", 4)
        if end >= 0:
            return text[end + 5:].lstrip("\n")
    return text


def _role_tools(path: pathlib.Path) -> str:
    text = path.read_text()
    for line in text.splitlines():
        if line.startswith("tools:"):
            return line[len("tools:"):].strip()
    return ""


ROOT = pathlib.Path(__file__).resolve().parents[2]
FILE_REVIEWER = ROOT / ".claude/agents/file-reviewer.md"
THREAD_VERDICT = ROOT / ".claude/agents/thread-verdict.md"


def _entry(d: dict, slug: str) -> dict:
    """A matrix entry by slug — a component, or one chunk of it. The component
    it belongs to supplies `kind` and the sibling file names."""
    entry = next((e for e in d.get("entries", []) if e["slug"] == slug), None)
    if entry is None:
        raise SystemExit(f"no such entry in the queue: {slug}")
    comp = next((g for g in d["queue"] if g["group"] == entry["group"]), None)
    if comp is None:
        raise SystemExit(f"entry {slug} names a component not in the queue: {entry['group']}")
    return {**entry, "kind": comp["kind"], "all_paths": comp["paths"]}


def _specs_text(d: dict) -> str:
    parts = []
    for p in d.get("specPaths", []):
        f = ROOT / p
        if f.is_file():
            parts.append(f"### {p}\n\n{f.read_text()}")
    return "\n\n".join(parts)


def reader_system(d: dict, slug: str) -> str:
    """The fixed, byte-identical system prefix for every file reader of one
    component's job — role body, then every rule file the component's paths
    route to (the union), then the delta specs."""
    rules_for = _load_module("review-rules").rules_for
    entry = _entry(d, slug)
    rule_files: list[str] = []
    for p in entry["all_paths"]:
        for r in rules_for(p):
            if r not in rule_files:
                rule_files.append(r)
    parts = [_role_body(FILE_REVIEWER).rstrip("\n")]
    for r in rule_files:
        f = ROOT / r
        text = f.read_text() if f.is_file() else ""
        parts.append(f"## RULE FILE: {r}\n\n{text}")
    specs = _specs_text(d)
    if specs:
        parts.append(f"## DELTA SPECS OF THIS CHANGE\n\n{specs}")
    return "\n\n".join(parts) + "\n"


def reader_tools() -> str:
    return _role_tools(FILE_REVIEWER)


def reader(d: dict, slug: str, path: str) -> str:
    """One file's prompt: coordinates only — the reader's own tools read the
    diff and the file. No thread, no previous finding: every read is an
    independent sample."""
    entry = _entry(d, slug)
    since = d.get("since", {}).get(path) or f"origin/{d['base']}"
    siblings = [p for p in entry["all_paths"] if p != path]
    lines = [
        f"REPO: {d['repo']}", f"PR NUMBER: {d['number']}",
        f"BASE REF: origin/{d['base']}", f"COMPONENT: {entry['group']} ({entry['kind']})",
        f"FILE: {path}", f"SINCE: {since}",
        f"  (diff this file with: git diff -M {since}...HEAD -- {path})",
        "OTHER CHANGED FILES IN THIS COMPONENT (names only — do not read them unless referenced):",
    ]
    lines += [f"  {p}" for p in siblings] if siblings else ["  none"]
    lines.append("Return ONE file reading: {\"path\": ..., \"findings\": [...], \"declares\": [...], \"references\": [...]}.")
    return "\n".join(lines) + "\n"


def verdict_system() -> str:
    return _role_body(THREAD_VERDICT)


def verdict_tools() -> str:
    return _role_tools(THREAD_VERDICT)


def verdict_paths(d: dict, slug: str) -> list[str]:
    """The entry's paths that carry an unresolved thread — the only ones the
    verdict loop runs a process for. A file with none costs nothing."""
    entry = _entry(d, slug)
    open_paths = {t["path"] for t in d["threads"] if not t["isResolved"]}
    return [p for p in entry["paths"] if p in open_paths]


def verdict(d: dict, slug: str, path: str) -> str | None:
    """One file's unresolved threads. `None` when it has none — the caller
    skips the process entirely; a file with no open thread costs nothing."""
    open_threads = [t for t in d["threads"] if t["path"] == path and not t["isResolved"]]
    if not open_threads:
        return None
    since = d.get("since", {}).get(path) or f"origin/{d['base']}"
    lines = [
        f"REPO: {d['repo']}", f"PR NUMBER: {d['number']}", f"BASE REF: origin/{d['base']}",
        f"FILE: {path}", f"SINCE: {since}",
        f"  (diff this file with: git diff -M {since}...HEAD -- {path})",
        "UNRESOLVED THREADS ON THIS FILE:",
        json.dumps([{"id": t["id"], "line": t["line"], "body": t["body"]} for t in open_threads], indent=1),
        "Return {\"threads\": [{\"id\": ..., \"verdict\": \"fixed|standing|gone|detached\", \"finding\"?: {...}}]}, one entry per thread above.",
    ]
    return "\n".join(lines) + "\n"


def _merge_chunks(readings: list[dict], component: str, expected_paths: list[str]) -> dict:
    """A component split across several jobs (`--chunk`), as one reading. A
    chunk that never built is folded in as `unbuilt`, joined."""
    unbuilt = [r["unbuilt"] for r in readings if r.get("unbuilt")]
    if unbuilt:
        unread = sorted(set(p for r in readings for p in r.get("unread", [])) | set(expected_paths))
        return {"component": component, "unbuilt": "\n---\n".join(unbuilt),
                "findings": [], "changedNames": [], "files": [], "threads": [], "unread": unread}
    files = [f for r in readings for f in r.get("files", [])]
    read_paths = {f["path"] for f in files}
    unread = sorted(set(p for r in readings for p in r.get("unread", []))
                    | {p for p in expected_paths if p not in read_paths})
    return {"component": component,
            "findings": [x for r in readings for x in r.get("findings", [])],
            "changedNames": sorted({n for r in readings for n in r.get("changedNames", [])}),
            "files": files,
            "threads": [t for r in readings for t in r.get("threads", [])],
            "unread": unread}


def _load_readings(d: dict, readings_dir: pathlib.Path) -> dict[str, dict]:
    by_component: dict[str, list[dict]] = {}
    for f in sorted(readings_dir.glob("**/*.json")) if readings_dir.exists() else []:
        try:
            r = json.loads(f.read_text())
        except (OSError, json.JSONDecodeError):
            continue
        if isinstance(r, dict) and isinstance(r.get("component"), str):
            by_component.setdefault(r["component"], []).append(r)
    entry_groups = {e["group"] for e in d.get("entries", [])}
    out: dict[str, dict | None] = {}
    for group in entry_groups:
        comp = next((g for g in d["queue"] if g["group"] == group), None)
        expected = comp["paths"] if comp else []
        out[group] = _merge_chunks(by_component[group], group, expected) if group in by_component else None
    return out


def coverage(d: dict, readings_dir: pathlib.Path) -> dict:
    """Every path this run actually read, with its quiet count BEFORE this
    run — from the VALIDATED readings, never the model's answer. An unbuilt
    or absent component contributes nothing: nothing of it was read."""
    by_group = _load_readings(d, readings_dir)
    quiet_at = d.get("quietAt", {})
    paths: dict[str, dict] = {}
    for reading in by_group.values():
        if reading is None or reading.get("unbuilt"):
            continue
        unread = set(reading.get("unread", []))
        for f in reading.get("files", []):
            p = f["path"]
            if p in unread:
                continue
            paths[p] = {"quietBefore": int(quiet_at.get(p, 0) or 0)}
    return {"sha": d.get("headSha") or "", "paths": paths}


def coordinator(d: dict, readings_dir: pathlib.Path) -> tuple[str, list[str]]:
    by_group = _load_readings(d, readings_dir)
    readings = [{"group": g, "reading": r} for g, r in sorted(by_group.items())]
    unreviewed = [g for g, r in by_group.items() if r is None]
    unbuilt = [g for g, r in by_group.items() if r is not None and r.get("unbuilt")]

    carried = d.get("carried", [])
    carried_lines = ["CARRIED PATHS (read at an earlier head; unresolved threads are STANDING by construction):"]
    if carried:
        for c in sorted(carried, key=lambda c: c["path"]):
            carried_lines.append(f"  {c['path']} (read at {c['since']}, quiet {c['quiet']}):")
            open_threads = [t for t in c.get("threads", []) if not t["isResolved"]]
            if open_threads:
                carried_lines.append("    " + json.dumps(open_threads))
            else:
                carried_lines.append("    no open thread")
    else:
        carried_lines.append("  none")

    read_count = len(d.get("since", {})) - sum(len(r["unread"]) for r in by_group.values() if r and r.get("unbuilt"))
    unbuilt_count = sum(len(by_group[g]["unread"]) for g in unbuilt)
    counts_line = (f"COVERAGE: {read_count} of {len(d['paths'])} changed file(s) read this run · "
                   f"{len(carried)} carried (quiet) · {unbuilt_count} in unbuilt component(s)")
    reason = d.get("coverageInvalidated") or ""

    lines = [header(d).rstrip("\n"),
             "CHANGED PATHS:", *("  " + p for p in d["paths"]),
             "REVIEW THREADS:", json.dumps(d["threads"], indent=1),
             *carried_lines,
             counts_line]
    if reason:
        lines.append(f"COVERAGE INVALIDATED: {reason}")
    lines += ["READINGS (one per component with a read path this run; null = unreviewed; "
              "a component the build step could not build carries `unbuilt`):",
              json.dumps(readings, indent=1)]
    return "\n".join(lines) + "\n", unreviewed


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("role", choices=["reader-system", "reader", "reader-tools",
                                      "verdict-system", "verdict", "verdict-paths", "verdict-tools", "coordinator"])
    ap.add_argument("--input", type=pathlib.Path, default=pathlib.Path("review-input.json"))
    ap.add_argument("--group", help="the matrix entry's slug")
    ap.add_argument("--path", help="reader / verdict: the file")
    ap.add_argument("--readings", type=pathlib.Path, default=pathlib.Path("readings"),
                    help="coordinator: the directory of validated readings")
    ap.add_argument("--coverage", type=pathlib.Path,
                    help="coordinator: write the coverage input here instead of the message")
    args = ap.parse_args()

    if args.role == "reader-tools":
        sys.stdout.write(reader_tools() + "\n")
        return 0
    if args.role == "verdict-tools":
        sys.stdout.write(verdict_tools() + "\n")
        return 0
    if args.role == "verdict-system":
        sys.stdout.write(verdict_system())
        return 0

    d = json.loads(args.input.read_text())

    if args.role == "reader-system":
        if not args.group:
            ap.error("reader-system needs --group")
        sys.stdout.write(reader_system(d, args.group))
        return 0
    if args.role == "reader":
        if not args.group or not args.path:
            ap.error("reader needs --group and --path")
        sys.stdout.write(reader(d, args.group, args.path))
        return 0
    if args.role == "verdict-paths":
        if not args.group:
            ap.error("verdict-paths needs --group")
        for p in verdict_paths(d, args.group):
            print(p)
        return 0
    if args.role == "verdict":
        if not args.group or not args.path:
            ap.error("verdict needs --group and --path")
        msg = verdict(d, args.group, args.path)
        if msg is None:
            return 1
        sys.stdout.write(msg)
        return 0

    if args.coverage:
        args.coverage.write_text(json.dumps(coverage(d, args.readings), indent=1) + "\n")
        return 0
    text, unreviewed = coordinator(d, args.readings)
    sys.stdout.write(text)
    if unreviewed:
        print(f"unreviewed: {', '.join(unreviewed)}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
