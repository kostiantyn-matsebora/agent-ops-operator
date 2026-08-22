## ADDED Requirements

### Requirement: The site carries a guide per adoption tier, in learning order

The site SHALL publish how-to guides covering four tiers of adoption: writing an
agent and its wiring; giving that agent capabilities; implementing a signal or
channel adapter; and implementing an agent runtime.

The tiers SHALL be ordered by what a reader must UNDERSTAND, not by what they
can break. Risk is not monotonic along that order — a capability binding is pure
YAML and can grant more than an adapter's code ever could — so **each guide
SHALL state what its own mistake costs**, and no guide may imply that danger
grows with the tier.

Every guide SHALL have the same four parts: what the reader is doing, a minimal
custom resource to fill in, a link to the full field reference, and the in-repo
implementation to copy from.

A guide SHALL NOT restate reference material. The CRD reference and the contract
documents are linked, never reproduced.

#### Scenario: A reader finishes Getting started

- **WHEN** they look for what to do next
- **THEN** the Introduction's guides section names the guides, and the first one
  takes them from the demo install to an agent of their own

#### Scenario: A guide would explain a CRD field in full

- **WHEN** a guide needs field semantics beyond what its task uses
- **THEN** it links the reference and does not reproduce it

#### Scenario: A tier looks safe because it is early

- **WHEN** a guide covers a tier that is early in the order but wide in effect
- **THEN** it states that effect as plainly as a later guide states its own

### Requirement: Custom resource templates and examples are generated, not written

Every custom resource template on the site SHALL be generated from the CRDs the
chart ships. Every worked example SHALL be rendered from the chart's own values
for the bundle that owns that lane.

Neither SHALL be hand-written or invented. An invented example is a second set
of values to keep true, and it is how a real identifier gets pasted in as the
better-looking example.

Because the site is built by a branch deploy with no build step, generated files
SHALL be committed — and CI SHALL regenerate them and FAIL on any difference.
Committing generated output without that check produces a file that is correct
the day it is written and silently wrong after the next field rename.

A guide SHALL carry the MINIMAL resource — the required fields plus what that
guide teaches — and link the full generated reference for the rest.

#### Scenario: A CRD field is renamed

- **WHEN** a field is added, renamed or removed in the CRDs
- **THEN** CI fails until the generated templates are regenerated

#### Scenario: A chart value changes

- **WHEN** a bundle's values change what it renders
- **THEN** CI fails until the generated examples are regenerated

#### Scenario: A guide covers a large kind

- **WHEN** a kind has more fields than a reader can usefully scan
- **THEN** the guide shows the minimal resource and links the full reference,
  rather than reproducing every field inline

#### Scenario: An example needs a value the chart does not carry

- **WHEN** a worked example would require an invented identifier
- **THEN** the chart's own placeholder is used, so the example and the shipped
  values cannot disagree
