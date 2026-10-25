# crd-spec-pages Delta

## Purpose

A published, concise specification page per CRD on the adopter site: what each
kind is for, a commented example instance, and every field with its meaning —
replacing the generated `cr-reference.md` dump as the field reference an
adopter reads.

## ADDED Requirements

### Requirement: Every CRD has its own published spec page

The site SHALL carry one page per CRD under `docs/crds/`, published the same
way the three adapter contracts were promoted: front matter, a declared
permalink (`/crds/<kind>/`, lowercase kind), and a nav entry.

Each page SHALL carry, in this order:

1. one or two sentences saying what the kind is for and which kinds reference
   it
2. **spec, at a glance** — one realistic example instance as a fenced YAML
   block, each top-level field carrying a short trailing comment
3. **every field** — a table of the kind's spec fields: name, type, required
   or optional, and one sentence of meaning

The pages SHALL use the model's own vocabulary: a Pipeline is wiring, a
profile is behaviour, a signal STARTS a conversation.

#### Scenario: An adopter looks up a field

- **WHEN** an adopter writing a `Pipeline` opens `/crds/pipeline/`
- **THEN** every `spec` field of the CRD is on the page, each with its type
  and one sentence of meaning

#### Scenario: A kind is added

- **WHEN** a CRD is added to `chart/crds/`
- **THEN** the change that adds it also adds its page and nav entry, and CI
  fails while the page set and the CRD set disagree

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

### Requirement: The spec pages replace cr-reference.md

`docs/cr-reference.md` SHALL be deleted. Every document that pointed a reader
at it for CRD fields SHALL point at the per-kind pages instead.

#### Scenario: A reader follows an old pointer

- **WHEN** a page that used to link `cr-reference.md` is read
- **THEN** its reference pointer reaches the per-kind spec pages

### Requirement: The spec pages are reachable from the navigation and the diagram

The nav SHALL carry a group listing the thirteen pages, and every kind on the
Introduction's domain-model diagram SHALL link to that kind's page.

#### Scenario: A reader clicks a kind on the diagram

- **WHEN** a reader clicks any kind on the Introduction's domain-model
  diagram
- **THEN** they land on that kind's spec page
