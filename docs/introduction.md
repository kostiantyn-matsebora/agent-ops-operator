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

A signal starts a conversation, the wiring decides which agent answers and
what it may reach, and a runtime executes it — every kind below plays one
part in that sentence, and links to its spec page.

<figure class="ao-domain-model">
{% include domain-model.svg %}
<figcaption>Every kind, grouped so each cluster pair gets one
connector — an arrowhead only where something flows or opens, a plain line
where one kind merely references another.</figcaption>
</figure>

- **[SignalAdapter]({{ '/crds/signal-adapter/' | relative_url }})** — the
  workload serving a SignalSource. It watches one transport — a webhook, a
  poll, a schedule — and normalises what arrives into signals.

- **[SignalSource]({{ '/crds/signal-source/' | relative_url }})** — **what
  starts a conversation** — an alert receiver, a schedule, cluster events, a
  chat surface. One ingest lane, served by a SignalAdapter.

- **[AgentProfile]({{ '/crds/agent-profile/' | relative_url }})** — who the
  agent is and how it decides — the repository it works from, its role, its
  prompts, its limits. **No tools and no permissions**: an agent cannot grant
  itself reach.

- **[MCPToolset]({{ '/crds/mcp-toolset/' | relative_url }})** — the tools an
  agent may use, a named allowlist. One of the two walls around what an agent
  may touch.

- **[MCPConfig]({{ '/crds/mcp-config/' | relative_url }})** — the MCP servers
  behind those tools, each with its own credentials. The other wall.

- **[AgentRuntime]({{ '/crds/agent-runtime/' | relative_url }})** — **what
  executes an agent** — the image, the credential it uses, the identity it
  runs as. One per vendor and trust level, shared by every route.

- **[AgentCapability]({{ '/crds/agent-capability/' | relative_url }})** — **a
  named capability, declared once.** A Pipeline or a Coordinator references it
  by name instead of inlining the same fields. Carries no wiring of its own.

- **[Pipeline]({{ '/crds/pipeline/' | relative_url }})** — **the wiring for
  one agent** — the sources it listens on, the profile that answers, the tools
  that answer may use, the channels it answers on. To learn what one agent can
  do, read its Pipeline.

- **[Coordinator]({{ '/crds/coordinator/' | relative_url }})** — **wiring for
  a composition of agents, not one** — a root conversation that invokes named
  members by purpose, reads what each returns, and escalates to a human only
  when it decides to. A member may itself be a Coordinator.

- **[Conversation]({{ '/crds/conversation/' | relative_url }})** — **the
  running thing** — one incident, one task, one question. It holds the thread
  and the pod. The operator creates it. Look here to see what happened.

- **[ConversationInput]({{ '/crds/conversation-input/' | relative_url }})** —
  one large payload — a full alert, say — carried out of line, so the
  Conversation itself stays small.

- **[Channel]({{ '/crds/channel/' | relative_url }})** — **where an agent
  answers** — Telegram, the console, anything you write a ChannelAdapter for.
  Channels carry conversations. They never start one.

- **[ChannelAdapter]({{ '/crds/channel-adapter/' | relative_url }})** — the
  workload serving a Channel. It renders answers on its surface and carries
  what people reply back in.

**Pipeline and Coordinator share one capability shape** — the same six
fields, inline on either or factored out as an AgentCapability.

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
   — another vendor, your own harness. Two are already shipped:
   [Ollama]({{ '/runtimes/ollama/' | relative_url }}) and
   [GitHub Copilot]({{ '/runtimes/copilot/' | relative_url }}).
8. **[Compose agents]({{ '/guides/coordinate-agents/' | relative_url }})** —
   once one agent isn't enough. A `Coordinator` delegates to several, by
   purpose rather than by trigger.

Past what a guide needs:
[concepts.md](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md)
holds every kind in full, and each kind's spec page — every shape on the
diagram above — holds every field, generated from the CRDs the chart ships.
