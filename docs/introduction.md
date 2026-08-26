---
title: Introduction
permalink: /introduction/
description: >-
  The parts agent-ops is built from — what starts a conversation, what decides what it
  may touch, what executes it — and the guides that take you from there.

next:
  eyebrow: Next
  title: Try it in fifteen minutes
  body: >-
    Install the read-only demo, ask an agent about your cluster from the
    console, and watch a first run happen.
  url: /agent-ops-operator/getting-started/
---

agent-ops runs LLM agents in your cluster as ordinary Kubernetes objects.

Something happens — an alert fires, a pod crashloops, someone asks a question.
A conversation opens, an agent works in its own pod, and answers in a thread
you can reply to.

{: .ao-callout}
> **Identity, wiring and execution are three different objects.** An agent does
> not carry its own permissions.

That is what separates this from a chatbot or a runbook. **What an agent may
reach comes from the route that started it.** The same agent, reached by two
different signals, has two different reaches.

## Understand the concepts

Seven kinds. Each card links to the guide that teaches it.

{: .ao-cards}
- [AgentProfile]({{ '/guides/agent-profile/' | relative_url }})
  {: .ao-icon-profile}

  Who the agent is and how it decides — the repository it works from, its role,
  its prompts, its limits. **No tools and no permissions**: an agent cannot
  grant itself reach.

- [SignalSource]({{ '/guides/signal-adapter/' | relative_url }})
  {: .ao-icon-source}

  **What starts a conversation** — an alert receiver, a schedule, cluster
  events, a chat surface. A SignalAdapter serves it and normalises what it
  watches.

- [MCPToolset and MCPConfig]({{ '/guides/toolsets/' | relative_url }})
  {: .ao-icon-toolset}

  **What an agent may touch** — the tools it may use, and the servers behind
  them. Two walls: the server's own credentials, and the allowlist on top.

- [Channel]({{ '/guides/channel-adapter/' | relative_url }})
  {: .ao-icon-channel}

  **Where an agent answers** — Telegram, the console, anything you write a
  ChannelAdapter for. Channels carry conversations. They never start one.

- [AgentRuntime]({{ '/guides/agent-runtime/' | relative_url }})
  {: .ao-icon-runtime}

  **What executes an agent** — the image, the credential it uses, the identity
  it runs as. One per vendor and trust level, shared by every route.

- [Conversation](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md#conversation)
  {: .ao-icon-conversation}

  **The running thing** — one incident, one task, one question. It holds the
  thread and the pod. The operator creates it, so there is no guide to writing
  one. Look here to see what happened.

- [Pipeline]({{ '/guides/pipeline/' | relative_url }})
  {: .ao-icon-pipeline}

  **The wiring, and the only object that carries any** — the sources it listens
  on, the profile that answers, the tools that answer may use, the channels it
  answers on. To learn what an agent can do, read its Pipeline. There is nowhere
  else to look.

## Follow the guides

The same objects, in the order they build on each other. **The order is what you
must understand, not what you can break** — every guide states what its own
mistake costs, and the earliest one is not the smallest.

1. **[Put an agent to work]({{ '/guides/pipeline/' | relative_url }})** — start
   here. It creates nothing: everything a route names, your install already has.
2. **[Add your own agent]({{ '/guides/agent-profile/' | relative_url }})** —
   when none of the installed agents is the one you want.
3. **[Run your agent from a repository]({{ '/guides/agent-from-a-repository/' | relative_url }})**
   — once the role text outgrows a string in a resource.
4. **[Give your agent tools]({{ '/guides/toolsets/' | relative_url }})** — so it
   can do more than talk.
5. **[React to signals]({{ '/guides/signal-adapter/' | relative_url }})** — for a
   transport nothing here serves yet.
6. **[Talk to agents from your own chat]({{ '/guides/channel-adapter/' | relative_url }})**
   — the same, for the surface people type on.
7. **[Run agents on your own backend]({{ '/guides/agent-runtime/' | relative_url }})**
   — another vendor, your own harness. A local model is already shipped:
   [Ollama]({{ '/runtimes/ollama/' | relative_url }}).

Past what a guide needs:
[concepts.md](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md)
holds every kind in full, and
[cr-reference.md](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/cr-reference.md)
holds every field of every kind, generated from the CRDs the chart ships.
