# security-posture-page Specification

## Purpose

The adopter site's answer to the question an operator asks before installing:
an agent runs model output inside my cluster, so what is it trusted with, what
does the default install grant, and what is still open.

Presented as a THREAT MODEL — trust boundaries, the flows that cross them, and
the control on each crossing — never as a list of chart values.

## Requirements

### Requirement: The site carries one security page, indexed by threat

The site SHALL publish exactly one security page. It SHALL be organised as a
threat model — what am I trusting, what crosses a trust boundary, what control
sits on each crossing, what is still open — and SHALL NOT be organised as a list
of chart values.

**It SHALL use the vocabulary a security reviewer already has**, not names
invented for this page: defence in depth, network segmentation, egress control,
authorization, residual risk. An invented word makes a reader translate before
they can evaluate.

One page, because the current failure is content scattered across pages indexed
by value. A second security page reproduces that failure with a security label
on it.

#### Scenario: A reader evaluates the project before installing

- **WHEN** a reader who has not installed anything looks for the security posture
- **THEN** one page answers what the default install grants, what each control
  bounds, and what the residual risk is
- **AND** they reach it without reading the installation page first

#### Scenario: A second security page is proposed

- **WHEN** security content would be added to the site
- **THEN** it goes on the existing security page, and no second security page is
  created

### Requirement: The page opens with a threat model, and a register keyed to it

The page SHALL open with a drawing of the **trust boundaries** and the flows that
cross them, and a table naming each crossing, its threat and the control on it.
The two SHALL be joined by a NUMBER, so a reader moves between picture and table
without guessing.

**Every crossing SHALL appear, including any that carries no control.** A threat
model showing only the mitigated crossings is the diagram equivalent of a page
listing only what is handled, and the unmitigated one SHALL be drawn distinctly.

The drawing SHALL be committed build output of a script, in both themes, and
SHALL be composed LANDSCAPE at the width the site's diagram frame renders. A
canvas wider than the frame is scaled down and takes its labels with it.

The page SHALL carry further illustrations at the claims prose states poorly —
at minimum the Secret-through-the-kubelet path and the context mount.

#### Scenario: A reviewer opens the page

- **WHEN** the reader arrives with no knowledge of the product
- **THEN** the first thing on the page is the trust boundaries and what crosses
  them, and the register beneath states each crossing's threat and control

#### Scenario: A crossing has no control

- **WHEN** a flow crosses a trust boundary with nothing bounding it
- **THEN** it is on the drawing, marked distinctly from the mitigated crossings,
  and it appears in the residual-risk section

#### Scenario: A trust boundary moves

- **WHEN** a change alters a boundary or a flow across one
- **THEN** the drawing's script is re-run and its output committed, because no
  CI job regenerates it

### Requirement: The page states that the default install grants nothing

The page SHALL state, before any control is described, that a Pipeline naming no
service account runs as an identity bound to nothing, that **no preset posture
exists at any level** — more than nothing is an account an install declares,
stating its own rules, and names on the routes that need it — and that pod
execution is off by default.

It SHALL NOT read as though every control is off. Cluster permissions default to
nothing; **egress mediation defaults on**. A blanket "grants nothing" understates
one control while describing the others, and a reader who acts on it plans to
enable something that is already enabled.

This is the answer most readers arrive for. Stating it after the controls makes a
reader assemble it from three sections that each describe a switch.

#### Scenario: A reader asks what a default install can do to their cluster

- **WHEN** the reader opens the page
- **THEN** the default posture is stated before any individual control is
  described
- **AND** it names the floor identity bound to nothing, the absence of any preset
  posture, and pod execution

#### Scenario: A control that ships enabled is described

- **WHEN** the page states the default posture
- **THEN** a control that defaults on is named as on, rather than folded into a
  blanket statement that the install grants nothing

### Requirement: The page presents defence in depth as three controls, each with its threat and its cost

The page SHALL present the three independent controls under the heading
**defence in depth**, naming each by its standard term:

| Control | Bounds |
|---|---|
| network segmentation | reach between components |
| egress control | the bound toolsets, enforced outside the agent |
| cluster authorization | the identity the agent runs as, and Secret exposure |

Each SHALL be stated in the SAME four parts, so a reader compares them rather
than reading three essays: **threat**, **control**, **cost**, **residual risk**.

Each SHALL name its residual risk rather than only its benefit. Network policy
applies cleanly on a cluster that does not enforce it and protects nothing.
Egress control covers neither stdio nor `https` MCP endpoints, and governs no
tool ARGUMENTS. No `secrets` verb is not the same as cannot read Secrets, because
the kubelet resolves a Secret when it builds a pod.

