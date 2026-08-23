## MODIFIED Requirements

### Requirement: Bindings materialize on the Conversation with lazy content resolution

Conversations SHALL snapshot the ORIGINATING PIPELINE's wiring into the
Conversation spec at creation — materialized per-conversation state, following
the profileRef/channelRefs pattern.

That snapshot SHALL cover both tooling bindings (`toolsets`, `mcpConfigs`) AND
THE RESOLVED RUNTIME AND SERVICE ACCOUNT. That Pipeline is the only source: no
profile default, no baseline, no inheritance.

THE RESOLVED RUNTIME NAME IS SNAPSHOTTED, NOT THE PIPELINE'S RAW FIELD. A
conversation created while its Pipeline named no runtime SHALL keep the one it
actually ran on, rather than picking up a later edit to that Pipeline — or to
the deprecated profile ref below it.

THE SERVICE ACCOUNT IS SNAPSHOTTED ONLY WHERE THE PIPELINE NAMED ONE, and the
asymmetry is the ref/content rule, not an inconsistency. A Pipeline's account is
WIRING and is frozen. An `AgentRuntime`'s own account is that runtime's CONTENT,
so a conversation whose Pipeline named none SHALL leave the field empty and
resolve the runtime's at every pod build — otherwise correcting a mistyped
account would strand every conversation already created on a name the operator
has already fixed. Leaving it empty is SAFE against the escalation this rule
guards, because resolution never reads a Pipeline: the empty case falls to the
runtime, never to the edited wiring.

CONTENT SHALL be re-resolved at each use — toolset and MCPConfig contents at MCP
compilation and work-unit dispatch, and the `AgentRuntime`'s image, idle TTL,
volume AND its own service account at every pod build — so content edits reach
existing conversations while re-wiring affects only new ones.

**THE IDENTITY SNAPSHOT IS THE SHARPEST CASE OF THIS RULE.** Without it, editing
a Pipeline changes what service account an INFLIGHT conversation's next pod runs
as. That is not a re-wiring inconvenience, it is a privilege change applied to
work already in progress.

This SHALL hold identically for every origination path — a signal of any kind
through a claimed source, and a chat command naming a Pipeline — because all of
them read the same Pipeline for the same fields.

#### Scenario: Pipeline bindings follow the signal
- **WHEN** a signal routes through a pipeline with `toolsets: {refs: [vm-observability]}` and `mcpConfigs: {refs: [vm-logs]}`
- **THEN** the created conversation's spec records both ref sets

#### Scenario: Content edits heal running conversations
- **WHEN** a bound MCPConfig's server URL is corrected while conversations referencing it exist
- **THEN** subsequent MCP compilation for those conversations uses the corrected URL

#### Scenario: A posted task carries the claiming Pipeline's wiring
- **WHEN** a `kind: task` signal is posted to a source claimed by a Pipeline that binds toolsets and mcpConfigs
- **THEN** the created conversation carries that Pipeline's profile, channel set, and both tooling bindings

#### Scenario: A Pipeline declaring nothing yields no bindings
- **WHEN** a conversation originates from a Pipeline with neither binding declared
- **THEN** it carries no bindings and dispatches with an empty allowlist, with nothing supplying a default

#### Scenario: Re-wiring never moves a running conversation's identity

- **WHEN** a Pipeline's `serviceAccountName` is changed while one of its
  conversations is inflight
- **THEN** that conversation's next pod runs under the account it was created
  with, and only conversations created afterwards use the new one

#### Scenario: Fixing a runtime still heals running conversations

- **WHEN** the `AgentRuntime` a conversation snapshotted has its image corrected
- **THEN** the next pod uses the corrected image, because the REF is frozen and
  the CONTENT is not

#### Scenario: Fixing a runtime's own account heals them too

- **WHEN** a conversation whose Pipeline named NO service account runs on an
  `AgentRuntime` whose `serviceAccountName` is then corrected
- **THEN** the next pod uses the corrected account, because the Pipeline named
  none to freeze and the runtime's is content

#### Scenario: A conversation predating the snapshot resolves as before

- **WHEN** a conversation created before these fields existed dispatches
- **THEN** resolution falls through to the deprecated profile ref and the
  `default` runtime, and nothing backfills the snapshot
