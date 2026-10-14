## ADDED Requirements

### Requirement: The pack exercises a non-default RuntimeClass

The pack SHALL provision a `runsc` `RuntimeClass` inside its cluster and
verify both of the following run under it:

1. **A component workload** — at least one chart-rendered workload
   outside the conversation-pod path (for example the console or a
   signal/channel adapter), rendered with `runtimeClassName: runsc`.
2. **A full conversation** — an `AgentRuntime` pod, including its
   `context-sync` and `egress-proxy` sidecars where configured, rendered
   with `runtimeClassName: runsc` and run to completion.

`egress-proxy`'s behavior under `runsc` SHALL be explicitly asserted, pass
or fail, or explicitly marked skipped with a stated reason. It SHALL NOT
be silently absent from the pack's result.

A failure or skip in this lane SHALL be isolated from the rest of the
pack, so that `runsc` provisioning issues while this coverage is new do
not block an otherwise-passing run.

#### Scenario: A component workload runs under the non-default class
- **WHEN** the pack renders a signal/channel adapter or console workload with `runtimeClassName: runsc`
- **THEN** the workload reaches Ready under the `runsc` RuntimeClass

#### Scenario: A full conversation runs under the non-default class
- **WHEN** the pack starts a conversation on a Pipeline resolving to `runtimeClassName: runsc`
- **THEN** the conversation's runtime pod runs to completion under `runsc`, with its sidecars present where the runtime configures them

#### Scenario: egress-proxy's result is never silently missing
- **WHEN** the conversation lane under `runsc` includes a runtime with `egressMediation.enabled`
- **THEN** the pack's result states whether mediation worked under `runsc`, or names the reason it was skipped

#### Scenario: New RuntimeClass plumbing does not block the rest of the pack
- **WHEN** the `runsc` lane fails while its provisioning is still being tuned
- **THEN** that failure is reported on its own lane and does not fail the pack's other, unrelated lanes
