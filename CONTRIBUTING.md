# Contributing

Thank you for looking. This project is small and its workflow is unusual enough
that **you cannot infer it from the tree** — so this page states it rather than
assuming a contributor will guess.

Before anything else: the [code of conduct](CODE_OF_CONDUCT.md) applies here,
and a security problem goes to [SECURITY.md](SECURITY.md), never to an issue.

## Start with an issue or a discussion

- **A bug** → an issue, using the bug template. It asks for the chart version,
  the image tags and the condition on the object, because those three are where
  every diagnosis in this repository has actually started.
- **An idea, or a question about whether something fits** →
  [Discussions](https://github.com/kostiantyn-matsebora/agent-ops-operator/discussions).
- **A feature** → an issue, using the feature template. It asks what you are
  trying to do before what you want built: the second is often not the cheapest
  answer to the first.

**Please open one before a large pull request.** This project has strong
opinions about its model — what a `Pipeline` is allowed to carry, where wiring
lives, what the manager may never read — and several of them were arrived at by
building the alternative first. A short conversation is cheaper than a rewrite.

## How a change is proposed here

**This repository plans changes as specifications, in `openspec/`.** Every
non-trivial change is a directory under `openspec/changes/<name>/` holding a
proposal, delta specs, a design note and a task list; when the work is done the
deltas fold into `openspec/specs/` and the change is archived.

That is why the diff on a substantial pull request contains prose as well as
code, and why `openspec/specs/` — not the code — is the answer to "is this
behaviour intended".

You do **not** have to author an openspec change to contribute. The rule of
thumb:

| Your change | What it needs |
|---|---|
| a typo, a broken link, a one-line fix | a pull request, nothing else |
| a bug fix that does not alter documented behaviour | a pull request, and a test |
| anything that changes what a CRD means, what a contract carries, or what the chart grants | an openspec change first — say so in the issue and we will work it out |

If you have the [OpenSpec CLI](https://github.com/Fission-AI/OpenSpec)
installed, `openspec list` shows what is in flight and `openspec show <name>`
prints one. Reading an archived change under `openspec/changes/archive/` is the
fastest way to see the shape.

## Documentation is part of the change, not a follow-up

**This is the rule most likely to send a pull request back**, so it is stated
here rather than discovered in review.

A change is not finished until every document it made untrue is updated **in
the same pull request**. Both halves, and they are skipped independently:

1. **The reference docs** — [`docs/concepts.md`](docs/concepts.md) for CRD
   semantics, [`docs/contracts.md`](docs/contracts.md) for the HTTP and adapter
   contracts, a bundle page for a subchart, `docs/CHANGELOG.md` for a breaking
   change.
2. **The adopter site** — the landing page, the Introduction, Getting started,
   Installation and the guides under `docs/guides/`. This is the half that gets
   skipped, because a change feels done once the reference is right, and the
   adopter never reads the reference.

`.claude/rules/documentation.md` carries the routing table: what kind of change
goes in which file.

**Some blocks are generated and are never edited in place.** A page's `yaml`
examples come from `<!-- generated: … -->` markers filled by
`.github/scripts/docs-generate.py`; the console screenshots and the landing
recording come from `npm run screenshots` and `npm run demo` in
`platform/console/ui`. Editing the output by hand looks like it worked and is
reverted by the next run. CI fails on a stale one.

## The tree

**One directory per container**, grouped by what that container is at runtime,
and **the path is the published image name**:

```
platform/   manager, console, housekeeping, context-sync, egress-proxy
runtimes/   the client side of the work contract
signals/    push to /signal/inbound
channels/   serve /channel/*
gateways/   speak no agent-ops contract at all

signals/cron  →  agentops-signal-cron       (a plural group lends its singular)
platform/console  →  agentops-console       (a singular group lends nothing)
```

`.github/components.sh` derives that list from the tree, so **moving a directory
renames a published image**. A component's build context is its own directory —
nothing copies across a boundary, and shared code lives inside its consumer
until sharing is a decision somebody makes deliberately.

`.claude/rules/structure.md` is the full map. The rest of `.claude/rules/` is
one topic per file — terminology, wiring, invariants, and the gotchas that were
each paid for in debugging. **Read `invariants.md` before changing manager
behaviour.**

## Build and test

Every module is its own Go module with standard-library-only dependencies,
except the operator. From the repository root:

```sh
# every module, discovered rather than listed — the same answer CI's matrix gets
for m in $(.github/components.sh modules | jq -r '.[]'); do
  (cd "$m" && go build ./... && go vet ./... && go test ./...)
done
```

The operator lives in `platform/manager/`:

```sh
cd platform/manager

# after editing api/v1alpha1/ — regenerates deepcopy and the CRDs the chart ships
go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5 object paths=./api/...
go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5 crd paths=./api/... \
  output:crd:artifacts:config=../../chart/crds

# unit tests plus the envtest integration suite against a real API server
KUBEBUILDER_ASSETS=$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.19 \
  use 1.31.x --bin-dir ~/.envtest -p path) go test ./...
```

The console's UI:

```sh
cd platform/console/ui && npm ci && npm test && npm run build
```

The chart:

```sh
helm lint chart/
helm template agent-ops chart/ --set global.demo.enabled=true
```

CI runs all of it on every pull request, plus `kubeconform` over each rendered
chart permutation and the publication guard below.

## Commit messages

**`type(scope): what the commit does, as a sentence.`**

```
feat(wiring)!: the Pipeline names the identity, and SILENCE MEANS NO POWER
fix(console): the asset harnesses wrote to platform/docs, and said it worked
docs(guides): the guides never explained the field a reader has to set
chore(git): ignore build artifacts by pattern, and drop the one that slipped
```

- **Types**: `feat`, `fix`, `docs`, `chore`, `refactor`, `test`, `ci`.
- **Scope** is the component, the subsystem or the surface — `manager`,
  `console`, `wiring`, `site`, `openspec`.
- **`!` before the colon marks a breaking change**, and a breaking change owes
  an entry in `docs/CHANGELOG.md`.
- **The subject says what changed and why it mattered**, not what files were
  touched. `git log --oneline` is meant to read as an account of the project.

## The publication guard

**This repository is read by strangers, and a guard enforces that nothing in it
names a private deployment** — no hostname, address, chat identifier or person
outside a documented allowlist, across the whole tree *and* the commit messages
of the range under review.

```sh
python3 .github/scripts/publication-guard.py            # verdict, positions and rules
python3 .github/scripts/publication-guard.py --show     # LOCAL ONLY — prints matches
```

Two things follow for a contributor:

- **Every shipped example carries a documented placeholder**, never a working
  value. An example that works when pasted unchanged is a leak that looks like
  a courtesy. `.github/publication-allowlist.json` declares the permitted
  shapes; a legitimate new reference is an **allowlist entry**, never a
  loosened pattern.
- **Record the verdict, never the text.** If you write down that the guard
  passes, write "the guard passes" — pasting a match into a commit message or a
  pull request description reintroduces exactly what the guard removed, in a
  place the guard cannot read.

## Pull requests

The template asks three things, and they are the review:

1. **What changed**, in the terms the commit convention uses.
2. **What it affects** — a CRD's meaning, a contract, what the chart grants, or
   none of those.
3. **Whether the documentation the change made untrue was updated in the same
   commit.** See above; this is not a follow-up.

Keep a pull request to one concern. A green CI run is necessary and not
sufficient — a rendered chart is not a running one, and several of the gotchas
in `.claude/rules/gotchas.md` are things that passed every check and still
failed in a cluster.

## Licence

By contributing you agree that your contributions are licensed under the
[Apache License 2.0](LICENSE), the same terms that cover the project.
