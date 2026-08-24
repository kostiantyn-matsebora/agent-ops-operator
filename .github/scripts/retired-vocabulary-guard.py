#!/usr/bin/env python3
"""Retired-vocabulary guard.

`openspec/specs/` is PUBLISHED and read as the current contract, and so are the
site's pages. A removed CRD field, a withdrawn rule or a superseded command that
reappears there as a CURRENT CLAIM tells a stranger the project does something it
stopped doing — which is worse than saying nothing, because a spec is trusted.

WHY THIS ONE IS A DENYLIST, while the publication guard next to it is an
allowlist: that guard protects things naming them would publish, so it can only
describe what is PERMITTED. Everything here is public by construction. Listing it
costs nothing and the list IS the value — it is the record of what this project
stopped doing, in the one place that fails a build when someone brings it back.

RECORDING A REMOVAL IS NOT ASSERTING IT, and the whole difficulty is telling the
two apart. `pipeline-model` says the oldest-claimant tiebreak is REMOVED and is
correct; `channel-type-model` said a default profile "comes from its oldest Ready
Pipeline" and was wrong. Same phrase, opposite verdicts. No pattern separates
them, so each term carries an `allow` list of words that mark the text as a
RECORD, and a match near one is passed.

THE UNIT IS A WINDOW, NOT A LINE AND NOT A PARAGRAPH. Prose here is hard-wrapped,
so the word marking a sentence as a record routinely lands on the line above or
below the retired name — a line-scoped guard fails on correct text for a reason
nobody can see from the message, which is how a guard gets turned off. A
PARAGRAPH-scoped one goes too far the other way: one "removed" anywhere in a long
paragraph would pass an assertion smuggled into its other end, and the paragraph
that lists every retired Channel field is exactly where such an assertion would
land. So the allow words are searched over the matched line and its immediate
neighbours, which is the span a wrapped sentence actually occupies.

That makes the guard deliberately weak in one direction: a sentence recording a
removal can smuggle an assertion past it. That trade is correct — a guard that
fails on every mention of a retired name would be turned off within a week, and
then it would catch nothing at all.

Usage:
    retired-vocabulary-guard.py                  scan the configured paths
    retired-vocabulary-guard.py --show           ... and print the matched line
    retired-vocabulary-guard.py --counts         per-term counts, nothing else
    retired-vocabulary-guard.py --path <p> [...] scan named paths instead

Exit status is 1 when anything is reported.
"""

import argparse
import glob
import json
import os
import re
import sys
from collections import Counter

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
CONFIG = os.path.join(ROOT, ".github", "retired-vocabulary.json")

# Fenced code blocks are EXEMPT. A generated manifest or a migration example may
# legitimately show the old field being rejected, and a guard that failed on the
# example teaching the migration would be arguing with the documentation.
FENCE = re.compile(r"^\s*(```|~~~)")


def load():
    with open(CONFIG) as fh:
        cfg = json.load(fh)
    terms = []
    for t in cfg["terms"]:
        flags = 0 if t.get("case") else re.IGNORECASE
        terms.append({
            "id": t["id"],
            "says": t["says"],
            "pattern": re.compile(t["pattern"], flags),
            "allow": [re.compile(a, re.IGNORECASE) for a in t.get("allow", [])],
        })
    return cfg["scan"], cfg.get("skip", []), terms


def files(patterns, skip=()):
    """Every configured path, minus the ones `skip` names.

    THE CHANGELOG IS SKIPPED, and it is the only file that is. Its job is to
    record what a version REMOVED, so it accumulates every retired name this
    guard exists to catch — and an entry for a released version must not be
    edited to satisfy a guard added afterwards, because somebody may be
    following it. The archives under `changelog/` were never scanned for the
    same reason: `docs/*.md` does not recurse.
    """
    skipped = {os.path.join(ROOT, s) for s in skip}
    seen = []
    for pat in patterns:
        for path in sorted(glob.glob(os.path.join(ROOT, pat), recursive=True)):
            if os.path.isfile(path) and path not in seen and path not in skipped:
                seen.append(path)
    return seen


