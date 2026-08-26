## ADDED Requirements

### Requirement: The runtime is the harness; Ollama is only inference

`runtime-ollama` SHALL implement the work contract in full: long-poll
`GET $CONTROL_URL/work`, execute the returned unit, stream progress to stdout,
`POST $CONTROL_URL/work/done`, and exit 0 after `RUNTIME_IDLE_TTL_M` minutes
without work. It SHALL check out the profile's repository at `/data/workspace`
when one is configured, exactly as the reference runtime does.

The agent loop, tool dispatch, transcript and context handle SHALL be owned by
this process. Ollama SHALL be called only to produce the next assistant message.
No behavior the contract specifies SHALL be delegated to the model server,
because it implements none of it.

This SHALL require no change to the manager, to any CRD, or to the work
contract. If it does, the contract was vendor-specific and that is the finding
to report before the code is written.

#### Scenario: A unit is executed end to end

- **WHEN** the manager dispatches a work unit to a `runtime-ollama` pod
- **THEN** the runtime executes it and reports `{convo, runId, status,
  runtimeContextId, result}` to `/work/done`
- **AND** the manager handles the report with no runtime-specific branch

#### Scenario: The pod goes idle

- **WHEN** no work arrives for `RUNTIME_IDLE_TTL_M` minutes
- **THEN** the process exits 0 and the pod is not restarted until the manager
  needs it

#### Scenario: The contract would need widening

- **WHEN** implementing any part of this runtime requires a new field on the
  work unit or the done report
- **THEN** that requirement is raised as a contract change on its own, not
  worked around inside this image

### Requirement: The agent loop is bounded, observable and ends in an answer

The runtime SHALL run a bounded loop: send the conversation to Ollama, execute
any tool calls the model returns, append their results, and repeat until the
model returns no tool calls or the unit's `maxTurns` is reached. Each iteration
SHALL be counted as a turn.

Every step SHALL be visible on stdout — the assistant's text, each tool call
with its arguments truncated to a readable length, and each tool result's
outcome — because pod logs are the only place a run can be watched.

The final assistant message SHALL be the `result` reported to the manager. A run
that exhausts `maxTurns` without a final answer SHALL report `failed` with a
result naming the exhaustion, never an empty result and never a tool trace
presented as an answer.

#### Scenario: A tool-using run completes

- **WHEN** the model requests two tools and then answers
- **THEN** both calls and their outcomes appear in the pod log, and the reported
  result is the model's final text

#### Scenario: The turn budget is exhausted

- **WHEN** the model keeps calling tools until `maxTurns` is reached
- **THEN** the run reports `failed` with a result stating the turn limit was hit
- **AND** the person reading it sees a reason rather than silence

#### Scenario: Nothing is invented on failure

- **WHEN** any run fails for any reason
- **THEN** the reported result is non-empty and states the reason

### Requirement: Tools come from two sources under one allowlist

The runtime SHALL offer the model tools from exactly two sources: the MCP
servers in the compiled config at `$MCP_CONFIG`, advertised as
`mcp__<server>__<tool>`, and its own built-in tools. Both SHALL pass through the
SAME allowlist gate.

The allowlist SHALL be composed the way the contract specifies: the unit's
`allowedTools` is the wiring's half, the `tools:` frontmatter of
`.claude/agents/<agent>.md` in the checkout is the agent definition's half, and
`toolsMode` decides — `merge` unions them, `overwrite` uses the wiring's alone.
A runtime with no checkout uses `allowedTools` as given.

A tool SHALL NOT be advertised to the model unless the composed allowlist
carries it. An empty allowlist SHALL mean NO tools are advertised — nothing is
substituted, and the run proceeds as a text-only answer rather than being
granted a tool nobody wrote down.

Pattern matching SHALL fail closed: an exact name matches itself, a trailing
`*` matches a prefix, and a pattern whose syntax the runtime does not understand
grants nothing.

#### Scenario: Wiring and definition are merged

- **WHEN** a unit carries `allowedTools: Read,Bash`, `toolsMode: merge`, and the
  named agent definition declares `tools: [Grep]`
- **THEN** the model is offered `Read`, `Bash` and `Grep`, and nothing else

#### Scenario: Overwrite ignores the definition

- **WHEN** the same unit carries `toolsMode: overwrite`
- **THEN** the model is offered `Read` and `Bash` only

#### Scenario: An empty allowlist offers nothing

- **WHEN** a unit carries an empty `allowedTools` and the definition declares
  nothing
- **THEN** the request to Ollama carries no tools at all, and the run answers
  from the prompt alone

#### Scenario: An MCP tool is gated

- **WHEN** the mounted MCP config exposes ten tools and the allowlist carries
  two of them
- **THEN** only those two are advertised, and a model that names a third is
  answered with a denial it can read, not with an execution

#### Scenario: An unparseable pattern is not a grant

