# The runtime contract

**Version 1.0.0.** The versioned specification of what an `AgentRuntime`
image owes the manager, and what the manager owes it. Build against this
page alone — no shipped runtime's source is required.

Two parties, each exhaustive for its own obligations:
[Manager's provisions](#managers-provisions) and
[Runtime's obligations](#runtimes-obligations).

The [Conformance checklist](#conformance-checklist) at the foot is how a
claim of conformance is verified, item by item. The
[Changelog](#changelog) records every version.

## Manager's provisions

| Provision | Content |
|---|---|
| One pod per conversation | The manager creates one runtime pod per `Conversation` and owns its lifecycle. |
| Serial dispatch | One inflight work unit per conversation, never two. |
| Context restoration | Where the deployment checkpoints context, durable context is restored before the first `GET /work` answers. |
| Checkpoint ordering | The context is checkpointed before `POST /work/done` reaches the manager, so a recorded handle always names persisted context. |
| Start-deadline reaping | A pod that never starts is reaped at a deadline, freeing its admission slot. |
| Environment | Every variable in the [environment table](#environment), and the git credentials for the profile's repository. |

**Context restoration and checkpoint ordering are invisible to the
runtime.** A runtime declaring `contextSync` talks to a sidecar in its own
pod, which forwards every request to the manager.

- The sidecar restores durable context into the pod-local home before the
  first `GET /work` answers.
- It checkpoints that context before `POST /work/done` leaves the pod.

(`platform/context-sync/main.go`: `BeforeFirstWork` runs `restore`,
`BeforeWorkDone` runs `checkpoint`.) A runtime needs no awareness of this —
nothing in the contract changes underneath it.

**That ordering is a guarantee, not an implementation detail.** The manager
records the context handle from the completion report. Checkpointing
afterwards could leave a recorded handle whose context was never persisted,
and the next run would then fail a continuation that should have worked.

**Start-deadline reaping** bounds how long a runtime pod may sit without
reaching `Running` (`platform/manager/internal/controller/runtimestart.go`).
A pod past the deadline is reaped and its reason recorded on the
`RuntimeStarted` condition, freeing the admission slot it held.

## Runtime's obligations

An `AgentRuntime` image MUST:

1. **Long-poll** `GET $CONTROL_URL/work?convo=$CONVO_ID&pod=$POD_NAME&wait=25`.
2. **Clone the repository** named by `REPO_URL`/`REPO_REF`, using the git
   credentials the environment carries, when `REPO_URL` is not empty.
3. **Execute the returned unit** — see [Work unit](#work-unit) — streaming
   progress to **stdout**.
4. **Compose the tool allowlist** as `toolsMode` directs — see
   [Enums](#enums).
5. **Report** the outcome on `POST $CONTROL_URL/work/done` within the
   [bounds](#bounds) — see [Completion report](#completion-report).
6. **Exit `0`** after `RUNTIME_IDLE_TTL_M` minutes without work.

**Repository checkout is the runtime's obligation.** The manager provides
`REPO_URL`, `REPO_REF` and the git credentials. It never checks the
repository out itself — every shipped runtime clones it independently, at
`/data/workspace` (see [Filesystem](#filesystem)).

**`$CONTROL_URL` is not always the manager.** When a runtime declares
`contextSync`, the manager points it at the sidecar described above. The
runtime makes the same requests either way.

### Composing the allowlist

`allowedTools` is the route's half of the allowlist, never the whole of it.
A runtime holding the repository reads the named `agent`'s definition, takes
its `tools:` frontmatter as the agent's own declaration, and composes the
two per `toolsMode`.

| `toolsMode` | Result |
|---|---|
| `merge` | the union, the agent's own tools keeping position |
| `overwrite` | `allowedTools` alone |

A runtime that cannot see a repository uses `allowedTools` as-is. That is
what `merge` degrades to.

**An empty allowlist means empty.** Substituting a tool nobody declared is a
grant the operator did not write down. **An absent or unreadable
definition contributes NOTHING**, whatever a vendor's own default would be —
a runtime reading "no `tools:` key" as "every tool" is not conformant.

WHERE an agent's definition lives is the runtime's own fact, not the
contract's — `runtimes/claude` and `runtimes/ollama` read
`.claude/agents/<agent>.md`, `runtimes/copilot` reads
`.github/agents/<agent>.agent.md`, and another runtime may read somewhere
else.

### Optional capabilities

| Capability | A runtime may | Trigger |
|---|---|---|
| Context continuity | mint and report its own `runtimeContextId`, and resume one it is given | present only where [`ContinuityPossible`](#lifecycle) holds |
| `contextSync` | keep its live context on pod-local storage, delegating durable checkpoints to the sidecar | the `AgentRuntime` CR declares `spec.contextSync` |
| the aops MCP server | reach the coordination verbs behind `AOPS_MCP_TOKEN` | the conversation's wiring binds the `aops` MCP toolset and a Coordinator resolves |

A runtime implementing none of these stays fully conformant. Each is
additive — see [Compatibility](#compatibility).

### Security posture

- **The manager never reads a secret.** Every credential compiles to
  `valueFrom`/`envFrom` for the kubelet (`GIT_SSH_KEY` from a mounted
  Secret, `GIT_TOKEN` and `AOPS_MCP_TOKEN` via `SecretKeyRef`/derivation).
- **`AOPS_MCP_TOKEN` is derived, never stored.** The manager validates any
  presented token by re-deriving it.
- **The runtime gets no Kubernetes permissions from the operator.** Its
  `spec.serviceAccountName` is a reference the chart or operator may bind,
  never a grant the reconciler creates.
- **A tool's input and output are never reported.** A shell command or a
  file's contents is content, and content has no place in telemetry — see
  [Completion report](#completion-report).

## Lifecycle

A runtime pod moves through one state machine, from the manager's `GET
/work` and `POST /work/done` observing every transition:

| State | Entered | Left by |
|---|---|---|
| Starting | the manager creates the pod | reaching `Running` (or the start deadline, see [Manager's provisions](#managers-provisions)) |
| Polling | `Running`, no inflight run | `GET /work` returns `200` with a unit, or the idle TTL elapses with no unit ever returned |
| Working | `GET /work` returned a unit | `POST /work/done` is accepted |
| Idle | `POST /work/done` is accepted, nothing queued | the next `GET /work` returns a unit, or `RUNTIME_IDLE_TTL_M` minutes elapse |
| Exited | the runtime calls `exit 0` from Idle, or `/exit` deletes the pod | — |

**One inflight unit, never two.** The manager admits one work unit at a time
per conversation. A runtime polling while Working is a contract violation —
there is nothing for it to be returned, because the manager holds no second
unit to hand out.

**Continuation is promised only where it is possible.** A work unit carries
`runtimeContextId` only in a deployment whose runtime can carry context
between runs.

Where it is possible, the durable context named by that handle was
restored into the pod before this `GET /work` answered — see [Manager's
provisions](#managers-provisions).

**Exit is the runtime's own decision, on one condition only:** no work
received for `RUNTIME_IDLE_TTL_M` minutes.

- A pod that exits earlier abandons whatever the manager might still
  dispatch to it.
- A pod that never exits is bounded from outside instead — `/exit` deletes
  it, refused while a run is in progress.

## Environment

| Kind | Variable | Meaning |
|---|---|---|
| always present | `ROLE` | A fixed, single-value constant naming this pod's role in the manager's process topology. The runtime does not need to inspect it. |
| always present | `CONVO_ID` | The `Conversation` this pod serves. |
| always present | `CONTROL_URL` | Where to long-poll and report — the manager, or a `contextSync` sidecar on loopback. |
| always present | `REPO_URL` | The profile's repository URL. **May be empty.** Empty means no repository — the runtime clones nothing. |
| always present | `REPO_REF` | The ref to check out. May be empty alongside an empty `REPO_URL`. |
| always present | `RUNTIME_IDLE_TTL_M` | Minutes without work before the runtime must exit `0`. |
| always present | `HOME` | The live context mount — see [Filesystem](#filesystem). |
| always present | `MCP_CONFIG` | Path to the composed MCP configuration — see [Filesystem](#filesystem). |
| always present | `POD_NAME` | This pod's own name, for the long-poll query and for logging. |
| conditional | `GIT_AUTH_TYPE` + `GIT_SSH_KEY` | The profile declares SSH repository auth. `GIT_SSH_KEY` names a mounted key file. |
| conditional | `GIT_AUTH_TYPE` + `GIT_TOKEN` | The profile declares HTTPS repository auth. `GIT_TOKEN` carries the credential. |
| conditional | `AOPS_MCP_TOKEN` | The conversation's wiring bound the `aops` MCP toolset and a Coordinator resolves for it — see [Optional capabilities](#optional-capabilities). |

**`REPO_URL` and `REPO_REF` are always set, and may both be empty.** An
empty `REPO_URL` means this conversation has no repository. A conformant
runtime skips cloning rather than treating the empty string as an error or
as "clone the default".

## Filesystem

| Path | Is |
|---|---|
| `/data/workspace` | The repository checkout. **Never moves** — the stored transcript directory some runtimes key sessions by is named for this path. |
| `$HOME` | The live context mount. Points at pod-local storage under `contextSync`, or directly at the durable volume otherwise. |
| the path `MCP_CONFIG` names | The composed MCP configuration (`mcp.json`), read-only. |

## Work unit

What a runtime receives from `GET /work`.

### The work unit

<!-- generated: struct-table file=platform/manager/internal/dispatch/dispatch.go struct=WorkUnit -->
| Field | Type | Meaning |
|---|---|---|
| `runId` | string | RunID is a unique id for this unit, echoed back in the completion report. |
| `convo` | string | Convo is the Conversation this unit belongs to. |
| `threadId` | string, optional | ThreadID is the bound channel's thread id, present only for a conversation bound to exactly one channel. The runtime never posts to it -- the manager delivers to every bound channel after the report. |
| `runtimeContextId` | string, optional | RuntimeContextID is the runtime's OWN handle for this conversation's accumulated context, echoed back from the last run. Continue that context, or report that you could not — where it is stored is the runtime's business and the manager assumes nothing about it. Empty means "start fresh, nothing is being continued", which is also what a deployment that cannot carry context sends every time, deliberately. |
| `resumeSessionId` | string, optional | ResumeSessionID is the former name of RuntimeContextID, sent alongside it for ONE release so a runtime image can be upgraded independently of the manager. Runtimes must read RuntimeContextID. DEPRECATED. |
| `promptFile` | string, optional | PromptFile is a repo-relative path to render, when the profile supplies its own prompt file. The runtime renders PromptVars into it. |
| `promptText` | string, optional | PromptText is the fully rendered prompt, when the manager rendered it from a built-in template. Exactly one of PromptFile/PromptText is set. |
| `promptVars` | map of string to string, optional | PromptVars carries template variables for PromptFile. |
| `agent` | string, optional | Agent is the agent definition's name, for the runtime to resolve from the checked-out repository. |
| `systemPrompt` | string, optional | SystemPrompt is inline role text the runtime APPENDS to the agent's system prompt. Identity only — it never affects the allowlist. |
| `allowedTools` | string, optional | AllowedTools is the wiring's half of the tool allowlist, comma-separated. |
| `toolsMode` | string, optional | ToolsMode is "merge" or "overwrite", directing how AllowedTools composes with the agent definition's own declared tools. |
| `maxTurns` | integer, optional | MaxTurns is the profile's turn budget for this run, when set. |
<!-- /generated -->

Source: `platform/manager/internal/dispatch/dispatch.go`, type `WorkUnit`.

## Completion report

What a runtime posts to `POST /work/done`.

### The completion report

<!-- generated: struct-table file=platform/manager/internal/httpapi/server.go struct=workDone -->
| Field | Type | Meaning |
|---|---|---|
| `convo` | string | Convo is the Conversation this report is for. |
| `runId` | string | RunID is the unit's own RunID, round-tripped. |
| `status` | string | Status is "succeeded" or "failed". The manager treats any other value as a failed run. |
| `exitCode` | integer, optional | ExitCode is the process exit code, when the runtime has one to report. |
| `runtimeContextId` | string, optional | RuntimeContextID is the handle for the context this run leaves behind — the one to continue next time. Runtimes may report it on a FAILED run too: a crash after the context was established should not strand it. |
| `sessionId` | string, optional | SessionID is the retired spelling, accepted for one release so an older runtime image keeps working. DEPRECATED. |
| `continuity` | string, optional | Continuity says what happened to the context this run was asked to continue: "continued", "new", or "unavailable" (a new one was needed because the named context could not be reached). The manager cannot infer this. It sends a handle and receives a handle, and when they differ it has no way to tell an agent that branched deliberately from one that was forced to start over. The runtime knows, because it is the component that tried. ABSENT MEANS NO CLAIM: a runtime that does not implement it keeps today's behaviour, because an addition to the contract must not make an existing runtime look broken. |
| `continuityReason` | string, optional | ContinuityReason is free text from the runtime explaining why a context could not be continued. Recorded VERBATIM — the manager does not know where any given runtime keeps its context, so any explanation it invented would be confident and sometimes wrong. |
| `result` | string, optional | Result is the run's answer. Truncated to 2000 bytes by the manager, never refused for length. |
| `brief` | string, optional | Brief is what the runtime extracted as this conversation's one- or two-sentence description (design D-I) — a contract field, never a parsed section of Result. LATEST-WINS, the same rule as RuntimeContextID: absent leaves the stored one untouched. |
| `turns[]` | array of Turn, optional | Turns is one entry per model call this run made, bounded in count and field size (workreport.go). It becomes one activity hop and nothing else — telemetry, never written to the Conversation. |
| `toolCalls[]` | array of ToolCall, optional | ToolCalls is one entry per tool call this run made, bounded in count and field size (workreport.go). It becomes one activity hop and nothing else — telemetry, never written to the Conversation. |
<!-- /generated -->

Source: `platform/manager/internal/httpapi/server.go`, type `workDone`.

**The manager never sees a model call or a tool call. The runtime does**, so
it may report both with its result. The manager records one activity hop
each — `model.call` and `tool.call`, see [the activity
contract](../contracts.md#the-activity-contract) — and writes neither to
the `Conversation`.

**Both `turns[]` and `toolCalls[]` are optional.** A runtime that reports
neither stays conformant, and its runs draw exactly the hops they drew
before this field existed.

### Turns

One entry per model call, from `platform/manager/internal/httpapi/workreport.go`, type `Turn`.

| Field | Type | Meaning |
|---|---|---|
| `model` | string, optional | The model the call went to, as the runtime names it. |
| `tokensIn` | integer, optional | Every input token the call consumed, cache reads included, as far as the vendor reports them. |
| `tokensOut` | integer, optional | The output tokens the call produced. |
| `cacheReadTokens` | integer, optional | The part of `tokensIn` served from the provider's prompt cache. |
| `cacheCreationTokens` | integer, optional | The part of `tokensIn` written into the provider's prompt cache by this call. |
| `stopReason` | string, optional | Why the model stopped: end of turn, tool use, max tokens. |

### Tool calls

One entry per tool call, from `platform/manager/internal/httpapi/workreport.go`, type `ToolCall`.

| Field | Type | Meaning |
|---|---|---|
| `tool` | string | The tool's name in the agent-ops vocabulary: `Bash`, `Read`, or `mcp__<server>__<tool>`. |
| `server` | string, optional | The MCP server the call reached. **Empty for a built-in tool**, which then records a hop with no destination. |
| `durationMs` | integer, optional | Wall time from the call to its result. |
| `resultBytes` | integer, optional | The size of what the tool returned. |

**A fact the runtime cannot determine is OMITTED, never sent as zero.** Zero
tokens is a real, different value. **A tool's input and output are never
reported.** A shell command or a file's contents is content, and content has
no place in telemetry.

## Enums

| Enum | Values | Reading |
|---|---|---|
| `status` | `succeeded`, `failed` | **The manager treats any value other than `succeeded` as a failed run.** A typo or an unrecognized status is not a free pass. |
| `continuity` | `continued`, `new`, `unavailable` | Absent means no claim — a runtime that does not implement it leaves the manager's continuity condition untouched rather than appearing to have lost something. |
| `toolsMode` | `merge`, `overwrite` | An absent or unrecognized value composes as `merge` — see [Composing the allowlist](#composing-the-allowlist). |

## Bounds

The completion report's telemetry (`turns[]` and `toolCalls[]`) is bounded.

| Bound | Value |
|---|---|
| entries in `turns[]` | at most 200 |
| entries in `toolCalls[]` | at most 200 |
| any string field of a turn or a tool call | at most 200 bytes |
| any integer field of a turn or a tool call | not negative |

**A report over any of these bounds is refused WHOLE, with `400` naming the
field and the bound it broke — never trimmed.** The refusal covers the
entire `POST /work/done`, `result` and `brief` included.

A conformant runtime trims `turns[]`/`toolCalls[]` to these bounds before
posting. All three shipped runtimes do — see the [conformance
checklist](#conformance-checklist).

`result` and `brief` are a different case: the manager truncates each
silently (`result` to 2000 bytes, `brief` to 512) rather than refusing the
report. Nothing else in the report is bounded.

Source: `platform/manager/internal/httpapi/workreport.go`, constants
`MaxReportedTurns`, `MaxReportedToolCalls`, `MaxReportedField` and function
`validateReport`.

## Compatibility

A runtime MUST ignore work-unit fields it does not know. The manager MUST
ignore report fields it does not know. Both already hold: both sides decode
with plain `encoding/json` (`json.Unmarshal`), neither calls
`DisallowUnknownFields`. Together they make an additive field a MINOR
change, never a breaking one.

| Version part | Moves when |
|---|---|
| MAJOR | a runtime conformant with the previous version stops conforming |
| MINOR | a new optional capability or field is added |
| PATCH | a clarification that changes no obligation |

**Section anchors are stable names.** The adopter guide and the shipped
runtime pages link by anchor. Renaming a published anchor is a breaking
change to this document, and moves MAJOR — even though nothing about the
wire contract itself changed.

**`GET /work` reserves a future version parameter.** None is implemented.
There is no `?version=` query parameter today, and every runtime omits
one.

The reservation is this sentence: should wire negotiation ever become
necessary, adding that parameter is a MINOR change against this contract,
not a MAJOR one. An absent parameter already means "the version this
document describes."

## Conformance checklist

Each item is independently verifiable against a running runtime or its
source. A conformance claim is this list walked item by item, never an
impression of the whole.

1. **Long-polls `GET $CONTROL_URL/work?convo=&pod=&wait=`** and treats `204`
   as "no work yet", not an error.
   Tests: `runtimes/claude/runtime.mainloop.test.js`,
   `runtimes/copilot/runtime.test.js` (`pollNextUnit`),
   `runtimes/ollama/work_test.go` (`TestPollDecodesAWorkUnit`,
   `TestPollNoContentMeansNoWork`).
2. **Clones the repository from `REPO_URL`/`REPO_REF`** when `REPO_URL` is
   non-empty, and clones nothing when it is empty.
   Tests: `runtimes/claude/runtime.gitclone.test.js`,
   `runtimes/copilot/runtime.gitclone.test.js`,
   `runtimes/ollama/repo_sync_test.go` (`TestSyncWithNoURLIsANoOp`,
   `TestSyncClonesThenFetchesAndResetsAgainstARealRepo`).
3. **Authenticates the clone from `GIT_AUTH_TYPE`/`GIT_SSH_KEY` or
   `GIT_AUTH_TYPE`/`GIT_TOKEN`**, whichever the environment carries.
   Tests: `runtimes/claude/runtime.sshenv.test.js`,
   `runtimes/claude/runtime.httpsenv.test.js`,
   `runtimes/copilot/runtime.sshenv.test.js`,
   `runtimes/copilot/runtime.httpsenv.test.js`,
   `runtimes/ollama/repo_sync_test.go`
   (`TestCloneURLEmbedsTheTokenOnlyForHTTPSAuth`,
   `TestEnvSetsGitSSHCommandOnlyForSSHAuth`).
4. **Composes `allowedTools` with the agent definition's own `tools:`** per
   `toolsMode`, with an absent/unknown mode reading as `merge`.
   Tests: `runtimes/claude/tools.test.js`, `runtimes/copilot/tools.test.js`,
   `runtimes/ollama/tools_test.go` (`TestFrontmatterForms` for reading the
   definition, `TestCompose` for the merge/overwrite composition).
5. **An empty allowlist stays empty.** No tool is substituted for one
   nobody declared.
   Tests: `runtimes/claude/tools.test.js` (section "empty means empty"),
   `runtimes/copilot/tools.test.js`, `runtimes/ollama/tools_test.go`
   (`TestCompose`, "empty means empty" cases).
6. **Reads `runtimeContextId` for continuation, and reports both
   `runtimeContextId` and the deprecated `sessionId`/`resumeSessionId` for
   one release.**
   Tests: `runtimes/claude/runtime.mainloop.test.js` (asserts both fields
   on the posted report), `runtimes/ollama/work_test.go`
   (`TestReportPostsTheDoneBodyAndSucceeds`), `runtimes/copilot/runtime.test.js`
   (`contextIdOf prefers runtimeContextId over the retired resumeSessionId`,
   `contextIdOf falls back to resumeSessionId for one release`).
7. **Reports `status: succeeded` on a clean run and `status: failed` on an
   error**, never a third value.
   Tests: `runtimes/claude/runtime.spawnerror.test.js`,
   `runtimes/ollama/agent_test.go` (`TestTurnLimit`,
   `TestModelWithoutToolsOnAToolRoute`, `TestEmptyPromptAndInferenceFailure`,
   `TestMissingContextFails`). **No runtime-copilot test asserts the literal
   `status: 'failed'` string** — see the open item in this report's
   hand-back.
8. **Reports `continuity` only when it has a claim**, and omits it
   otherwise.
   Tests: `runtimes/ollama/agent_test.go` (`TestTextOnly` for `new` then
   `continued`, `TestMissingContextFails` for `unavailable`).
   `runtimes/copilot/continuity.test.js` covers the missingness check
   `confirmContextMissing` that feeds this decision, but **no copilot test
   asserts the literal `continuity` string on a posted report** — see the
   open item in this report's hand-back.
9. **Trims `turns[]`/`toolCalls[]` to the manager's bounds (200 entries,
   200 bytes per string field) before posting**, so a run is never refused
   for exceeding them.
   Tests: `runtimes/claude/report.test.js` ("trimmed to the manager
   bounds"), `runtimes/copilot/report.test.js`,
   `runtimes/ollama/report_test.go`
   (`TestReportIsTrimmedToTheManagersBounds`).
10. **Omits a telemetry fact it cannot determine, never reports it as
    zero.**
    Tests: `runtimes/claude/report.test.js` ("an unknown fact is omitted,
    never zero"), `runtimes/ollama/report_test.go`
    (`TestChatWithoutCountsLeavesThemUnset`).
11. **Never reports a tool's input or output as telemetry.**
    Tests: `runtimes/claude/report.test.js` ("no tool input or output
    reaches the report"), `runtimes/copilot/report.test.js`,
    `runtimes/ollama/report_test.go`
    (`TestRunReportsItsTurnsAndToolCalls`).
12. **Exits `0` after `RUNTIME_IDLE_TTL_M` minutes without work**, and with
    `RUNTIME_IDLE_TTL_M=0` exits before ever polling.
    Tests: `runtimes/claude/runtime.mainloop.test.js`,
    `runtimes/copilot/runtime.mainentry.test.js`. **No dedicated test
    exercises `runtimes/ollama`'s idle-TTL loop** — the logic lives inline
    in `main()` (`ollama/main.go`), untested directly. See the report's
    open item below.
13. **Streams run progress to stdout**, never to a transport of its own —
    an agent produces output, the operator delivers it.
    Tests: `runtimes/claude/runtime.test.js` (`formatEvent` renders each
    stream event to a stdout line), `runtimes/ollama/ollama_test.go`
    (`TestChatAlwaysSetsNumCtxAndStreams` asserts the partial content
    written to `out` as it streams).
14. **Reports neither `turns[]` nor `toolCalls[]` when it saw neither a
    model call nor a tool call**, rather than reporting an empty list.
    Tests: `runtimes/claude/report.test.js` ("a synthetic message is not a
    model call, and a silent stream reports nothing"),
    `runtimes/ollama/report_test.go` (`TestSilentRunReportsNothing`).

## Changelog

| Version | Date | Changed |
|---|---|---|
| 1.0.0 | 2026-10-17 | First versioned edition. Documents existing shipped behavior — no behavior change. Replaces the narrative "work contract" section of `docs/contracts.md`, adding the full work-unit and report field tables, every enum's values, the bounds and their whole-report refusal, the environment table split into always-present and conditional, the filesystem contract, the lifecycle state machine, and the compatibility rules (including the reserved `GET /work` version parameter). |
