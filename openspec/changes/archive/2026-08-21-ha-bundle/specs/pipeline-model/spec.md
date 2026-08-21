## MODIFIED Requirements

<!-- Reconciled with the `k8s-bundle-wiring` change, which modifies this same
requirement and lands first. The text below is that change's version verbatim,
plus one scenario of this change's own. Whichever archives second folds rather
than restates — so if `k8s-bundle-wiring` has already archived, this delta is
additive against the synced requirement and nothing here re-argues the rule.

IMPLEMENTATION CORRECTION: this bundle's wiring flag was to default ON, under
the clause below permitting a subchart that substantially owns its lane to
document an exception. It does not — it ships DEFAULT OFF like the other two
bundles that qualify, so the exception clause goes unused by this change and the
general rule applies unmodified. -->

### Requirement: Chart-managed wiring is declared once, at the top
Wiring names a profile, signal sources and channels that routinely originate in
DIFFERENT components, and a subchart can see only itself. The parent scope is
therefore the DEFAULT place wiring is declared, and remains the only scope that
sees every component. A declaration SHALL require a profile and MAY name signal
sources, channels, toolsets and MCP configs; a declaration naming no profile
SHALL fail the render, because a Pipeline with no profile has no agent to run.

A subchart MAY render `Pipeline` objects only when ALL of the following hold:

- rendering is gated by an explicit wiring flag the operator can turn off,
  leaving every other component of that subchart intact;
- every reference to an object the subchart does not itself render is a
  values-supplied NAME, omitted from the rendered Pipeline when unset, so the
  subchart never names an object no component created;
- each rendered Pipeline renders only when its own profile renders;
- the wiring flag DEFAULTS OFF, so enabling a subchart for its components never
  silently adds a route to an install that declares its own.

The default-off rule has one permitted exception: a values path whose declared
purpose is a turnkey install — a demo or quickstart mode — MAY force a
subchart's wiring flag on, and SHALL force on only the LEAST-PRIVILEGED route
that subchart offers. A subchart that substantially owns its lane MAY document
an exception to the default in its own specification; the general rule does not
carry it.

A subchart that cannot meet these conditions SHALL render no wiring. Shipping
wiring is a choice a bundle makes for a lane it substantially owns — it does not
become the norm, and a bundle whose sources and channels all come from elsewhere
has nothing to gain from it.

#### Scenario: One route spanning several components
- **WHEN** an install combines a cluster-events source from one component with a
  chat surface from another, answered by one agent
- **THEN** a single Pipeline declared at the parent scope claims both sources
  and delivers to the channel

#### Scenario: A component's source is inert until claimed
- **WHEN** a component renders a signal source and neither the install nor that
  component declares a Pipeline claiming it
- **THEN** the source reports `Wired=False` and drops its signals, exactly as an
  unclaimed source always does

#### Scenario: A profile-less declaration is refused
- **WHEN** a wiring entry omits its profile
- **THEN** the render fails naming the entry

#### Scenario: A bundle's wiring can be declined
- **WHEN** a subchart that ships wiring has its wiring flag turned off
- **THEN** it renders no Pipeline, every other component of that subchart still
  renders, and the install's own declarations are unaffected

#### Scenario: Enabling a subchart adds no route by itself
- **WHEN** an operator enables a subchart that ships wiring, without setting its
  wiring flag or a turnkey mode
- **THEN** no Pipeline is rendered by that subchart, and the install's own
  `pipelines:` declarations remain the only routes

#### Scenario: A turnkey mode forces the safe route
- **WHEN** a demo or quickstart values path turns a subchart's wiring on
- **THEN** the route it renders is the subchart's least-privileged one, and a
  more privileged route renders only where the install asked for it explicitly

#### Scenario: Two claimants are permitted, not refused
- **WHEN** a subchart's wiring and an install-declared Pipeline both claim one
  source
- **THEN** both Pipelines render and the source fans out to both, because
  sources are shareable and no conflict guard exists to reinstate

#### Scenario: A bundle never names what nobody rendered
- **WHEN** a subchart renders wiring while an optional channel name is unset
- **THEN** the rendered Pipeline omits that reference entirely rather than
  naming an object that does not exist
