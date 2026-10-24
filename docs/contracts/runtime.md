---
title: AgentRuntime specification
permalink: /contracts/runtime/
description: >-
  The versioned, normative specification of what an AgentRuntime image owes
  the manager, and what the manager owes it — for building a runtime
  agent-ops does not ship.

next:
  eyebrow: Reference
  title: Every field of every kind
  body: >-
    The generated custom resource reference, and the contracts the adapter and
    runtime kinds serve.
  url: https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/cr-reference.md
---

**Version 2.0.1**

This specification gives everything necessary to build and maintain an
`AgentRuntime` — a harness agent-ops does not ship.

## Conventions

MUST / MUST NOT / SHOULD / MAY follow RFC 2119. Every requirement carries
a stable ID, prefixed by the section it belongs to:

| Prefix | Section |
|---|---|
| `API-n` | `GET /work` / `POST /work/done` |
| `LC-n` | [Lifecycle](#lifecycle) |
| `RT-n` | [Runtime's obligations](#runtimes-obligations) |
| `MGR-n` | [Manager's provisions](#managers-provisions) |
| `SEC-n` | [Security posture](#security-posture) |
| `COMPAT-n` | [Compatibility](#compatibility) |

## GET /work

Long-polls for this conversation's next work unit.

`GET $CONTROL_URL/work?convo=$CONVO_ID&pod=$POD_NAME&wait=25`

| Param | Required | Meaning |
|---|---|---|
| `convo` | yes | The `Conversation` to poll for. |
| `pod` | no | This pod's own name, logged by the manager. |
| `wait` | no | Seconds to block. Clamped to 30 server-side — a runtime asking for more still gets at most 30. |

| Status | Body |
|---|---|
| `400` | `{"error": "missing convo"}` — `convo` was empty. |
| `204` | No work yet. Not an error. |
| `200` | A work unit, below. |

**Example — `200`:**

```json
{"runId": "r-7f2a", "convo": "convo-abc12", "agent": "release-notes",
 "allowedTools": "Bash,Read", "toolsMode": "merge", "maxTurns": 40}
```

### Work unit

<!-- generated: struct-table file=platform/manager/internal/dispatch/dispatch.go struct=WorkUnit -->
| Field | Type | Required | Description |
|---|---|---|---|
| `runId` | `string` | **yes** | A unique id for this unit, echoed back in the completion report. |
| `convo` | `string` | **yes** | The Conversation this unit belongs to. |
| `threadId` | `string` |  | The bound channel's thread id, present only for a conversation bound to exactly one channel. |
| `runtimeContextId` | `string` |  | The runtime's OWN handle for this conversation's accumulated context, echoed back from the last run. |
| `resumeSessionId` | `string` |  | The former name of `runtimeContextId`, sent alongside it for ONE release so a runtime image can be upgraded independently of the manager. |
| `promptFile` | `string` |  | A repo-relative path to render, when the profile supplies its own prompt file. |
| `promptText` | `string` |  | The fully rendered prompt, when the manager rendered it from a built-in template. |
| `promptVars` | `map[string]string` |  | The set of template variables rendered into `promptFile`. |
| `agent` | `string` |  | The agent definition's name, for the runtime to resolve from the checked-out repository. |
| `systemPrompt` | `string` |  | Inline role text the runtime APPENDS to the agent's system prompt. |
| `allowedTools` | `string` |  | The wiring's half of the tool allowlist, comma-separated. |
| `toolsMode` | `string` |  | "merge" or "overwrite", directing how `allowedTools` composes with the agent definition's own declared tools. |
| `maxTurns` | `integer` |  | The profile's turn budget for this run, when set. |
<!-- /generated -->

`resumeSessionId` is **deprecated** — the former name of `runtimeContextId`,
sent alongside it for one release. `toolsMode` is `merge` or `overwrite` —
see [Composing the allowlist](#composing-the-allowlist).

| ID | Requirement |
|---|---|
| `API-1` | A runtime MUST long-poll this endpoint while able to accept work. `204` is not an error. |
| `API-2` | Exactly one of `promptFile`/`promptText` is set. |
| `API-3` | Where continuity is possible for the deployment, context named by `runtimeContextId` MUST already be restored before this call answers. |

## POST /work/done

Reports a run's outcome and closes out the inflight unit.

`POST $CONTROL_URL/work/done`

| Status | Body |
|---|---|
| `200` | `{"ok": true}` |
| `400` | `{"error": "<message>"}` — malformed JSON, or a [bound](#bounds) exceeded (message names the field and the bound broken). Whole report refused, never trimmed. |
| `404` | `{"error": "<message>"}` — `convo` names no `Conversation`. |
| `500` | `{"error": "<message>"}` — transient, not a runtime fault. Retrying the same report is not refused. |

**Example — request:**

```json
{"convo": "convo-abc12", "runId": "r-7f2a", "status": "succeeded",
 "runtimeContextId": "ctx-91c3", "continuity": "continued",
 "result": "Opened PR #482.", "brief": "Drafted the v2 release notes."}
```

### Completion report

<!-- generated: struct-table file=platform/manager/internal/httpapi/server.go struct=workDone -->
| Field | Type | Required | Description |
|---|---|---|---|
| `convo` | `string` | **yes** | The Conversation this report is for. |
| `runId` | `string` | **yes** | The unit's own id, round-tripped. |
| `status` | `string` | **yes** | "succeeded" or "failed". |
| `exitCode` | `integer` |  | The process exit code, when the runtime has one to report. |
| `runtimeContextId` | `string` |  | The handle for the context this run leaves behind — the one to continue next time. |
| `sessionId` | `string` |  | The retired spelling, accepted for one release so an older runtime image keeps working. |
| `continuity` | `string` |  | Continuity says what happened to the context this run was asked to continue: "continued", "new", or "unavailable" (a new one was needed because the named context could not be reached). |
| `continuityReason` | `string` |  | Free text from the runtime explaining why a context could not be continued. |
| `result` | `string` |  | The run's answer. |
| `brief` | `string` |  | What the runtime extracted as this conversation's one- or two-sentence description (design D-I) — a contract field, never a parsed section of `result`. |
| `turns` | `[]Turn` |  | One entry per model call this run made, bounded in count and field size (workreport.go). |
| `toolCalls` | `[]ToolCall` |  | One entry per tool call this run made, bounded in count and field size (workreport.go). |
<!-- /generated -->

`sessionId` is **deprecated** — the former spelling of `runtimeContextId`,
accepted for one release.

#### Turns

<!-- generated: struct-table file=platform/manager/internal/httpapi/workreport.go struct=Turn -->
| Field | Type | Required | Description |
|---|---|---|---|
| `model` | `string` |  | The model the call went to, as the runtime names it. |
| `tokensIn` | `integer` |  | Every input token the call consumed, cache reads included, as far as the vendor reports them. |
| `tokensOut` | `integer` |  | The output tokens the call produced. |
| `cacheReadTokens` | `integer` |  | The part of `tokensIn` served from the provider's prompt cache. |
| `cacheCreationTokens` | `integer` |  | The part of `tokensIn` written INTO the provider's prompt cache by this call — where a cache a later turn reads was made. |
| `stopReason` | `string` |  | Why the model stopped: end of turn, tool use, max tokens. |
<!-- /generated -->

#### Tool calls

<!-- generated: struct-table file=platform/manager/internal/httpapi/workreport.go struct=ToolCall -->
| Field | Type | Required | Description |
|---|---|---|---|
| `tool` | `string` | **yes** | The tool's name in the agent-ops vocabulary: `Bash`, `Read`, or `mcp__<server>__<tool>`. |
| `server` | `string` |  | The MCP server the call reached. |
| `durationMs` | `integer` |  | The wall time from the call to its result. |
| `resultBytes` | `integer` |  | The size of the result the tool returned. |
<!-- /generated -->

| ID | Requirement |
|---|---|
| `API-4` | `turns[]`/`toolCalls[]` MUST be trimmed to the [bounds](#bounds) before posting. |
| `API-5` | `runtimeContextId`/`brief` are LATEST-WINS. Absent leaves the stored value untouched. |
| `API-6` | `turns[]`/`toolCalls[]` become one activity hop each (`model.call`/`tool.call`), never written to the `Conversation` — see [the activity contract](../contracts.md#the-activity-contract). |
| `API-7` | A fact the runtime cannot determine MUST be omitted, never reported as zero. |
| `API-8` | A tool's input or output MUST NOT be reported. |
| `API-9` | Neither array is reported when the run made neither a model call nor a tool call. |

### Delivering the report

A definitive rejection means the manager refused the report's content, and
resending the same bytes cannot change the verdict. Retrying it only blocks
the poll loop.

| Response | Runtime behaviour |
|---|---|
| transport error, timeout, `408`, `429`, any `5xx` | bounded retry, then log the loss and resume polling |
| any other `4xx` | NO retry — log the status and body, resume polling |
| `2xx` | done |

| ID | Requirement |
|---|---|
| `API-10` | A definitive rejection — any `4xx` other than `408`/`429` — MUST NOT be retried. |
| `API-11` | A transport error, timeout, `408`, `429`, or any `5xx` MUST get a bounded retry before the runtime gives up and resumes polling. |
| `API-12` | Every undelivered report MUST leave a log line carrying the response status and body. A report MUST NEVER be dropped silently. |

## Lifecycle

| State | Entered | Left by |
|---|---|---|
| Starting | the manager creates the pod | reaching `Running`, or the start deadline (`MGR-3`) |
| Polling | `Running`, no inflight run | `GET /work` returns `200`, or the idle TTL elapses |
| Working | `GET /work` returned a unit | `POST /work/done` is accepted |
| Idle | `POST /work/done` is accepted, nothing queued | the next `GET /work` returns a unit, or `RUNTIME_IDLE_TTL_M` elapses |
| Exited | `exit 0` from Idle, or `/exit` deletes the pod | — |

| ID | Requirement |
|---|---|
| `LC-1` | One inflight unit per conversation. A runtime polling while Working MUST get nothing back. |
| `LC-2` | A runtime MUST exit on exactly one LIFECYCLE condition: no work for `RUNTIME_IDLE_TTL_M` minutes. |
| `LC-3` | `/exit` deletes the pod. Refused while a run is in progress. |
| `LC-4` | Missing or unusable required configuration, detected before the first `GET /work` poll, MUST end the process with a nonzero exit naming what is missing. That is a CRASH the kubelet surfaces and restarts, not a lifecycle exit, and it does not breach `LC-2`'s one-exit rule. |
| `LC-5` | Honouring the pod's own termination signal is likewise not a lifecycle exit. |

## Runtime's obligations

| ID | Requirement |
|---|---|
| `RT-1` | Clone `REPO_URL`/`REPO_REF` with the environment's git credentials, when `REPO_URL` is not empty. |
| `RT-2` | Execute the returned unit, streaming progress to **stdout**, never a transport of its own. |
| `RT-3` | Compose the tool allowlist as `toolsMode` directs — see [Composing the allowlist](#composing-the-allowlist). |

### Composing the allowlist

| `toolsMode` | Result |
|---|---|
| `merge` | the union, the agent's own tools keeping position |
| `overwrite` | `allowedTools` alone |

| ID | Requirement |
|---|---|
| `RT-3.1` | A runtime that cannot see a repository MUST use `allowedTools` as-is — what `merge` degrades to. |
| `RT-3.2` | An empty allowlist MUST stay empty. No tool substitutes for one nobody declared. |
| `RT-3.3` | An absent or unreadable agent definition MUST contribute nothing, whatever a vendor default would be. |
| `RT-3.4` | Definition path is runtime-specific: `runtimes/claude`/`runtimes/ollama` read `.claude/agents/<agent>.md`, `runtimes/copilot` reads `.github/agents/<agent>.agent.md`. |
| `RT-3.5` | A per-invocation permission DECISION satisfies `RT-3` as well as passing the composed list as a flag, provided the decision point is reached for every governed call and denies by default. A vendor flag that shadows the decision for some calls (auto-approving before the decision runs) does not satisfy this for those calls. |

### Optional capabilities

| Capability | A runtime may | Trigger |
|---|---|---|
| Context continuity | mint and report `runtimeContextId`, resume one it is given | continuity possible for the deployment |
| `contextSync` | keep live context on pod-local storage, delegate checkpoints to the sidecar | `AgentRuntime.spec.contextSync` |
| aops MCP server | reach coordination verbs behind `AOPS_MCP_TOKEN` | the conversation's wiring binds the `aops` MCP toolset |

A runtime implementing none of these stays fully conformant — see
[Compatibility](#compatibility).

### Security posture

| ID | Requirement |
|---|---|
| `SEC-1` | The manager MUST NOT read a secret. Every credential compiles to `valueFrom`/`envFrom`. |
| `SEC-2` | `AOPS_MCP_TOKEN` MUST be derived, never stored. |
| `SEC-3` | The operator MUST grant the runtime no Kubernetes permissions of its own. `spec.serviceAccountName` is a reference only. |

## Manager's provisions

| ID | Requirement |
|---|---|
| `MGR-1` | One pod per conversation. The manager creates it and owns its lifecycle. |
| `MGR-2` | Context MUST be checkpointed before `POST /work/done` reaches the manager — a handle MUST NOT be recorded before its context is persisted. |
| `MGR-3` | A pod that never reaches `Running` MUST be reaped at a deadline, freeing its admission slot. |
| `MGR-4` | The manager MUST supply every variable in [Environment](#environment), and the profile's git credentials. |

## Environment

| Kind | Variable | Meaning |
|---|---|---|
| always | `ROLE` | This pod's role in the manager's process topology. |
| always | `CONVO_ID` | The `Conversation` this pod serves. |
| always | `CONTROL_URL` | Where to long-poll and report — the manager, or a `contextSync` sidecar on loopback. |
| always | `REPO_URL` | The profile's repository URL. May be empty. |
| always | `REPO_REF` | The ref to check out. May be empty alongside an empty `REPO_URL`. |
| always | `RUNTIME_IDLE_TTL_M` | Minutes without work before exit (LC-2). |
| always | `HOME` | The live context mount — see [Filesystem](#filesystem). |
| always | `MCP_CONFIG` | Path to the composed MCP configuration. |
| always | `POD_NAME` | This pod's own name. |
| conditional | `GIT_AUTH_TYPE` + `GIT_SSH_KEY` | SSH repository auth. |
| conditional | `GIT_AUTH_TYPE` + `GIT_TOKEN` | HTTPS repository auth. |
| conditional | `AOPS_MCP_TOKEN` | Set when the conversation's wiring binds the `aops` MCP toolset. |

| ID | Requirement |
|---|---|
| `MGR-5` | An empty `REPO_URL` MUST NOT be read as an error or as "clone the default" — a runtime skips cloning. |

## Filesystem

| Path | Is |
|---|---|
| `/data/workspace` | The repository checkout. Never moves. |
| `$HOME` | The live context mount. |
| the path `MCP_CONFIG` names | The composed MCP configuration (`mcp.json`), read-only. |

## Enums

| Enum | Values | Reading |
|---|---|---|
| `status` | `succeeded`, `failed` | Any other value is read as `failed`. |
| `continuity` | `continued`, `new`, `unavailable` | Absent means no claim, not a lost context. |
| `toolsMode` | `merge`, `overwrite` | Absent or unrecognized reads as `merge`. |

## Bounds

| Bound | Value |
|---|---|
| entries in `turns[]` | ≤ 200 |
| entries in `toolCalls[]` | ≤ 200 |
| any string field of a turn or a tool call | ≤ 200 bytes |
| any integer field of a turn or a tool call | ≥ 0 |
| `result` | truncated to 2000 bytes, never refused |
| `brief` | truncated to 512 bytes, never refused |

## Compatibility

| ID | Requirement |
|---|---|
| `COMPAT-1` | A runtime MUST ignore work-unit fields it does not know. |
| `COMPAT-2` | The manager MUST ignore report fields it does not know. |
| `COMPAT-3` | A published section anchor on this page is a stable name. Renaming one is MAJOR. |

| Version part | Moves when |
|---|---|
| MAJOR | a runtime conformant with the previous version stops conforming |
| MINOR | a new optional capability or field is added |
| PATCH | a clarification that changes no obligation |

`GET /work` reserves a future `?version=` query parameter. None is
implemented — an absent parameter means "the version this document
describes," and adding it would be MINOR, not MAJOR.

## Conformance

A runtime implementing every requirement on this page except
[Optional capabilities](#optional-capabilities) is fully conformant — no
optional capability is required, see [Compatibility](#compatibility). A
conformance claim is this checklist walked item by item, each with its own
verdict, never an impression of the whole:

1. Long-polls `GET /work` while able to accept work. `204` is not an error (`API-1`).
2. A work unit carries exactly one of `promptFile`/`promptText` (`API-2`).
3. Where continuity is possible, context named by `runtimeContextId` is restored before `GET /work` answers (`API-3`).
4. `turns[]`/`toolCalls[]` are trimmed to the [bounds](#bounds) before posting (`API-4`).
5. `runtimeContextId`/`brief` are LATEST-WINS — absent leaves the stored value untouched (`API-5`).
6. `turns[]`/`toolCalls[]` become one activity hop each, never written to the `Conversation` (`API-6`).
7. A fact the runtime cannot determine is omitted, never reported as zero (`API-7`).
8. A tool's input or output is never reported (`API-8`).
9. Neither array is reported when the run made neither a model call nor a tool call (`API-9`).
10. A definitive `4xx` (not `408`/`429`) from `POST /work/done` is never retried (`API-10`).
11. A transport error, timeout, `408`, `429`, or any `5xx` gets a bounded retry before giving up (`API-11`).
12. Every undelivered report leaves a log line carrying the response status and body — never dropped silently (`API-12`).
13. One inflight unit per conversation — polling while Working gets nothing back (`LC-1`).
14. The runtime exits on exactly one LIFECYCLE condition: no work for `RUNTIME_IDLE_TTL_M` minutes (`LC-2`).
15. `/exit` deletes the pod, refused while a run is in progress (`LC-3`).
16. Missing or unusable required configuration before the first poll ends the process with a nonzero exit naming the gap — a crash, not a lifecycle exit (`LC-4`).
17. Honouring the pod's own termination signal is likewise not a lifecycle exit (`LC-5`).
18. Clones `REPO_URL`/`REPO_REF` with the environment's git credentials, when `REPO_URL` is not empty (`RT-1`).
19. Executes the returned unit, streaming progress to stdout, never a transport of its own (`RT-2`).
20. Composes the tool allowlist as `toolsMode` directs (`RT-3`).
21. A runtime that cannot see a repository uses `allowedTools` as-is (`RT-3.1`).
22. An empty allowlist stays empty — no tool substitutes for one nobody declared (`RT-3.2`).
23. An absent or unreadable agent definition contributes nothing (`RT-3.3`).
24. The definition path is runtime-specific, per vendor (`RT-3.4`).
25. A per-invocation decision satisfies `RT-3` as well as a flag, reached for every governed call and denying by default (`RT-3.5`).
26. The manager never reads a secret — every credential compiles to `valueFrom`/`envFrom` (`SEC-1`).
27. `AOPS_MCP_TOKEN` is derived, never stored (`SEC-2`).
28. The operator grants the runtime no Kubernetes permissions of its own (`SEC-3`).
29. One pod per conversation, created and owned by the manager (`MGR-1`).
30. Context is checkpointed before `POST /work/done` reaches the manager (`MGR-2`).
31. A pod that never reaches `Running` is reaped at a deadline (`MGR-3`).
32. The manager supplies every variable in [Environment](#environment), and the profile's git credentials (`MGR-4`).
33. An empty `REPO_URL` is never read as an error or as "clone the default" (`MGR-5`).
34. A runtime ignores work-unit fields it does not know (`COMPAT-1`).
35. The manager ignores report fields it does not know (`COMPAT-2`).
36. A published section anchor is a stable name — renaming one is MAJOR (`COMPAT-3`).

## Changelog

| Version | Date | Changed |
|---|---|---|
| 2.0.1 | 2026-10-24 | Clarified `RT-3` (`RT-3.5`): a per-invocation permission decision satisfies the allowlist obligation as well as passing it as a flag, provided every governed call reaches the decision point and it denies by default. No obligation changes — this names an enforcement mechanism an implementation already had to pick. |
| 2.0.0 | 2026-10-22 | Added the startup-configuration-failure crash ruling (`LC-4`, `LC-5`) — missing or unusable required config before the first poll is a nonzero crash, not a lifecycle exit. Added the `/work/done` definitive-rejection table (`API-10`, `API-11`, `API-12`) — a definitive `4xx` is never retried, a transient failure keeps its bounded retry, and every undelivered report is logged. The work-unit and completion-report tables are now GENERATED from the manager's own Go structs, and [Conformance](#conformance) is now a numbered checklist reaching every requirement on this page. BREAKING: `API-10`/`11`/`12` constrain a `/work/done` 4xx response that 1.0.0 left unconstrained — a runtime that retried a definitive rejection forever was conformant with 1.0.0 and stops conforming here, per this page's own Compatibility rule. |
| 1.0.0 | 2026-10-17 | Initial release. |
