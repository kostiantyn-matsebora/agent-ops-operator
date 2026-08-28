---
title: Telegram
permalink: /integrations/telegram/
description: >-
  A bundle that puts your agents in a Telegram group — every conversation in its
  own forum topic. Configure it, and wire whichever agents answer on it.

next:
  eyebrow: Next
  title: Give your agent tools
  body: >-
    Any MCP server becomes a capability a Pipeline can grant. The binding, the
    allowlist and the privilege split, end to end.
  url: /agent-ops-operator/guides/toolsets/
---

**This is the integration a person talks to.** Every other one starts work.
Telegram is where the work gets asked for, and where the answer arrives.

![One poller reads every update for a bot token: a message on the group surface starts a conversation, and a message in a topic continues the one that owns it.]({{ '/assets/img/integrations/telegram-light.svg' | relative_url }}){: .ao-diagram}

## What you get

| You get | Which means |
|---|---|
| **A thread per conversation** | Each one gets its own forum topic. An incident does not scroll away under the next one. |
| **A composer that knows your agents** | Telegram completes `/<pipeline>` for every Ready route, so nobody has to remember names. |
| **Somewhere for every integration to answer** | Bind it as a channel on any pipeline and its answers arrive here. |
| **Bursts that arrive late, not lost** | Telegram rejects rather than queues, so the adapter paces itself against its limits. |

**Agents never post to Telegram.** The operator delivers, so nothing an agent
runs holds a bot token.

## Turn it on

**You need a bot and a forum supergroup before the chart helps.** Create the bot
with `@BotFather`, add it to the group, and turn Topics on in the group's
settings.

```sh
kubectl -n agent-ops create secret generic agentops-telegram \
  --from-literal=botToken=<your-bot-token>

helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  -n agent-ops --create-namespace \
  --set telegram.enabled=true \
  --set telegram.surface.enabled=true \
  --set telegram.surface.chatId=-1001234567890 \
  --set telegram.surface.credentials.existingSecret=agentops-telegram
```

```powershell
kubectl -n agent-ops create secret generic agentops-telegram `
  --from-literal=botToken=<your-bot-token>

helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
  -n agent-ops --create-namespace `
  --set telegram.enabled=true `
  --set telegram.surface.enabled=true `
  --set telegram.surface.chatId=-1001234567890 `
  --set telegram.surface.credentials.existingSecret=agentops-telegram
```

> **The chat id above is a placeholder.** Substitute your own — a forum
> supergroup id begins `-100`, and any bot API method that returns the chat
> reports it.
{: .ao-callout}

### It ships in two layers

| Layer | Set | Gets you |
|---|---|---|
| The implementations | `telegram.enabled` alone | the adapter CRs, wiring nothing — right when you manage Channels yourself |
| The surface | `surface.enabled` too | the Channel, the chat source, the bot Secret and the poller |

**Turning the surface on makes everything unguessable REQUIRED**, so a missing
field fails the render naming what to set rather than installing half a surface.

> **Never run two pollers against one bot token.** Migrating from another
> system? Stop its poller, confirm none remains, and only then install. Two
> consumers means 409s and stolen updates — which is why this bundle is one
> Deployment per token, and a missing environment variable exits at startup.
{: .ao-callout}

### Nothing answers until you wire an agent

**This bundle ships no `Pipeline`, deliberately.** A route that answers chat
names a profile and tools that come from *elsewhere*, and a subchart sees only
itself — so it could only ever wire itself.

See [Wire an agent to it](#wire-an-agent-to-it).

## The values you set

Everything below is under `telegram:`.

### What exists

| Value | Default | Set it to |
|---|---|---|
| `enabled` | `false` | `true` to render the adapters |
| `surface.enabled` | `false` | `true` to render the group's Channel and source |
| `surface.chatId` | **empty, required** | your forum supergroup id |
| `surface.credentials.existingSecret` | **empty** | a Secret holding `botToken` — preferred |
| `surface.credentials.botToken` | empty | the token inline, which then lives in your values *and* the release |
| `surface.approvers` | none | numeric user ids allowed to approve, never `@names` |
| `surface.feedThreadId` | none | an existing topic to post unthreaded notices into |
| `apiBase` | empty | the Bot API root both the router and the channel adapter call, for a self-hosted Bot API server or a test double. Empty renders nothing and the real host is used. Deployment-level on purpose — never a `Channel` field, which would let a channel edit redirect its own token. With `existingSecret`, add the same value as the Secret's `apiBase` key |

### What things are called

```yaml
telegram:
  surface:
    name: k8s-ops                       # the Channel AND the chat SignalSource
                                        # — one name for one surface
  channelAdapter:
    name: telegram                      # the ChannelAdapter, and the routing
                                        # key a Channel selects it by
  signalAdapter:
    name: telegram                      # the SignalAdapter for chat origination
