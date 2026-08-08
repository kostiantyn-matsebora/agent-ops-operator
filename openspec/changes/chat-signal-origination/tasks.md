## 0. Settle the blocking decision

- [ ] 0.1 Decide the router's deployment vehicle (design Open Questions): a
      `SignalAdapter` plus an unclaimed credential-carrying `SignalSource`,
      collapsing the router into `signal-telegram`, or letting an adapter CR
      declare a credential source. This decides whether an invariant moves —
      settle before writing code

## 1. ChannelAdapter port parity

- [ ] 1.1 Add `spec.port` to `ChannelAdapter` mirroring `SignalAdapter`;
      regenerate deepcopy + CRDs
- [ ] 1.2 Extend `adapterworkload.go` so the ChannelAdapter reconciler owns the
      Service `agentops-channel-<name>` and injects `LISTEN_ADDR` when `port` is
      set — reuse the SignalAdapter path, do not fork it
- [ ] 1.3 envtest: a port-declaring ChannelAdapter gets a reconciler-owned
      Service; one without stays serviceless

## 2. telegram-router module

- [ ] 2.1 New dependency-free module `telegram-router/` mirroring
      `signal-cron/`: `main.go`, `manager.go`, `telegram.go`, Dockerfile
- [ ] 2.2 Move `GetUpdates`, `pollToken`, and `tokenGroup` out of
      `channel-telegram/` into it — one loop per distinct bot token,
      single-instance
- [ ] 2.3 Classify locally on `IsTopicMessage`: absent → forward to the signal
      adapter, present → forward to the channel adapter. No manager reads, no
      channel config
- [ ] 2.4 Offset handling: obtain at startup and report each confirmed offset
      downstream for persistence via the existing adapter state API; no
      ServiceAccount token, no RBAC
- [ ] 2.5 Confirm the router forwards raw updates and performs no chat-id or
      approver filtering

## 3. signal-telegram module

- [ ] 3.1 New dependency-free module `signal-telegram/` mirroring
      `signal-vmalertmanager/` (HTTP-receiving, `spec.port`), plus Dockerfile
- [ ] 3.2 Receive forwarded general-surface updates; apply chat-id matching and
      approver filtering against `GET /signal/sources?adapter=`
- [ ] 3.3 Normalize to `{fingerprint: tg-<update_id>, kind: chat, payload, labels:
      {agentops.dev/channel, agentops.dev/sender}}` and post `/signal/inbound`

## 4. channel-telegram becomes receive-only

- [ ] 4.1 Remove `pollManager`, `pollToken`, `tokenGroup`, and `GetUpdates`;
      keep the ops loop, `Send`, `CreateTopic`, chat-id matching, approver
      filtering, and the `/channel/inbound` post
- [ ] 4.2 Turn `dispatch` into an HTTP handler for forwarded topic updates;
      add the listen port
- [ ] 4.3 Accept and persist offset reports from the router through the existing
      `PUT /channel/state/{channel}/{key}`

## 5. Manager: origination moves to the signal path

- [ ] 5.1 Make `threadId` required on `POST /channel/inbound`; reject a missing
      one with a message naming the signal path
- [ ] 5.2 Delete `adoptThread`, the bare-text branch, the `/<profile>` branch,
      and `defaultPipeline` from `internal/chat/router.go`; keep `convByThread`,
      `appendInput`, `FanOutSend`, `relayToSiblings` untouched
- [ ] 5.3 Delete `PipelineForChannel` from `internal/chat/pipelines.go` and every
      caller
- [ ] 5.4 Add the `chat` kind to the signal core: task lane, no signature
      grouping by default, `cooldownHours: 0` default for chat sources
- [ ] 5.5 Reject a `chat` signal missing `agentops.dev/channel` at
      `/signal/inbound`, naming the missing label
- [ ] 5.6 Move command handling (`/agents`, `/help`, unknown agent, usage error)
      into the chat signal path, emitting `send` ops to the originating channel
      without creating a Conversation; `/<profile> <task>` still creates one
- [ ] 5.7 Route the unclaimed-source drop reason back to the originating surface
      so an unwired chat source is visible to the user, not just in conditions

## 6. Chart

- [ ] 6.1 Render all three adapter CRs under the existing
      `telegramAdapter.enabled` (default false) — `ChannelAdapter telegram`,
      the signal adapter, and the router's vehicle per task 0.1
- [ ] 6.2 Add `signalTelegram`/`telegramRouter` image values; no Deployment and
      no Service templates — the reconcilers own both
- [ ] 6.3 Sample Pipeline in `config/samples/samples.yaml` pairing the chat
      `SignalSource` with its `Channel` under one Pipeline
- [ ] 6.4 Bump chart version; verify `helm template` with the flag on and off

## 7. Tests

- [ ] 7.1 envtest: general-surface chat signal creates a conversation with the
      claiming Pipeline's profile and channel set, and bumps `receivedTotal`
- [ ] 7.2 envtest: unclaimed chat source drops, reports `Wired=False`, and the
      reason reaches the originating surface
- [ ] 7.3 envtest: two Ready Pipelines sharing a channel — resolution is by
      claim, unaffected by creation order
- [ ] 7.4 envtest: `/channel/inbound` without `threadId` is rejected; an unknown
      thread is not adopted
- [ ] 7.5 envtest: reply in a known thread still appends, acks, and relays to
      siblings — unchanged behavior, pinned
- [ ] 7.6 envtest: `/agents` emits a send op and creates no Conversation;
      `/<profile> <task>` creates one
- [ ] 7.7 envtest: repeated identical chat text creates two conversations
      (cooldown off for chat)
- [ ] 7.8 envtest: `chat` signal without `agentops.dev/channel` is rejected
- [ ] 7.9 Unit: router classification by `IsTopicMessage`; adapters' chat-id and
      approver filtering (same test for both, to catch drift)
- [ ] 7.10 Full suite: `go build ./... && go vet ./...` in all modules,
      `KUBEBUILDER_ASSETS=… go test ./...`

## 8. Docs and migration

- [ ] 8.1 Write the migration: add the chat `SignalSource` and claim it, scale
      the old adapter to zero, confirm no `getUpdates` consumer remains, enable
      the new stack, carry the offset annotation across
- [ ] 8.2 Update `CLAUDE.md`: origination is signal-only, the three-component
      telegram topology, the module map, and the single-consumer rule now
      naming the router
- [ ] 8.3 Update README / `docs/` — the one workflow (signal originates, channel
      carries) and the telegram stack diagram
- [ ] 8.4 Reconcile with `capabilities-are-wiring`: `/<profile>` commands now
      originate from a claimed chat source and so have a routing pipeline —
      check whether that removes one of its reasons for capability-only
      Pipelines before either lands
