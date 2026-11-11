## Context

See `proposal.md` for motivation. The shape to follow is `runtimes/copilot`:
a Node runtime, a vendor SDK in-process, one translation boundary per vendor
fact (`vocabulary.js`, `tools.js`, `mcp.js`, `continuity.js`, `report.js`),
and a bundle that declares only the vendor.

Constraints that shape the approach:

- `docs/contracts/runtime.md` is the one law for this surface. Every
  obligation is walked against its 35-item checklist.
- The manager reads no secret and grants the runtime no Kubernetes
  permission. The credential is env from a Secret reference.
- `platform/egress-proxy` walls MCP traffic only. It sees no built-in tool
  call, so it cannot be what enforces `RT-3` for `Bash`, `Edit` or `Write`.
- Pi is a 0.x package whose npm scope has moved once, from
  `@mariozechner/pi-coding-agent` to `@earendil-works/pi-coding-agent`.
  Its SDK surface can change between minors.
- Another in-flight change, `agent-definition-adopted-by-runtime`, specifies
  that each runtime appends the agent definition's body to its system prompt.
  Pi implements the same rule, so the two changes agree and neither
  restates the other.

## Goals / Non-Goals

**Goals:**
- A route moves to pi by changing `runtimeRef` and nothing else.
- Pi's tool surface is exactly what the composed allowlist names.
- The runtime consumes `mcp.json` and nothing else for MCP.

**Non-Goals:**
- Per-command shell narrowing. Pi's built-ins are enabled whole or not at
  all, as in `runtimes/ollama`.
- A pi-specific toolset, CRD field or chart value for tools or MCP.
- Running pi's own extension ecosystem. Only the one extension we ship is
  loaded.
- A conformance lane if pi cannot be pointed at a fake provider. The unit
  tier carries the assertions instead, and the design records that.

## Decisions

### D1. The SDK in-process, not RPC

The runtime embeds pi's SDK as `runtimes/copilot` embeds its own.

| Option | For | Against |
|---|---|---|
| SDK in-process | per-turn events and tool registration live in our process, one place to filter tools | tracks a 0.x API surface |
| RPC subprocess | a process boundary absorbs API churn | a second protocol to keep, events arrive as text to re-parse |

→ SDK. The fallback is RPC, and D1 is revisited only if a spike shows the SDK
cannot express D2 or D5.

### D2. Built-ins are enabled by name, whole or not at all

The composed allowlist is mapped to pi's built-in names. Only a built-in the
allowlist names is enabled for the session.

| agent-ops | Pi built-in |
|---|---|
| `Read` | read |
| `Write` | write |
| `Edit` | edit |
| `Bash` | bash |
| `Grep` | grep |
| `Glob` | find |

The pi names above are the working assumption, pinned by spike S1. A
`Bash(<prefix>:*)` pattern enables nothing and is logged, the posture
`runtimes/ollama` already takes. An empty composition enables nothing.

Alternative: a per-call permission hook that honours `Bash(kubectl:*)`.
Rejected for now.

- It would make this runtime the second one able to narrow a shell.
- It needs a hook reached for every call, which S1 has not established.
- Enablement is availability-level and needs nothing beyond selecting tools.

### D3. The MCP extension is ours, small, and a client of `mcp.json`

Pi core has no MCP, so the image carries one extension we write. It is the
client half only.

```
MCP_CONFIG ──▶ parse ──▶ expand ${ENV} ──▶ connect (listed servers only)
                                                │
                                   tools/list ──┘
                                        │
                    name = mcp__<server>__<tool>
                                        │
              register only if the composed allowlist permits it
```

- The runtime starts no server of its own choosing and reads no other MCP
  source. `internal/mcpcompile` already decides what a conversation binds.
- A stdio entry is spawned by the runtime, as the claude and copilot runtimes
  spawn it. That is the residual risk `docs/security.md` already states.
  HTTP endpoints stay behind the egress proxy.
- `${ENV}` placeholders are expanded in-process and never logged. A missing
  one fails that server alone.
- The translation mirrors `runtimes/copilot/mcp.js` field for field, so a
  reviewer reads one shape twice.

Alternative: a third-party pi MCP package. Rejected. It would put an
unreviewed dependency between the manager's compiled config and the agent,
and its server-selection logic is exactly what this design keeps out.

### D4. The agent definition's body joins the system prompt

The runtime reads `.claude/agents/<agent>.md`, appends the body to pi's
system prompt, then the unit's `systemPrompt`, on every run. Its `tools:`
compose per `toolsMode` through the same `composeAllowedTools` shape the
other Node runtimes share.

