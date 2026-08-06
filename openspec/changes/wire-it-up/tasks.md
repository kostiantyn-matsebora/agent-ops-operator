# Tasks: wire-it-up

## 1. API types

- [x] 1.1 Add `pipeline_types.go` (`signalSourceRefs`, `channelRefs`, `profileRef`; status conditions `Ready`/`SourceConflict`; printcolumns) and restructure `conversation_types.go`: `spec.channelRefs []ObjectRef`, `status.threads []ThreadBinding{channel, threadId}` (**BREAKING**; update printcolumn)
- [x] 1.2 Fix all compile-time consumers of `ChannelRef`/`ThreadID` (router, ops, conversation reconciler, httpapi dispatch gate + `/task` + signal routing, dispatch `WorkUnit`, integration tests)
- [x] 1.3 Regenerate deepcopy + CRDs; add a Pipeline sample to `config/samples/` wiring the existing sample sources + channels + profile

## 2. Pipeline resolution + reconciler

- [x] 2.1 Resolution helpers (cached-client reads): source → claiming Ready Pipeline (oldest wins), channel → oldest Ready Pipeline referencing it; used by signal routing, router inbound, and `/task`
- [x] 2.2 `PipelineReconciler`: ref validation → `Ready`; one-pipeline-per-source guard → `SourceConflict` on the newer claimant; watches on referenced kinds mapped back to pipelines
- [x] 2.3 Envtests: Ready/dangling-ref, source-conflict (older wins), channel shared by two pipelines OK

## 3. Multi-channel conversation core

- [x] 3.1 Reconciler + OpQueue: per-channel ensure-topic (`topic:<conv>:<channel>` ids), completion writes `{channel, threadId}` binding; dispatch gate = at least one binding
- [x] 3.2 Router: create/adopt bound to resolved channel set; `convByThread(channel, threadId)` pairwise; acks fan out to all bound channels; attributed relay (`💬 <channel>[/<sender>]: <text>`) to sibling channels on inbound
- [x] 3.3 Dispatch: multi-channel forces `result` delivery; `WorkUnit.ThreadID` = the single binding for single-channel agent-mode conversations, empty otherwise
- [x] 3.4 `/work/done` reply fan-out for multi-channel conversations (result or failure notice as `send` per bound channel); `/task` gains optional `pipeline` (binds its channels); signal routing binds pipeline-first
- [x] 3.5 Envtests: two-channel ensure-topic + binding landing, single-broken-channel dispatch, inbound from either channel into one conversation, ack fan-out + relay ops enqueued, reply fan-out on done, forced-result dispatch, single-channel behavior unchanged (existing tests)

## 4. Chart, verification, docs, live

- [x] 4.1 Chart: `pipelines` CRD (regen) + manager RBAC `pipelines`(+status); chart 1.3.0, manager 0.6.0 (build + push); helm lint/template
- [x] 4.2 `go build/vet` all modules; full envtest suite green
- [x] 4.3 Live on gitops: upgrade (no Pipeline = behavior unchanged; migrate any active conversation bindings), then apply a Pipeline binding `alertmanager` + `home-ops` and verify a stub-profile signal fans out to the telegram thread via manager `send` ops; clean up test CRs
- [x] 4.4 README (Pipeline concept + example, mirroring semantics, delivery note) + CLAUDE.md (terminology, map, invariants: pipeline-first resolution, no-relay-loop rule); rebase note in the pending `add-web-chat-channel` change (multi-channel transcript/no-echo rule)
