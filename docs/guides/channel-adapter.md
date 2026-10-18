---
title: "Talk to agents from your own chat"
permalink: /guides/channel-adapter/
description: >-
  What a channel adapter is, and how to implement one so people can talk to
  agents from a surface agent-ops does not serve, station by station against
  the channel adapter specification.

next:
  eyebrow: Next
  title: "Run agents on your own backend"
  body: >-
    Implement the work contract, and understand what the manager trusts a
    runtime to enforce on its behalf.
  url: /agent-ops-operator/guides/agent-runtime/
---

A **channel adapter** is how a person and an agent talk to each other. It is a
container that renders the manager's outbound operations onto a transport, and
pushes what people type back in.

Both directions, on one surface: Telegram and the console are two, and yours is
a third.

**Your adapter dials the manager, never the reverse.** NetworkPolicies stay
simple and your transport's credentials never leave your pod.

![The manager queues outbound operations for your channel adapter, which renders them onto your chat, and pushes what people type back to the manager.]({{ '/assets/img/guides/channel-adapter-light.svg' | relative_url }}){: .ao-diagram}

## Before you start

Writing one is appropriate when you want conversations carried on a surface
nothing here serves — another chat service, a ticketing system, an app of your
own.

It is **not** what you want when:

- You want to **start** conversations from a surface. A channel never
  originates, so that is a chat `SignalSource` and a signal adapter —
  [React to signals]({{ '/guides/signal-adapter/' | relative_url }}).
- You want a second surface on a served transport. That is a `Channel` naming
  the adapter that already exists.

{: .ao-callout}
> **Never re-ingest your own outbound posts as inbound.** One adapter may serve
> several surfaces of one conversation, so a message can be delivered *toward*
> the transport it entered through — that is not the loop this rule forbids.

Review [the channel adapter specification]({{ '/contracts/channel-adapter/' | relative_url }})
first. This page builds the smallest thing that satisfies it, station by
station, and every station names the exact section it builds toward.

The specification wins on any disagreement — this page states no
requirement of its own.

## The overall shape

Four pieces:

1. **A container** implementing the channel contract — long-poll for
   operations, complete each one, push replies back.
2. **A `ChannelAdapter` resource** — image and workload knobs, plus the
   interface metadata describing what you serve.
3. **A `Channel` naming it**, with that surface's credentials.
4. **A `Pipeline` listing that channel**, so conversations are bound to it.

