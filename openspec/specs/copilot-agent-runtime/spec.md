# copilot-agent-runtime Specification

## Purpose
The third reference runtime, driving GitHub Copilot through its SDK: its obligations under the work contract, the runtime-minted session id behind `runtimeContextId`, the two-layer translation of agent-ops tool patterns into the vendor's vocabulary and what is denied when it cannot translate, MCP configuration with secrets resolved in the pod, the continuity failure when session state is gone, and the bundle that declares it.

## Requirements

### Requirement: A third reference runtime implements the same work contract
The project SHALL ship a third reference `AgentRuntime` image, `runtime-copilot`, driving GitHub Copilot through its SDK. It SHALL be a self-contained component under `runtimes/copilot/` with no dependency on any other module here, and SHALL implement the work contract unchanged: long-poll `GET $CONTROL_URL/work`, execute the returned unit against the checkout at `/data/workspace`, stream a human-readable transcript to stdout, `POST $CONTROL_URL/work/done` carrying `continuity`/`continuityReason`, and exit `0` after `RUNTIME_IDLE_TTL_M` minutes without work.

No CRD field, manager behaviour, `Pipeline`, `AgentProfile` or `MCPToolset` SHALL need to change for a conversation to run on it. `Pipeline.spec.runtimeRef` SHALL be the entire switch between vendors.

The image SHALL stay generic by construction — the repository checkout it owns plus generic shell utilities, and no domain tooling. What an agent may reach is wiring.

#### Scenario: The same wiring runs on any vendor
- **WHEN** a `Pipeline`'s `runtimeRef` is pointed from a claude or ollama runtime at the copilot runtime, changing nothing else
- **THEN** conversations it starts continue to run, with the same profile, the same channels, the same toolsets and the same MCP servers

#### Scenario: The contract is honoured end to end
- **WHEN** the copilot runtime receives a work unit
- **THEN** it executes it against the checkout, streams the transcript to stdout, reports the outcome to `/work/done`, and exits `0` once idle for the configured TTL
- **AND** the manager handles the report with no runtime-specific branch

#### Scenario: No domain tooling is baked in
- **WHEN** an agent on this runtime needs to reach a system outside its checkout
- **THEN** it reaches it through MCP servers its Pipeline binds, not through a CLI shipped in the image

### Requirement: The context handle is minted by the runtime and stays opaque
The runtime SHALL supply the vendor session identifier itself rather than discovering one after the fact. On a unit carrying no `runtimeContextId` it SHALL generate an identifier, create the vendor session under it, and report it on `/work/done` with `continuity: new`. On a unit carrying one it SHALL resume that session and report `continuity: continued`. The handle it reports SHALL be the one that now exists — latest-wins, the rule the manager applies when storing it.

The identifier SHALL NOT encode the conversation name or any other derivable value: a derivable handle re-introduces write-once behaviour, because a lost context could then never be replaced by a different one.

The runtime SHALL keep `contextStorage: volume` — its session state lives under `$HOME`, the CONTEXT volume at `/data/context` or the pod-local copy `context-sync` restores and snapshots. Its bundle SHALL declare the session-state directory in `contextSync.paths`, because only the runtime knows where its backend keeps context; a route with no durable context claim is told its context is not promised by the existing rule.

#### Scenario: A first run establishes a handle
- **WHEN** a conversation's first work unit arrives with no `runtimeContextId`
- **THEN** the runtime creates the vendor session under an identifier it generated and reports it with `continuity: new`, which the manager records

#### Scenario: A later run continues it
- **WHEN** a work unit carries a `runtimeContextId` whose session state is present
- **THEN** the runtime resumes that session, the agent answers with the conversation's accumulated context, and the run reports `continuity: continued`

#### Scenario: A replacement handle is reported, not hidden
- **WHEN** a run legitimately ends in a different vendor session than it was asked to continue
- **THEN** the runtime reports the new identifier, and the manager records it in place of the old one

#### Scenario: The handle is opaque
- **WHEN** the manager records the reported handle
- **THEN** it stores a string, and no manager code parses, validates or derives anything from its shape

### Requirement: A conversation whose context is gone fails rather than answering
When a resume fails, the runtime SHALL distinguish a store that says the context is GONE from one that merely did not ANSWER, by re-checking the session state after short bounded delays. A state directory that reappears, or a path that cannot be read at all, SHALL be treated as present — unreadable is not absent — and the run SHALL be retried once.

When the state is confirmed absent, the run SHALL FAIL with `continuity: unavailable` and a `continuityReason` naming the context volume as the likely cause. It SHALL NOT silently start a fresh session and present its answer as a continuation, and it SHALL NOT fail with an empty result: the reported result SHALL say the conversation cannot be continued and why.

#### Scenario: A slow store is not a lost context
- **WHEN** a resume fails and the session state is present on re-check
- **THEN** the runtime retries the run once and answers with the conversation's context

#### Scenario: An unreadable path is not a lost context
- **WHEN** the session state path cannot be read — a stale mount, an I/O error
- **THEN** the runtime treats the context as present rather than gone

#### Scenario: A genuinely lost context fails loudly
- **WHEN** a resume fails and the session state is confirmed absent after re-checking
- **THEN** the run fails with `continuity: unavailable` and a result explaining that the conversation cannot be continued, and no answer is produced from an empty context

### Requirement: Bound tool patterns are translated, and what cannot be translated is denied
`MCPToolset` patterns SHALL remain opaque and vendor-neutral in the API. The copilot runtime SHALL translate the composed allowlist into the vendor's own vocabulary at the point of use, across BOTH of the vendor's layers: which tools are available to the session, and whether a given invocation is permitted.

