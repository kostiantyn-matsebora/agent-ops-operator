## MODIFIED Requirements

### Requirement: The credential and home volume are wired, not restated
When `runtime.credentialsSecret.token` is supplied the component SHALL create that Secret, so the credential is release-managed; when it is empty the `AgentRuntime` SHALL reference the named Secret without creating it, and the post-install notes SHALL warn that the reference is unsatisfied. The credential SHALL reach the runtime as env via `valueFrom` — the manager SHALL read no Secrets.

The rendered `AgentRuntime` SHALL take `home.pvcRef` from the parent's own `persistence` block (its claim name, or `existingClaim`) with an explicit `runtime.homePvcRef` override for a claim the chart did not create. It SHALL take an optional `workspace.pvcRef` from the parent's workspace persistence block the same way. No operator SHALL have to copy a claim name between values blocks, for either volume.

Home persistence SHALL be enabled by default and workspace persistence SHALL be disabled by default. The asymmetry is deliberate: losing session files silently costs conversational history, whereas losing a checkout costs a re-clone.

`runtime.idleTtlMinutes` SHALL default to empty and SHALL then follow the release's `runtimeIdleTtlMinutes`, so there is one number unless an operator deliberately wants two. The chart SHALL WRITE the resolved value into the rendered CR rather than omitting the field: `AgentRuntime.spec.idleTtlMinutes` carries a CRD default, so an omitted field is not unset — the API server stores the CRD default and the manager prefers any non-zero spec value over its own configured TTL, which makes an omitted field render a correct-looking manifest and a wrong stored object.

#### Scenario: Credential comes back with the release
- **WHEN** `runtime.credentialsSecret.token` is set from a secret store
- **THEN** the Secret renders with the release and the runtime references it by name only

#### Scenario: Unsatisfied reference is announced, not silent
- **WHEN** `runtime.enabled=true` and no token is supplied
- **THEN** the install succeeds and the notes state that runtime pods will reach `CreateContainerConfigError` until the named Secret exists, because the kubelet resolves the reference and nothing else reports it

#### Scenario: Persistence needs no second declaration
- **WHEN** `persistence.enabled=true`
- **THEN** the rendered `AgentRuntime` carries `home.pvcRef` naming the chart's claim, with no runtime-side value set

#### Scenario: Sessions persist without being asked for
- **WHEN** the chart is installed with no persistence values supplied
- **THEN** the home claim is provisioned and the rendered `AgentRuntime` references it

#### Scenario: Workspace is wired the same way when enabled
- **WHEN** workspace persistence is enabled
- **THEN** the rendered `AgentRuntime` carries `workspace.pvcRef` naming the chart's workspace claim, with no runtime-side value set

#### Scenario: Idle TTL has one default
- **WHEN** `runtime.idleTtlMinutes` is left empty
- **THEN** the rendered `AgentRuntime` carries the release's `runtimeIdleTtlMinutes`, and runtime pods use that number rather than the CRD's default
