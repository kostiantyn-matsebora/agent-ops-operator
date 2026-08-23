# Scrub identity: remove what names a person or a private cluster, tree and history

## Why

This repository is **private today and is going to be published**. Publishing
discloses every commit, not the tip — and after the first stranger clones it, a
history rewrite stops being an option.

Four classes of content identify the author personally or the private cluster
they run:

| Class | Where it is |
|---|---|
| The author's given name, as a fixture sender | test fixtures across several modules |
| The private GitOps repository, by clone URL | a shipped sample, and archived openspec tasks |
| An internal MCP domain | the initial import's samples, history only |
| A real forum chat identifier | **shipped as the documented example** — an adopter page, chart values, samples, a changelog archive, and a test that derives a working deep link from it |

The fourth is the sharpest: it is not a leftover, it is **presented to adopters
as the value to copy**, sitting beside the obviously-fake placeholder that was
there first.

Two properties make now the moment:

- **No release tag exists.** Nothing published carries a commit SHA, so a
  rewrite costs a force-push and nothing else. The first tag ends that.
- **Only the author has a clone.** After publication, a rewrite is a break for
  everyone who forked.

## What Changes

- **The tree**: every occurrence of the four classes becomes a **documented
  placeholder** — a reserved example domain, a placeholder repository, a
  role-shaped fixture name, the chat identifier the documentation already uses
  as its example. Placeholders are chosen so the publication guard's allowlist
  can permit them by name.
- **The openspec archive**: same substitution. The archive is published with
  everything else.
- **The commit history**: ONE rewrite pass, over all commits, run LAST.
- **Verification is the guard**, not a checklist. `sdlc-setup` adds an
  allowlist-shaped publication guard to CI; this change is complete when it
  passes over the tree and over the message range.

**This change names none of the four literals, in any artifact, ever.** That is
a requirement of the work, not a style preference — see `design.md` D2. The
concrete map lives in a gitignored working file, is used once, and is deleted.

## Impact

- **Every commit SHA changes.** Nothing external references them yet.
- **A force-push to the private origin.** Every working copy must re-clone, so
  the rewrite needs a quiet tree: no session mid-change, nothing staged.
- **Ordering.** After `sdlc-setup`'s guard lands, before its first release tag,
  before `public-exposure` flips the repository.
- **Affected specs**: `documentation-structure` gains the durable half — what a
  shipped example is allowed to contain. The scrub is a one-off; the rule that
  keeps it true is not.

## Out of scope

- **The author's git identity.** It is their public account name; it stays.
- **The container registry namespace.** `sdlc-setup` replaces it as part of the
  registry cut-over; it is a published handle, not private.
- **The publication guard itself** — `sdlc-setup` §9 owns it. This change
  consumes it.
- **Licence, README, community files, repository settings** — `public-exposure`.
