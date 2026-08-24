## Purpose

The adopter site's answer to the question an operator asks before installing:
an agent runs model output inside my cluster, so what is it trusted with, what
does the default install grant, and what is still open. Indexed by threat, not
by chart value.

## ADDED Requirements

### Requirement: The site carries one security page, indexed by threat

The site SHALL publish exactly one security page. It SHALL be organised by the
question a reader brings — what am I trusting, what does the default grant, what
does each control bound, what is still open — and SHALL NOT be organised as a
list of chart values.

One page, because the current failure is content scattered across pages indexed
by value. A second security page reproduces that failure with a security label
on it.

#### Scenario: A reader evaluates the project before installing

- **WHEN** a reader who has not installed anything looks for the security posture
- **THEN** one page answers what the default install grants, what each control
  bounds, and what is not addressed
- **AND** they reach it without reading the installation page first

#### Scenario: A second security page is proposed

- **WHEN** security content would be added to the site
- **THEN** it goes on the existing security page, and no second security page is
  created

### Requirement: The page states that the default install grants nothing

The page SHALL state, before any control is described, that a Pipeline naming no
service account runs as an identity bound to nothing, that the runtime RBAC mode
is off by default, and that pod execution is off by default.

This is the answer most readers arrive for. Stating it after the controls makes a
reader assemble it from three sections that each describe a switch.

#### Scenario: A reader asks what a default install can do to their cluster

- **WHEN** the reader opens the page
- **THEN** the default posture is stated before any individual control is
  described
- **AND** it names the floor identity, the runtime RBAC mode and pod execution

### Requirement: The page presents three walls, each with its threat and its cost

The page SHALL present the three independent controls as walls, each stating what
it bounds, what it costs to close, and the honest limit of the control:

| Wall | Bounds |
|---|---|
| who may connect | reach between components |
| what a connected agent may do | the bound toolsets, enforced outside the agent |
| what it may do to the cluster | the runtime's cluster RBAC, and the Secrets boundary |

Each wall SHALL name its limit rather than only its benefit. Network restriction
applies cleanly on a cluster that does not enforce it and protects nothing.
Toolset enforcement outside the agent does not cover stdio or `https` MCP
endpoints. No `secrets` verb is not the same as cannot read Secrets, because the
kubelet resolves a Secret when it builds a pod.

#### Scenario: A reader weighs enabling a control

- **WHEN** the reader reads any of the three walls
- **THEN** the threat it answers, what enabling it costs, and its limit are all
  present in that section

#### Scenario: A wall is described without its limit

- **WHEN** a wall's section states only what the control protects
- **THEN** the section is incomplete, and the limit is added rather than the
  claim being softened

### Requirement: The page states what agent-ops itself holds

The page SHALL state the properties of the product that a reader cannot discover
from the values: that the manager holds no verb on `secrets` and reads none, that
per-adapter tokens are derived rather than stored, that runtime pods run non-root
with per-conversation workspace isolation, how a conversation's accumulated
context is isolated, that no component logs message content, and what the
published images carry.

These are true today and stated nowhere on the site. A reader assessing the
product cannot credit a property nobody wrote down.

#### Scenario: A reader asks what the operator itself can reach

- **WHEN** the reader looks for what agent-ops holds rather than what it grants
  an agent
- **THEN** the page states the manager's own Secret access, how adapter
  credentials are held, and the runtime pod's isolation

### Requirement: The page states how a conversation's context is isolated, and that the isolation is opt-in

A conversation's accumulated context is the record of everything an agent was
told and everything it produced. The page SHALL state where that context lives
and which other agents can reach it, in each of the modes an install can be in.

It SHALL state that the isolating mode is **structural rather than
permissive**: the agent container holds the live context on ephemeral pod-local
storage and holds no mount of the durable volume at all, so an agent cannot read
another conversation's context because there is nothing to read from, not because
a permission denies it. Durability is preserved by a separate component that
holds the volume and snapshots to it.

It SHALL equally state that this mode is **opt-in and not the default**, and that
in the default mode an install with a shared context volume mounts that whole
volume into every agent container — so any conversation's context is readable by
any other conversation's agent. That consequence SHALL appear in the
not-addressed section as well, because a reader who skips to it is the reader
most exposed to it.

The page SHALL NOT present the isolating mode as the posture an install has. It
is the posture an install can choose.

#### Scenario: A reader asks whether one agent can read another conversation's context

- **WHEN** the reader looks for cross-conversation isolation
- **THEN** the page answers per mode, and names the default mode's answer first
- **AND** the isolating mode is described as structural — no mount rather than a
  denied permission

#### Scenario: The isolating mode is described

- **WHEN** the page describes context isolation
- **THEN** it states that the mode is opt-in
- **AND** the default mode's shared-volume consequence is stated in the
  not-addressed section as well

#### Scenario: Durability is assumed to be the cost of isolation

- **WHEN** the reader asks what isolating the context costs in durability
- **THEN** the page states that context remains durable in the isolating mode,
  held by a separate component rather than by the agent

### Requirement: The supply-chain answer is stated as it is, and no further

The page SHALL state what the published artifacts actually carry and SHALL name
what they do not. Images carrying provenance and an SBOM SHALL be stated together
with the absence of signatures and of chart attestation.

It SHALL NOT be given a section of its own while the answer is partial: a heading
promises a subject is handled, and this one is handled in part.

#### Scenario: A reviewer asks whether artifacts are verifiable

- **WHEN** the reader looks for supply-chain provenance
- **THEN** what images carry is stated, with the command that reads it
- **AND** the absence of signing and of chart attestation is stated in the same
  place, not omitted

### Requirement: What is not addressed is a section of the page, and it is load-bearing

The page SHALL carry a section naming what is NOT addressed, including every
surface that authenticates nobody, controls whose enforcement the chart cannot
verify, isolation an install has only if it opted in, and governance the product
does not attempt.

**This section is the page's whole value and SHALL NOT be removed, shortened into
a caveat, or moved into a callout beside a control.** A security page that lists
only what is handled is worth less than no page, because it is read as a claim
that the rest is handled too.

Where the reasoning behind a control's limit is recorded in an architecture
decision record, the page SHALL reference that record rather than reproduce its
analysis.

#### Scenario: A reviewer asks what is still open

- **WHEN** the reader reaches the end of the page
- **THEN** the unauthenticated surfaces, the unverifiable control, the isolation
  that is opt-in and the ungoverned areas are named in one place

#### Scenario: A later edit tidies the page

- **WHEN** an edit would remove or fold away the not-addressed section
- **THEN** the edit is refused, and the section stays a section

#### Scenario: A control's reasoning is already recorded

- **WHEN** the page describes a control whose trade-off analysis exists as an
  architecture decision record
- **THEN** the page states the decision in brief and links the record
- **AND** the record is not copied onto the page and is not edited to be linked

### Requirement: The page states no chart value that the installation page owns

The page SHALL NOT carry values tables, default values, or the YAML that sets a
control. It SHALL name the control in prose and link the installation page, which
remains the single source for keys and defaults.

The two pages cover the same subject on different axes — one by threat, one by
key — which is what makes covering it twice not duplicating it. A values table on
the security page is a second source of truth that rots silently, because nothing
fails when the two disagree.

#### Scenario: A control's key is needed

- **WHEN** the security page describes a control an operator must enable
- **THEN** the page names the control and links the installation page for the key
- **AND** the key, its default and its YAML appear on the installation page only

#### Scenario: A default changes in the chart

- **WHEN** a chart default changes
- **THEN** the installation page is the only site page stating that default
