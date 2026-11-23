#!/usr/bin/env python3
"""Coverage of the lines a pull request CHANGED, for one tier.

A repository-wide number blocks every change until the legacy gap is closed, and says
nothing about the code the change added. This reads a tier's coverage output, the
pull request's added lines, and answers one question: of the executable lines this
change added, how many did the tier run.

INPUTS (any mix, repeated)
  --go FILE       a Go cover profile (`go test -coverprofile`, or `go tool covdata textfmt`)
  --lcov FILE     an lcov file (vitest, `node --test`, Playwright via V8)
  --cobertura F   a coverage.py XML report
                  `--lcov` and `--cobertura` take `FILE@PREFIX` when the report names files relative
                  to a directory (vitest relative to `platform/console/ui/`): PREFIX is put before each.
  --map P=D       a Go import-path prefix P that lives in directory D. Every `go.mod` in the tree adds
                  its own module path automatically, so this is for the odd one out.

THE DIFF      --diff-file F (a unified diff, `git diff -U0`) or --base REF (diffs REF...HEAD)
THE DOMAIN    --tier NAME reads `testing.domains.<NAME>` and `testing.excluded_from_coverage`
              from the vocabulary. A changed line outside the domain is not this tier's.

THE OUTPUT is JSON: {tier, covered, total, collected, unmeasured, files: {path: [uncovered lines]}}.
`collected` is False when none of the inputs held a record: a tier that ran and left no data
FAILS, it never reads as "nothing changed" (the instrumented operator wrote no counters, and
an empty directory must not look like a clean diff). `domain_files` is how many changed files
fall in the tier's domain: zero means the tier is not applicable, whatever `collected` says.

A changed file in the domain that NO input mentions is UNMEASURED, and its added lines count
as uncovered, since a source file no test ever loaded is the plainest untested code there is.
Only the paths and line numbers are printed, never a line of source.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import re
import subprocess
import sys
import xml.etree.ElementTree as ET

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import conveyor  # noqa: E402  (the one glob reader)


def parse_diff(text: str) -> dict:
    """path -> set of added line numbers, from a unified diff."""
    out: dict = {}
    path = None
    for raw in text.splitlines():
        if raw.startswith("+++ "):
            tgt = raw[4:].strip()
            path = None if tgt == "/dev/null" else (tgt[2:] if tgt.startswith("b/") else tgt)
            if path is not None:
                out.setdefault(path, set())
        elif raw.startswith("@@") and path is not None:
            m = re.match(r"@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@", raw)
            if m:
                start, count = int(m.group(1)), int(m.group(2) if m.group(2) is not None else 1)
                out[path].update(range(start, start + count))
    return {p: ls for p, ls in out.items() if ls}


def parse_go(text: str) -> dict:
    """import-path file -> {line: covered}. A line is covered when any block on it ran."""
    out: dict = {}
    for raw in text.splitlines():
        if raw.startswith("mode:") or not raw.strip():
            continue
        m = re.match(r"(.+?):(\d+)\.\d+,(\d+)\.\d+ (\d+) (\d+)$", raw.strip())
        if not m:
            continue
        f, a, b, _n, hits = m.group(1), int(m.group(2)), int(m.group(3)), m.group(4), int(m.group(5))
        lines = out.setdefault(f, {})
        for ln in range(a, b + 1):
            lines[ln] = lines.get(ln, False) or hits > 0
    return out


def parse_lcov(text: str) -> dict:
    out: dict = {}
    cur = None
    for raw in text.splitlines():
        if raw.startswith("SF:"):
            cur = out.setdefault(raw[3:].strip(), {})
        elif raw.startswith("DA:") and cur is not None:
            ln, _, rest = raw[3:].partition(",")
            hits = rest.split(",")[0]
            if ln.isdigit() and hits.lstrip("-").isdigit():
                cur[int(ln)] = cur.get(int(ln), False) or int(hits) > 0
        elif raw.startswith("end_of_record"):
            cur = None
    return out


def parse_cobertura(text: str) -> dict:
    out: dict = {}
    root = ET.fromstring(text)
    for cls in root.iter("class"):
        f = cls.get("filename") or ""
        lines = out.setdefault(f, {})
        for ln in cls.iter("line"):
            n = int(ln.get("number"))
            lines[n] = lines.get(n, False) or int(ln.get("hits", "0")) > 0
    return out


def with_prefix(record: dict, prefix: str) -> dict:
    if not prefix:
        return record
    return {prefix.rstrip("/") + "/" + name.lstrip("./"): lines for name, lines in record.items()}


def split_prefix(arg: str) -> tuple:
    path, _, prefix = arg.partition("@")
    return path, prefix


def module_maps(root: pathlib.Path = pathlib.Path(".")) -> list:
    """(module path, directory) for every go.mod under `root`: what a Go profile's import paths mean."""
    out = []
    try:
        files = subprocess.run(["git", "ls-files", "--", "go.mod", "**/go.mod"], capture_output=True, text=True,
                               cwd=root, check=True).stdout.split()
    except (subprocess.CalledProcessError, FileNotFoundError):
        files = []
    if not files:   # not a git checkout, or nothing tracked: read the tree
        files = [str(p.relative_to(root)) for p in root.glob("**/go.mod") if "node_modules" not in p.parts]
    for f in files:
        m = re.search(r"^module\s+(\S+)", (root / f).read_text(), re.M)
        if m:
            parent = pathlib.PurePosixPath(f).parent.as_posix()
            out.append((m.group(1), "" if parent == "." else parent))
    return out


