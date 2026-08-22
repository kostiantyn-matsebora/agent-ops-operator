# Design — public exposure

## D1. Apache-2.0, not MIT

The author's other public repository is MIT, so this is a deliberate divergence
rather than an oversight.

Kubernetes-ecosystem operators are overwhelmingly Apache-2.0, and the reason is
the **patent grant** MIT does not carry. An operator is infrastructure other
companies run; the licence that lets their lawyers say yes is the one the
ecosystem already standardised on.

**It is the one file that cannot be quietly changed later.** Once a fork exists,
relicensing needs every contributor's agreement — today that is one person, and
that is the cheapest this decision will ever be.

## D2. The README stops restating the site

The existing README requirement was written when there was **no site**. It
therefore made the README carry the pitch, the kind list, the behaviours, the
start and the index — correctly, because nothing else could.

The site now owns the model, the walkthrough, the console tour and the install.
A README that repeats them is not merely long, it is a **second source of truth
that drifts**, and the drift is invisible until an adopter follows the wrong one.

So the README answers what a stranger asks in the first two minutes:

```
   what is this        →  the pitch and the diagram
   is it real          →  the licence, the status, one honest sentence
   can I try it        →  ONE command that works without cloning
   where do I go next  →  the site first, then the reference pages
```

Everything else is a link. **What moves out is not deleted** — the existing
requirement's rule that removed content stays reachable in one hop still holds,
and the site is where it goes.

## D3. The install command decides whether the README can be honest

`helm install ./chart` cannot appear in a README a stranger reads: it requires a
clone the previous line did not tell them to make.

The OCI chart from `sdlc-setup` is what makes a one-line install true. Until it
publishes, this change **cannot finish the README** — which is a dependency
between changes, not a note. The gate carries it.

## D4. The gate, because the flip cannot be undone

Publication is one switch and no rollback: after it, the history is public and a
rewrite breaks every clone.

So the last task is a **gate**, and each condition is something that becomes
impossible or expensive afterwards:

| Condition | Owner | Why before |
|---|---|---|
| The publication guard is green | `sdlc-setup` | it is what proves the next two conditions |
| Identifying content is gone from tree, archive AND history | `scrub-identity` | a rewrite after publication breaks every clone |
| The published specs are true | `truthful-specs` | a contradiction read by a stranger is read once and remembered |
| Images are public and the chart installs by OCI | `sdlc-setup` | the README's one command must work on the day it is readable |
| Licence, community files and templates exist | here | an issue filed before there is a template is a template nobody will retrofit |

**Pages is enabled BEFORE the flip**, not after: it is what makes the README's
links resolve, and a first visitor who hits a 404 does not come back for the
fix.

## D5. Security reporting is a contact link, never an issue type

A security issue template invites the report into a public issue, which is the
disclosure it was meant to prevent.

Blank issues are disabled and the security route is a `contact_links` entry
pointing at private advisories, so the only paths are a template that fits or a
private channel.

**The acknowledgement target is one a single maintainer can actually keep.** A
policy promising 24 hours from one person is a promise that will be broken in
public.

## D6. The hygiene items are not incidental

Two of them would be noise on their own and are not here:

- **The committed binary** is over half the packed repository. Publishing means
  every clone pays for it forever, and a git object is not removed by deleting
  the file — it goes when history is rewritten, which `scrub-identity` is
  already doing once. **Removing the file here and letting that rewrite drop the
  object is the only way to do it without a second rewrite.**
- **The ignore list no longer matches the tree** after the component
  restructure, which is why the binary was committed at all. Correcting it after
  publication means the next contributor commits the next one.

`.gitattributes` is the third: line endings flipped under editing twice in one
week here. A public repository adds contributors on other platforms, which is
the condition that turns an occasional annoyance into recurring churn.

## D7. The CRD kind table stays in the README

Eleven kinds IS the product. A reader who cannot see the shape of the model
without following a link has not been told what this is.

The two-minute budget is met by cutting what the site says better — the
behaviours section, the expanded start — not by removing the one table that
answers the first question.
