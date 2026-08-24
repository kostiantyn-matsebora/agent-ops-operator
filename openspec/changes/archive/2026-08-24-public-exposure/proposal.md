# Public exposure: the repository presents itself to strangers

## Why

The repository is private. Everything it needs in order to be adopted exists —
a chart, thirteen images, a documentation site built and sitting unpublished in
`docs/` — and none of it is reachable.

**The front door does not work yet**, and a README rewrite alone does not fix
it:

| A stranger lands on the repository and… | today |
|---|---|
| looks for a licence | `README.md` says **"License TBD"** — nobody can adopt it |
| clicks the documentation site | **404** — Pages has never been enabled |
| copies the install command | `helm install ./chart` needs a clone first |
| reads the roadmap | it promises a Helm chart that shipped long ago |
| opens an issue | no template, no security policy, no code of conduct |
| looks for the project | no description, no topics, no homepage |

Two smaller things belong with it. A build artifact is committed — one adapter
binary, several megabytes, more than half the repository's packed size — because
the ignore list names every module except the newest, and no longer matches the
tree after the component restructure. And line endings have flipped under
editing twice in one week, which a `.gitattributes` settles permanently.

## What Changes

- **Apache-2.0.** The Kubernetes ecosystem's norm, and the patent grant is why.
  This is the one file that cannot be quietly changed once forks exist.
- **A two-minute README.** The site now exists and owns the model, the
  walkthrough and the reference. The README keeps what this is, one install
  command a stranger can actually run, and the links onward — and stops
  restating the site.
- **Community health files**: `SECURITY.md` (private advisories, supported
  versions, an acknowledgement target a single maintainer can keep),
  `CODE_OF_CONDUCT.md` (Contributor Covenant by reference), `CONTRIBUTING.md`
  (how a change is proposed here, which is unusual enough to need saying).
- **Issue and PR templates**, with blank issues disabled and the security route
  a contact link rather than an issue type.
- **Repository settings**: description, topics, homepage pointing at the site,
  Discussions on, Issues on, Wiki and Projects off.
- **Pages enabled** from `master` `/docs`, which is what makes every site link
  in the README resolve.
- **Hygiene**: the committed binary removed, the ignore list corrected against
  the current tree, and `.gitattributes` normalising line endings.
- **A publication gate** — the checklist that decides when the flip is safe,
  because the flip is the one step that cannot be undone.

## Impact

- **Publication is irreversible.** Every commit becomes readable, and after the
  first clone a history rewrite stops being available. The gate exists for this.
- **Blocked by three changes**, and the gate names each: the identity scrub and
  its rewrite, the spec audit, and the artifact distribution that makes the
  install command real. This change does not begin the flip until they are done.
- **Affected specs**: `documentation-structure` — the README requirement changes
  shape, because it was written when no site existed. A new `public-repository`
  capability carries what the repository owes a stranger.

## Out of scope

- **The publication guard** — `sdlc-setup` owns it.
- **Removing identifying content, and the history rewrite** — `scrub-identity`.
- **Making the published specs true** — `truthful-specs`.
- **Registry, image publishing and the OCI chart** — `sdlc-setup`. This change
  consumes the install command they produce; it does not define it.
