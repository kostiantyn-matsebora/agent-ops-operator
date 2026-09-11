#!/usr/bin/env python3
"""The writing rules, checked by a program.

`.claude/rules/writing.md` says what every markdown file in a repository owes:
one thought per sentence, no semicolon, small paragraphs. `authoring.md` says
what a rules file owes on top: one `## ` heading, one topic. Both were written
and then broken on the very next page, twice, and caught each time by the
reader rather than the writer — so the check is a program, run by the
`rules_compliance` hook after every markdown write and by hand over a tree.

WHAT IT REPORTS, AND NEVER THE TEXT: file, line, rule. A public build log may
read this output, and a match is often the sentence somebody would rather not
have republished.

Rules:
  semicolon          a `;` in prose — fenced code, inline code, HTML entities
                     and comments do not count
  long-paragraph     a paragraph over --max-words words (default 45)
  rules-heading      a file under `.claude/rules/` or `docs/.claude/` whose
                     first content line is not a `## ` heading
  rules-second-h2    a rules file (same scope) with a second `## ` heading —
                     one topic per file

    rules_compliance.py <file.md>...   file:line rule, exit 1 when anything is found
"""
from __future__ import annotations

import argparse
import pathlib
import re
import sys
from typing import NamedTuple

MAX_WORDS = 45

FENCE = re.compile(r"^\s*(```|~~~)")
INLINE_CODE = re.compile(r"`[^`]*`")
ENTITY = re.compile(r"&[a-zA-Z]+;|&#\d+;")
COMMENT = re.compile(r"<!--.*?-->", re.S)
# Lines that are structure rather than prose. They end a paragraph and their
# words are not counted; the semicolon rule still reads them.
STRUCTURE = re.compile(r"^(\||\s*[-*+]\s|\s*\d+[.)]\s|>|#{1,6}\s|\s{2,}\S)")


class Finding(NamedTuple):
    path: str
    line: int
    rule: str


def is_rules_file(path: pathlib.Path) -> bool:
    """`.claude/rules/*.md`, or `docs/.claude/*.md` -- authoring.md governs
    both, since a site's own shell context follows the same
    one-`## `-heading, one-topic convention every other rules file does.
    ANCHORED AT THE TRAILING SHAPE (`.../docs/.claude/<file>.md`), not "docs
    anywhere in the path": a resolved absolute path carries the whole
    filesystem prefix, so unanchored matching would fire on a repo checked
    out under a directory coincidentally named docs. This is NOT the same as
    requiring docs/ to be the repository ROOT -- a nested
    `some/other/docs/.claude/*.md` still matches by this same trailing
    shape, which is harmless only because structure.md's own invariant
    keeps this repository's docs/ singular and root-level; the function
    itself does not check or enforce that."""
    parts = path.resolve().parts
    if len(parts) >= 3 and parts[-3] == ".claude" and parts[-2] == "rules" and path.suffix == ".md":
        return True
    return len(parts) >= 3 and parts[-3] == "docs" and parts[-2] == ".claude" and path.suffix == ".md"


def prose_lines(text: str) -> list[tuple[int, str, bool]]:
    """(line number, text with code and comments removed, is_content) for
    every line outside front matter and fenced code. A line inside a
    multi-line HTML comment is blank."""
    out: list[tuple[int, str, bool]] = []
    in_front = False
    in_fence = False
    in_comment = False
    for i, raw in enumerate(text.splitlines(), 1):
        if i == 1 and raw.strip() == "---":
            in_front = True
            continue
        if in_front:
            if raw.strip() == "---":
                in_front = False
            continue
        if FENCE.match(raw):
            in_fence = not in_fence
            out.append((i, "", False))   # a fence ends a paragraph
            continue
        if in_fence:
            continue
        line = raw
        if in_comment:
            end = line.find("-->")
            if end < 0:
                continue
            line = line[end + 3:]
            in_comment = False
        line = COMMENT.sub("", line)
        start = line.find("<!--")
        if start >= 0:
            line = line[:start]
            in_comment = True
        line = INLINE_CODE.sub("", line)
        line = ENTITY.sub("", line)
        out.append((i, line, bool(raw.strip())))
    return out


def check_text(text: str, path: str, max_words: int = MAX_WORDS) -> list[Finding]:
    found: list[Finding] = []
    words = 0
    start = 0

    def flush() -> None:
        nonlocal words, start
        if words > max_words:
            found.append(Finding(path, start, "long-paragraph"))
        words = 0
        start = 0

    for n, line, content in prose_lines(text):
        if ";" in line:
            found.append(Finding(path, n, "semicolon"))
        if not content or STRUCTURE.match(line) or not line.strip():
            flush()
            continue
        if words == 0:
            start = n
        words += len(line.split())
    flush()
    return sorted(found, key=lambda f: (f.line, f.rule))


def check_rules_shape(text: str, path: str) -> list[Finding]:
    found: list[Finding] = []
    first_seen = False
    h2 = 0
    for n, line, content in prose_lines(text):
        if not content:
            continue
        if not first_seen:
            first_seen = True
            if not line.startswith("## "):
                found.append(Finding(path, n, "rules-heading"))
        if line.startswith("## "):
            h2 += 1
            if h2 == 2:
                found.append(Finding(path, n, "rules-second-h2"))
    return found


def check(path: pathlib.Path, max_words: int = MAX_WORDS) -> list[Finding]:
    text = path.read_text(encoding="utf-8")
    shown = str(path)
    found = check_text(text, shown, max_words)
    if is_rules_file(path):
        found += check_rules_shape(text, shown)
    return found


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("paths", nargs="+", type=pathlib.Path)
    ap.add_argument("--max-words", type=int, default=MAX_WORDS)
    args = ap.parse_args()
    findings: list[Finding] = []
    for p in args.paths:
        if p.suffix != ".md" or not p.is_file():
            continue
        findings += check(p, args.max_words)
    for f in findings:
        print(f"{f.path}:{f.line} {f.rule}")
    return 1 if findings else 0


if __name__ == "__main__":
    sys.exit(main())
