## Purpose

The fifth reference runtime, driving the pi coding harness through its SDK: its obligations under the work contract, tool enablement from the composed allowlist, MCP servers reached from the compiled configuration, the agent definition as system prompt, the runtime-minted context handle, telemetry, and the bundle that declares it.

## ADDED Requirements

### Requirement: A fifth reference runtime implements the same work contract
The project SHALL ship a fifth reference `AgentRuntime` image, `runtime-pi`, driving pi through its SDK in-process. It SHALL be a self-contained component under `runtimes/pi/` with no dependency on any other module here, and SHALL implement the work contract unchanged: long-poll `GET $CONTROL_URL/work`, execute the returned unit against the checkout at `/data/workspace`, stream a human-readable transcript to stdout, `POST $CONTROL_URL/work/done` carrying `continuity`/`continuityReason`, and exit `0` after `RUNTIME_IDLE_TTL_M` minutes without work.

No CRD field, manager behaviour, `Pipeline`, `AgentProfile` or `MCPToolset` SHALL need to change for a conversation to run on it. `Pipeline.spec.runtimeRef` SHALL be the entire switch between vendors.

The image SHALL stay generic by construction: the repository checkout it owns, generic shell utilities and the MCP extension it needs, and no domain tooling. What an agent may reach is wiring.

The runtime SHALL conform to the version of `docs/contracts/runtime.md` its page claims, walked item by item against the numbered conformance checklist.

#### Scenario: The same wiring runs on any vendor
- **WHEN** a `Pipeline`'s `runtimeRef` is pointed from another runtime at the pi runtime, changing nothing else
- **THEN** conversations it starts continue to run, with the same profile, the same channels, the same toolsets and the same MCP servers

#### Scenario: The contract is honoured end to end
- **WHEN** the pi runtime receives a work unit
- **THEN** it executes it against the checkout, streams the transcript to stdout, reports the outcome to `/work/done`, and exits `0` once idle for the configured TTL
- **AND** the manager handles the report with no runtime-specific branch

#### Scenario: No domain tooling is baked in
- **WHEN** an agent on this runtime needs to reach a system outside its checkout
- **THEN** it reaches it through MCP servers its Pipeline binds, not through a CLI shipped in the image

#### Scenario: Missing configuration is a crash, not an idle exit
- **WHEN** the runtime starts without `CONTROL_URL`, `CONVO_ID` or the model credential it needs
- **THEN** it exits nonzero before the first poll, naming what is missing

### Requirement: Built-in tools are enabled only when the composed allowlist names them
The runtime SHALL compose the allowlist as the work unit's `toolsMode` directs, from the wiring's `allowedTools` and the named agent's declared `tools:`. It SHALL enable a pi built-in tool for the session only when the composed allowlist names its agent-ops equivalent: `Read`, `Write`, `Edit`, `Bash`, `Grep` or `Glob`.

Enablement SHALL be all or nothing per tool. A sub-command-scoped shell pattern such as `Bash(kubectl:*)` SHALL NOT enable the shell and SHALL NOT narrow it. It SHALL be logged as a pattern the runtime cannot enforce and SHALL grant nothing, so it never widens a route. Each runtime's page states which of the two postures it takes.

An empty composition SHALL leave the session with no built-in tools. The runtime SHALL NOT substitute a default, and SHALL NOT let a vendor default apply. A pattern the runtime does not recognise SHALL be logged and SHALL contribute nothing.

No run SHALL be able to block on an interactive permission prompt. Every decision SHALL be answered from the composed allowlist, denying by default, because in a pod nobody can answer a prompt.

#### Scenario: Only named built-ins exist
- **WHEN** the composed allowlist names `Read` and `Grep` and nothing else
- **THEN** the session has those two built-in tools and no shell, no edit and no write tool

#### Scenario: A scoped shell pattern grants nothing
- **WHEN** the composed allowlist contains `Bash(kubectl:*)` and no bare `Bash`
- **THEN** the session has no shell tool, and the runtime logs that the pattern cannot be enforced

#### Scenario: A bare shell grant is whole
- **WHEN** the composed allowlist contains bare `Bash`
- **THEN** the session has the shell tool, un-narrowed

#### Scenario: Nothing composed means nothing granted
- **WHEN** the wiring binds no toolset and the agent declares none
- **THEN** the run proceeds with no tools available

