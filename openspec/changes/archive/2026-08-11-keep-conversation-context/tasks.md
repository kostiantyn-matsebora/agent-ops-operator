## 1. Rename the handle out of the runtime's vocabulary

- [x] 1.1 Rename `Conversation.status.sessionId` to `runtimeContextId` in `api/v1alpha1/conversation_types.go`; regenerate deepcopy and CRDs
- [x] 1.2 READ BOTH fields for one release — prefer the new, adopt the old when only it is present, write only the new — so no in-flight conversation loses its handle on upgrade
- [x] 1.3 Rename the work unit's `resumeSessionId` to `runtimeContextId` and send BOTH for the transition, so a runtime image upgrades independently of the manager
- [x] 1.4 Update every reader: `internal/dispatch`, `internal/httpapi/signals.go`, `console/conversations.go` and the SPA
- [x] 1.5 Tests: a conversation carrying only the old field is adopted and rewritten under the new one, without restarting its context

## 2. The declared prerequisite

- [x] 2.1 Add `AgentRuntime.spec.contextStorage` (`volume` | `external` | `none`, default `volume`) in `api/v1alpha1/agentruntime_types.go`; regenerate deepcopy and CRDs
- [x] 2.2 In dispatch, withhold the handle when a runtime needs a home volume the deployment does not provide, and record on the conversation that it cannot be continued
- [x] 2.3 Run such conversations FRESH rather than failing — never-promised is a configuration, promised-and-lost is a loss
- [x] 2.4 `NOTES.txt`: state that conversations cannot be continued when persistence is off and the runtime keeps context on its volume; do NOT fail the render, since one-shot-per-message is a legitimate install
- [x] 2.5 Tests: no handle is issued without the prerequisite; such a conversation answers fresh and says so; a runtime declaring external storage is unaffected

## 3. Stop the permanent loss

- [x] 3.1 Change `internal/httpapi/server.go` to record the reported handle on every completed run, replacing the stored one, instead of the current write-once guard
- [x] 3.2 Record the handle on a FAILED run too, when the run reports one
- [x] 3.3 Tests: a fallback handle replaces the stored one; a successful continuation leaves it unchanged; a failed run's handle is kept; several messages after a loss all continue the SAME post-loss context

## 4. The contract addition

- [x] 4.1 Define the contract in runtime-agnostic terms — opaque handle + continue-or-report obligation — and add the continuity report to `/work/done` (continued / new / new-after-unavailable + optional reason) as optional fields
- [x] 4.2 Treat an absent field as "no claim", preserving today's behaviour for runtimes that do not set it
- [x] 4.3 Report it from `runtime-claude/runtime.js`, including the REASON when its session files are missing, since only it knows where they live
- [x] 4.4 REMOVE the retry-without-resume fallback in `runtime.js`: an unavailable context now fails the run instead of answering without it, with a stated reason and never an empty result
- [x] 4.5 Before declaring a context unavailable, distinguish GONE from NO ANSWER — re-check the session path after a short delay, so a shared-filesystem lag (share-manager restart, stale handle, cross-node visibility) does not end a conversation
- [x] 4.6 Make a failed run's recorded result reach the bound threads — today a failure fans out a bare "run failed" notice and discards the reason, which is exactly the inarticulate failure the fallback existed to avoid
- [x] 4.7 Ensure the triggering input is consumed, not redispatched, so an unavailable context fails once rather than looping
- [x] 4.8 Tests: each of the three reports is parsed; an omitted field records the handle and sets no condition; an unavailable context fails the run, invokes no agent, and reaches the thread with its reason

## 5. Availability tactics before failure

- [x] 5.1 Bounded retry with backoff in `runtime-claude` before declaring a context unavailable — seconds, not minutes, since a person is waiting on the reply
- [x] 5.2 Track unavailability reports across conversations in the manager, windowed
- [x] 5.3 Open a breaker past a threshold: HOLD affected inputs instead of failing them, pause dispatch of continuations, and report the state
- [x] 5.4 Close it when a continuation succeeds, and release the held work with its context intact
- [x] 5.5 Fail a run for unavailable context ONLY when retries are exhausted and the breaker is closed
- [x] 5.6 Tests: many simultaneous reports hold rather than fail; held work resumes and continues its context; an isolated report still fails its run; persistent unavailability stays reported rather than queueing silently

## 6. Making the loss visible

- [x] 6.1 Set a `ContextContinuity` condition on the Conversation when a run reports a fallback, naming when the context restarted
- [x] 6.2 Record the reason the RUNTIME reported, verbatim; the manager infers no cause, since it does not know where a given runtime stores its context
- [x] 6.3 Clear the condition when a later run continues successfully
- [x] 6.4 Keep the runtime's existing reply warning — the person reading the thread is the one most affected and is not reading conditions
- [x] 6.5 Tests: the condition appears on fallback, names the cause, and clears on the next successful continuation

## 7. Storage topologies for continuity

- [x] 7.1 Document both supported topologies in `docs/concepts.md`: shared (RWX, pods anywhere) and single-node (RWO or a node-affine PV + `nodeSelector`), with the local-PV recipe for clusters with no dynamic provisioner
- [x] 7.2 `NOTES.txt`: when persistence uses a single-attach claim and runtime pods are not pinned, state that a second concurrent conversation will fail to attach and name the remedy — do NOT fail the render, since a single-node cluster needs no pinning
- [x] 7.3 Chart render test: the warning appears for single-attach-without-pinning and is absent for RWX or when a selector is set

## 8. Documentation

- [x] 8.1 `docs/contracts.md` — the handle's semantics: what the manager sends, what a run reports, that it is latest-wins, and that its content is opaque
- [x] 8.2 `docs/concepts.md` — what carries an agent's context, the three records (runtime context / thread transcript / run history) and which is authoritative for what
- [x] 8.3 Add `runtimeContextId` to the binding terminology in `CLAUDE.md`: it is agent-ops' name for a RUNTIME's handle, never "session", it is LATEST-WINS, and write-once is what made one loss permanent
- [x] 8.4 `CHANGELOG.md` — the rename, the dual-read window, and the manager-then-runtime ordering

## 9. Verification

- [x] 9.1 `go build ./... && go vet ./...` at the root and in the console module; runtime image builds
- [x] 9.2 Full test run with `KUBEBUILDER_ASSETS` (unit + envtest)
- [x] 9.3 Integration test: a conversation whose stored handle is uncontinuable recovers on the NEXT message rather than restarting on every one — the behaviour this change exists to fix
- [x] 9.4 Integration test: a conversation carrying only the OLD field keeps its context across the upgrade
- [x] 9.5 Live: hold a multi-message conversation, delete the runtime pod between messages, and confirm the agent still refers to earlier turns
- [x] 9.6 Live: delete the session files under `/data/home` mid-conversation, confirm the next message FAILS with a legible reason on the thread and the condition on the object — and that no agent invocation was spent on a contextless answer
