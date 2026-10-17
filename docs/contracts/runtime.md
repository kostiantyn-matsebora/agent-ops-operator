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

**Version 1.0.0**

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

| Field | Type | Required | Description |
|---|---|---|---|
| `runId` | string | yes | Unique id for this unit, echoed back in the completion report. |
| `convo` | string | yes | The `Conversation` this unit belongs to. |
| `threadId` | string | no | The bound channel's thread id. Set only for a single-channel conversation. |
| `runtimeContextId` | string | no | The runtime's own context handle from the last run. Empty means start fresh. |
| `resumeSessionId` | string | no | **Deprecated.** Former name of `runtimeContextId`, sent for one release. |
| `promptFile` | string | no | Repo-relative path to the profile's own prompt file. |
| `promptText` | string | no | The fully rendered prompt, from a built-in template. |
| `promptVars` | map of string to string | no | Template variables for `promptFile`. |
| `agent` | string | no | The agent definition's name, resolved from the checked-out repository. |
| `systemPrompt` | string | no | Inline role text, appended to the agent's own system prompt. |
| `allowedTools` | string | no | The wiring's half of the tool allowlist, comma-separated. |
| `toolsMode` | string | no | `merge` or `overwrite` — see [Composing the allowlist](#composing-the-allowlist). |
| `maxTurns` | integer | no | The profile's turn budget for this run. |

| ID | Requirement |
|---|---|
| API-1 | A runtime MUST long-poll this endpoint while able to accept work. `204` is not an error. |
| API-2 | Exactly one of `promptFile`/`promptText` is set. |
| API-3 | Where continuity is possible for the deployment, context named by `runtimeContextId` MUST already be restored before this call answers. |

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

| Field | Type | Required | Description |
|---|---|---|---|
| `convo` | string | yes | The `Conversation` this report is for. |
| `runId` | string | yes | The unit's own `runId`, round-tripped. |
| `status` | string | yes | `succeeded` or `failed` — see [Enums](#enums). |
| `exitCode` | integer | no | The process exit code, when there is one to report. |
| `runtimeContextId` | string | no | The context handle this run leaves behind, to continue next time. |
| `sessionId` | string | no | **Deprecated.** Former spelling of `runtimeContextId`, accepted for one release. |
| `continuity` | string | no | `continued`, `new`, or `unavailable` — see [Enums](#enums). |
| `continuityReason` | string | no | Free text explaining why a context could not be continued. |
| `result` | string | no | The run's answer. |
| `brief` | string | no | A one- or two-sentence summary of the conversation. |
| `turns[]` | array of Turn | no | One entry per model call — see [Turns](#turns). |
| `toolCalls[]` | array of ToolCall | no | One entry per tool call — see [Tool calls](#tool-calls). |

#### Turns

| Field | Type | Required | Description |
|---|---|---|---|
| `model` | string | no | The model the call went to, as the runtime names it. |
| `tokensIn` | integer | no | Input tokens consumed, cache reads included. |
| `tokensOut` | integer | no | Output tokens produced. |
| `cacheReadTokens` | integer | no | Part of `tokensIn` served from the provider's prompt cache. |
| `cacheCreationTokens` | integer | no | Part of `tokensIn` written into the provider's prompt cache. |
| `stopReason` | string | no | Why the model stopped: end of turn, tool use, max tokens. |

#### Tool calls

| Field | Type | Required | Description |
|---|---|---|---|
| `tool` | string | yes | `Bash`, `Read`, or `mcp__<server>__<tool>`. |
| `server` | string | no | The MCP server reached. Empty for a built-in tool. |
| `durationMs` | integer | no | Wall time from the call to its result. |
| `resultBytes` | integer | no | Size of what the tool returned. |

| ID | Requirement |
|---|---|
| API-4 | `turns[]`/`toolCalls[]` MUST be trimmed to the [bounds](#bounds) before posting. |
| API-5 | `runtimeContextId`/`brief` are LATEST-WINS. Absent leaves the stored value untouched. |
| API-6 | `turns[]`/`toolCalls[]` become one activity hop each (`model.call`/`tool.call`), never written to the `Conversation` — see [the activity contract](../contracts.md#the-activity-contract). |
| API-7 | A fact the runtime cannot determine MUST be omitted, never reported as zero. |
| API-8 | A tool's input or output MUST NOT be reported. |
| API-9 | Neither array is reported when the run made neither a model call nor a tool call. |

## Lifecycle

| State | Entered | Left by |
|---|---|---|
| Starting | the manager creates the pod | reaching `Running`, or the start deadline (MGR-3) |
| Polling | `Running`, no inflight run | `GET /work` returns `200`, or the idle TTL elapses |
| Working | `GET /work` returned a unit | `POST /work/done` is accepted |
| Idle | `POST /work/done` is accepted, nothing queued | the next `GET /work` returns a unit, or `RUNTIME_IDLE_TTL_M` elapses |
| Exited | `exit 0` from Idle, or `/exit` deletes the pod | — |

| ID | Requirement |
|---|---|
| LC-1 | One inflight unit per conversation. A runtime polling while Working MUST get nothing back. |
| LC-2 | A runtime MUST exit on exactly one condition: no work for `RUNTIME_IDLE_TTL_M` minutes. |
| LC-3 | `/exit` deletes the pod. Refused while a run is in progress. |

## Runtime's obligations

| ID | Requirement |
|---|---|
| RT-1 | Clone `REPO_URL`/`REPO_REF` with the environment's git credentials, when `REPO_URL` is not empty. |
| RT-2 | Execute the returned unit, streaming progress to **stdout**, never a transport of its own. |
| RT-3 | Compose the tool allowlist as `toolsMode` directs — see [Composing the allowlist](#composing-the-allowlist). |

### Composing the allowlist

| `toolsMode` | Result |
|---|---|
| `merge` | the union, the agent's own tools keeping position |
| `overwrite` | `allowedTools` alone |

| ID | Requirement |
|---|---|
| RT-3.1 | A runtime that cannot see a repository MUST use `allowedTools` as-is — what `merge` degrades to. |
| RT-3.2 | An empty allowlist MUST stay empty. No tool substitutes for one nobody declared. |
| RT-3.3 | An absent or unreadable agent definition MUST contribute nothing, whatever a vendor default would be. |
| RT-3.4 | Definition path is runtime-specific: `runtimes/claude`/`runtimes/ollama` read `.claude/agents/<agent>.md`, `runtimes/copilot` reads `.github/agents/<agent>.agent.md`. |

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
| SEC-1 | The manager MUST NOT read a secret. Every credential compiles to `valueFrom`/`envFrom`. |
| SEC-2 | `AOPS_MCP_TOKEN` MUST be derived, never stored. |
| SEC-3 | The operator MUST grant the runtime no Kubernetes permissions of its own. `spec.serviceAccountName` is a reference only. |

## Manager's provisions

| ID | Requirement |
|---|---|
| MGR-1 | One pod per conversation. The manager creates it and owns its lifecycle. |
| MGR-2 | Context MUST be checkpointed before `POST /work/done` reaches the manager — a handle MUST NOT be recorded before its context is persisted. |
| MGR-3 | A pod that never reaches `Running` MUST be reaped at a deadline, freeing its admission slot. |
| MGR-4 | The manager MUST supply every variable in [Environment](#environment), and the profile's git credentials. |

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
| MGR-5 | An empty `REPO_URL` MUST NOT be read as an error or as "clone the default" — a runtime skips cloning. |

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
| COMPAT-1 | A runtime MUST ignore work-unit fields it does not know. |
| COMPAT-2 | The manager MUST ignore report fields it does not know. |
| COMPAT-3 | A published section anchor on this page is a stable name. Renaming one is MAJOR. |

| Version part | Moves when |
|---|---|
| MAJOR | a runtime conformant with the previous version stops conforming |
| MINOR | a new optional capability or field is added |
| PATCH | a clarification that changes no obligation |

`GET /work` reserves a future `?version=` query parameter. None is
implemented — an absent parameter means "the version this document
describes," and adding it would be MINOR, not MAJOR.

## Conformance

| Contract | Satisfies |
|---|---|
| Minimal — MUST, to interoperate at all | API-1–API-9, LC-1–LC-3, RT-1–RT-3, RT-3.1–RT-3.4, MGR-1–MGR-5, SEC-1–SEC-3, COMPAT-1–COMPAT-3 |
| Optional — MAY, additive | [Optional capabilities](#optional-capabilities) |

A runtime implementing the minimal contract alone is fully conformant —
every optional capability may be skipped with no loss of conformance, see
[Compatibility](#compatibility).

## Changelog

| Version | Date | Changed |
|---|---|---|
| 1.0.0 | 2026-10-17 | Initial release. |
