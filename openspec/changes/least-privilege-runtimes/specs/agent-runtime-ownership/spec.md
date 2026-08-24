## REMOVED Requirements

### Requirement: Unset RBAC mode grants nothing except in demo mode
**Reason**: The setting it governs is removed. `rbacMode` rendered an extra,
named ServiceAccount carrying a preset posture, and that account granted nothing
until a `Pipeline` named it — so the name described a mode the runtime was in,
which is the reading it caused an incident for and was reverted from.

Its one load-bearing behaviour was demo mode resolving empty to `readonly`. That
does not survive scrutiny: an agent reaches the cluster through the MCP SERVER,
which carries its own account and its own grant, and the runtime image ships no
kubectl. Demo's read access is the Kubernetes bundle's to provide, and that
bundle already renders one identity per route it ships.

The posture it protected — an ordinary upgrade widening nothing, and `full` never
inferred — is kept and strengthened below: the default is NO permissions, with no
mode able to change it.

### Requirement: The main chart owns the default agent runtime
**Reason**: Replaced, because both halves of the heading became false. The parent
no longer owns THE runtime — a bundle may ship one — and it no longer renders a
single `runtime:` component. What it still owns is the DEFAULTS every runtime
inherits and the FLOOR account, which the replacement states.

### Requirement: The floor identity can do NOTHING, and a release has many
**Reason**: Replaced. Every scenario it carried was written around `rbacMode` —
one knob rendering one binding set, the mode never widening the floor, targeted
grants composing with the mode. The knob is removed, so those scenarios describe
a mechanism that no longer exists.

The PROPERTY it protected is not lost. The replacement strengthens it: the floor
holds nothing, no setting can widen it, and there is no preset posture at all.

## ADDED Requirements

### Requirement: The main chart owns the runtime DEFAULTS, and a bundle may ship a runtime
The parent chart SHALL own the runtime DEFAULTS — a complete configuration every
runtime inherits — and the FLOOR ServiceAccount. It SHALL NOT be the only thing
that may render an `AgentRuntime`.

A BUNDLE SHALL be able to ship a runtime, declaring it in its own values and
rendering its own CR, exactly as a bundle already ships pipelines, sources and
profiles. A vendor arriving as a bundle SHALL NOT require a hand-written CR.

**THIS REVERSES A RULE THAT WAS AN INVARIANT, AND THE TWO FAILURES BEHIND IT ARE
ANSWERED RATHER THAN FORGOTTEN:**

1. **A chat-only install could execute nothing**, because the runtime lived in a
   bundle and no bundle was on. That is now caught: the render FAILS when no
   runtime answers to the default name while a Pipeline resolves to it — see
   `runtime-declaration`. A failed render is recoverable; conversations stuck in
   `Pending` with the reason in no one's view are not.
2. **Two runtime ServiceAccounts existed and one was granted everything.** That
   was a consequence of a release-wide MODE binding a shared account. Accounts
   are per-route now, the floor is bound to nothing, and no mode exists to widen
   anything.

What stays exclusively the parent's is the DEFAULTS and the FLOOR. A bundle
SHALL NOT render either: defaults differing per bundle would be one fact in as
many places as there are vendors, and a floor account a bundle could render
would make "a route naming nothing holds nothing" a claim no single file checks.

#### Scenario: A bundle-free install still executes
- **WHEN** an install enables no bundle at all and supplies the model credential
- **THEN** a runtime answering to the default name is rendered and conversations execute

#### Scenario: A bundle ships its vendor
- **WHEN** a bundle that ships a runtime is enabled
- **THEN** its `AgentRuntime` renders from the bundle's own values and inherits the release-wide defaults

#### Scenario: Removing the last runtime is refused, not discovered later
- **WHEN** an install disables the only bundle shipping a runtime while a Pipeline resolves to the default name
- **THEN** the render fails naming what is missing

#### Scenario: No bundle renders the defaults or the floor
- **WHEN** any combination of bundles is enabled
- **THEN** the runtime defaults and the floor ServiceAccount come from the parent alone

### Requirement: The floor holds nothing, and there is no preset posture
The account a `Pipeline` naming no `serviceAccountName` runs as SHALL hold no
Kubernetes permissions, and the chart SHALL refuse to bind anything to it.
SILENCE SHALL MEAN NO POWER, with no setting able to change it.

It shipped inverted once: a release-wide mode bound its posture to the account
every unnamed route inherited, and three of four routes in the reference install
held pod-delete and node-patch because nobody typed a field — two of them routes
that reach no Kubernetes API at all.

**THERE SHALL BE NO PRESET POSTURE.** An install wanting more than nothing SHALL
declare an account and name it on the routes that need it, or use one a bundle
renders for its own routes. A named posture is a grant nobody reviewed; a
declared account is one somebody wrote down.

**THE DEFAULT ACCOUNT IS A REFERENCE, NOT A CREATION.** Where an install names
the account a Pipeline inherits, the chart SHALL reference it and SHALL NOT
create it — the posture adapters already have, where naming is not creating.

The chart SHALL always render its own floor account regardless. That is what
keeps it NAMEABLE as a way to restrict one route to nothing, on an install whose
inherited default is an account of the operator's that carries rights.

#### Scenario: A route that names nothing can do nothing
- **WHEN** a Pipeline declares no `serviceAccountName`
- **THEN** its conversations run under an account denied every verb on every resource, whatever else the install configures

#### Scenario: Naming the default does not create it
- **WHEN** an install points the inherited default at an account it already owns
- **THEN** the chart references that account and creates only its own floor

#### Scenario: The floor stays available to restrict a route
- **WHEN** the inherited default is an account carrying rights
- **THEN** a Pipeline may name the chart's floor account and hold nothing instead

#### Scenario: No preset can widen an upgrade
- **WHEN** an existing install upgrades without declaring an account
- **THEN** nothing it runs gains a permission it did not already have

#### Scenario: A second trust level needs no second runtime
- **WHEN** an install wants an observing route and an acting route on one runtime image
- **THEN** it declares a second ServiceAccount with its own RBAC and names it on the acting Pipeline, and does NOT clone the `AgentRuntime`

#### Scenario: No bundle renders the floor
- **WHEN** any bundle is enabled
- **THEN** no bundle renders the floor account, though a bundle DOES render an identity for each route it ships and may render a runtime of its own
