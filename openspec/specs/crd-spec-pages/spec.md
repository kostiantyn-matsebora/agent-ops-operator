# crd-spec-pages Specification

## Purpose
A published, concise specification page per CRD on the adopter site: what each
kind is for, a commented example instance, and every field with its meaning.
These pages replaced the generated `cr-reference.md` dump as the field
reference an adopter reads.

## Requirements

### Requirement: Every CRD has its own published spec page

The site SHALL carry one page per CRD under `docs/crds/`, published the same
way the three adapter contracts were promoted: front matter, a declared
permalink (`/crds/<kind>/`, lowercase kind), and a nav entry.

**A kind whose CRD declares itself scaffolding is excluded by name** — today
`ControlPlaneProbe`, which carries no business meaning an adopter could act
on.

The exclusion SHALL live in the generator, and the check SHALL fail when an
excluded name matches no CRD, so the exclusion dies with the kind.

Each page SHALL carry, in this order:

1. one or two sentences saying what the kind is for and which kinds reference
   it
2. **spec, at a glance** — one realistic example instance as a fenced YAML
   block, each top-level field carrying a short trailing comment
3. **every field** — a table of the kind's spec fields with nested shapes
   unrolled as dotted paths (`attributeFilters[].key`): name, type, required
   or optional, and one sentence of meaning carrying the schema's own enum
   choices and default

**The page alone SHALL be enough to write the CR.**

- A recognized embedded core-Kubernetes shape (`EnvVarSource`) is one row
  stating its choices, never an unrolled subtree.
- A value vocabulary the schema cannot state — the filterable signal
  labels, an opaque `config`'s shape — is said in the page's hand prose.

**The spec page SHALL be the single source of truth for a kind's fields.**
Another page needing field detail references `/crds/<kind>/` rather than
restating it — `concepts.md` keeps the model.

The pages SHALL use the model's own vocabulary: a Pipeline is wiring, a
profile is behaviour, a signal STARTS a conversation.

#### Scenario: An adopter looks up a field

- **WHEN** an adopter writing a `Pipeline` opens `/crds/pipeline/`
- **THEN** every `spec` field of the CRD is on the page, nested ones
  included, each with its type, its enum and default where the schema has
  one, and one sentence of meaning

#### Scenario: An adopter writes a filter

- **WHEN** an adopter writing a `SignalSource` opens `/crds/signal-source/`
- **THEN** the page itself says the operators a requirement takes and which
  label keys the shipped adapters send, with no second page required

#### Scenario: A kind is added

- **WHEN** a CRD is added to `chart/crds/`
- **THEN** the change that adds it also adds its page and nav entry, and CI
  fails while the page set and the CRD set disagree

#### Scenario: A scaffolding kind is retired

- **WHEN** an excluded kind is removed from `chart/crds/`
- **THEN** the check fails until its exclusion entry is removed too

### Requirement: Field tables are generated, prose is written

The example instance and the field table SHALL be generated from
`chart/crds/` through the same marker mechanism the guides use, and CI SHALL
fail on a stale block. The opening prose and every sentence of meaning SHALL
be hand-written.

A generated block SHALL fail loudly on a field the CRDs no longer have,
rather than silently keeping it.

#### Scenario: A field is renamed

- **WHEN** a CRD field is renamed and the generator runs
- **THEN** the page's generated blocks carry the new name, and CI fails until
  they are regenerated

#### Scenario: A hand edit lands in a generated block

- **WHEN** a contributor edits inside a generated block
- **THEN** the next generator run reverts it, and the CI check reports the
  difference

### Requirement: The spec pages replaced cr-reference.md

`docs/cr-reference.md` SHALL be deleted. Every document that pointed a reader
at it for CRD fields SHALL point at the per-kind pages instead.

#### Scenario: A reader follows an old pointer

- **WHEN** a page that linked the deleted `cr-reference.md` is read
- **THEN** its reference pointer reaches the per-kind spec pages

### Requirement: The spec pages are reachable from the navigation and the diagram

The nav SHALL carry a group listing every adopter-facing kind's page — the
set derived from `chart/crds/`, never a fixed count — and every kind on the
Introduction's domain-model diagram SHALL link to that kind's page.

#### Scenario: A reader clicks a kind on the diagram

- **WHEN** a reader clicks any kind on the Introduction's domain-model
  diagram
- **THEN** they land on that kind's spec page

### Requirement: Every shipped runtime has a published page, the reference runtime first

The Runtimes nav group SHALL list one page per shipped runtime bundle, and
the reference runtime's page SHALL come first.

`docs/runtimes/claude.md` SHALL state that the vendor ships as two images —
the SDK bundle (the default) and `claude-cli`, the spawn-per-unit bundle
kept for compatibility — and link the comparison.

`docs/claude.md` SHALL be deleted, every live pointer repointed at the
published page. An archived changelog entry is left verbatim.

#### Scenario: An adopter looks for the default runtime

- **WHEN** an adopter opens the Runtimes nav group
- **THEN** the first entry is the reference runtime's page, and the page
  says what it executes, what it needs, where its context lives and what it
  costs

### Requirement: A nav entry may carry its mark

A `_data/nav.yml` entry MAY name an `icon:` file from `assets/img/logos/`,
and the sidebar SHALL render it beside the title with an empty `alt`. The
Integrations and Runtimes entries SHALL carry their marks.

A themed pair is named by its `-light` file, resolved by the site's one
theme resolver.

#### Scenario: A reader scans the sidebar

- **WHEN** a reader opens any page in either theme
- **THEN** every Integrations and Runtimes entry shows its mark beside its
  title, legible on that theme's ground
