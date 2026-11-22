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

A **channel adapter** is a container that renders the manager's outbound operations onto a chat transport and pushes what people type back in. Your adapter dials the manager, never the reverse.

![The manager queues outbound operations for your channel adapter, which renders them onto your chat, and pushes what people type back to the manager.]({{ '/assets/img/guides/channel-adapter-light.svg' | relative_url }}){: .ao-diagram}

## Before you start

Write one for conversations on a surface nothing here serves. Review [the specification]({{ '/contracts/channel-adapter/' | relative_url }}) first — this builds the smallest conformant implementation.

## 1. Read an operation, then acknowledge it

Long-poll [GET /channel/ops]({{ '/contracts/channel-adapter/' | relative_url }}#get-channelops) with `contract=3` (`204` = nothing queued, `200` = [operation envelope]({{ '/contracts/channel-adapter/' | relative_url }}#the-operation-envelope)). **Acknowledge before you act** (`OPS-18`). Retry `204`/`503` at once (`OPS-1`, `OPS-2`).

```sh
curl -H "Authorization: Bearer $ADAPTER_TOKEN" \
  "$MANAGER_URL/channel/ops?adapter=$ADAPTER_NAME&contract=3&wait=25"
```

```powershell
curl -H "Authorization: Bearer $env:ADAPTER_TOKEN" `
  "$env:MANAGER_URL/channel/ops?adapter=$env:ADAPTER_NAME&contract=3&wait=25"
```

```sh
curl -X POST -H "Authorization: Bearer $ADAPTER_TOKEN" \
  "$MANAGER_URL/channel/ops/op-91c3/ack?adapter=$ADAPTER_NAME"
```

## 2. Deliver it and report the outcome

[POST /channel/ops/{id}/done]({{ '/contracts/channel-adapter/' | relative_url }}#post-channelopsiddone). `ensure-topic` returns `{"threadId":"…"}`. Others return `{}`. Report `{"error":"…"}` only for terminal failures (`ADA-3`).

## 3. Render the message

`send` carries [typed message]({{ '/contracts/channel-adapter/' | relative_url }}#the-typed-message) (`signal`, `answer`, `relay`, `notice`), never rendered text. Prose is [markdown]({{ '/contracts/channel-adapter/' | relative_url }}#prose-fields-are-markdown). `answer` and `notice` may carry [BLOCK GRAMMAR]({{ '/contracts/channel-adapter/' | relative_url }}#the-body-grammar) if `outputFormat: blocks`.

{: .ao-callout}
> **Never drop `choices`** (`ADA-8`).

## 4. Push replies back

[POST /channel/inbound]({{ '/contracts/channel-adapter/' | relative_url }}#post-channelinbound). Replies echo back only if `echoesOwnMessages: false` (`ADA-5`).

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
> **`threadId` required** (`IN-1`). Unknown threads dropped (`IN-2`).

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

**`echoesOwnMessages`**: `true` = transport shows what person typed; `false` = viewer renders only what it's sent.

**`externals`**: systems adapter talks to (metadata).

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

Manager projects credentials as `AGENTOPS_CRED_<CHANNEL>_*`, never reads them (`MGR-10`).

## 7. Test end to end

List channel on a Pipeline's `channelRefs`, start conversation, tail logs:

```sh
kubectl -n agent-ops logs deploy/agentops-adapter-my-adapter -f
```

```powershell
kubectl -n agent-ops logs deploy/agentops-adapter-my-adapter -f
```

## Verify

1. **Long-poll**: `204` when idle, `200` with op when conversation starts. Read twice without ack → same op returns
2. **Thread lifecycle**: `ensure-topic` → Conversation records thread ID. No `threadId`/`error` → fails (`OPS-6`)
3. **Rendering**: `answer` with tags → handled. No tags → plain prose (`ADA-12`)
4. **Inbound**: unknown thread → dropped. Real thread → reaches other surfaces, not origin

## What comes next

1. **Read [the specification]({{ '/contracts/channel-adapter/' | relative_url }}) in full** — every field, compatibility rules, 23-item conformance checklist
2. **Copy the reference** — [`channels/telegram`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/channels/telegram/) or [`platform/console`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/platform/console/)
3. **[Run agents on your own backend]({{ '/guides/agent-runtime/' | relative_url }})** — the work contract and what the manager trusts a runtime to enforce
