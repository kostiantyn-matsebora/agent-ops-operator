## MODIFIED Requirements

### Requirement: The main chart owns the default agent runtime
The parent chart SHALL render the default `AgentRuntime` from a top-level `runtime:` component, enabled by default. The component SHALL own everything describing how agents execute — image, LLM credential reference, idle TTL, `nodeSelector`, resources, and the home volume — and SHALL render the runtime named `runtime.name` (default `default`), which is the name a profile with no `runtimeRef` falls back to.

The parent chart MAY additionally render further named runtimes from a values list, defaulting to empty, for installs running more than one VENDOR. Each entry SHALL carry only vendor facts — name, image, its own credential Secret reference, context storage, and pod-level defaults — and SHALL reuse the release's substrate for everything else: the same runtime ServiceAccount, the same home and workspace claims, the same RBAC mode. An entry SHALL NOT be able to name its own ServiceAccount: a second trust level is a second identity and stays a hand-written CR, because letting values choose the identity would make adding a runtime a privilege-escalation path.

No subchart SHALL render an `AgentRuntime`, a runtime ServiceAccount, or a runtime credential Secret. Bundles contribute domain — signal sources, profiles, tooling, channels — and reference the substrate the parent provides. A bundle MAY name a different runtime through its profile's `runtimeRef`, but SHALL NOT create one.

`runtime.enabled: false` SHALL render nothing from the component, for installs that manage `AgentRuntime` CRs themselves.

#### Scenario: A default install can execute a conversation
- **WHEN** the chart is installed with default values and no bundle enabled
- **THEN** an `AgentRuntime` named `default` renders, so any profile reaching the manager resolves a runtime — and no bundle objects render

#### Scenario: A bundle install renders exactly one runtime
- **WHEN** the chart is installed with any combination of bundles enabled and no additional runtimes configured
- **THEN** exactly one `AgentRuntime` and one runtime ServiceAccount render, both from the parent

#### Scenario: A second vendor is an entry, not a second substrate
- **WHEN** an install configures one additional runtime for another vendor
- **THEN** two `AgentRuntime` objects render, each with its own image and credential Secret, while still exactly one runtime ServiceAccount and one home volume exist for the release

#### Scenario: An additional runtime cannot pick its own identity
- **WHEN** an install tries to give an additional runtime a different ServiceAccount through values
- **THEN** the chart does not render one: the runtime SA is the release's, and a different trust level requires an `AgentRuntime` applied outside the chart

#### Scenario: Chat-only install works
- **WHEN** only `telegram-bundle.enabled=true` is set
- **THEN** the install renders a runtime and can execute conversations started from chat, without enabling a Kubernetes bundle for its substrate

#### Scenario: Bring your own runtime
- **WHEN** `runtime.enabled=false`
- **THEN** no `AgentRuntime`, credential Secret, or runtime object renders from the component, and profiles resolve whichever runtimes the operator applied