Appended, not replacing. The other runtimes append to the vendor's own
prompt, and a route moving between vendors should see one behaviour.

Whether pi's SDK offers append or only replace is spike S2. If only replace,
the runtime composes pi's default text with ours. The difference is then
stated on the runtime's page.

This needs `RT-3.4` to name pi's path, a PATCH edit to the specification,
2.0.1 to 2.0.2. The user approved that edit explicitly for this change.

### D5. Context is a pi session under `$HOME`

The runtime mints an identifier, creates the session under it and reports it
as `runtimeContextId`. The bundle declares the session directory in
`contextSync.paths`.

Whether pi accepts a caller-chosen id is spike S3. Whether its state root can
sit under `$HOME` is spike S4.

The runtime always mints the identifier it reports, as the spec requires.

If pi names its own sessions, the runtime keeps a small mapping from the
minted id to pi's session inside the declared state directory. The mapping
checkpoints and restores with the context. The handle stays opaque to the
manager.

### D6. Telemetry comes from pi's events

`turns[]` and `toolCalls[]` are built from the SDK's events, through a
recorder shaped like `runtimes/copilot/report.js`. A figure pi does not
report is omitted. Whether per-turn usage and cache counts are exposed is
spike S5.

### D7. One bundle, off by default

`chart/charts/pi/` follows `chart/charts/copilot/`: the credential Secret, one
`AgentRuntime` through the parent's shared renderer, a parent
`agentops.piRuntimeEntry` helper beside `agentops.copilotRuntimeEntry`, and the
branch in the declared-runtime list. Provider, model and credential are env.
The bundle adds no value that is not env, an image or a context path.

### D8. Pin exactly, from a lockfile

`package.json` pins one version of the SDK package and `npm ci` installs from
the lockfile. The Dockerfile copies copilot's npm CVE-pin block, since the
base image is the same. The package scope is pinned in the lockfile, and a
rename upstream is a deliberate bump.

## Spikes

Each is a task that ends in a written finding under this section, before any
code depends on the answer.

| # | Question | Decides |
|---|---|---|
| S1 | Can a session enable a chosen set of built-in tools, and does pi expose a per-call hook | D2, whether narrowing is possible later |
| S2 | System-prompt option: append or replace, and does it apply on resume | D4 |
| S3 | Create and resume a session by caller-chosen id | D5 |
| S4 | Relocate the state directory under `$HOME`, and which paths are the session | D5, `contextSync.paths` |
| S5 | Per-turn usage, cache counts and stop reason in events | D6 |
| S6 | Does pi load `AGENTS.md` or `CLAUDE.md` from the workspace by itself, and can that be turned off | D4, the transcript contract |
| S7 | Can the provider base URL point at a fake, for a conformance lane | the system and conformance tiers |

S6 matters because the checkout is the profile's repository. A file pi loads
on its own would be a prompt nobody composed, so if it cannot be turned off
the runtime's page says so.

## Risks / Trade-offs

- **Pi's SDK moves under a pinned version** → exact pin and lockfile, and the
  unit tests assert the SDK surface the runtime uses, so a bump that changes
  it fails there rather than live.
- **A built-in name differs from the table in D2** → the runtime checks every
  mapped name against the session's registered inventory at start and logs
  any missing target, as copilot does.
- **No per-command shell narrowing** → stated on the runtime's page. A route
  needing `Bash(kubectl:*)` semantics stays on a runtime that has it.
- **A stdio MCP entry runs as a child of the agent's pod** → the existing,
  documented residual risk, unchanged by this runtime.
- **Pi's own context files could add an uncomposed prompt** → S6 settles it
  before the runtime's behaviour is claimed.
- **Fifth image to scan and release** → components.sh discovers it, the
  release workflow and the image scan treat it like the others.

## Migration Plan

Nothing migrates. The bundle is off by default and no CRD changes.

1. Release `runtime-pi-v<semver>` by tag, after the image is in the registry.
2. An adopter enables the `pi` bundle and sets `runtimeRef: pi` on a route.
3. Rollback is `runtimeRef` back to the previous runtime, then disabling the
   bundle. Conversation context is runtime-specific, so a conversation does
   not continue across vendors, which the manager already reports as
   `continuity: unavailable` rather than answering from nothing.

## Open Questions

- Whether the pi bundle may become the `default` runtime. The shared helper
  allows it by construction, and the spec scenario for it is omitted until a
  system lane proves it.
