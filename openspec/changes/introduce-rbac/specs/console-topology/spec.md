## MODIFIED Requirements

### Requirement: The topology is three views of one activity feed

The topology SHALL be offered as three views of one activity feed, each a
layer of the architecture with its own node identity. A hop SHALL be mapped
onto every view's nodes, so the same event animates each view.

**Model.** The declared objects and what references what: SignalSources,
SignalAdapters, Pipelines, AgentProfiles, AgentRuntimes, Channels,
ChannelAdapters, MCPToolsets, MCPConfigs and Conversations.

Edges SHALL cover `feeds`, `answers`, `runs-on`, `posts`, `served-by`, `uses`
and `opened`. The runtime edge SHALL run from the Pipeline, never from the
profile. A runtime's image, harness and vendor SHALL be facts in its panel,
not nodes.

**Components.** One node per component the repository builds: each signal and
channel adapter, the gateway, the manager, each runtime image, context-sync,
egress-proxy and housekeeping, plus the systems outside: each model, each MCP
server, each repository, the senders an adapter declares, and the Kubernetes
API.

**A Deployment or CronJob the console recognises as none of the above SHALL
still draw, as a `workload` node**, rather than being silently absent from a
namespace it actually runs in.

A runtime image running in several pods SHALL be one node carrying the count.

**Infrastructure.** Every pod, and the externals. Nothing from the model is a
node, and a pod's pipeline and conversation SHALL be attributes in its panel.

A conversation pod SHALL be one node until expanded into its containers, and
SHALL collapse again.

Node health SHALL be computed exclusively from conditions reconcilers already
write, and from pod phase for pods. The console SHALL compute no health of its
own. References resolving to nothing SHALL render as broken edges to
placeholder nodes rather than being omitted.

This no-omission rule covers broken references only, not the role filtering below.

For a `user`-role requester, a Conversation node for a conversation that
requester cannot see under the console-application siloing rule SHALL be
OMITTED ENTIRELY from the Model view — never anonymized, never relabeled.

The Infrastructure view SHALL likewise omit the pod of such a conversation,
since its name and attributes carry the conversation's name.

`Conversation.metadata.name` is a deterministic word-chain derived from the
conversation's own text, so it is not a safe label to show for a
conversation the requester cannot otherwise open. A `responder`-role or
`administrator`-role requester SHALL see every Conversation node, exactly
as today.

#### Scenario: One hop, three pictures
- **WHEN** a run is dispatched
- **THEN** the Model view moves the pipeline's runtime edge, the Components view moves manager to context-sync to the runtime image, and the Infrastructure view moves the manager's pod to the conversation's pod

#### Scenario: The runtime is wired from the pipeline
- **WHEN** a Pipeline names a runtime
- **THEN** the Model view draws the edge from the Pipeline to the runtime, and no edge from the profile to a runtime

#### Scenario: A component appears once
- **WHEN** three conversation pods run the same runtime image
- **THEN** the Components view shows one runtime image node with a count of three

#### Scenario: A pod opens into its containers
- **WHEN** a conversation pod is expanded on the Infrastructure view
- **THEN** it becomes a box holding context-sync, agent and egress-proxy, the work hop reaches the agent through context-sync, every model and tool call leaves through egress-proxy, and collapsing restores the one node

#### Scenario: A typo is visible
- **WHEN** `spec.adapter` names an adapter that does not exist
- **THEN** a broken edge to a placeholder node is drawn, and the Channel's `Served=False` reason is shown

#### Scenario: Unclaimed source is visibly dropped
- **WHEN** a SignalSource is claimed by no Pipeline (`Wired=False`)
- **THEN** it renders as a disconnected node carrying the Wired condition's reason

#### Scenario: The graph agrees with kubectl
- **WHEN** a reconciler reports a failing condition
- **THEN** the node shows that condition's reason verbatim on every view that draws it, and no node is colored by a judgement the cluster did not make

#### Scenario: Tooling is on the graph
- **WHEN** a Pipeline binds toolsets and MCP configs
- **THEN** the Model view shows them as nodes joined by `uses` edges, and the Components view shows the MCP servers those configs point at

#### Scenario: Healthy pipeline renders connected
- **WHEN** a Ready Pipeline wires a Served SignalSource to a profile, a runtime and two Served channels
- **THEN** the Model view shows the source, pipeline, profile, runtime and both channels connected, with healthy status coloring

#### Scenario: Unserved adapter reference is diagnosable
- **WHEN** a Channel names `spec.adapter: slak` and no such ChannelAdapter exists
- **THEN** the channel node shows `Served=False` with the condition reason, and no edge to an adapter node is drawn

#### Scenario: A conversation is opened and its runtime is wired
- **WHEN** a Pipeline resolves to an AgentRuntime and opens a Conversation
- **THEN** the Model view draws an `opened` edge from the pipeline to the conversation node and a `runs-on` edge from the pipeline to the runtime

#### Scenario: A user's graph omits conversations they cannot see

- **WHEN** a `user`-role requester opens the Model view
- **THEN** conversation nodes for conversations they cannot see are
  entirely absent from the graph, with no placeholder or anonymized node
  left in their place

#### Scenario: A user's Infrastructure view omits pods of conversations they cannot see

- **WHEN** a `user`-role requester opens the Infrastructure view
- **THEN** the pod of a conversation they cannot see is entirely absent,
  with no placeholder or anonymized node left in its place

#### Scenario: Responder and administrator see every conversation node

- **WHEN** a `responder`-role or `administrator`-role requester opens the
  Model view
- **THEN** every Conversation node is shown, unfiltered
