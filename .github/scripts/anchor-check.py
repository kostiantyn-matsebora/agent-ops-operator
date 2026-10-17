#!/usr/bin/env python3
"""Relative markdown links and their anchors, checked against the real tree.

Several agents working this repository's `runtime-specification` openspec
change retargeted links by hand while splitting `docs/contracts.md` into
per-extension-point pages (`docs/contracts/{channel-adapter,signal-adapter,
runtime}.md`) -- each one re-deriving the GitHub/kramdown heading-slug rule
and re-walking the tree informally, with a throwaway script thrown away
afterward. This is the permanent version, so the next change that moves a
heading or a page does not have to re-derive either.

WHAT IT CHECKS, deliberately narrow:

    [text](relative/path.md)
    [text](relative/path.md#anchor)
    [text](#anchor)                     -- same file

For every such link in README.md, CLAUDE.md and every `*.md` under `docs/`:
the path resolves to a real file (or directory) relative to the LINKING
file's own directory, and an anchor resolves to a real heading in the
TARGET file (the same file, for a bare `#anchor`), slugified the way
GitHub/kramdown number headings.

WHAT IT DELIBERATELY SKIPS:

  - `http(s)://` and `mailto:` links -- not relative, nothing to resolve.
  - `{{ '...' | relative_url }}` Jekyll links -- these resolve to PUBLISHED
    site paths, not repository paths, and the adopter-site pre-flight in
    `docs/CLAUDE.md` ("Before calling a page done") already covers the built
    site. Checking them here would be a second, divergent implementation of
    that same resolution.
  - fenced code blocks and inline code spans -- a literal example of
    markdown syntax (`` `[text](url)` ``) is prose, not a link.
  - YAML front matter.

Usage:
    anchor-check.py                 every README.md, CLAUDE.md, docs/**/*.md
    anchor-check.py <file.md>...    only the named files
    anchor-check.py --show-slugs <file.md>   debug: print every heading's slug

Exit status is 1 when anything is reported. Reports file:line and the broken
link/anchor text -- nothing here is publication-sensitive, unlike the
publication and retired-vocabulary guards beside it.
"""
from __future__ import annotations

import argparse
import os
import pathlib
import re
import sys

REPO = pathlib.Path(__file__).resolve().parents[2]

FENCE_RE = re.compile(r"^\s*(```|~~~)")
INLINE_CODE_RE = re.compile(r"`[^`]*`")
HEADING_RE = re.compile(r"^(#{1,6})\s+(.+?)\s*$")


def find_markdown_links(line: str) -> list[str]:
    """Every link/image target in one line, balanced-bracket aware.

    A plain regex (`\\[[^\\]]*\\]\\(([^)]+)\\)`) stops at the FIRST `]`, which
    silently drops every real link after a badge: `[![alt](badge-url)](page)`
    has a `]` inside its own link text, so a non-recursive match consumes
    `[![alt]` + `(badge-url)` as if that were the whole thing and the real
    target, `page`, is never seen -- measured on this repository's own
    README, whose `LICENSE`/`cr-reference.md`/`CHANGELOG.md` badge links
    were silently never checked. This balances both the `[...]` and the
    `(...)` spans by hand (stdlib `re` has no recursion), and recurses into
    a link's own text so a nested image's target is also reported.
    """
    results: list[str] = []
    i, n = 0, len(line)
    while i < n:
        if line[i] != "[":
            i += 1
            continue
        depth, j = 1, i + 1
        while j < n and depth > 0:
            if line[j] == "[":
                depth += 1
            elif line[j] == "]":
                depth -= 1
            j += 1
        if depth != 0 or j >= n or line[j] != "(":
            i += 1
            continue
        depth2, k = 1, j + 1
        while k < n and depth2 > 0:
            if line[k] == "(":
                depth2 += 1
            elif line[k] == ")":
                depth2 -= 1
            k += 1
        if depth2 != 0:
            i += 1
            continue
        results.append(line[j + 1 : k - 1])
        results.extend(find_markdown_links(line[i + 1 : j - 1]))
        i = k
    return results


def strip_noise(text: str) -> str:
    """Blank out fenced code blocks, inline code spans and YAML front matter,
    line-for-line, so line numbers in a report still point at the real line."""
    lines = text.split("\n")
    out: list[str] = []
    i = 0
    if lines and lines[0].strip() == "---":
        out.append("")
        i = 1
        while i < len(lines) and lines[i].strip() != "---":
            out.append("")
            i += 1
        if i < len(lines):
            out.append("")  # the closing `---`
            i += 1
    in_fence = False
    while i < len(lines):
        line = lines[i]
        if FENCE_RE.match(line):
            in_fence = not in_fence
            out.append("")
        elif in_fence:
            out.append("")
        else:
            out.append(INLINE_CODE_RE.sub(lambda m: " " * len(m.group(0)), line))
        i += 1
    return "\n".join(out)


