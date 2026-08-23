# Tasks — scrub identity

**Every task below describes a CLASS. No task, note, commit message or
verification record names a literal.** The working map is a gitignored file,
consumed once, then deleted. Verification is recorded as "the guard passes" and
never as what it matched — pasting a search result into a ticked box is the
most natural way to reintroduce exactly what was removed.

## 0. Preconditions

- [x] 0.1 `sdlc-setup` §9 is merged and its guard job is green on `master`.
      Without it, this change is policed by nobody
- [x] 0.2 Resolve the open question in `design.md`: whether the legacy component
      name appearing in several modules and one sample comment is a fifth class
      or simply history
- [x] 0.3 Confirm no release tag exists. If one does, the rewrite in §4 orphans
      the SHA it references and this change needs re-planning, not re-running
- [x] 0.4 Confirm every other session has committed and pushed, and that no
      change is mid-apply. §4 force-pushes; a working copy with staged work is
      wedged by it — the checkout is clean with nothing staged, and the three
      peer sessions sharing it are idle or offline

## 1. The map

- [x] 1.1 Run the guard locally with matches shown, over the tree and over the
      full message range, and write the findings to a working file. Add that
      file to `.gitignore` in the same step, before anything else is done with
      it
- [x] 1.2 For each class, choose ONE replacement, per `design.md` D5: a value
      the guard's allowlist already permits, that reads as an example, and — for
      a class that already has a placeholder in the documentation — that
      existing placeholder rather than a new one
- [x] 1.3 For fixture people, choose ROLE-shaped names. A fixture sender should
      say what the test exercises
- [x] 1.4 Record in the working file only. The map is never committed

## 2. The tree

- [x] 2.1 Substitute across the shipped surfaces first — adopter pages, chart
      values, sample manifests — because these are the ones an adopter copies
- [x] 2.2 Substitute across module sources and test fixtures. Where a test
      DERIVES a value from an identifier, confirm the derivation still reads
      correctly with the placeholder and that the test still asserts the
      transformation rather than the constant
- [x] 2.3 Substitute across the changelog archives. They are append-only in
      spirit, but a published archive is published
- [x] 2.4 Substitute across the agent context under `.claude/`. A pasted
      diagnostic in a gotchas file is the same disclosure as one in a doc
- [x] 2.5 `go build ./...`, `go vet ./...` and the full test suite pass, and
      `helm template` renders every permutation the CI chart job covers

## 3. The openspec archive

- [x] 3.1 Substitute across archived changes. These are published with
      everything else and are the largest concentration of one class
- [x] 3.2 Treat the archive class as WIDER than a name. Archived tasks reference
      the private repository, the directory layout INSIDE it, and the object
      names of the install each change was verified against — together a
      description of someone's deployment, not merely a mention of it
- [x] 3.3 Leave the SHAPE of each archived document intact — substitution in
      place, never rewriting, summarising or deleting an entry that reads as
      dated. A section titled after a private repository keeps its heading and
      its position; only the name inside it changes. An archived change records
      what was decided, and the identifiers in it were never part of that
      decision — see `design.md` D7
- [x] 3.4 Decide, once, which live object names identify the author's install
      and which are ordinary vocabulary an adopter would choose too. Substitute
      the first kind; leave the second, because renaming generic vocabulary
      makes the record harder to read for no gain
- [x] 3.5 `openspec validate --specs --strict` and `--changes --strict` pass —
      specs clean; this change validates. Three OTHER in-flight changes fail on
      stale MODIFIED blocks, untouched by this change and failing before it

## 4. The history — LAST, once

- [x] 4.1 Re-run the guard over the tree. It passes. Only then is the tree ready
      to be the thing history is rewritten toward
- [ ] 4.2 Archive THIS change first, so the rewrite covers its own artifacts.
      They contain no literals, so they pass through unchanged — which is the
      property `design.md` D3 exists to preserve
- [ ] 4.3 Rewrite every commit in one pass, substituting in blob content AND in
      commit messages, driven by the working file from §1
- [ ] 4.4 Delete the working file
- [ ] 4.5 Verify: the guard passes over the tree AND over the full message
      range, from the root commit. Record the result as a pass, with per-rule
      counts of zero — never as a list
- [ ] 4.6 Force-push to the private origin. Every clone re-clones; there is no
      merge that recovers the old history and none should be attempted

## 5. Keep it true

- [x] 5.1 The guard runs on every pull request, so the rule is enforced rather
      than remembered. Confirm it fails a scratch pull request that adds an
      out-of-allowlist identifier to a file, and one that adds it only to a
      commit message — both lanes fail on a scratch branch, running the two
      commands the CI job runs, and each report names position and rule only
- [x] 5.2 Record in `.claude/rules/` the rule this change creates: a shipped
      example carries a placeholder, and verification states that the guard
      passes rather than what it matched
- [x] 5.3 Note in the adopter documentation that the example identifiers ARE
      examples and what to substitute — the guard stops a real value going in,
      it cannot tell an adopter which of their own values goes there