#### Scenario: An unrecognised pattern grants nothing
- **WHEN** a bound toolset contains a pattern the runtime has no mapping for
- **THEN** the runtime logs it and the conversation does not receive it

### Requirement: Bound MCP servers reach the agent through the compiled configuration alone
Pi has no MCP client of its own, so the image SHALL ship an extension that provides one. The extension SHALL read the compiled MCP configuration at `MCP_CONFIG` and connect only to the servers that file lists. The runtime SHALL start no server of its own choosing, and SHALL read no other source of MCP configuration.

Both transports the compiled file carries SHALL be preserved. A server declared as a local command is started by the runtime exactly as the other runtimes start it. A server declared as an HTTP endpoint is reached as any other client in the pod reaches it, so egress mediation applies to this runtime exactly as to the others.

Each tool SHALL be registered under the name `mcp__<server>__<tool>`, and only when the composed allowlist permits it. A pattern scoping one server as a whole, `mcp__<server>__*`, SHALL permit every tool of that server and no tool of another. An absent configuration file SHALL mean no MCP tools, and an unreadable one SHALL be reported as a single logged failure.

Secret-backed values SHALL be resolved in-process from the pod environment before use, and SHALL never be written to the transcript, a log line or the run result. A placeholder that cannot be resolved SHALL fail that one server's registration with a logged reason, never reaching a server as literal placeholder text, and SHALL leave the other servers unaffected.

#### Scenario: A bound server's tools are reachable
- **WHEN** a Pipeline binds an `MCPConfig` and a toolset naming one of its tools, and the conversation runs on the pi runtime
- **THEN** the agent can call that tool, and no tool the allowlist does not name

#### Scenario: A server nobody bound does not exist
- **WHEN** an MCP server is not listed in the compiled configuration
- **THEN** the runtime never connects to it

#### Scenario: A server wildcard is honoured
- **WHEN** the composed allowlist contains `mcp__<server>__*`
- **THEN** every tool of that server is available and no tool of any other server is

#### Scenario: No configuration means no MCP tools
- **WHEN** the conversation binds no `MCPConfig`
- **THEN** the run starts with no MCP tools and no error

#### Scenario: A secret-backed value is resolved, not echoed
- **WHEN** a server's header or environment value is secret-backed
- **THEN** the runtime resolves it from the pod environment and the value appears in no log line, transcript or result

#### Scenario: An unresolvable value fails one server
- **WHEN** a placeholder has no corresponding environment value
- **THEN** that server fails to register with a logged reason, and the other servers register normally

### Requirement: The agent definition's body is appended to the system prompt
When a work unit names an agent and the checkout contains `.claude/agents/<agent>.md`, the runtime SHALL append the definition's body to the session's system prompt, and SHALL then append the work unit's `systemPrompt`, definition first. It SHALL do so on every run, resumes included. The definition's declared `tools:` SHALL take part in the composed allowlist as `toolsMode` directs.

A definition that is absent, unreadable, or has frontmatter the runtime cannot parse SHALL contribute nothing to the allowlist, SHALL be logged where unreadable, and SHALL NOT block the run. The work unit's `systemPrompt` alone SHALL then apply.

#### Scenario: The definition's body joins the prompt
- **WHEN** a unit names an agent whose definition exists in the checkout
- **THEN** the session's system prompt carries that definition's body, followed by the unit's `systemPrompt`

#### Scenario: A resumed run carries it too
- **WHEN** a unit continues an existing session
- **THEN** the definition's body and the unit's `systemPrompt` apply to that run as they did to the first

#### Scenario: The definition's tools merge
- **WHEN** an agent declares `Read, Grep` and the wiring grants `Bash` in `merge` mode
- **THEN** the session has `Read`, `Grep` and `Bash`

#### Scenario: Overwrite ignores the definition's tools
- **WHEN** the same agent is wired in `overwrite` mode granting `Bash`
- **THEN** the session has `Bash` alone, while the definition's body still joins the system prompt

#### Scenario: A missing definition degrades
- **WHEN** the named agent's file does not exist or its frontmatter cannot be parsed
- **THEN** the run proceeds on the unit's `systemPrompt` and the wiring's tools alone

