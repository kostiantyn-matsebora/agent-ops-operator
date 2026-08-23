## MODIFIED Requirements

### Requirement: Continuity is promised only where its prerequisite is met
An `AgentRuntime` SHALL declare where its context lives — on its context volume,
in storage the operator does not provide, or nowhere — and the manager SHALL
check that declaration against the deployment before promising continuity.

The declaration and the volume it names SHALL carry the same word. A setting
called `contextStorage` whose `volume` case pointed at something called the home
volume made the reader resolve one concept through two names, and the volume is
named for what it holds.

When a runtime keeps context on its context volume and no durable context volume
is configured, continuity is impossible in that deployment. The manager SHALL NOT
send a context handle, and the conversation SHALL record from the outset that it
cannot be continued.

Such a conversation SHALL run each input fresh rather than failing. A deployment
that cannot carry context is a configuration an operator chose, not a fault, and
failing every follow-up in it would make a supported configuration look broken —
with a short idle timeout the runtime pod exits between almost any two messages.

The distinction is between continuity that was NEVER PROMISED and continuity that
was promised and then lost. Only the second fails a run.

Where the volume is storage the chart did not create — an existing claim, or a
pre-created volume — continuity SHALL be promised on exactly the same terms. What
decides the promise is whether a durable volume is configured, never who
provisioned it.

#### Scenario: An ephemeral install is single-run by declaration
- **WHEN** the runtime keeps context on its context volume and the deployment provides none
- **THEN** no handle is issued, the conversation states from its first run that it cannot be continued, and each message is answered fresh rather than failing

#### Scenario: A runtime that stores context elsewhere is unaffected
- **WHEN** a runtime declares that its context lives outside the operator's storage
- **THEN** continuity is promised regardless of whether a context volume is configured

#### Scenario: Losing what was promised still fails
- **WHEN** continuity was possible and a run finds the context gone
- **THEN** the run fails, because this is a loss rather than a configuration

#### Scenario: Operator-provisioned storage promises the same continuity
- **WHEN** the context volume is an existing claim or a pre-created volume rather than one the release provisioned
- **THEN** continuity is promised exactly as it would be for a release-provisioned claim