def resolve(name: str, changed, maps) -> str | None:
    """The changed path a coverage file name stands for, or None."""
    name = name.replace("\\", "/")
    for prefix, directory in maps:
        if name.startswith(prefix.rstrip("/") + "/"):
            cand = (directory.rstrip("/") + "/" + name[len(prefix.rstrip("/")) + 1:]).lstrip("./")
            if cand in changed:
                return cand
    if name in changed:
        return name
    best = None
    for p in changed:
        if name.endswith("/" + p) or p.endswith("/" + name.lstrip("./")) or name.lstrip("./") == p:
            # THE LONGEST MATCH, and the name breaks a tie, so the answer never depends on dict order.
            if best is None or (len(p), p) > (len(best), best):
                best = p
    return best


def measure(diff: dict, records, domain, excluded, maps) -> dict:
    """`records` is a list of {file: {line: covered}} (one per input)."""
    in_scope = {p: ls for p, ls in diff.items()
                if conveyor.path_matches(p, domain) and not conveyor.path_matches(p, excluded)}
    merged: dict = {}
    seen_any = False
    for rec in records:
        for name, lines in rec.items():
            seen_any = True
            path = resolve(name, in_scope, maps)
            if path is None:
                continue
            dst = merged.setdefault(path, {})
            for ln, hit in lines.items():
                dst[ln] = dst.get(ln, False) or hit
    covered = total = 0
    files: dict = {}
    unmeasured = []
    for path, added in sorted(in_scope.items()):
        if path in merged:
            exe = {ln: merged[path][ln] for ln in added if ln in merged[path]}
        else:
            unmeasured.append(path)
            exe = {ln: False for ln in added}
        total += len(exe)
        covered += sum(1 for v in exe.values() if v)
        miss = sorted(ln for ln, v in exe.items() if not v)
        if miss:
            files[path] = miss
    return {"covered": covered, "total": total, "collected": seen_any,
            "domain_files": len(in_scope), "unmeasured": unmeasured, "files": files}


def read_diff(args) -> str:
    if args.diff_file:
        return pathlib.Path(args.diff_file).read_text()
    return subprocess.run(["git", "diff", "-U0", "--no-color", f"{args.base}...HEAD"],
                          check=True, capture_output=True, text=True).stdout


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--tier", required=True, choices=list(conveyor.TIERS))
    ap.add_argument("--go", action="append", default=[])
    ap.add_argument("--lcov", action="append", default=[])
    ap.add_argument("--cobertura", action="append", default=[])
    ap.add_argument("--map", action="append", default=[], metavar="PREFIX=DIR")
    ap.add_argument("--diff-file")
    ap.add_argument("--base", default="origin/master")
    ap.add_argument("--vocabulary", type=pathlib.Path,
                    default=pathlib.Path(__file__).resolve().parents[1] / "review-triage.json")
    args = ap.parse_args(argv)

    vocab = json.loads(args.vocabulary.read_text())["testing"]
    records = []
    for f in args.go:
        records.append(parse_go(pathlib.Path(f).read_text()) if pathlib.Path(f).exists() else {})
    for arg in args.lcov:
        f, prefix = split_prefix(arg)
        records.append(with_prefix(parse_lcov(pathlib.Path(f).read_text()), prefix) if pathlib.Path(f).exists() else {})
    for arg in args.cobertura:
        f, prefix = split_prefix(arg)
        records.append(with_prefix(parse_cobertura(pathlib.Path(f).read_text()), prefix) if pathlib.Path(f).exists() else {})
    maps = [tuple(m.split("=", 1)) for m in args.map] + module_maps()
    result = measure(parse_diff(read_diff(args)), records, vocab["domains"][args.tier],
                     vocab["excluded_from_coverage"], maps)
    result["tier"] = args.tier
    json.dump(result, sys.stdout, indent=2, sort_keys=True)
    print()
    return 0


if __name__ == "__main__":
    sys.exit(main())
