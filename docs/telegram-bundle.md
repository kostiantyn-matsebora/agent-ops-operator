# Telegram bundle (subchart)

The Telegram subchart: the three-component ingest stack and the chat surface.


`chart/charts/telegram-bundle/` packages the Telegram experience — off by
default. It ships in two layers, because the implementations are guessable and
the surface is not.

**Layer 1 — the implementations** (`telegram-bundle.enabled=true` alone). Three
adapter CRs.

**Telegram serves exactly one update stream per bot token.** A second concurrent
`getUpdates` gets `409`, and confirming an offset destructively consumes updates
for every reader.

So origination and continuation cannot each poll for themselves. One process
reads the stream and fans it out:

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

### Bursts are paced, not dropped

Telegram **rejects** rather than queues. A 44-alert burst on 2026-08-13 produced
105 `createForumTopic` and 74 `sendMessage` rejections in four minutes, and the
messages were lost rather than delayed.

The adapter now paces itself against two budgets, and honours a `retry_after`
exactly when it still gets one:

| Budget | Limit |
|---|---|
| per bot, global | 30 sends/second |
| per `chat_id` | 20 sends/minute |

**Expect an alert burst to take minutes to appear in full.** Every topic in a
forum shares one `chat_id`, so cards, replies and topic creations for the whole
surface contend for the same 20/minute.

A 44-alert burst is roughly 144 calls, which is over seven minutes of drain.

That is Telegram's limit rather than a tuning choice, and the alternative is the
old behaviour, which lost the messages outright. A single alert is unaffected.

Pacing gates the **claim**, not the send: work the adapter cannot yet deliver
stays queued in the manager, still derivable from conversation state, so an
adapter restart mid-burst loses nothing.

### Deleting a conversation's topic

By default, deleting a conversation leaves its forum topic in place: the adapter
un-archives it, posts a tombstone saying the conversation is gone, and closes it
again. The transcript above that line is what a person scrolls back to after an
incident.

A busy group pays for that in clutter — one archived topic per conversation,
forever. Opt out per surface:

```yaml
telegram-bundle:
  surface:
    deleteTopicOnDelete: true
```

The topic is then **deleted** instead, and no tombstone is posted into it — a
thread about to disappear has nobody to tell.

**It destroys the transcript**, which is why it is off by default and why the
setting is worth a second thought on a group whose history anyone might want.

One line does go to the chat's **general surface**, naming the conversation.
Without it a topic simply vanishes: the conversation object is gone too, so
nothing anywhere would record that agent-ops removed it, and a reader would
reasonably assume someone deleted it by hand.

Two practical notes:

- **The bot must hold `can_delete_messages`.** Without it the operation is
  reported as failed — deliberately, rather than falling back to archiving,
  because a silent fallback would leave you with a growing list of archived
  topics and no sign that the setting was doing nothing.
- **The conversation is still deleted either way**, once the manager's grace
  expires.

And the setting is on the **Channel**, not the `ChannelAdapter`. Whether a
group's threads should outlive their conversations is a property of that group,
so two surfaces served by one adapter can differ.

Only DELETION is affected. Closing still archives the topic, because a closed
conversation can be reopened into it.

The router is the odd one out of the three components: it is the only
`getUpdates` consumer, but it produces no signals, so it is **not an adapter**.

It has no `SignalAdapter` CR and no served CR. The bundle owns its Deployment
and injects the two forwarding URLs and the bot token as env, and it never
contacts the manager.

One Deployment per bot token makes "exactly one poller" structural rather than
bookkeeping. The trade is that one router serves one bot, so a second surface
means a second router.

The credential comes in either form, and exactly one of them:

| Value | Meaning |
|---|---|
| `credentials.existingSecret` | a Secret you already manage, holding key `botToken` — prefer this when the token comes from an external secret manager |
| `credentials.botToken` | the token itself — the bundle creates the Secret (`<surface.name>-telegram`, override with `credentials.secretName`). Convenient, but the value then lives in your values file *and* in the release stored in-cluster |

One Secret serves the whole surface either way: the `Channel` references it to
**send**, the router's `SignalSource` to **poll** — the same bot. Neither the
manager nor any reconciler reads it. Both are projected into their pods and
resolved by the kubelet.

**No bundle ships a `Pipeline`, so nothing answers yet** — declare the route
under the parent chart's `pipelines:`. Wiring would pull a
profile, a runtime and its credentials into what is otherwise a transport
bundle, so it stays yours. The consequence is real and worth stating plainly:
wiring lives only on Pipelines, so until a Ready one CLAIMS these sources,
every message drops with `Wired=False` — the person typing is told so on their
own surface, but nothing runs. The post-install notes print the exact Pipeline
to apply, pre-filled with your names.