A pattern the runtime cannot translate SHALL be logged and SHALL contribute NOTHING — neither passed through verbatim nor discarded silently. A pattern scoping an MCP server as a whole (`mcp__<server>__*`) SHALL be refused when the vendor's filters cannot express a per-server wildcard, because the only available approximation grants every other MCP server bound to that conversation.

A sub-command-scoped shell pattern SHALL be enforced by permitting only invocations matching it; anything not matching SHALL be denied and logged. This is a fact about THIS runtime: another runtime without a per-invocation hook grants nothing on such a pattern and reports it, and each runtime's page states which it does.

At session start the runtime SHALL log every mapped target the vendor did not register, so a wrong wire name surfaces on the first run rather than as a tool that silently never appears.

#### Scenario: The catalog is bound unchanged
- **WHEN** a Pipeline binds the chart's built-in toolsets and routes to the copilot runtime
- **THEN** the conversation gets the vendor's equivalents of those tools, with no Copilot-specific toolset CR anywhere

#### Scenario: An unmapped pattern grants nothing
- **WHEN** a bound toolset contains a pattern the runtime has no mapping for
- **THEN** the runtime logs it and the conversation does not receive it, neither as a raw string nor as a silently dropped entry

#### Scenario: A per-server wildcard is refused, never widened
- **WHEN** a bound toolset contains `mcp__<server>__*` and the vendor admits only "all MCP tools" or exact names
- **THEN** the runtime refuses the pattern and logs it, rather than granting every MCP server bound to that conversation

#### Scenario: A scoped shell pattern is enforced per invocation
- **WHEN** the composed allowlist scopes shell access to one command family and the agent attempts a command outside it
- **THEN** the invocation is denied and logged, while a matching command is permitted

#### Scenario: A wrong wire name is visible on the first run
- **WHEN** a mapping targets a tool name the vendor session did not register
- **THEN** the runtime logs the mapping and its missing target at session start

### Requirement: An empty allowlist stays empty and nothing prompts
The runtime SHALL pass an explicit tool allowlist on every run, including when the composition produced nothing, and SHALL NOT substitute any default. It SHALL NOT let a vendor default — such as an agent definition with no declared tools receiving every tool — apply.

No run SHALL be able to block on an interactive permission prompt: every permission decision SHALL be answered programmatically from the composed allowlist, denying by default. In a pod there is nobody to answer a prompt, so prompting would hang the run until its idle TTL and report nothing.

#### Scenario: Nothing composed means nothing granted
- **WHEN** a conversation's wiring binds no toolset and its agent declares none
- **THEN** the run proceeds with no tools available, rather than with a substituted default

#### Scenario: A vendor's permissive default does not leak through
- **WHEN** the named agent's definition declares no tools and the vendor would otherwise grant all of them
- **THEN** the runtime still passes only the composed allowlist

#### Scenario: A denial is an answer, not a hang
- **WHEN** the agent attempts a tool outside the allowlist
- **THEN** the attempt is denied immediately and the run continues, rather than waiting on a prompt nobody can answer

### Requirement: Bound MCP servers reach the vendor with their secrets resolved
The runtime SHALL translate the compiled MCP configuration at `MCP_CONFIG` into the vendor's server configuration, preserving both transports: local/stdio servers with command, arguments and environment, and HTTP servers with URL and headers. HTTP servers SHALL be reached as any other client in the pod reaches them, so egress mediation applies to this runtime exactly as to the others.

Secret-backed values SHALL be resolved in-process from the pod environment before the configuration reaches the vendor, and SHALL never be written to the transcript or the run result. A placeholder that cannot be resolved SHALL fail that server's registration with a logged reason rather than reaching an MCP server as literal placeholder text.

#### Scenario: A bound MCP server is reachable
- **WHEN** a Pipeline binds an `MCPConfig` and the conversation runs on the copilot runtime
- **THEN** the vendor session registers that server and its tools are reachable subject to the allowlist

#### Scenario: A secret-backed value is resolved, not echoed
- **WHEN** an MCP server's header or environment value is secret-backed
- **THEN** the runtime resolves it from the pod environment and the value appears in no log line, transcript or result

#### Scenario: An unresolvable value fails the server, not the placeholder
- **WHEN** a placeholder has no corresponding environment value
- **THEN** that server fails to register with a logged reason, rather than being configured with the literal placeholder

### Requirement: The runtime ships as a bundle that declares only its vendor
The chart SHALL ship the runtime as `chart/charts/copilot/`, OFF by default, in the same shape as the other runtime bundles: one `AgentRuntime` rendered through the parent's shared runtime renderer over the release-wide defaults, carrying only what names this vendor — the image, its credential Secret reference (creatable from a supplied token), its `contextSync.paths`, and its own env. It SHALL render no defaults, no floor account, no context volume and no identity of its own; identity and storage are the route's.

The bundle SHALL be visible to the parent's declared-runtime list, so that the default-runtime guard, the `default` copy and the manager's bootstrap env account for it exactly as for any other runtime.

#### Scenario: Existing installs render byte-identically
- **WHEN** the chart is upgraded with no `copilot` values set
- **THEN** nothing of the bundle renders and every other object is unchanged

#### Scenario: A route selects it by name
- **WHEN** the bundle is enabled and a `Pipeline` states `runtimeRef: copilot`
- **THEN** that route's conversations run on the copilot runtime, and every route naming nothing keeps running on `default`

#### Scenario: It can be the default without a rename
- **WHEN** the reference bundle is disabled and the copilot bundle is enabled
- **THEN** `default` renders as a copy of the copilot runtime, with no other value set

#### Scenario: The bundle ships no substrate
- **WHEN** the bundle is enabled
- **THEN** it renders no runtime defaults, no floor ServiceAccount, no context volume and no ServiceAccount of its own