- **WHEN** the allowlist carries a pattern the runtime cannot interpret
- **THEN** it grants nothing and the pattern is logged

### Requirement: Built-in tools are implemented natively and scoped

The runtime SHALL implement the built-in vocabulary the chart's toolsets name —
`Read`, `Grep`, `Glob`, `Edit`, `Write`, `Bash` — so that binding
`agentops-observe`, `agentops-shell` or `agentops-edit` to a Pipeline means the
same thing on this runtime as on the reference one.

File tools (`Read`, `Grep`, `Glob`, `Edit`, `Write`) SHALL be confined to the
workspace checkout: a path escaping it — absolute, `..`-relative, or via a
symlink — SHALL be refused as a tool error, not served. Results SHALL be
size-bounded so one `Read` of a large file cannot displace the whole context
window.

`Bash` SHALL execute in the workspace with a per-call timeout and bounded
captured output. Its risk is stated rather than mitigated: a route that binds
`agentops-shell` gives the model the pod's shell, with whatever the runtime's
ServiceAccount can reach — the same posture as the reference runtime, and the
reason the toolsets are risk-split.

#### Scenario: Observation without execution

- **WHEN** a Pipeline binds `agentops-observe` only
- **THEN** the model can `Read`, `Grep` and `Glob` in the checkout and has no
  `Bash` tool available at all

#### Scenario: A path escapes the workspace

- **WHEN** the model calls `Read` with `/etc/shadow` or `../../secrets`
- **THEN** the call returns a tool error naming the refusal, and no file outside
  the workspace is opened

#### Scenario: A shell command hangs

- **WHEN** a `Bash` call exceeds its timeout
- **THEN** it is terminated, the timeout is reported to the model as the tool
  result, and the loop continues

#### Scenario: A large read is bounded

- **WHEN** the model reads a file larger than the output bound
- **THEN** the result is truncated with the truncation stated in the result

### Requirement: A tool this runtime cannot provide is reported, never faked

When the composed allowlist names a tool this runtime does not implement and no
connected MCP server exposes, the runtime SHALL log it at run start, name it in
the run's stdout, and proceed with the tools it does have. It SHALL NOT
substitute another tool, silently drop the name, or report the run as if the
allowlist had been satisfied.

Reporting is what makes a capability gap visible to the operator who wrote the
binding; silence makes a Pipeline look correctly wired while the agent cannot do
what it was granted.

#### Scenario: A binding names an unimplemented tool

- **WHEN** a unit's allowlist carries a tool name this runtime does not
  implement and no MCP server exposes
- **THEN** the run log names it as unavailable on this runtime
- **AND** the run proceeds with the remaining tools

#### Scenario: An MCP server fails to connect

- **WHEN** a server in the mounted MCP config cannot be reached
- **THEN** its absence and the tools consequently unavailable are stated in the
  run log, and the run continues with the rest

### Requirement: The runtime stores the context it hands back

The runtime SHALL persist each conversation's message transcript under `$HOME`
— the context volume, or the pod-local copy `context-sync` restores and
snapshots — SHALL return an opaque `runtimeContextId` naming it, and SHALL keep
`contextStorage: volume` on its `AgentRuntime`. Its bundle SHALL declare the
transcript directory in `contextSync.paths`, because only the runtime knows
where its backend keeps context; continuity here depends on a durable context
claim outliving the pod, and a route with none is told its context is not
promised by the existing rule.

Given a handle, the runtime SHALL load that transcript, append the new turn, and
report `continuity: continued`. Given none, it SHALL create one and report
`continuity: new`. The handle it reports SHALL be the one that now exists —
latest-wins, the same rule the manager applies when storing it.

The transcript SHALL be the runtime's own format and its layout SHALL be known
to nothing else. The manager stores a string and interprets nothing.

#### Scenario: A conversation continues

- **WHEN** a second unit arrives carrying the `runtimeContextId` from the first
- **THEN** the model sees the earlier exchange, and the run reports
  `continuity: continued`

#### Scenario: A conversation starts

- **WHEN** a unit arrives with no handle
- **THEN** a transcript is created, its id is reported, and `continuity: new`

#### Scenario: The handle is opaque

- **WHEN** the manager records the reported handle
- **THEN** it stores it as a string, and no manager code parses, validates or
  derives anything from its shape

### Requirement: Unreachable context is an outage before it is a loss

When a run is given a handle whose transcript cannot be found, the runtime SHALL
re-check over a short bounded delay before concluding it is gone — a shared
volume can fail to answer for seconds without having lost anything, and treating
that as loss would turn a storage hiccup into a destroyed conversation. An
unreadable store SHALL be treated as "did not answer", never as "not there".

If the transcript reappears, the run SHALL proceed as a continuation. If it is
confirmed missing, the run SHALL FAIL with `continuity: unavailable`, a
`continuityReason` naming the volume as the likely cause, and a NON-EMPTY result
telling the person that this conversation cannot be continued and why. The run
SHALL NOT answer without the context it was promised.

