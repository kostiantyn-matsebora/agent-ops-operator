## MODIFIED Requirements

### Requirement: The main chart owns the default agent runtime

The parent chart SHALL render the default `AgentRuntime` from a top-level
`runtime:` component, enabled by default. The component SHALL own everything
describing how agents execute — image, LLM credential reference, idle TTL,
`nodeSelector`, resources, and the home volume — and SHALL render the runtime
named `runtime.name` (default `default`), which is the name a profile with no
`runtimeRef` falls back to.

The parent chart MAY additionally render further `AgentRuntime` CRs from an
operator-declared list, empty by default, so one install can execute different
routes on different backends — a vendor runtime for hard lanes and a local-model
runtime for routine ones, selected per profile by `runtimeRef`. Each entry SHALL
carry its own image, `contextStorage`, environment and workload knobs, because
those are exactly what differs between backends; each SHALL default to the same
home and workspace claims and the same runtime ServiceAccount the `default` one
uses, so an extra runtime is an execution choice and never, by itself, a
privilege change. An entry MAY name a different ServiceAccount, which is the
supported way to give one backend a different trust level — the identity stays
runtime-level, never pipeline-level.

An empty list SHALL render exactly what it renders today, byte for byte.

No subchart SHALL render an `AgentRuntime`, a runtime ServiceAccount, or a
runtime credential Secret — this is unchanged and is the rule the additional
list must not be read as relaxing. Bundles contribute domain — signal sources,
profiles, tooling, channels — and reference the substrate the parent provides. A
bundle MAY name a different runtime through its profile's `runtimeRef`, but
SHALL NOT create one.

`runtime.enabled: false` SHALL render nothing from the component, for installs
that manage `AgentRuntime` CRs themselves.

#### Scenario: A default install can execute a conversation

- **WHEN** the chart is installed with default values and no bundle enabled
- **THEN** an `AgentRuntime` named `default` renders, so any profile reaching the
  manager resolves a runtime — and no bundle objects render

#### Scenario: A bundle install renders no runtime of its own

- **WHEN** the chart is installed with any combination of bundles enabled and no
  additional runtimes declared
- **THEN** exactly one `AgentRuntime` and one runtime ServiceAccount render,
  both from the parent, and no bundle renders either

#### Scenario: Two backends on one install

- **WHEN** an operator declares one additional runtime alongside the default
- **THEN** both `AgentRuntime` CRs render from the parent chart, sharing the
  runtime ServiceAccount and the home and workspace claims unless the entry says
  otherwise
- **AND** a profile pointing `runtimeRef` at the additional one executes there

#### Scenario: Declaring an extra runtime changes nothing else

- **WHEN** the additional-runtimes list is empty
- **THEN** the rendered manifest is identical to the release before the list
  existed

#### Scenario: Chat-only install works

- **WHEN** only `telegram-bundle.enabled=true` is set
- **THEN** the install renders a runtime and can execute conversations started
  from chat, without enabling a Kubernetes bundle for its substrate

#### Scenario: Bring your own runtime

- **WHEN** `runtime.enabled=false`
- **THEN** no `AgentRuntime`, credential Secret, or runtime object renders from
  the component, and profiles resolve whichever runtimes the operator applied
