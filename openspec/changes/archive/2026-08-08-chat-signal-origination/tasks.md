## 0. Settle the blocking decision

- [x] 0.1 Router vehicle — **SETTLED, see design D4b**: the router is its own
      component and container, doing classification only; its vehicle is a
      `SignalAdapter` serving a credential-carrying `SignalSource` whose
      `credentialsSecretRef` names the same Secret as the `Channel` (shared
      bot credential — both call Telegram); `signal-telegram` holds no
      credentials. No invariant moves

## 1. ChannelAdapter port parity

- [x] 1.1 Add `spec.port` to `ChannelAdapter` mirroring `SignalAdapter`;
      regenerate deepcopy + CRDs
- [x] 1.2 Extend `adapterworkload.go` so the ChannelAdapter reconciler owns the
      Service and injects `LISTEN_ADDR` when `port` is set — reuse the
      SignalAdapter path, do not fork it. Done by lifting `ensureService` out of
      `signaladapter_controller.go` into shared `ensureAdapterService`, now used
      by both. Service name is the DEPLOYMENT name (`agentops-adapter-<name>`),
      not `agentops-channel-<name>` as this task first said — SignalAdapter's
      Service already shares its Deployment name, and diverging would fork the
      naming the shared helper exists to unify
- [x] 1.3 envtest: a port-declaring ChannelAdapter gets a reconciler-owned
      Service; one without stays serviceless

## 2. telegram-router module

- [x] 2.1 New dependency-free module `telegram-router/`: `main.go`,
      `manager.go`, `telegram.go`, `downstream.go`, Dockerfile
- [x] 2.2 Move `GetUpdates`, `pollToken`, and `tokenGroup` out of
      `channel-telegram/` into it — one loop per distinct bot token,
      single-instance (removal from `channel-telegram` is task 4.1)
- [x] 2.3 Classify locally on `IsTopicMessage`: absent → forward to the signal
      adapter, present → forward to the channel adapter. No per-message manager
      round-trip, no channel config. NOTE: it does make ONE listing read of its
      OWN `SignalSource` (`GET /signal/sources?adapter=`) for its forwarding
      targets and credential prefix — adapter CRs carry no config or env by
      invariant, so a served CR's config is the only path per-deployment
      settings can travel. No Channel config is ever read; spec wording
      reconciled in `telegram-ingest-router`
- [x] 2.4 Offset handling: obtain at startup and report each confirmed offset
      downstream for persistence via the existing adapter state API; no
      ServiceAccount token, no RBAC
- [x] 2.5 Confirm the router forwards raw updates and performs no chat-id or
      approver filtering — pinned by `TestRouteSendsToTheRightTarget`, which
      asserts the forwarded body is byte-identical to the update

## 3. signal-telegram module

- [x] 3.1 New dependency-free module `signal-telegram/` mirroring
      `signal-vmalertmanager/` (HTTP-receiving, `spec.port`), plus Dockerfile.
      Holds NO credentials — it never contacts Telegram
- [x] 3.2 Receive forwarded general-surface updates; apply chat-id matching and
      approver filtering against `GET /signal/sources?adapter=`
- [x] 3.3 Normalize to `{fingerprint: tg-<update_id>, kind: chat, payload, labels:
      {agentops.dev/channel, agentops.dev/sender}}` and post `/signal/inbound`.
      `spec.config.channel` names the Channel the chat is the general surface
      of — required, since replies have nowhere to go without it

## 4. channel-telegram becomes receive-only

- [x] 4.1 Remove `pollManager`, `pollToken`, `tokenGroup`, and `GetUpdates`;
      keep the ops loop, `Send`, `CreateTopic`, chat-id matching, approver
      filtering, and the `/channel/inbound` post. `spec.config.pollingEnabled`
      is gone too — a channel cannot opt into a loop that no longer exists here
- [x] 4.2 Turn `dispatch` into an HTTP handler for forwarded topic updates;
      add the listen port. It now also DROPS a general-surface update rather
      than adopting it — that path is the signal adapter's
- [x] 4.3 Accept and persist offset reports from the router through the existing
      `PUT /channel/state/{channel}/{key}` (`GET/PUT /offset`)

## 5. Manager: origination moves to the signal path

- [x] 5.1 Make `threadId` required on `POST /channel/inbound`; reject a missing
      one with a message naming the signal path
- [x] 5.2 Delete `adoptThread`, the bare-text branch, the `/<profile>` branch,
      and `defaultPipeline` from `internal/chat/router.go`; keep `convByThread`,
      `appendInput`, `FanOutSend`, `relayToSiblings` untouched. `handleCommand`
      became exported `HandleCommand` — it now runs from the chat SIGNAL path
      (5.6), not from channel inbound
- [x] 5.3 Delete `PipelineForChannel` from `internal/chat/pipelines.go` and every
      caller. `boundChannels` now takes the originating Pipeline directly
      instead of re-resolving one from the channel
- [x] 5.4 Add the `chat` kind to the signal core: task lane, no signature
      grouping by default, `cooldownHours: 0` default for chat sources. Chat
      with no declared `signatureLabels` keys the group on the FINGERPRINT —
      the default alert labels are vocabulary chat never carries, so every
      message would otherwise hash alike and pile into one conversation
- [x] 5.5 Reject a `chat` signal missing `agentops.dev/channel` at
      `/signal/inbound`, naming the missing label
- [x] 5.6 Move command handling (`/agents`, `/help`, unknown agent, usage error)
      into the chat signal path, emitting `send` ops to the originating channel
      without creating a Conversation; `/<pipeline> <task>` still creates one
