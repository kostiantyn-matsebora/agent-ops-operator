#!/usr/bin/env bash
# anchor-check.py: every relative markdown link resolves to a file, and
# every anchor resolves to a heading, slugified the way GitHub/kramdown do.
#
# Every case below runs the real script against FIXTURE files under a
# tmpdir (passed as explicit positional arguments, the same way
# retired-vocabulary-guard.test.sh drives its guard via `--path` instead of
# touching the real repository tree) — this file pins the CHECKER's own
# correctness. The real tree is verified separately, by hand, once per
# change that touches docs, because the tree already carries pre-existing
# broken links/anchors this change did not introduce (see the handback for
# this task): asserting the whole tree clean here would make this
# permanent suite fail on somebody else's unrelated page forever.
. "$(dirname "$0")/lib.sh"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
CHECK="$ROOT/.github/scripts/anchor-check.py"
tmp=$(mktemp -d)

it "a relative link to a real file, and a matching anchor in it, both resolve"
mkdir -p "$tmp/clean/docs"
cat > "$tmp/clean/docs/target.md" <<'MD'
## A Target Heading

Some body text.
MD
cat > "$tmp/clean/docs/source.md" <<'MD'
See [the target](target.md#a-target-heading) for detail.
MD
out=$(python3 "$CHECK" "$tmp/clean/docs/source.md" 2>&1)
assert_status 0 "$?"
assert_contains "$out" "every relative link and anchor resolves"

it "a link to a file that does not exist is reported, naming the file and line"
mkdir -p "$tmp/brokenlink"
cat > "$tmp/brokenlink/source.md" <<'MD'
line one

See [missing](nowhere.md) for detail.
MD
out=$(python3 "$CHECK" "$tmp/brokenlink/source.md" 2>&1)
assert_status 1 "$?"
assert_contains "$out" "source.md:3"
assert_contains "$out" "nowhere.md"

it "a link whose anchor matches no heading in the target is reported"
mkdir -p "$tmp/brokenanchor"
cat > "$tmp/brokenanchor/target.md" <<'MD'
## A Real Heading
MD
cat > "$tmp/brokenanchor/source.md" <<'MD'
See [it](target.md#not-a-real-heading).
MD
out=$(python3 "$CHECK" "$tmp/brokenanchor/source.md" 2>&1)
assert_status 1 "$?"
assert_contains "$out" "not-a-real-heading"
assert_contains "$out" "target.md"

it "a bare #anchor link checks headings in the SAME file"
mkdir -p "$tmp/samefile"
cat > "$tmp/samefile/page.md" <<'MD'
## Section One

See [below](#section-two) for the rest.

## Section Two
MD
out=$(python3 "$CHECK" "$tmp/samefile/page.md" 2>&1)
assert_status 0 "$?"

it "a bare #anchor with no matching heading in the same file is reported"
mkdir -p "$tmp/samefilebad"
cat > "$tmp/samefilebad/page.md" <<'MD'
## Section One

See [below](#section-nine) for the rest.
MD
out=$(python3 "$CHECK" "$tmp/samefilebad/page.md" 2>&1)
assert_status 1 "$?"
assert_contains "$out" "section-nine"

it "http(s) links and Jekyll relative_url links are never resolved as repository paths"
mkdir -p "$tmp/skip"
cat > "$tmp/skip/page.md" <<'MD'
[external](https://example.com/nowhere)
[site asset]({{ '/assets/img/nowhere.svg' | relative_url }})
MD
out=$(python3 "$CHECK" "$tmp/skip/page.md" 2>&1)
assert_status 0 "$?"

it "a link inside a fenced code block or inline code span is prose, not checked"
mkdir -p "$tmp/fenced"
cat > "$tmp/fenced/page.md" <<'MD'
Markdown syntax looks like `[text](nowhere.md)`.

```
[also fenced](nowhere-else.md)
```
MD
out=$(python3 "$CHECK" "$tmp/fenced/page.md" 2>&1)
assert_status 0 "$?"

it "a link to a directory resolves by existence, no heading check applies"
mkdir -p "$tmp/dirlink/rules"
cat > "$tmp/dirlink/page.md" <<'MD'
See [the rules](rules/) for detail.
MD
out=$(python3 "$CHECK" "$tmp/dirlink/page.md" 2>&1)
assert_status 0 "$?"

it "a badge link (image nested inside a link) still checks the OUTER target"
# Regression: a non-recursive bracket match stops at the image's own `]`
# and silently drops the real link after it — measured on this
# repository's own README (LICENSE/cr-reference.md/CHANGELOG.md badges).
mkdir -p "$tmp/badge"
cat > "$tmp/badge/page.md" <<'MD'
[![alt text](https://img.shields.io/badge/ok-ok)](nowhere.md)
MD
out=$(python3 "$CHECK" "$tmp/badge/page.md" 2>&1)
assert_status 1 "$?"
assert_contains "$out" "nowhere.md"

it "a badge link's outer target is checked even when it IS real"
mkdir -p "$tmp/badgeok"
cat > "$tmp/badgeok/target.md" <<'MD'
## Fine
MD
cat > "$tmp/badgeok/page.md" <<'MD'
[![alt text](https://img.shields.io/badge/ok-ok)](target.md)
MD
out=$(python3 "$CHECK" "$tmp/badgeok/page.md" 2>&1)
assert_status 0 "$?"

it "heading slugs strip punctuation per character, not per run -- pinned against a real heading shape"
# `### Releasing a runtime by hand -- \`/exit\`` (em dash + backtick code)
# slugifies to a DOUBLE hyphen on the real tree (docs/concepts.md) because
# the em dash and the slash are each removed on their own, leaving both of
# their neighbouring spaces behind. Collapsing whitespace instead of
# replacing it character-for-character produces a single hyphen, which
# never matches.
mkdir -p "$tmp/slug"
printf '### Releasing a runtime by hand \xe2\x80\x94 `/exit`\n' > "$tmp/slug/target.md"
cat > "$tmp/slug/page.md" <<'MD'
See [it](target.md#releasing-a-runtime-by-hand--exit).
MD
out=$(python3 "$CHECK" "$tmp/slug/page.md" 2>&1)
assert_status 0 "$?"

it "a duplicate heading's second anchor carries GitHub's -1 suffix"
mkdir -p "$tmp/dup"
cat > "$tmp/dup/target.md" <<'MD'
## Repeated

first

## Repeated

second
MD
cat > "$tmp/dup/page.md" <<'MD'
See [the first](target.md#repeated) and [the second](target.md#repeated-1).
MD
out=$(python3 "$CHECK" "$tmp/dup/page.md" 2>&1)
assert_status 0 "$?"

it "several files scan together, and only the offending line is reported"
mkdir -p "$tmp/multi"
cat > "$tmp/multi/clean.md" <<'MD'
nothing to link
MD
cat > "$tmp/multi/dirty.md" <<'MD'
[broken](still-nowhere.md)
MD
out=$(python3 "$CHECK" "$tmp/multi/clean.md" "$tmp/multi/dirty.md" 2>&1)
assert_status 1 "$?"
assert_contains "$out" "dirty.md:1"
assert_not_contains "$out" "clean.md:"

rm -rf "$tmp"
summary
