#!/usr/bin/env python3
"""Which rule files a reader of a path must READ.

NO READING INHERITS THE RULES. Every `claude -p` in the review excludes
`.claude/rules/*.md` from its context — measured on the coordinator: 34 turns
each re-sending some 76 thousand tokens, of which its readings and threads
were a few thousand and the rest was fifteen rule files it never used. A file
reader is instead TOLD which rules apply to its path, and reads those.

The routing is a program so that it can be checked: every file it names
exists, and every rule file that IS a review criterion is reachable from some
path. A rule no path routes to is one the review has silently stopped
enforcing. The rules that are not criteria are named below, each with why.

    review-rules.py <path>...      the rule files, one per line, for each path (union)
    review-rules.py --check        the two assertions, against the tree
"""
from __future__ import annotations

import argparse
import fnmatch
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
RULES = ".claude/rules"

# Rules that are NOT REVIEW CRITERIA, and why. A reader reads a rule to judge
# a diff against it; these govern something else:
#   - the six SESSION rules: how a person builds, delivers, names a window,
#     keeps a private thing private, looks at a UI, answers in chat;
#   - gotchas: operational lore for somebody running helm or docker — nothing
#     in it is a property of a diff;
#   - structure: a MAP. Its one enforceable rule (a directory is a component,
#     one docs/) is routed to the files that derive the tree, below, and to
#     nothing else — a reader of a Go file does not need the repository map.
NOT_REVIEW = {"build-test", "worktree-delivery", "session-naming", "publication",
              "visual-check", "answering", "gotchas"}

# Every path gets these.
ALWAYS = ["retired-vocabulary"]

# THE SCOPED RULES' OWN `paths:` FRONTMATTER IS THE AUTHORITY for where they
# apply — chart.md, signal-rules.md, palette-and-mark.md each name theirs, and
# the rows below repeat them; `review-rules.test.sh` holds them to it.
#
# Rows ADD to the set — a path matching several rows gets the union. What a
# row names is what a reader of that path judges a diff AGAINST, and nothing
# more: doctrine for the operator's code, the chart rule for the chart, the
# writing rules for prose, the palette rule for the theme.
TABLE: list[tuple[str, list[str]]] = [
    ("platform/manager/internal/ingest/**", ["signal-rules"]),
    ("platform/manager/internal/integration/charttemplate_test.go", ["signal-rules", "chart"]),
    ("platform/manager/**", ["invariants", "terminology", "wiring", "adapters"]),
    ("signals/**", ["signal-rules", "invariants", "terminology", "adapters"]),
    ("channels/**", ["invariants", "terminology", "adapters"]),
    ("gateways/**", ["invariants", "terminology", "adapters"]),
    ("runtimes/**", ["invariants", "terminology", "wiring"]),
    ("platform/console/ui/src/theme/**", ["palette-and-mark"]),
    ("platform/console/ui/src/components/Logo.tsx", ["palette-and-mark"]),
    ("docs/assets/js/**", ["palette-and-mark"]),
    ("docs/_includes/logo.svg", ["palette-and-mark"]),
    ("docs/assets/img/logos/**", ["palette-and-mark"]),
    ("chart/charts/kubernetes/**", ["signal-rules"]),
    ("chart/charts/home-assistant/**", ["signal-rules"]),
    ("platform/**", ["invariants", "terminology", "adapters"]),
    ("chart/**", ["chart", "wiring", "invariants", "terminology"]),
    ("docs/assets/css/**", ["palette-and-mark"]),
    ("docs/**", ["documentation", "terminology", "docs/CLAUDE.md"]),
    (".claude/**", ["authoring", "terminology"]),
    ("openspec/**", ["authoring", "terminology", "documentation"]),
    (".github/components.sh", ["structure"]),
    (".github/workflows/**", ["structure"]),
    ("**/Dockerfile", ["structure"]),
    ("**/go.mod", ["structure"]),
    (".github/**", ["authoring"]),
    ("*", ["authoring", "documentation"]),   # a root file: README, CONTRIBUTING, CLAUDE.md
]


def _file(name: str) -> str:
    return name if "/" in name else f"{RULES}/{name}.md"


def rules_for(path: str) -> list[str]:
    path = path.strip().removeprefix("./")
    out: list[str] = [_file(n) for n in ALWAYS]
    matched = False
    for pattern, names in TABLE:
        if pattern == "*":
            if matched or "/" in path:
                continue
        elif pattern.startswith("**/"):
            if path.rsplit("/", 1)[-1] != pattern[3:]:
                continue
        elif not (fnmatch.fnmatchcase(path, pattern) or (pattern.endswith("/**") and path.startswith(pattern[:-3] + "/"))):
            continue
        matched = True
        for n in names:
            f = _file(n)
            if f not in out:
                out.append(f)
    return out


def check(root: pathlib.Path) -> list[str]:
    errs: list[str] = []
    named = {_file(n) for n in ALWAYS} | {_file(n) for _, ns in TABLE for n in ns}
    for f in sorted(named):
        if not (root / f).is_file():
            errs.append(f"routed to a file that does not exist: {f}")
    for rule in sorted((root / RULES).glob("*.md")):
        if rule.stem in NOT_REVIEW:
            continue
        if f"{RULES}/{rule.name}" not in named:
            errs.append(f"no path routes to {RULES}/{rule.name} — the review has stopped enforcing it")
    return errs


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("paths", nargs="*")
    ap.add_argument("--check", action="store_true")
    ap.add_argument("--root", type=pathlib.Path, default=ROOT)
    args = ap.parse_args()
    if args.check:
        errs = check(args.root)
        for e in errs:
            print(f"::error::{e}", file=sys.stderr)
        print("review-rules: ok" if not errs else f"review-rules: {len(errs)} problem(s)")
        return 1 if errs else 0
    seen: list[str] = []
    for p in args.paths:
        for f in rules_for(p):
            if f not in seen:
                seen.append(f)
    print("\n".join(seen))
    return 0


if __name__ == "__main__":
    sys.exit(main())