```

**Rename `surface.name` to something that describes your group**, since it is
the name your pipelines bind as both a source and a channel.

## Wire an agent to it

The surface is inert on its own. Every route below goes in the **parent chart's**
`pipelines:`.

### Answer chat with an agent

```yaml
pipelines:
  - name: ops
    profile: k8s-engineer
    signalSources: [k8s-ops]      # surface.name — the chat source
    toolsets: [agentops-observe, k8s-observability]
    mcpConfigs: [k8s-api]
    channels: [k8s-ops]           # surface.name — the channel
```

Now anyone can ask in the group, and the answer opens a topic.

### Send another integration's answers here

Add the channel to a route that already exists:

```sh
--set kubernetes.pipelines.channels={k8s-ops}
```

```powershell
--set kubernetes.pipelines.channels={k8s-ops}
```

Cluster events now answer in your group. **The console stays bound too** —
`channels` merges rather than replaces.

### Several agents on one group

```yaml
pipelines:
  - name: cluster
    profile: k8s-engineer
    signalSources: [k8s-ops]
    channels: [k8s-ops]
  - name: house
    profile: ha-user
    signalSources: [k8s-ops]
    channels: [k8s-ops]
```

**Both claim the surface, and that is a supported configuration.** An
*unaddressed* message is then answered with the list of agents serving the group,
each as a button — tapping one sends the message you already typed to it.

Prefer no ambiguity? **Claim the source on one route and leave it off the
others.** They stay reachable by name with `/house <task>`, because addressing a
pipeline needs no claim.

## What to expect in the group

### The composer completes your pipeline names

The adapter registers the manager's vocabulary per chat, so `/pipelines`,
`/close`, `/exit` and every Ready route are offered as you type.

**A hyphenated name is completed with underscores** — `k8s-observe` is offered as
`/k8s_observe`, because Telegram command names admit only lowercase letters,
digits and underscores.

- The **CR is untouched**, and both forms work when typed.
- A name Telegram cannot express at all — one with a dot, or over 32 characters —
  is simply not registered. It stays typable.

### A burst takes minutes to arrive in full

Telegram limits **30 sends a second per bot** and **20 a minute per chat**, and
every topic in a forum shares one chat.

A 44-alert burst is roughly 144 calls, which is over seven minutes of drain.

**That is Telegram's limit, not a tuning choice.** The alternative was the old
behaviour, which lost the messages outright.

**Nothing is dropped.** A single alert is unaffected, and work the adapter cannot
yet deliver stays queued in the manager — so a restart mid-burst loses nothing.

### Deleting a conversation leaves its topic

The adapter un-archives it, posts a tombstone, and closes it again — the
transcript above that line is what a person scrolls back to after an incident.

Opt out per surface where a busy group would rather not keep them.

### What the bundle renders

<!-- generated: renders bundle=telegram -->
```text
# Always
ChannelAdapter/telegram
SignalAdapter/telegram

# Surface  (surface.enabled)
Channel/k8s-ops
Deployment/agentops-gateway-telegram
Secret/k8s-ops-telegram
SignalSource/k8s-ops
```
<!-- /generated -->

- **The gateway is not an adapter.** It speaks no agent-ops contract, emits no
  signals and never contacts the manager, so it has no CR — the chart owns its
  Deployment directly.
- **The `Channel` and the `SignalSource` share one name**, so a pipeline binds
  the same word as its source and its channel.
- **No `Pipeline` is rendered at all**, which is the whole of why the wiring is
  yours.

## Going deeper

How a chat message becomes a conversation, and how replies reach every bound
thread: [concepts]({{ '/concepts.md' | relative_url }}).