def extract_headings(text: str) -> list[str]:
    """Rendered heading text, in document order, markdown emphasis/code
    markers already irrelevant because `slugify` strips punctuation too."""
    headings = []
    in_fence = False
    for line in text.split("\n"):
        if FENCE_RE.match(line):
            in_fence = not in_fence
            continue
        if in_fence:
            continue
        m = HEADING_RE.match(line)
        if m:
            headings.append(m.group(2))
    return headings


def slugify(heading: str) -> str:
    """GitHub/kramdown's heading-anchor rule: lowercase, strip everything
    that is not a word character, space or hyphen, EACH remaining
    whitespace character to its own hyphen.

    NOT a collapse of runs of whitespace to one hyphen -- a heading reading
    `Releasing a runtime by hand \u2014 \\`/exit\\`` strips the em dash and the
    slash (both punctuation, neither a word character) but keeps the SPACE
    on each side of where they stood, so the real anchor is
    `releasing-a-runtime-by-hand--exit`, two hyphens. Collapsing instead
    produces one, which never matches and was caught against the real tree.
    """
    s = heading.strip().lower()
    s = re.sub(r"[^\w\s-]", "", s)
    s = re.sub(r"\s", "-", s)
    return s


def heading_slugs(text: str) -> set[str]:
    """Every slug a heading in this file answers to, GitHub's own duplicate
    rule applied (`foo`, `foo-1`, `foo-2`, ...) so a repeated heading does
    not falsely look unresolved for its later occurrences."""
    seen: dict[str, int] = {}
    slugs: set[str] = set()
    for heading in extract_headings(text):
        base = slugify(heading)
        n = seen.get(base, 0)
        slugs.add(base if n == 0 else f"{base}-{n}")
        seen[base] = n + 1
    return slugs


def iter_links(text: str):
    """(line number, target) for every markdown link/image in cleaned text."""
    cleaned = strip_noise(text)
    for lineno, line in enumerate(cleaned.split("\n"), 1):
        for target in find_markdown_links(line):
            yield lineno, target.strip()


def should_skip(target: str) -> bool:
    if not target:
        return True
    if target.startswith(("http://", "https://", "mailto:", "{{", "#!")):
        return True
    return False


def display(path: pathlib.Path) -> str:
    """A path for a message: relative to REPO when it is under REPO (every
    real page is), `os.path.relpath` otherwise -- a test fixture lives
    under a tmpdir outside REPO on purpose (never the real tree), and
    `Path.relative_to` raises on that rather than falling back."""
    try:
        return str(path.relative_to(REPO))
    except ValueError:
        return os.path.relpath(path, pathlib.Path.cwd())


def default_files() -> list[pathlib.Path]:
    files = [REPO / "README.md", REPO / "CLAUDE.md"]
    docs = REPO / "docs"
    files += sorted(p for p in docs.rglob("*.md") if "_site" not in p.parts)
    return files


def check_file(path: pathlib.Path) -> list[str]:
    problems: list[str] = []
    text = path.read_text()
    for lineno, raw_target in iter_links(text):
        if should_skip(raw_target):
            continue
        link_path, _, anchor = raw_target.partition("#")
        if link_path:
            base = REPO if link_path.startswith("/") else path.parent
            target_path = (base / link_path.lstrip("/")).resolve()
            if not target_path.exists():
                where = display(base)
                problems.append(
                    f"{display(path)}:{lineno}: "
                    f"broken link {raw_target!r} -- no such file or directory "
                    f"{link_path!r} relative to {where}/"
                )
                continue
        else:
            target_path = path
        if anchor and target_path.is_file() and target_path.suffix == ".md":
            slugs = heading_slugs(target_path.read_text())
            if anchor not in slugs:
                problems.append(
                    f"{display(path)}:{lineno}: "
                    f"broken anchor {raw_target!r} -- no heading slugifies to "
                    f"{anchor!r} in {display(target_path)}"
                )
    return problems


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("files", nargs="*", help="markdown files to check (default: README.md, CLAUDE.md, docs/**/*.md)")
    parser.add_argument("--show-slugs", metavar="FILE", help="print every heading's slug for FILE and exit")
    args = parser.parse_args()

    if args.show_slugs:
        p = pathlib.Path(args.show_slugs)
        for heading in extract_headings(p.read_text()):
            print(f"{slugify(heading)}\t{heading}")
        return 0

    files = [pathlib.Path(f) for f in args.files] if args.files else default_files()

    problems: list[str] = []
    for path in files:
        problems += check_file(path)

    if problems:
        print(f"{len(problems)} broken link(s)/anchor(s):", file=sys.stderr)
        for problem in problems:
            print(f"  {problem}", file=sys.stderr)
        return 1
    print(f"{len(files)} file(s), every relative link and anchor resolves")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