#### Scenario: A reader weighs enabling a control

- **WHEN** the reader reads any of the three
- **THEN** the threat it answers, what enabling it costs, and its residual risk
  are all present in that section, in the same shape as the other two

#### Scenario: A control is described without its residual risk

- **WHEN** a section states only what the control protects
- **THEN** the section is incomplete, and the residual risk is added rather than
  the claim being softened

### Requirement: The page states what agent-ops itself holds

The page SHALL state the properties of the product that a reader cannot discover
from the values: that the manager holds no verb on `secrets` and reads none, that
per-adapter tokens are derived rather than stored, that runtime pods run non-root
with per-conversation workspace isolation, how a conversation's accumulated
context is isolated, **where conversation content reaches a log**, and what the
published images carry.

These are true today and stated nowhere on the site. A reader assessing the
product cannot credit a property nobody wrote down.

**The logging property SHALL be stated per component, because it does not hold
uniformly.** The manager, the channel and signal adapters and the console log
identifiers, counts, op ids and errors — never a message body. The reference
RUNTIME writes the agent's own output, its tool-call arguments and its result to
the pod's stdout, so conversation content is in that pod's log and readable by
anyone holding `pods/log` in the operator's namespace. The page SHALL state both
halves and SHALL NOT generalise the first into a claim about "no component".

The blanket form was in this spec and was REFUTED at verification, before a line
of the page was written. It is recorded because the blanket form is the one a
later edit reaches for — it is shorter and it sounds stronger.

#### Scenario: A reader asks what the operator itself can reach

- **WHEN** the reader looks for what agent-ops holds rather than what it grants
  an agent
- **THEN** the page states the manager's own Secret access, how adapter
  credentials are held, and the runtime pod's isolation

#### Scenario: A reader asks whether conversation content reaches a log

- **WHEN** the reader looks for where a message body can be read outside a thread
- **THEN** the page names the components that log none of it, and names the
  runtime pod's log as carrying the agent's output
- **AND** the runtime pod log appears in the not-addressed section as well

#### Scenario: The logging property is generalised

- **WHEN** an edit would state that no component logs message content
- **THEN** the edit is refused as false, and the per-component form stands

### Requirement: The page states how a conversation's context is isolated, and where that isolation does not hold

A conversation's accumulated context is the record of everything an agent was
told and everything it produced. The page SHALL state where that context lives
and which other agents can reach it, in each of the modes an install can be in.

It SHALL state that the isolating mode is **structural rather than
permissive**: the agent container holds the live context on ephemeral pod-local
storage and holds no mount of the durable volume at all, so an agent cannot read
another conversation's context because there is nothing to read from, not because
a permission denies it. Durability is preserved by a separate component that
holds the volume and snapshots to it.

It SHALL state that this mode is **what a default install runs**: the reference
runtime ships the context paths its own backend uses, so synchronisation is on
without an operator setting anything.

It SHALL equally state **where that does not hold**. The mode needs a runtime
that declares its context paths, a sidecar image, and a durable context volume;
short any of the three, the pod is the unsynchronised one. Where a durable volume
exists and the runtime declares no paths — which is what a second vendor's
runtime gets until its own entry states them — that whole volume is mounted into
the agent container, and any conversation's context is readable by any other
conversation's agent. That consequence SHALL appear in the not-addressed section
as well, because a reader who skips to it is the reader most exposed to it.

The page SHALL NOT present the isolating mode as unconditional. It is the posture
a default install has, and the posture any runtime declaring its paths has.

#### Scenario: A reader asks whether one agent can read another conversation's context

- **WHEN** the reader looks for cross-conversation isolation
- **THEN** the page answers per mode, and names the default install's answer
  first
- **AND** the isolating mode is described as structural — no mount rather than a
  denied permission

#### Scenario: The isolating mode is described

- **WHEN** the page describes context isolation
- **THEN** it states the three conditions the mode needs and what a pod short of
  any of them gets
- **AND** the unsynchronised mode's shared-volume consequence is stated in the
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

### Requirement: Residual risk is a section of the page, and it is load-bearing

The page SHALL carry a section named for **residual risk** — the standard term —
naming what is NOT addressed, including every
surface that authenticates nobody, controls whose enforcement the chart cannot
verify, isolation a runtime has only where it declares the paths the mode needs,
conversation content readable from a runtime pod's log, and governance the
product does not attempt.

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
  a non-declaring runtime does not get, the conversation content in a runtime
  pod's log, and the ungoverned areas are named in one place

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