#### Scenario: The volume is slow

- **WHEN** the transcript is not visible on the first look but appears on a
  re-check
- **THEN** the run continues normally and reports `continuity: continued`

#### Scenario: The context is genuinely gone

- **WHEN** re-checks confirm the transcript is absent
- **THEN** the run reports `failed` with `continuity: unavailable`, a reason
  naming the context volume, and a readable message telling the user to start a new
  conversation
- **AND** no answer is produced from an empty context

#### Scenario: The store errors

- **WHEN** reading the transcript directory returns an error rather than an
  empty listing
- **THEN** that is treated as unavailability of the STORE, not absence of the
  context

### Requirement: The model must actually be able to call the tools it is given

At startup the runtime SHALL determine whether the configured model supports
tool calling, and SHALL state the answer in its log. When a unit carries a
non-empty allowlist and the model does not support tool calling, the runtime
SHALL report the run as failed with a result naming the model and the
limitation, rather than answering as though no tools had been bound.

A model that cannot call tools is a valid configuration for text-only routes;
what is not valid is a route whose Pipeline grants tools and whose agent
silently never uses them.

#### Scenario: A text-only model on a text-only route

- **WHEN** the model does not support tools and the unit's allowlist is empty
- **THEN** the run proceeds normally

#### Scenario: A text-only model on a tool-bound route

- **WHEN** the model does not support tools and the unit carries an allowlist
- **THEN** the run fails with a result naming the model and the missing
  capability

### Requirement: The context window is set explicitly and truncation is never silent

The runtime SHALL set the Ollama context length explicitly on every request
rather than relying on the server default, because the default silently drops
the front of a long prompt — and a system prompt or an alert payload lost that
way produces a confident, wrong answer with no error anywhere.

When an assembled conversation exceeds the configured budget, the runtime SHALL
trim it by a stated policy, SHALL keep the system prompt and the current turn,
and SHALL state in the run log that trimming occurred and how much was dropped.
Trimming SHALL NOT be reported as `continuity: unavailable` — the context was
reached; part of it did not fit.

#### Scenario: A long conversation is trimmed

- **WHEN** the stored transcript plus the new input exceeds the configured
  context budget
- **THEN** the request carries the system prompt and the most recent messages
  that fit, and the log states what was dropped

#### Scenario: The window is never left to the default

- **WHEN** any request is sent to Ollama
- **THEN** it carries an explicit context length

### Requirement: The endpoint is configuration, and its failure is a reported failure

The Ollama endpoint, the model, and the request options SHALL come from
environment on the `AgentRuntime` (`OLLAMA_URL`, `OLLAMA_MODEL`, and the
tuning knobs), so a second model is a second `AgentRuntime` — runtimes are what
executes, and the model is the most execution-shaped property there is. No CRD
field and no profile field SHALL be added for the model.

Startup SHALL verify the endpoint and that the model is present, and SHALL say
so in the log. A request that fails — endpoint unreachable, model missing, a
non-2xx response — SHALL fail the run with the reason in the result, and SHALL
be retried only where the contract already retries (reporting to `/work/done`),
never by re-running an agent turn that may have already acted.

#### Scenario: The endpoint is down

- **WHEN** Ollama cannot be reached during a run
- **THEN** the run reports `failed` with a result naming the endpoint and the
  error

#### Scenario: The model is not pulled

- **WHEN** the configured model is absent on the server
- **THEN** the startup log says so plainly, and a dispatched run fails with that
  reason rather than a generic error

#### Scenario: Two models are wanted

- **WHEN** an install wants a small model for one route and a larger one for
  another
- **THEN** it declares two `AgentRuntime` CRs — the bundle's and a `runtimes:`
  entry naming the same image and another model — and points each Pipeline's
  `runtimeRef` at one, with no CRD change

#### Scenario: A failed turn is not silently repeated

- **WHEN** an inference request fails after tools have already been executed
- **THEN** the run fails and is not retried from the start

### Requirement: The image stays generic

The runtime image SHALL carry what it needs to hold a checkout and run a shell —
git, openssh-client and ordinary shell utilities — and SHALL carry NO domain
tooling: no `kubectl`, no cloud CLI, no bundled MCP server. What an agent may
reach is wiring, expressed by `MCPConfig` and `MCPToolset` on a Pipeline.

An adopter needing a CLI in the sandbox SHALL derive an image and point
`AgentRuntime.spec.image` at it. The image SHALL NOT hold any channel
credential and SHALL post to no transport — agent output reaches chat through
the manager and its adapters.

#### Scenario: Cluster access is wanted

- **WHEN** an adopter wants the agent to reach Kubernetes
- **THEN** it is bound as MCP tooling on the Pipeline, not baked into this image

#### Scenario: The image is inspected

- **WHEN** the built image is inspected
- **THEN** it contains no cluster or cloud CLI and no channel credential
