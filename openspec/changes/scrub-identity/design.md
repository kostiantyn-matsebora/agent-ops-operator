# Design — scrub identity

## Current state, established by inspection

| Fact | Consequence |
|---|---|
| Repository is private; unauthenticated API returns not-found | Publication is a one-way door on every commit, not just the tip |
| 157 commits, one human author plus a bot | A rewrite is cheap and affects nobody |
| **Zero tags** | No published artifact references a SHA yet. The first release tag closes this window |
| No credential of any kind has ever been committed — no API key, no bot token, no private key | The scrub is about IDENTITY, not secrets. Nothing needs revoking |
| One of the four classes is shipped as the documented example an adopter copies | Substitution changes adopter-visible documentation, so the replacement must be a value the docs can keep recommending |
| The openspec archive is published with the repository | An archived change that names a literal republishes it |

## D1. Verification is an allowlist guard, not a checklist

A checklist of strings to remove has to contain the strings. It is also only
ever as complete as the person who wrote it.

`sdlc-setup` §9 adds a guard that permits SHAPES — reserved example domains,
cluster-internal names, this repository's own clone URL, documented placeholder
identifiers, a documented set of private-range literals — and fails on anything
else, across the whole tree and the commit messages under review.

This change is **complete when that guard passes**. It catches classes nobody
enumerated, and it keeps catching them afterwards.

**Consequence, accepted:** the archived change is not independently auditable.
A reader cannot confirm what was removed, only that the guard passes. The
alternative is a permanent public document listing what someone wanted hidden,
which is worse.

## D2. No artifact of this change names a literal, ever

The trap this change is built around:

```
   a change that removes X must NAME X
             │
             ▼  archived into openspec/changes/archive/
   the repository publishes the document that removed it
             │
             ▼  and the rewrite ran BEFORE the archive was written
   the literal lands in the NEW history, un-scrubbed
```

So every artifact — proposal, design, tasks, spec deltas, commit messages,
and any verification note — describes **classes**. The concrete map lives in a
gitignored working file, is consumed once by the rewrite, and is deleted.

Six surfaces leak, and the third is the one that gets missed:

| Surface | The slip |
|---|---|
| proposal / design / tasks | naming what is removed |
| spec deltas | a permanent public requirement naming a person |
| **verification notes** | pasting a search result into a ticked task |
| **commit messages** | permanent, and outside the tree a file-scoped guard scans |
| changelog entry | "the example identifier changed from …" |
| the guard's own rules | solved by D1: an allowlist names nothing it forbids |

**The rule this creates for every future change:** record that the guard passes,
never what it matched.

## D3. One rewrite, run LAST

The rewrite is the final act, after the archive is written.

Because the artifacts hold no literals (D2), the archived change **passes
through the rewrite unchanged**. Had it named them, the rewrite would have
substituted inside it and produced a document reading "replace the placeholder
with the placeholder" — self-contradictory, and a permanent monument to the
mistake.

Ordering, and each arrow is a constraint rather than a preference:

```
  guard lands in CI ──▶ tree + archive scrubbed ──▶ change archived
        │                                                │
        │  from here a literal fails the build           │
        └────────────────────────────────────────────────┘
                                                         ▼
                                              ONE history rewrite
                                                         │
                              (before the first release tag ─────┐
                               before publication ───────────────┤
                               while only the author has a clone)┘
```

## D4. Rewrite rather than accept the disclosure

Leaving history alone was considered and rejected by the repository owner.

**What it costs:** every SHA changes; every working copy must re-clone; the
rewrite needs a quiet tree with nothing staged and no session mid-change.

**What it buys:** the classes are gone from the only place a reader can still
find them. A scrub of the tip alone is theatre — `git log -S` finds the rest in
seconds.

## D5. Replacements are role-shaped and documented

Two constraints on every replacement:

1. **It must be a value the guard's allowlist permits by name**, or the guard
   fails on the fix. Placeholder and allowlist are one decision.
2. **It must read as an example**, so nobody pastes it back in believing it is
   real. The class currently shipped as documentation already has a
   documented placeholder sitting beside it — that one is chosen, not invented.

For fixture senders, ROLE-shaped names (what the fixture is for) beat a swapped
persona: the test then says what it is testing rather than who was typing.

## D6. The scrub is one-off; the rule that keeps it true is not

The durable half goes to `documentation-structure`: what a shipped example is
allowed to contain. Without it, the next person documenting a new field pastes
their own real value in, exactly as happened here — and the guard would catch
it, but nothing would have told them why.

## Open questions

- **`claude-runner`** appears in several modules and one sample comment. It
  reads as a legacy component name in the code and as the author's prior
  personal setup in the sample. Owner's call whether it is a fifth class or
  simply history. Resolved before task 1.1.