- [x] 5.7 Route the unclaimed-source drop reason back to the originating surface
      so an unwired chat source is visible to the user, not just in conditions

## 6. Chart

- [x] 6.1 Render all three adapter CRs under one flag (default false) —
      `ChannelAdapter telegram`, `SignalAdapter telegram`, `SignalAdapter
      telegram-router`, plus the router's credential-carrying `SignalSource`.
      SUPERSEDED IN SHAPE by 6.5: this now lives in the `telegram-bundle`
      subchart under `telegram-bundle.enabled`, not flat
      `telegramAdapter.enabled`
- [x] 6.2 Add `signalTelegram`/`telegramRouter` image values; no Deployment and
      no Service templates — the reconcilers own both (now
      `telegram-bundle.signalAdapter` / `.router`)
- [x] 6.3 Sample Pipeline in `config/samples/samples.yaml` pairing the chat
      `SignalSource` with its `Channel` under one Pipeline
- [x] 6.4 Bump chart version (3.0.0 → 3.1.0); `helm lint` clean and
      `helm template` verified with the flag on and off
- [x] 6.5 Repackage Telegram as the embedded `telegram-bundle` SUBCHART
      (requester decision, added after 6.1–6.4), matching `vm-bundle` /
      `k8s-bundle`. Two layers: `enabled` alone ships the three
      implementations and wires nothing; `surface.enabled` additionally ships
      the `Channel`, the chat `SignalSource`, and the router's credential
      source. Flat `telegramAdapter`/`signalTelegram`/`telegramRouter` values
      and `chart/templates/telegram-adapter.yaml` are gone
- [x] 6.7 `surface.enabled` makes validation EXPLICIT rather than inferred
      (requester decision): with it on, a missing `chatId`, a missing
      credential, or both credential forms at once each FAIL the render naming
      the offending value. Inferring intent from which fields are filled would
      read a typo'd key as "no surface wanted"
- [x] 6.8 Credentials two ways (requester decision):
      `credentials.existingSecret` (a Secret you manage, key `botToken`) or
      `credentials.botToken` (bundle creates `<surface.name>-telegram`,
      overridable via `credentials.secretName`). Exactly one; both is an error.
      Documented that form (b) puts the token in values AND in the in-cluster
      release
- [x] 6.9 The bundle ships NO Pipeline (requester decision — wiring drags in a
      profile, runtime and credentials). CONSEQUENCE, accepted deliberately:
      the rendered sources are unclaimed, so every chat message drops with
      `Wired=False` until the installer wires them. Mitigated by making it
      loud rather than silent — parent `NOTES.txt` prints "nothing answers
      yet" plus the exact Pipeline to apply with the rendered names filled in,
      and the values/README say the same
- [x] 6.6 Fix CRD doc comments the change made false — `Channel.spec.adapter`
      and `Pipeline.spec.profileRef` still described bare-message answering and
      the oldest-Ready default profile; regenerated

## 7. Tests

- [x] 7.1 envtest: general-surface chat signal creates a conversation with the
      claiming Pipeline's profile and channel set, and bumps `receivedTotal`
- [x] 7.2 envtest: unclaimed chat source drops, reports the reason, and it
      reaches the originating surface as a send op
- [x] 7.3 envtest: two Ready Pipelines sharing a channel — resolution is by
      claim, unaffected by creation order
- [x] 7.4 envtest: `/channel/inbound` without `threadId` is rejected; an unknown
      thread is not adopted
- [x] 7.5 envtest: reply in a known thread still appends, acks, and relays to
      siblings — unchanged behavior, pinned (existing tests, untouched and
      still green)
- [x] 7.6 envtest: `/agents` emits a send op and creates no Conversation;
      `/<pipeline> <task>` creates one
- [x] 7.7 envtest: repeated identical chat text creates two conversations
      (cooldown off for chat)
- [x] 7.8 envtest: `chat` signal without `agentops.dev/channel` is rejected
- [x] 7.9 Unit: router classification by `IsTopicMessage` (+ verbatim
      forwarding, offset delegation); adapters' chat-id and approver filtering
      as the same table in both modules, to catch drift
- [x] 7.10 Full suite: `go build ./... && go vet ./...` in all modules,
      `KUBEBUILDER_ASSETS=… go test ./...` — all green

## 8. Docs and migration

- [x] 8.1 Write the migration: add the chat `SignalSource` and claim it, scale
      the old adapter to zero, confirm no `getUpdates` consumer remains, enable
      the new stack, carry the offset annotation across — README "Migrating to
      chart 3.1", with the ordering-critical step called out
- [x] 8.2 Update `CLAUDE.md`: origination is signal-only (new invariant), the
      three-component telegram topology, the module map, the build/image
      commands, and the single-consumer rule now naming the router
- [x] 8.3 Update README — the one workflow (signal originates, channel carries)
      in "Behaviors that matter", the stack diagram in the migration, and the
      adapter contract's inbound step now reply-only with the origination path
      named
- [x] 8.4 Reconcile with `capabilities-are-wiring` — moot as a coordination
      task: it is already ARCHIVED (2026-08-08), as is
      `pipeline-addressed-conversations`, so there is no "before either lands".
      The reconciliation it asked for is done: chat commands address a
      PIPELINE and carry its bindings, so `CLAUDE.md`'s claim that
      "`POST /task` and `/profile`-command conversations carry none" was stale
      and now reads that every origination has a Pipeline to mirror. No
      capability-only Pipeline is needed for chat
