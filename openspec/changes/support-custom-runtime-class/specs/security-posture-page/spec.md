## MODIFIED Requirements

### Requirement: The page presents defence in depth as three controls, each with its threat and its cost

The page SHALL present the independent controls under the heading
**defence in depth**, naming each by its standard term:

| Control | Bounds |
|---|---|
| network segmentation | reach between components |
| egress control | the bound toolsets, enforced outside the agent |
| cluster authorization | the identity the agent runs as, and Secret exposure |
| kernel isolation | the syscall surface the agent's own container can reach |

Each SHALL be stated in the SAME four parts, so a reader compares them
rather than reading separate essays: **threat**, **control**, **cost**,
**residual risk**.

Each SHALL name its residual risk rather than only its benefit. Network
policy applies cleanly on a cluster that does not enforce it and protects
nothing.

Egress control covers neither stdio nor `https` MCP endpoints, and
governs no tool ARGUMENTS. No `secrets` verb is not the same as cannot
read Secrets, because the kubelet resolves a Secret when it builds a pod.

**Kernel isolation's entry SHALL state its validated scope exactly, never
a blanket claim.** It SHALL name which workload kinds have been run under
a non-default `runtimeClassName` and confirmed working.

It SHALL name any component known not to work under it, such as
`egress-proxy`'s interception mechanism, if that is what validation
found. It SHALL NOT claim compatibility beyond what was actually run and
observed.

#### Scenario: A reader weighs enabling a control

- **WHEN** the reader reads any of the four
- **THEN** the threat it answers, what enabling it costs, and its residual
  risk are all present in that section, in the same shape as the others

#### Scenario: A control is described without its residual risk

- **WHEN** a section states only what the control protects
- **THEN** the section is incomplete, and the residual risk is added
  rather than the claim being softened

#### Scenario: Kernel isolation's limitation is stated, not hidden

- **WHEN** validation finds a component that does not work under a
  non-default `runtimeClassName`
- **THEN** the kernel isolation entry names that component and the
  limitation, rather than describing the control as universally compatible
