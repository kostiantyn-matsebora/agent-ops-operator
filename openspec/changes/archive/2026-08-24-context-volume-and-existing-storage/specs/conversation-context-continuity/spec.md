## MODIFIED Requirements

### Requirement: Continuity is promised only where its prerequisite is met
An `AgentRuntime` SHALL declare the SHAPE of its backend's storage — on a
context volume, in storage the operator does not provide, or nowhere — and the
manager SHALL check that declaration against what the conversation actually
resolved before promising continuity.

The runtime SHALL NOT declare WHICH volume. That is the route's, on the
`Pipeline`, and it is resolved and snapshotted at conversation creation. The
runtime answers only whether a disk is involved at all, because that is the one
half only it can know.

The declaration and the volume it names SHALL carry the same word. A setting
called `contextStorage` whose `volume` case pointed at something called the home
volume made the reader resolve one concept through two names, and the volume is
named for what it holds.

When a runtime keeps context on a context volume and the conversation resolved
none — neither its route nor the release supplied one — continuity is impossible
for that conversation. The manager SHALL NOT
send a context handle, and the conversation SHALL record from the outset that it
cannot be continued.

Such a conversation SHALL run each input fresh rather than failing. A deployment
that cannot carry context is a configuration an operator chose, not a fault, and
failing every follow-up in it would make a supported configuration look broken —
with a short idle timeout the runtime pod exits between almost any two messages.

The distinction is between continuity that was NEVER PROMISED and continuity that
was promised and then lost. Only the second fails a run.

Where the volume is storage the chart did not create — an existing claim, or a
pre-created volume, bound by the route or by the release — continuity SHALL be
promised on exactly the same terms. What decides the promise is whether the
conversation resolved a durable volume, never who provisioned it or at which
level it was declared.

#### Scenario: An ephemeral install is single-run by declaration
- **WHEN** the runtime keeps context on a context volume and neither the route nor the release supplies one
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

#### Scenario: A route's own volume promises continuity where the release has none
- **WHEN** release-wide persistence is off and the originating Pipeline binds its own context volume
- **THEN** continuity is promised for that route's conversations, because what decides it is what the conversation resolved
