# Tasks: make-channel-type-architecture-extendable

## 1. CRD restructure

- [x] 1.1 Rewrite `ChannelSpec` in `api/v1alpha1/channel_types.go`: required immutable `type` (CEL `self == oldSelf`), `defaultProfileRef`, optional `delivery{mode, agentInstructions}`, opaque `config` (`x-kubernetes-preserve-unknown-fields` / RawExtension); delete `TelegramChannel`
- [x] 1.2 Change `ConversationStatus.ThreadID` to `*string` in `conversation_types.go`; update all compile-time consumers (reconciler, dispatch `WorkUnit`, httpapi, poller code slated for extraction)
- [x] 1.3 Regenerate deepcopy + CRDs into `chart/files/crds/`; update `config/samples/` to the new shape; write the migration recipe (telegram sub-struct → type/config, threadId note) into README/release notes

## 2. Router as the single inbound path (coordinate with add-web-chat-channel task group 2)

- [x] 2.1 Extract the transport-neutral `Router` from `internal/chat/poller.go` (commands, task-conversation creation, reply appending, busy-acks, per-channel thread resolution on string ids) — or rebase onto it if the sibling change landed first
- [x] 2.2 Delete `internal/chat/telegram.go` usage from the manager wiring; keep the Telegram client code compiling for reuse by the adapter (move under `channel-telegram/`)

## 3. Operation pipeline + provider registry

- [x] 3.1 Implement the outbound op queue in the manager: in-memory per-type queues, stable op ids, enqueue from reconciler (`ensure-topic` when threadId unset) and router (acks as `send`), regeneration semantics on restart
- [x] 3.2 Make the reconciler's topic flow asynchronous: enqueue + requeue instead of blocking `EnsureTopic`; store thread id on op completion; surface op errors as Conversation condition/event
- [x] 3.3 Implement `Registry` (type → in-process Provider, empty by default) with remote fallback; delete `ChatFactory`/`TokenReader` closures from `cmd/manager/main.go`; drop the manager's secret `get` RBAC
- [x] 3.4 Unit tests: op dedup ids, regeneration after queue loss, registry resolution, async topic completion

## 4. Adapter HTTP contract

- [x] 4.1 Add `/channel/ops` long-poll (204 on timeout), `/channel/ops/{id}/done`, `/channel/inbound` to `internal/httpapi`, wired to the op queue and Router
- [x] 4.2 Add bearer auth for `/channel/*` from a manager env var (constant-time compare, 401 otherwise); confirm the manager now performs zero Secret API reads
- [x] 4.3 Integration tests (envtest): inbound → conversation creation and threaded reply with acks as ops, ensure-topic round-trip landing a string threadId, at-least-once duplicate tolerance, restart regeneration, 401s

## 5. Telegram reference adapter

- [x] 5.1 Scaffold `channel-telegram/` (own Go module/binary): manager client (ops long-poll, done, inbound), Telegram client moved from `internal/chat/telegram.go`, getUpdates loop with offset persistence and approver filtering, config parsing from `spec.config` with not-ready condition on errors
- [x] 5.2 Adapter RBAC/wiring decisions from design open questions: offset storage (Channel annotation patch vs local state) and inbound batching — decide, implement, document
- [x] 5.3 Dockerfile + image `agentops-channel-telegram`; smoke test against a live bot: /agents, task creation, threaded reply, ack, offset resume after restart
- [x] 5.4 Verify single-consumer: adapter replicas=1/Recreate; migration steps ensure old poller stops before adapter starts

## 6. Dispatch delivery from metadata

- [x] 6.1 Replace hardcoded Telegram delivery wording with `spec.delivery`-driven selection (`result` default incl. chat-less; `agent` injects `agentInstructions`); update `format.md` phrasing if not already channel-neutral
- [x] 6.2 Update dispatch fixtures deliberately; add fixture for `delivery.mode: agent` with custom instructions

## 7. Helm chart

- [x] 7.1 Values + templates: `telegramAdapter.{enabled(false), image, botTokenSecret, resources}` Deployment (replicas 1, Recreate); shared `adapterAuth` token Secret injected into manager and adapters as env
- [x] 7.2 Remove manager bot-token RBAC from `rbac.yaml`; add adapter RBAC per 5.2 decision; bump chart major; `helm template`/`lint` across default, adapter-enabled, and custom-secret value sets

## 8. Verification, docs, sibling change

- [x] 8.1 `go build ./... && go vet ./...` (both modules) and full envtest suite green; grep for remaining int64 threadId or `spec.telegram` references
- [x] 8.2 Live end-to-end: migrate a real install per the documented steps (poller off → CR migration → upgrade → adapter on) and run a full Telegram conversation; verify no double getUpdates at any point
- [x] 8.3 Update README (adapter concept, contract reference, writing-your-own-adapter guide) and CLAUDE.md (terminology, invariants: "manager reads no secrets", poller invariant moves to adapter, map entries)
- [x] 8.4 Run `/opsx:update add-web-chat-channel` to rebase it onto this architecture (`type: web` + config, string thread ids, shared Router/op pipeline) per design D9