### Requirement: The context handle is minted by the runtime and stays opaque
The runtime SHALL mint `runtimeContextId` itself and map it to a pi session whose state lives under `$HOME`, the CONTEXT volume or the pod-local copy `context-sync` restores. On a unit carrying no `runtimeContextId` it SHALL create a session under a generated identifier and report it with `continuity: new`. On a unit carrying one it SHALL resume that session and report `continuity: continued`. The reported handle SHALL be latest-wins.

The identifier SHALL NOT encode the conversation name or any derivable value. The bundle SHALL declare the session-state directory in `contextSync.paths`, because only the runtime knows where its backend keeps context.

When a resume fails, the runtime SHALL tell a store that says the context is GONE from one that did not ANSWER, re-checking after short bounded delays. An unreadable path SHALL be treated as present, and the run retried once. A context confirmed absent SHALL fail the run with `continuity: unavailable` and a `continuityReason` naming the context volume, never starting a fresh session and presenting it as a continuation.

#### Scenario: A first run establishes a handle
- **WHEN** a first work unit arrives with no `runtimeContextId`
- **THEN** the runtime creates a session under a generated identifier and reports it with `continuity: new`

#### Scenario: A later run continues it
- **WHEN** a unit carries a `runtimeContextId` whose session state is present
- **THEN** the agent answers with the conversation's accumulated context and the run reports `continuity: continued`

#### Scenario: A slow store is not a lost context
- **WHEN** a resume fails and the state is present on re-check
- **THEN** the runtime retries once and answers with the conversation's context

#### Scenario: A genuinely lost context fails loudly
- **WHEN** a resume fails and the state is confirmed absent
- **THEN** the run fails with `continuity: unavailable` and no answer is produced from an empty context

### Requirement: Telemetry reports what pi observed, bounded and never invented
The runtime SHALL report one `turns[]` entry per model call and one `toolCalls[]` entry per tool call, from the events pi emits, trimmed to the published bounds before posting. A fact pi does not report SHALL be omitted, never reported as zero. A tool's input or output SHALL NOT be reported. Neither array SHALL be reported when the run made neither kind of call.

Tool names in `toolCalls[]` SHALL be in the agent-ops vocabulary: `Read`, `Write`, `Edit`, `Bash`, `Grep`, `Glob`, or `mcp__<server>__<tool>`.

#### Scenario: A run with calls reports them
- **WHEN** a run makes model calls and tool calls
- **THEN** the report carries one entry for each, within the bounds, with no tool input or output

#### Scenario: An unknown figure is omitted
- **WHEN** pi reports no cache-read count for a call
- **THEN** the turn carries no `cacheReadTokens` field rather than zero

#### Scenario: A run with no calls reports neither array
- **WHEN** a run answers without a model call or tool call being recorded
- **THEN** neither array appears in the report

### Requirement: Model and credential arrive as environment, never as a new field
The runtime SHALL take its model provider, model and credential from environment the bundle projects from a Secret reference. No CRD SHALL gain a field for them. The credential SHALL be resolved by the kubelet, never read by the manager.

#### Scenario: A credential is a Secret reference
- **WHEN** the bundle is enabled with a credential Secret
- **THEN** the runtime's pod receives the credential through `valueFrom` or `envFrom`, and the manager reads no secret

### Requirement: The runtime ships as a bundle that declares only its vendor
The chart SHALL ship the runtime as `chart/charts/pi/`, OFF by default, in the same shape as the other runtime bundles: one `AgentRuntime` rendered through the parent's shared runtime renderer, carrying only what names this vendor, which is the image, its credential Secret reference, its `contextSync.paths` and its own env. It SHALL render no defaults, no floor account, no context volume and no identity of its own.

The bundle SHALL be visible to the parent's declared-runtime list, so that the default-runtime guard, the `default` copy and the manager's bootstrap env account for it as for any other runtime. The image SHALL be pinned to an exact vendor version and installed from a lockfile.

#### Scenario: Existing installs render byte-identically
- **WHEN** the chart is upgraded with no `pi` values set
- **THEN** nothing of the bundle renders and every other object is unchanged

#### Scenario: A route selects it by name
- **WHEN** the bundle is enabled and a `Pipeline` states `runtimeRef: pi`
- **THEN** that route's conversations run on the pi runtime and every route naming nothing keeps running on `default`

#### Scenario: The bundle ships no substrate
- **WHEN** the bundle is enabled
- **THEN** it renders no runtime defaults, no floor ServiceAccount, no context volume and no ServiceAccount of its own
