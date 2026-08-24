# public-repository Specification

## Purpose
TBD - created by archiving change public-exposure. Update Purpose after archive.

## Requirements

### Requirement: The repository carries a licence before it is published

The repository SHALL carry an OSI-approved licence file at its root, and the
README SHALL name that licence. "To be decided" is not a licence: a reader who
cannot tell what they are permitted to do has to assume nothing.

The licence SHALL be Apache-2.0, whose patent grant is what makes an operator
adoptable inside another organisation.

#### Scenario: A stranger evaluates the project

- **WHEN** a reader looks for the terms
- **THEN** a licence file is at the repository root and the README names it

#### Scenario: The licence is changed later

- **WHEN** relicensing is proposed after publication
- **THEN** it requires every contributor's agreement, which is why the licence
  is settled before the first fork exists

### Requirement: A stranger has a route for every kind of contact

The repository SHALL provide a template for each kind of issue it accepts,
SHALL disable blank issues, and SHALL route security reports to a PRIVATE
channel as a contact link rather than as an issue type.

A security template is a public form for a confidential report, which is the
disclosure it exists to prevent.

The repository SHALL carry a code of conduct and a contributing guide. The
contributing guide SHALL describe how a change is proposed in this project,
which is unusual enough that a contributor cannot infer it.

#### Scenario: Someone reports a vulnerability

- **WHEN** a reader looks for how to report a security problem
- **THEN** they are directed to a private advisory, and no issue template
  invites the detail into a public thread

#### Scenario: Someone opens an issue that fits no template

- **WHEN** a reader tries to open a blank issue
- **THEN** they are offered the templates and the contact links instead

#### Scenario: A response time is promised

- **WHEN** the security policy states an acknowledgement target
- **THEN** it is one a single maintainer can keep, because a missed promise is
  missed in public

### Requirement: The repository is findable and describes itself

The repository SHALL carry a description, topics, and a homepage pointing at the
published documentation site. Discussions and Issues SHALL be enabled; Wiki and
Projects SHALL be off, because a surface nobody maintains reads as abandonment.

The documentation site SHALL be published BEFORE the repository is, so that
every link in the README resolves the first time anyone follows it.

#### Scenario: The site is reached from the README

- **WHEN** a first visitor follows a documentation link
- **THEN** the page loads, because Pages was enabled before publication rather
  than after

### Requirement: Publication is gated, because it cannot be undone

Publication SHALL NOT proceed until: identifying content is absent from the
tree, the archive AND the history; the published specs are true; the images and
chart the README names are installable by a stranger; and the licence,
community files and templates exist.

Each condition is one that becomes impossible or expensive after the flip — a
history rewrite breaks every clone, and a contradiction read once is remembered.

#### Scenario: A condition is unmet

- **WHEN** any gate condition is outstanding
- **THEN** the repository stays private, and the outstanding condition is named

#### Scenario: The repository is published

- **WHEN** every condition is met
- **THEN** the flip happens, and no further step in this change depends on the
  repository having been private
