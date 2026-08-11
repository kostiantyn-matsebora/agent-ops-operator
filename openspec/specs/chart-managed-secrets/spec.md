# chart-managed-secrets Specification

## Purpose
TBD - created by archiving change stable-generated-secrets. Update Purpose after archive.
## Requirements
### Requirement: A credential the chart generates has three sources in a fixed order
For every credential the chart can generate, the value SHALL be taken from the
first available of: an explicitly configured value, an existing Secret of that
name, then a freshly generated value. An operator MAY instead supply a whole
Secret by name, in which case the chart SHALL create none.

An explicitly configured value SHALL take precedence over an existing Secret.
Checking the existing Secret first makes the setting a silent no-op on any
install that already has one — accepted, documented and ignored, with no error
and no log line — and makes rotation require deleting an object rather than
editing a value.

#### Scenario: A pinned token wins over history
- **WHEN** an operator configures a token explicitly on an install that already has a generated Secret
- **THEN** the configured value is what the Secret holds after the upgrade

#### Scenario: Rotation is a values edit
- **WHEN** an operator changes an explicitly configured token and upgrades
- **THEN** the new value takes effect without deleting the Secret first

#### Scenario: Nothing configured generates once
- **WHEN** the chart is installed with no token configured and no Secret present
- **THEN** a value is generated and stored

### Requirement: An upgrade never renders a freshly generated credential
When no explicit value is configured, the Secret SHALL be rendered on install
only and SHALL carry a retention policy that keeps the object when it leaves the
release manifest. Upgrades SHALL NOT render it.

This is what makes stability independent of the renderer. Relying on a cluster
lookup leaves a random value on the upgrade path, and any renderer without
cluster access — `helm template` piped to apply, CI, a GitOps controller, a
client-side dry run — then produces and applies a new credential. Removing the
generated value from the upgrade path removes the hazard rather than making it
conditional.

#### Scenario: A cluster-less render proposes no credential change
- **WHEN** an unchanged install is rendered or diffed by a tool with no cluster access
- **THEN** no credential appears as changed, because none is generated on the upgrade path

#### Scenario: Leaving the manifest does not delete the object
- **WHEN** the first upgrade after this behaviour lands stops rendering a generated Secret
- **THEN** the Secret remains in the cluster with the same value, retained by policy

#### Scenario: An explicit value is still rendered on upgrade
- **WHEN** a token is configured explicitly
- **THEN** the Secret is rendered on install and on every upgrade, so changing the value takes effect

### Requirement: An upgrade that would orphan an unretained credential is refused
When the chart is about to stop rendering a generated Secret that exists in the
cluster without the retention policy, the render SHALL FAIL and SHALL name the
command that applies the policy.

The retention policy is read off the LIVE object, not off the manifest dropping
it, so a Secret created before the policy existed carries none and is deleted by
the first upgrade that stops rendering it. A migration note is not a safeguard:
the failure is silent, immediate and unrecoverable — every browser signed out,
every adapter credential invalidated. Refusing costs one command run once.

The guard SHALL be silent where it cannot act. A renderer with no cluster sees no
existing Secret and prunes nothing, so it has nothing to protect.

#### Scenario: An install predating the policy is protected
- **WHEN** an upgrade would drop a generated Secret that carries no retention policy
- **THEN** the render fails, naming the Secret and the command that retains it

#### Scenario: A retained credential upgrades silently
- **WHEN** the same upgrade runs after the policy has been applied to the object
- **THEN** it succeeds and the Secret keeps its value

### Requirement: Reinstalling does not overwrite a credential still in use
When the chart generates a credential at install time and a Secret of that name
already exists, the existing value SHALL be adopted rather than replaced.

A retained Secret outlives `helm uninstall`, so a reinstall would otherwise mint
a new credential while adapters and browsers still hold the old one.

#### Scenario: Install over a retained Secret adopts it
- **WHEN** the chart is installed into a namespace where a previous release left its credential Secret behind
- **THEN** the existing value is kept and no new credential is generated

### Requirement: Rotating the adapter master credential is announced as breaking
The adapter master token SHALL be configurable explicitly, with the same shape as
the console UI token. Because every per-adapter token is derived from it by HMAC,
changing it SHALL be documented at the setting as invalidating every adapter's
credential at once, until each adapter's pod restarts with the new value.

#### Scenario: The blast radius is stated where the value is set
- **WHEN** an operator reads the adapter token value in the chart's values
- **THEN** the comment states that changing it invalidates every derived adapter token until pods restart

### Requirement: Post-install notes name the source in effect
The notes SHALL print the recipe for reading a credential only when the chart
generated it, and SHALL otherwise name the source — configured value or supplied
Secret.

Printing "read your token here" after every upgrade is what teaches an operator
that the token changes on every deploy; the behaviour and the message about it
are corrected together or the belief survives the fix.

#### Scenario: A pinned token is not presented as something to fetch
- **WHEN** an install configures its UI token explicitly and is upgraded
- **THEN** the notes name the configured source instead of printing a command to read a newly generated value

