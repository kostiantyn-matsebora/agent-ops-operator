# Telegram bundle (subchart)

The Telegram subchart: the three-component ingest stack and the chat surface.


`chart/charts/telegram-bundle/` packages the Telegram experience — off by
default. It ships in two layers, because the implementations are guessable and
the surface is not.

**Layer 1 — the implementations** (`telegram-bundle.enabled=true` alone). Three
adapter CRs, because Telegram serves exactly one update stream per bot token: a
second concurrent `getUpdates` gets `409`, and confirming an offset
destructively consumes updates for every reader. So origination and
continuation cannot each poll for themselves — one process reads the stream and
fans it out:

```
getUpdates ─▶ telegram-router ─┬─ no topic ─▶ signal-telegram  ─▶ /signal/inbound
              (the only poller) └─ topic    ─▶ channel-telegram ─▶ /channel/inbound
```

- **`telegram-router`** classifies on `is_topic_message` — a field that rides
  on the update, so the decision is local with no manager round-trip — and
  forwards updates **verbatim**. It holds no channel configuration (chat-id
  matching and approver filtering stay in the receiving adapters), persists
  nothing, and needs no Kubernetes access.
- **`signal-telegram`** turns general-surface messages into `kind: chat`
  signals. It never contacts Telegram, so it holds no credentials.
- **`channel-telegram`** sends, creates topics, and receives forwarded topic
  updates on `spec.port`. It also persists the router's offset, being the
  component with a Channel to annotate.

This layer wires nothing — the right shape when you manage Channels yourself.

**Layer 2 — the chat surface**, opt-in and explicit. `surface.enabled: true`
makes everything unguessable REQUIRED, so a missing field fails the render
naming what to set, instead of quietly installing half a surface:

```yaml
telegram-bundle:
  enabled: true
  surface:
    enabled: true
    chatId: "-1004369687194"              # REQUIRED, a forum supergroup
    credentials:
      existingSecret: agentops-telegram   # REQUIRED — this, or botToken below
    approvers: [123456789]
```

That renders the `Channel`, the chat `SignalSource` (same name as the Channel —
one name for the whole surface), the bot `Secret`, and the **router Deployment**.

The router is the odd one out of the three components: it is the only
`getUpdates` consumer, but it produces no signals, so it is **not an adapter**.
It has no `SignalAdapter` CR and no served CR — the bundle owns its Deployment
and injects the two forwarding URLs and the bot token as env, and it never
contacts the manager. One Deployment per bot token makes "exactly one poller"
structural rather than bookkeeping; the trade is that one router serves one bot,
so a second surface means a second router.

The credential comes in either form, and exactly one of them:

| | |
|---|---|
| `credentials.existingSecret` | a Secret you already manage, holding key `botToken` — prefer this when the token comes from an external secret manager |
| `credentials.botToken` | the token itself; the bundle creates the Secret (`<surface.name>-telegram`, override with `credentials.secretName`). Convenient, but the value then lives in your values file *and* in the release stored in-cluster |

One Secret serves the whole surface either way: the `Channel` references it to
**send**, the router's `SignalSource` to **poll** — the same bot. Neither the
manager nor any reconciler reads it; both are projected into their pods and
resolved by the kubelet.

**No bundle ships a `Pipeline`, so nothing answers yet** — declare the route
under the parent chart's `pipelines:`. Wiring would pull a
profile, a runtime and its credentials into what is otherwise a transport
bundle, so it stays yours. The consequence is real and worth stating plainly:
wiring lives only on Pipelines, so until a Ready one CLAIMS these sources,
every message drops with `Wired=False` — the person typing is told so on their
own surface, but nothing runs. The post-install notes print the exact Pipeline
to apply, pre-filled with your names.