def paragraphs(path):
    """Yield (first line number, [lines]) per paragraph, skipping fenced blocks.

    A paragraph is a run of non-blank lines. That is the unit an allow word has
    to be searched over, because the prose is hard-wrapped and the word marking a
    sentence as a record lands wherever the wrap put it.
    """
    para, start, in_fence = [], 0, False
    with open(path, encoding="utf-8") as fh:
        for n, raw in enumerate(fh, 1):
            line = raw.rstrip("\n")
            if FENCE.match(line):
                if para:
                    yield start, para
                    para, start = [], 0
                in_fence = not in_fence
                continue
            if in_fence:
                continue
            if not line.strip():
                if para:
                    yield start, para
                    para, start = [], 0
                continue
            if not para:
                start = n
            para.append(line)
    if para:
        yield start, para


# WINDOW is how many lines either side of a match are joined before the allow
# words are searched. One is what a hard-wrapped sentence spans.
WINDOW = 1

# SPAN bounds that search in CHARACTERS, because some spec requirements are a
# single unwrapped line thousands of characters long. Without it, one "removed"
# at the start of such a line passes an assertion at its end — which is precisely
# the paragraph where a reintroduced field name would be written.
SPAN = 240


def records_removal(term, around, match):
    """Does the text NEAR this match mark it as a record rather than a claim?"""
    lo = max(0, match.start() - SPAN)
    hi = min(len(around), match.end() + SPAN)
    near = around[lo:hi]
    return any(a.search(near) for a in term["allow"])


def scan(path, terms):
    """Yield (line number, term, line) for every asserted use of a retired name."""
    hits = []
    for start, lines in paragraphs(path):
        for offset, line in enumerate(lines):
            lo = max(0, offset - WINDOW)
            before = " ".join(lines[lo:offset])
            around = (before + " " + line if before else line)
            hi = min(len(lines), offset + WINDOW + 1)
            after = " ".join(lines[offset + 1:hi])
            if after:
                around = around + " " + after
            shift = len(before) + 1 if before else 0
            for term in terms:
                for m in term["pattern"].finditer(line):
                    at = re.compile(re.escape(m.group(0)))
                    inwin = at.search(around, shift + m.start()) or m
                    if records_removal(term, around, inwin):
                        continue  # this sentence RECORDS the removal
                    hits.append((start + offset, term, line))
                    break
    return hits


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--show", action="store_true",
                    help="print the matched line (local fixing; noisy in a log)")
    ap.add_argument("--counts", action="store_true", help="per-term counts only")
    ap.add_argument("--path", nargs="+", help="scan these paths instead of the configured set")
    args = ap.parse_args()

    scan_globs, skip, terms = load()
    targets = args.path if args.path else files(scan_globs, skip)

    counts = Counter()
    reported = []
    for path in targets:
        rel = os.path.relpath(path, ROOT)
        for n, term, line in scan(path, terms):
            counts[term["id"]] += 1
            reported.append((rel, n, term, line))

    if args.counts:
        for tid, n in sorted(counts.items()):
            print(f"{tid}: {n}")
        print(f"total: {sum(counts.values())}")
        return 1 if counts else 0

    if not reported:
        print(f"retired-vocabulary guard: clean ({len(targets)} files)")
        return 0

    print("Retired vocabulary asserted as a current claim:\n")
    for rel, n, term, line in reported:
        print(f"  {rel}:{n}  [{term['id']}]")
        print(f"      {term['says']}")
        if args.show:
            print(f"      > {line.strip()}")
    print(f"\n{len(reported)} occurrence(s). Each is one of two things:")
    print("  - the retired name asserted as current -> write what replaced it;")
    print("  - a RECORD that it was removed -> say so in the same SENTENCE, and")
    print("    the guard passes (it looks for words like 'removed', 'retired',")
    print("    'no longer' on the matched line or the one either side of it).")
    print("\nRun with --show locally to see the lines.")
    return 1


if __name__ == "__main__":
    sys.exit(main())
