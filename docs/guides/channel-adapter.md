---
title: "Talk to agents from your own chat"
permalink: /guides/channel-adapter/
description: >-
  What a channel adapter is, why an operation carries meaning rather than
  markup, and how to implement one so people can talk to agents from a surface
  agent-ops does not serve.

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
> **Never re-ingest your own outbound posts as inbound.** An implementation that
> does loops rather than merely duplicating — one adapter may serve several
> surfaces of one conversation, so a message can be delivered *toward* the
> transport it entered through.

Review
[the channel adapter contract](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/contracts.md#the-channel-adapter-contract)
first.

## The overall shape

Four pieces:

1. **A container** implementing the channel contract — long-poll for
   operations, complete each one, push replies back.
2. **A `ChannelAdapter` resource** — image and workload knobs, plus the
   interface metadata describing what you serve.
3. **A `Channel` naming it**, with that surface's credentials.
4. **A `Pipeline` listing that channel**, so conversations are bound to it.

**The manager composes meaning. You compose presentation.** An operation carries
a typed message or a topic descriptor, and there is no `op.text` and no
`op.title`. Escaping, length limits, splitting and topic naming belong to the
component that knows them, which is yours.

## Take operations from the queue

Long-poll. `contract=2` is required, and an absent or outdated version is
refused at the door with a `400` naming the current one.

```sh
curl -H "Authorization: Bearer $ADAPTER_TOKEN" \
  "$MANAGER_URL/channel/ops?adapter=$ADAPTER_NAME&contract=2&wait=25"
```

```powershell
curl -H "Authorization: Bearer $env:ADAPTER_TOKEN" `
  "$env:MANAGER_URL/channel/ops?adapter=$env:ADAPTER_NAME&contract=2&wait=25"
```

| Operation | You do | You report to `POST /channel/ops/{id}/done` |
|---|---|---|
| `ensure-topic` | create a thread from `op.topic` | `{"threadId":"…"}`, opaque and in your own id space |
| `send` | post `op.message` | empty body |
| `close-topic` | archive the thread | empty body |
| `delete-conversation` | the conversation is **gone for good** — tombstone, archive, rename, your call | empty body |

**Delivery is at-least-once.** Dedup by `op.id`, and treat an already-closed
thread as success. Redelivery is normal.

Report a failure as `{"error":"…"}` and the manager surfaces it as a Conversation
condition and regenerates the operation.

## Render the message

`send` carries `op.message`, one of four kinds:

| Kind | Is |
|---|---|
| `signal` | the event that opened or advanced the conversation, as it arrived |
| `answer` | agent output |
| `relay` | a user message, from a surface that is not this one |
| `notice` | the manager on its own behalf — acks, listings, refusals |

Prose arrives in a **named markdown subset**, which the agent is instructed to
write. Escape it for your transport. An adapter that passes it through raw is
how tags reach a reader as literal characters.

**A body may also carry the BLOCK GRAMMAR**, and you parse it:

| Tag | Is |
|---|---|
| `<title>` | at most one, rendered first, a single line |
| `<details>` | **the fold** — present it collapsed |
| anything else | a section the AGENT named, above the fold, in written order |

- **Recognized only** when a tag stands alone on its own line, forms a
  well-formed pair, and sits outside fenced code. Everything else is literal —
  agent output is full of `<`.
- **`answer` and `notice` only.** A relay is somebody's typed words, and a
  `signal` is a card built from its structured fields.
- **No tags is one block**, which renders as prose does today.
- **The section vocabulary is OPEN.** Render a label generically — never carry a
  list of section names.

The manager parses none of this, exactly as it parses none of the markdown. Full
rules in [the channel adapter
contract](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/contracts.md#the-body-grammar).

**Not implementing it is fine.** `AgentProfile.spec.outputFormat` is required,
and a profile declaring `none` emits no tags at all.

`ensure-topic` carries a **descriptor** rather than a baked title, because the
manager cannot know your limits. A Telegram forum topic caps at 128 characters
and a web chat does not.

Any message may also carry `choices` and `inReplyTo`. Render controls if your
transport has them. **Never drop `choices`** — the body already names each one,
so a transport without controls renders them as a list.

## Push replies back

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
> **`threadId` is REQUIRED, and this endpoint never starts a conversation.** A
> message in a thread the manager does not know is dropped, not adopted. **A
> channel carries conversations. It never starts one.**

Delivery to the other bound surfaces and busy-acks happen manager-side.

A reply you push comes back to you **only** if your adapter declares
`echoesOwnMessages: false`.

## Declare the ChannelAdapter

<!-- generated: template kind=ChannelAdapter name=my-adapter fields=image,port,echoesOwnMessages,singleton,configSchema,credentialKeys comments=off -->
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
```
<!-- /generated -->

**`echoesOwnMessages` is transport knowledge, and only you can declare it.**

It defaults to `true`, meaning your transport already shows a person the message
they typed. Set it `false` on a viewer that renders only what it is sent, as the
console does.

An unreadable channel answers `true`, which is the conservative half.

This is what the chart renders for Telegram. Note how much of it is interface
metadata, so an operator learns what `config` needs without reading the source:

<!-- generated: example preset=tier3 kind=ChannelAdapter name=telegram -->
```yaml
# Source: agent-ops-operator/charts/telegram/templates/adapters.yaml
# CONTINUATION and delivery. Does NOT poll: gateway-telegram owns the single
# getUpdates loop and pushes topic updates here.
apiVersion: agentops.dev/v1alpha1
kind: ChannelAdapter
metadata:
  name: telegram
  namespace: agent-ops
  labels:
    app.kubernetes.io/name: agentops-telegram
spec:
  image: "ghcr.io/kostiantyn-matsebora/agentops-channel-telegram:0.24.2"
  # Receives forwarded topic updates: the reconciler owns Service
  # agentops-adapter-<name> and injects LISTEN_ADDR.
  port: 8080
  singleton: true
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
    # documentation only — the manager never reads Secrets
    - key: botToken
      required: false
      description: >-
        Bot API token for this surface, used to SEND. The same Secret backs the
        router, which uses it to poll. Optional because the adapter also
        accepts TELEGRAM_BOT_TOKEN from its environment.
```
<!-- /generated -->

## Declare a Channel

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

Credentials are **per surface**, not per implementation. The manager projects
the Secret into your pod as `AGENTOPS_CRED_<CHANNEL>_*` and never reads it.

Everything type-specific lives in `config`, which only your implementation
interprets:

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
spec:
  # names the ChannelAdapter serving this surface; that adapter's
  # implementation is what defines and validates `config` below
  adapter: telegram
  # projected into the adapter pod as env by the reconciler, kubelet-resolved —
  # nothing reads the Secret through the API
  credentialsSecretRef:
    name: k8s-ops-telegram
  # opaque to the operator; this shape is what channel-telegram parses
  config:
    chatId: "-1001234567890"
```
<!-- /generated -->

{: .ao-callout}
> **That chat id is a PLACEHOLDER**, as is every identifier in every example on
> this site. Substitute your own — an example that works when pasted unchanged
> would be naming somebody else's surface.

## Verify

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

## What comes next

1. **Copy the reference** —
   [`channels/telegram`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/channels/telegram/),
   and read it beside
   [`gateways/telegram`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/gateways/telegram/)
   and
   [`signals/telegram`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/signals/telegram/).
   One poll loop per credential is structural, and the split is why.
2. **Read a browser implementation** —
   [`platform/console`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/platform/console/),
   if your surface is a page rather than a chat service.
3. **Add the optional halves** — `POST /channel/read` for a read watermark and
   `GET /channel/vocabulary` for a command menu. An adapter that implements
   neither stays fully conformant.
4. **[Run agents on your own backend]({{ '/guides/agent-runtime/' | relative_url }})** —
   the execution side, and what it is trusted to enforce.
