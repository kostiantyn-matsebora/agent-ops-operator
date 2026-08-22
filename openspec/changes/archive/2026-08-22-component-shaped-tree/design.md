## Context

See `proposal.md` — Why. Four facts about the repository as it stands.

- **Thirteen images, twelve module directories plus the root.** Each is one
  container, and `.github/components.sh` derives the whole release matrix from
  that: every Dockerfile is an image, and the published name is
  `agentops-<component>`.
- **The root is a special case in that derivation**, because the root
  directory's basename is the checkout directory rather than an identity, so
  the script hardcodes `manager`.
- **Nothing is shared between modules.** All eleven submodules have zero
  requires — standard library only — and nothing outside the manager imports
  `api/` or `internal/`.
- **A build context is the Dockerfile's directory.** `COPY ../x` is illegal, so
  a component's directory already has to hold everything it builds from.

## Goals / Non-Goals

**Goals**

- A directory says what its component IS, without being opened.
- A new component is placed by a rule, not by precedent.
- Nothing that leaves the repository changes by ACCIDENT. One name changes on
  purpose, and the check that proves it is a diff against the inventory recorded
  before anything moved.

**Non-Goals**

- Making `api/` or any package importable across modules. Nothing imports it
  today, and the first import would give a stdlib-only binary its first
  dependency — `k8s.io/apimachinery`, which is why `console` and
  `signal-k8s-events` hand-roll their own object types.
- Splitting or merging any component.
- Moving the chart, which is a different view and stays where it is.

## Decisions

### D1. The tree is a projection of the C&C view

One container, one directory, grouped by the kind of runtime element it is.

For thirteen containers this keeps the module-to-component mapping *derivable*
rather than documented — the thing `components.sh` already relies on — and it
gives a placement rule with no judgement in it.

*Alternative rejected:* grouping by what installs a component (parent chart vs
bundle). That is the allocation view, the chart already carries it, and it would
tie source location to packaging: extracting a component into a bundle would
move its directory, and a move is a rename.

*Alternative rejected:* grouping by the system integrated with (`telegram/`,
`ha/`, `k8s/`). It has real cohesion — the Telegram trio does change together —
but it puts three different runtime elements under one name and leaves the
components that integrate with nothing homeless.

### D2. The name is derived from the PATH, and the path says each thing once

A PLURAL group names a KIND of component and lends its singular as a prefix. A
SINGULAR group is a namespace and lends nothing.

```
signals/cron       -> signal-cron         platform/manager  -> manager
channels/telegram  -> channel-telegram    platform/console  -> console
runtimes/claude    -> runtime-claude      gateways/telegram -> gateway-telegram
```

The first shape considered was `signals/signal-cron` — leaf name equals image
name, derivation stays `basename`, nothing renamed. It was rejected for saying
"signal" twice on one line, which is exactly the duplication a grouped tree is
supposed to remove.

The second was to drop the prefixes and keep `basename`, which does not work:
`signals/telegram` and `channels/telegram` would both derive `agentops-telegram`,
two components claiming one image.

Deriving from the path resolves both, because the group is already in the path.
It reproduces twelve of thirteen published names exactly — the existing names
encode the role for precisely the kinds that have one — and the thirteenth is
renamed on purpose.

*Alternative rejected:* a Dockerfile `LABEL` declaring the image name. It also
works, but it puts the name somewhere the tree cannot show, and it makes
uniqueness a property to check rather than one the path mostly provides.

**Uniqueness is asserted anyway.** A derived name is not unique by construction
the way a flat directory name was, and the release workflow matches a tag against
exactly this list.

### D2a. `telegram-router` becomes `gateway-telegram`

The one rename, and it is the point of having groups: the component's name now
says what kind of thing it is, in the same vocabulary as its directory.

**It renames a published image and a live workload.** The old image stays
published — the `signal-vmalertmanager` precedent — and an in-place upgrade has
Helm create the new Deployment before deleting the old, so two consumers poll one
bot token for a few seconds. The repository's own rule says never to do that; the
exposure is seconds, on one bundle, with the same image on both sides, and the
alternative is a workload named after a component that no longer exists.

### D3. The operator moves to `platform/manager/`, whole

`api/`, `cmd/`, `config/` and `internal/` go with it, because the build context
rule leaves no alternative: a Dockerfile at `platform/manager/` cannot copy from
`platform/api/`.

**Its module path follows it**, to
`github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager`, and every
submodule's does the same.

Keeping the old path was the first instinct — `go.mod` is declarative and nothing
internal imports it — and it is wrong: Go resolves a module by looking for
`go.mod` at the path the module CLAIMS. A module claiming the repository root
while its `go.mod` sits two directories down is not fetchable at all. Following
the directory is what keeps `api/v1alpha1` importable, at its new path.

Only the manager pays anything for this: it is the one module that imports its
own packages, so seventy-seven files change import lines. The other eleven are
single-package and change one line each.

**This deletes the root special case.** The root Dockerfile was the reason
`components.sh` hardcoded a name at all.

### D4. CI trades one hardcoded name for another, deliberately

`ci.yml` splits the operator (which needs envtest) from every other module by
"the root's `go.mod`". After the move there is no root module, so the envtest job
names `platform/manager` explicitly.

The alternative — deriving "the module that needs envtest" — would mean
inventing a marker for a property one module has. Naming it once, in the job
that provisions the assets, is smaller and legible where it matters.

### D5. Path anchors in specs are corrected, not deltaed

Seven specs under `openspec/specs/` name a module path in passing. None of them
describes behaviour that changes, so none is a modified capability — a path is
an anchor, and a stale anchor is a factual error to fix in place.

### D6. The move is `git mv`, in one change

History follows the files (`git log --follow`), and blame gains one rename hop.

Splitting it per component would leave the repository in a half-shaped state
across several commits, with `components.sh`'s root branch alive for some of
them and dead for others — and the CI derivation would be describing two
different trees on either side of each merge.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| **A move silently renames an image**, since the path is the identity. | The inventory is recorded BEFORE anything moves, and the derived list is diffed against it at the end. The diff must contain exactly one line: `telegram-router` out, `gateway-telegram` in. |
| **Two directories derive one component name.** | Asserted in `components.sh`, which fails rather than publishing one image from two contexts. |
| **The renamed workload overlaps the old one** on an in-place upgrade, and two consumers poll one bot token. | Stated in the changelog with the upgrade step. Seconds of 409s with identical images on both sides, and an install can uninstall the bundle first. |
| **The manager's build breaks** because its context no longer holds `api/`. | `api/`, `cmd/`, `config/`, `internal/`, `go.mod`, `go.sum` and `.dockerignore` move together with the Dockerfile. Verified by building the image, not by reading the diff. |
| **CI's operator/modules split silently degrades** — the module matrix picks up `platform/manager` without envtest and its suite fails, or the envtest job stops running. | The suite FAILS without assets rather than skipping, which the existing workflow already asserts. The matrix must exclude the module the envtest job owns. |
| **`.claude/rules` scoping goes stale**, so a scoped rule stops loading and its guidance quietly disappears. | `paths:` frontmatter in `signal-rules.md` and `palette-and-mark.md` is part of the change, and `structure.md` is rewritten around the buckets. |
| **Open changes under `openspec/changes/` reference old paths** and their tasks stop matching the tree. | Active changes are checked and updated. Archived ones are left exactly as they are — they record what was true. |
| **A reader's muscle memory breaks**: `cd console` no longer works. | Unavoidable and cheap, and the README plus `structure.md` state the new shape. |
