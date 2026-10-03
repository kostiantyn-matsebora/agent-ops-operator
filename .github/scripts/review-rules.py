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
#   - the SEVEN SESSION rules: how a person builds, delivers, names a window,
#     keeps a private thing private, looks at a UI, answers in chat, and works
#     in a cloud session rather than on the workstation;
#   - gotchas: operational lore for somebody running helm or docker — nothing
#     in it is a property of a diff;
#   - structure: a MAP. Its one enforceable rule (a directory is a component,
#     one docs/) is routed to the files that derive the tree, below, and to
#     nothing else — a reader of a Go file does not need the repository map.
NOT_REVIEW = {"build-test", "worktree-delivery", "session-naming", "publication",
              "visual-check", "answering", "gotchas", "remote-session"}

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
    ("tools/conveyor-engine/**", ["conveyor-engine"]),
    ("platform/manager/internal/ingest/**", ["signal-rules"]),
    ("platform/manager/internal/integration/charttemplate_test.go", ["signal-rules", "chart"]),
    ("platform/manager/**", ["invariants", "terminology", "wiring", "adapters", "writing"]),
    ("signals/**", ["signal-rules", "invariants", "terminology", "adapters", "writing"]),
    ("channels/**", ["invariants", "terminology", "adapters", "writing"]),
    ("gateways/**", ["invariants", "terminology", "adapters", "writing"]),
    ("runtimes/**", ["invariants", "terminology", "wiring", "writing"]),
    ("platform/console/ui/src/theme/**", ["palette-and-mark"]),
    ("platform/console/ui/src/components/Logo.tsx", ["palette-and-mark"]),
    ("docs/assets/js/**", ["palette-and-mark"]),
    ("docs/_includes/logo.svg", ["palette-and-mark"]),
    ("docs/assets/img/logos/**", ["palette-and-mark"]),
    ("chart/charts/kubernetes/**", ["signal-rules"]),
    ("chart/charts/home-assistant/**", ["signal-rules"]),
    ("platform/**", ["invariants", "terminology", "adapters", "writing"]),
    ("chart/**", ["chart", "wiring", "invariants", "terminology", "writing"]),
    ("docs/assets/css/**", ["palette-and-mark"]),
    ("docs/**", ["documentation", "writing", "terminology", "docs/CLAUDE.md"]),
    (".claude/**", ["authoring", "writing", "terminology"]),
    ("openspec/**", ["authoring", "writing", "terminology", "documentation", "change-tests"]),
    (".github/components.sh", ["structure"]),
    (".github/workflows/**", ["structure"]),
    ("**/Dockerfile", ["structure"]),
    ("**/go.mod", ["structure"]),
    (".github/**", ["authoring", "writing"]),
    ("*", ["authoring", "writing", "documentation"]),   # a root file: README, CONTRIBUTING, CLAUDE.md
]

AGENTS = ".claude/agents"

# WHICH ROLE'S REVIEW CRITERIA a reader of a path holds, beside the rules.
# FIRST MATCH WINS per path — a path holds at most one role's bar, and the
# component's job unions across its paths. The criteria are the role file's
# own `## Review criteria` section, never a rule restated (the rules above
# already ride the same prefix). A path matching no row gets no role, and the
# prefix is exactly what it was before roles existed.
ROLE_TABLE: list[tuple[str, str]] = [
    ("platform/manager/api/v1alpha1/**", "api-architect"),
    ("openspec/specs/**", "api-architect"),
    ("openspec/changes/*/specs/**", "api-architect"),
    ("docs/contracts.md", "api-architect"),
    ("**/*_test.go", "testing-specialist"),
    ("test/**", "testing-specialist"),
    ("platform/manager/test/**", "testing-specialist"),
    (".github/tests/**", "testing-specialist"),
    ("platform/console/ui/**", "frontend-developer"),
    ("docs/_layouts/**", "frontend-developer"),
    ("docs/_includes/**", "frontend-developer"),
    ("docs/assets/**", "frontend-developer"),
    ("docs/_data/**", "frontend-developer"),
    ("chart/**", "deployment-engineer"),
    (".github/workflows/**", "deployment-engineer"),
    (".github/actions/**", "deployment-engineer"),
    (".github/docker/**", "deployment-engineer"),
    ("**/Dockerfile", "deployment-engineer"),
    # The CI and review scripts are Python programs with their own suite, not
    # workflow YAML or chart templates, so the code-shaped bar fits them.
    (".github/scripts/**", "backend-developer"),
    ("platform/**", "backend-developer"),
    ("signals/**", "backend-developer"),
    ("channels/**", "backend-developer"),
    ("gateways/**", "backend-developer"),
    ("runtimes/**", "backend-developer"),
]

ROLE_CRITERIA_HEADING = "## Review criteria"


def _file(name: str) -> str:
    return name if "/" in name else f"{RULES}/{name}.md"


def _match(path: str, pattern: str) -> bool:
    if pattern.startswith("**/"):
        return fnmatch.fnmatchcase(path.rsplit("/", 1)[-1], pattern[3:])
    if pattern.endswith("/**") and path.startswith(pattern[:-3] + "/"):
        return True
    return fnmatch.fnmatchcase(path, pattern)


def role_for(path: str) -> str | None:
    """The role whose review criteria a reader of this path holds — first
    matching row, or None for a path no role fits."""
    path = path.strip().removeprefix("./")
    for pattern, role in ROLE_TABLE:
        if _match(path, pattern):
            return role
    return None


def role_criteria(root: pathlib.Path, role: str) -> str:
    """The role file's `## Review criteria` section alone, frontmatter
    stripped — empty when the file or the section is missing. The
    implementer's workflow and hand-back never reach a reader."""
    f = root / AGENTS / f"{role}.md"
    if not f.is_file():
        return ""
    text = f.read_text()
    if text.startswith("---\n"):
        end = text.find("\n---\n", 4)
        if end >= 0:
            text = text[end + 5:]
    lines = text.splitlines()
    start = next((i for i, ln in enumerate(lines)
                  if ln.strip() == ROLE_CRITERIA_HEADING), None)
    if start is None:
        return ""
    body = [lines[start]]
    for ln in lines[start + 1:]:
        if ln.startswith("## "):
            break
        body.append(ln)
    return "\n".join(body).strip() + "\n"


def rules_for(path: str) -> list[str]:
    path = path.strip().removeprefix("./")
    out: list[str] = [_file(n) for n in ALWAYS]
    matched = False
    for pattern, names in TABLE:
        # `*` IS THE ONE PATTERN `_match` DOES NOT KNOW: a root file, matched
        # only once nothing more specific already did. Every other pattern is
        # the SAME glob `_match` already answers for `ROLE_TABLE`, so this is
        # that one question asked twice rather than a second implementation.
        if pattern == "*":
            if matched or "/" in path:
                continue
        elif not _match(path, pattern):
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
    for role in sorted({r for _, r in ROLE_TABLE}):
        f = root / AGENTS / f"{role}.md"
        if not f.is_file():
            errs.append(f"role routed to a file that does not exist: {AGENTS}/{role}.md")
        elif not role_criteria(root, role):
            errs.append(f"{AGENTS}/{role}.md has no '{ROLE_CRITERIA_HEADING}' section to route")
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