**The manager composes meaning. You compose presentation.** An operation
carries a typed message or a topic descriptor, and there is no `op.text`
and no `op.title` — see [The typed message]({{ '/contracts/channel-adapter/' | relative_url }}#the-typed-message).

Escaping, length limits, splitting and topic naming belong to the
component that knows them, which is yours.

## 1. Take operations from the queue

Long-poll — see [GET /channel/ops]({{ '/contracts/channel-adapter/' | relative_url }}#get-channelops).
`contract=2` is required, and an absent or outdated version is refused at the
door with a `400` naming the current one.

```sh
curl -H "Authorization: Bearer $ADAPTER_TOKEN" \
  "$MANAGER_URL/channel/ops?adapter=$ADAPTER_NAME&contract=2&wait=25"
```

```powershell
curl -H "Authorization: Bearer $env:ADAPTER_TOKEN" `
  "$env:MANAGER_URL/channel/ops?adapter=$env:ADAPTER_NAME&contract=2&wait=25"
```

A `204` means nothing queued — poll again. A delivered op is
[the operation envelope]({{ '/contracts/channel-adapter/' | relative_url }}#the-operation-envelope),
one of
[four typed kinds]({{ '/contracts/channel-adapter/' | relative_url }}#typed-operations),
carrying
[a claim window]({{ '/contracts/channel-adapter/' | relative_url }}#the-claim-window):

```json
{"id":"op-91c3","channel":"my-surface","kind":"send",
 "message":{"kind":"answer","body":"…"},"reclaimAfterSeconds":120}
```

**A `503` means this replica is not the manager's current leader.** Retry at
once, on a fresh connection, exactly as you already retry an empty `204` —
never log either as a failure (`OPS-1`, `OPS-2`).

**Verify:** the `curl` above returns `204` against an idle channel, and
returns an envelope shaped like the one above once a conversation queues
you work.

## 2. Complete each op

Report the outcome to
[POST /channel/ops/{id}/done]({{ '/contracts/channel-adapter/' | relative_url }}#post-channelopsiddone)
with
[the completion result]({{ '/contracts/channel-adapter/' | relative_url }}#the-completion-result):

| Operation | You do | You report |
|---|---|---|
| `ensure-topic` | create a thread from `op.topic` | `{"threadId":"…"}`, opaque, your own id space |
| `send` | post `op.message` | an empty body |
| `close-topic` | archive the thread | an empty body, or — on failure — just a log line, never a Conversation condition |
| `delete-conversation` | gone for good — tombstone, archive, rename, your call | an empty body |

**Delivery is at-least-once.** Dedup by `op.id`, and treat an already-closed
thread as success — redelivery is normal.

A transport backpressure signal — `429`, a `retry_after`, a token-bucket
rejection — is not an operation failure. Absorb it inside your claim window
and pace your own sending before you claim, never before you send — see
[Backpressure]({{ '/contracts/channel-adapter/' | relative_url }}#backpressure-is-the-adapters-to-absorb).

**Verify:** complete an `ensure-topic` with no `threadId` and no `error`, and
confirm the manager treats it as a failed op (`OPS-6`) — then complete one
correctly and confirm the Conversation records your thread id.

## 3. Render the message

`send` carries
[the typed message]({{ '/contracts/channel-adapter/' | relative_url }}#the-typed-message),
one of four kinds — `signal`, `answer`, `relay`, `notice` — never rendered
text.

Prose fields arrive in
[a named markdown subset]({{ '/contracts/channel-adapter/' | relative_url }}#prose-fields-are-markdown).
Escape it for your transport — passing it through raw is how tags reach a
reader as literal characters.

**An `answer` or `notice` body may also carry the BLOCK GRAMMAR.** Its
recognition rules and the fold belong to the `structured-agent-output`
capability, shared verbatim with `channels/telegram/blocks.go` and the
console's `blocks.ts`.

See [The body grammar]({{ '/contracts/channel-adapter/' | relative_url }}#the-body-grammar)
for the rules themselves. This guide states none of its own.

**Not implementing it is fine.** `AgentProfile.spec.outputFormat` is
required, and a profile declaring `none` emits no tags at all.

`ensure-topic` carries
[a topic descriptor]({{ '/contracts/channel-adapter/' | relative_url }}#the-topic-descriptor)
rather than a baked title, because the manager cannot know your limits — a
Telegram forum topic caps at 128 characters, a web chat does not.

Any message may also carry
[`choices`]({{ '/contracts/channel-adapter/' | relative_url }}#a-message-choice)
and `inReplyTo`. Render controls if your transport has them. **Never drop
`choices`** (`ADA-8`) — the body already names each one, so a transport
without controls renders them as a list.

**Verify:** send yourself an `answer` carrying `<title>`/`<details>` tags and
confirm your renderer folds `<details>` and shows the rest in written order.
Send one with no tags and confirm it renders as plain prose — the
backward-compatibility case (`ADA-12`).

## 4. Push replies back

[POST /channel/inbound]({{ '/contracts/channel-adapter/' | relative_url }}#post-channelinbound)
with
[the inbound request]({{ '/contracts/channel-adapter/' | relative_url }}#the-inbound-request):

```sh
curl -X POST -H "Authorization: Bearer $ADAPTER_TOKEN" \
  "$MANAGER_URL/channel/inbound" \
  -d '{"channel":"my-surface","threadId":"t-42","text":"…"}'
```

```powershell
curl -X POST -H "Authorization: Bearer $env:ADAPTER_TOKEN" `
  "$env:MANAGER_URL/channel/inbound" `
  -d '{\"channel\":\"my-surface\",\"threadId\":\"t-42\",\"text\":\"…\"}'
```

{: .ao-callout}
> **`threadId` is REQUIRED, and this endpoint never starts a conversation**
> (`IN-1`). A message in a thread the manager does not know is dropped, not
> adopted (`IN-2`).

Delivery to the other bound surfaces, and busy-acks, happen manager-side. A
reply you push comes back to you **only** if you declare
`echoesOwnMessages: false` (`ADA-5`).

**Verify:** push a reply naming a thread the manager does not know, and
confirm nothing is adopted. Push one on a real thread and confirm it reaches
every other bound surface, never the one it entered on.

## 5. Declare the ChannelAdapter

<!-- generated: template kind=ChannelAdapter name=my-adapter fields=image,port,echoesOwnMessages,singleton,configSchema,credentialKeys,externals comments=off -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: ChannelAdapter
metadata:
  name: my-adapter
spec:
  image: <image>
  port: 8080
  echoesOwnMessages: true
  singleton: true
  configSchema: {}
  credentialKeys:
  - key: <key>
  externals:
  - kind: sender   # sender | api | kubernetes
    name: <name>
```
<!-- /generated -->

**`echoesOwnMessages` is transport knowledge, and only you can declare it.**

It defaults to `true`, meaning your transport already shows a person the message
they typed. Set it `false` on a viewer that renders only what it is sent, as the
console does.

An unreadable channel answers `true`, which is the conservative half.

**`externals` names the systems your implementation talks to**, each with a
`kind` of `sender`, `api` or `kubernetes`. The console's topology draws them
beside your adapter. It is metadata only and grants nothing.

This is what the chart renders for Telegram. Note how much of it is interface
metadata, so an operator learns what `config` needs without reading the source:

<!-- generated: example preset=tier3 kind=ChannelAdapter name=telegram -->
```yaml
# Source: agent-ops-operator/charts/telegram/templates/adapters.yaml
# CONTINUATION and delivery. Does NOT poll: gateway-telegram owns the single
# getUpdates loop and pushes topic updates here. It also does not SEND
# directly any more (chat-ops-broker-telegram-gateway): every Bot API call
# goes through gateway-telegram's outbound relay instead.
apiVersion: agentops.dev/v1alpha1
kind: ChannelAdapter
metadata:
  name: telegram
  namespace: agent-ops
  labels:
    app.kubernetes.io/name: agentops-telegram
    helm.sh/chart: "telegram-0.5.0"
spec:
  image: "ghcr.io/kostiantyn-matsebora/agentops-channel-telegram:0.26.0"
  # Receives forwarded topic updates: the reconciler owns Service
  # agentops-adapter-<name> and injects LISTEN_ADDR.
  port: 8080
  singleton: true
  # Metadata only: what this implementation faces outside the install. The
  # manager verifies none of it and grants nothing from it.
  # The router polls the same API, but it is no adapter and declares nothing.
  externals:
    - name: Telegram Bot API
      kind: api
  # Interface metadata: makes `kubectl get channeladapter` answer "what goes in
  # spec.config?" without reading adapter source. Advisory — the manager
  # reports ConfigValid, the adapter stays the authority.
  configSchema:
    type: object
    properties:
      chatId:
        type: string
        description: Telegram chat id the adapter posts to (forum supergroup).
      feedThreadId:
        type: integer
        description: Topic id for the raw alerts feed.
      approvers:
        type: array
        items:
          type: integer
        description: Telegram user ids allowed to talk to the agent.
      deleteTopicOnConversationDelete:
        type: boolean
        description: >-
          Delete the forum topic when its conversation is deleted, instead of
          leaving it archived with a tombstone. Destroys the transcript and
          needs the bot to hold can_delete_messages. Absent = false.
    required:
      - chatId
  credentialKeys:
    # documentation only — the manager never reads Secrets.
    - key: gatewayUrl
      required: false
      description: >-
        gateway-telegram's in-cluster base URL for this surface. Optional
        because the adapter also accepts TELEGRAM_GATEWAY_URL from its
        environment.
    - key: gatewayToken
      required: false
      description: >-
        Shared secret authenticating this surface's calls to
        gateway-telegram's POST /telegram/call. Optional because the adapter
        also accepts TELEGRAM_GATEWAY_TOKEN from its environment.
```
<!-- /generated -->

**The Telegram adapter holds no Telegram credential.**

- It sends through `gateway-telegram`'s outbound relay.
- What it needs projected is the gateway binding, `gatewayUrl` and `gatewayToken`, never the bot token.

**Verify:** `kubectl apply -f my-adapter.yaml` succeeds, and
`kubectl get channeladapter my-adapter` shows the interface metadata back —
the next station's `Channel` is what actually serves a surface through it.

## 6. Declare a Channel

<!-- generated: template kind=Channel name=my-surface fields=credentialsSecretRef,config comments=off -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: Channel
metadata:
  name: my-surface
spec:
  adapter: <adapter>
  credentialsSecretRef:
    name: <name>
  config: {}
```
<!-- /generated -->

Credentials are **per surface**, not per implementation — see
[Credentials]({{ '/contracts/channel-adapter/' | relative_url }}#credentials).
The manager projects the Secret into your pod as `AGENTOPS_CRED_<CHANNEL>_*`
and never reads it (`MGR-10`).

Everything type-specific lives in `config`, which only your implementation
interprets — see
[The channel-listing entry]({{ '/contracts/channel-adapter/' | relative_url }}#the-channel-listing-entry):

<!-- generated: example preset=tier3 kind=Channel name=k8s-ops -->
```yaml
# Source: agent-ops-operator/charts/telegram/templates/surface.yaml
# The Channel CARRIES conversations — it never starts one. No wiring lives
# here: who answers is declared by whichever Pipeline claims the chat source.
apiVersion: agentops.dev/v1alpha1
kind: Channel
metadata:
  name: k8s-ops
  namespace: agent-ops
  labels:
    app.kubernetes.io/name: agentops-telegram
    helm.sh/chart: "telegram-0.5.0"
spec:
  # names the ChannelAdapter serving this surface; that adapter's
  # implementation is what defines and validates `config` below
  adapter: telegram
  # projected into the adapter pod as env by the reconciler, kubelet-resolved —
  # nothing reads the Secret through the API.
  # The gateway binding Secret, not the bot token.
  credentialsSecretRef:
    name: agentops-telegram-gateway
  # opaque to the operator; this shape is what channel-telegram parses
  config:
    chatId: "-1001234567890"
```
<!-- /generated -->

**`credentialsSecretRef` names the gateway binding Secret, not the bot token.**

- `channel-telegram` reads only `gatewayUrl` and `gatewayToken` from it.
- It is independent of how the bot token itself is supplied, including `surface.credentials.existingSecret`.

{: .ao-callout}
> **That chat id is a PLACEHOLDER**, as is every identifier in every example on
> this site. Substitute your own — an example that works when pasted unchanged
> would be naming somebody else's surface.

## 7. Verify end to end

```sh
kubectl -n agent-ops get channel my-surface
kubectl -n agent-ops logs deploy/agentops-adapter-my-adapter -f
```

```powershell
kubectl -n agent-ops get channel my-surface
kubectl -n agent-ops logs deploy/agentops-adapter-my-adapter -f
```

List that channel on a Pipeline's `channelRefs`, then start a conversation on
that Pipeline. You should see an `ensure-topic` arrive, then a `send`.

**Verify:** the pod log shows the `ensure-topic` envelope arriving, your
completion carrying the thread id back, then the `send` rendering the
agent's answer — stations 1 through 3, in order, on a real conversation.

## What comes next

1. **Read the specification in full** —
   [the channel adapter specification]({{ '/contracts/channel-adapter/' | relative_url }})
   covers every field, every enum, the compatibility rules, and the
   23-item conformance checklist a claim of conformance is walked against.
2. **Copy the reference** —
   [`channels/telegram`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/channels/telegram/),
   and read it beside
   [`gateways/telegram`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/gateways/telegram/)
   and
   [`signals/telegram`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/signals/telegram/).
   One poll loop per credential is structural, and the split is why.
3. **Read a browser implementation** —
   [`platform/console`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/platform/console/),
   if your surface is a page rather than a chat service.
4. **Add the optional halves** —
   [`POST /channel/read`]({{ '/contracts/channel-adapter/' | relative_url }}#post-channelread)
   for a read watermark and
   [`GET /channel/vocabulary`]({{ '/contracts/channel-adapter/' | relative_url }}#get-channelvocabulary)
   for a command menu. An adapter implementing neither stays fully
   conformant.
5. **[Run agents on your own backend]({{ '/guides/agent-runtime/' | relative_url }})** —
   the execution side, and what it is trusted to enforce.
