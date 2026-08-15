---
title: Introduction
permalink: /introduction/
description: >-
  The parts agent-ops is built from — what wakes an agent, what decides what it
  may touch, what executes it — and the guides that take you from there.
---

agent-ops runs LLM agents in your cluster as ordinary Kubernetes objects.

Something happens — an alert fires, a pod crashloops, someone asks a question.
An agent wakes up, works in its own pod, and answers in a thread you can reply
to.

{: .ao-callout}
> **Identity, wiring and execution are three different objects.** An agent does
> not carry its own permissions.

That is what separates this from a chatbot or a runbook. **What an agent may
reach comes from the route that woke it.** The same agent, woken by two
different signals, has two different reaches.

## Understand the concepts

{: .ao-cards}
- [AgentProfile](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md#agentprofile)
  {: .ao-icon-profile}

  Who the agent is — the repository it checks out, its role, its prompts, its
  limits. **Identity only**: no tools, no permissions. An agent cannot grant
  itself reach.

- [SignalSource](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md#signalsource)
  {: .ao-icon-source}

  **What wakes an agent** — an alert receiver, a schedule, cluster events, a
  chat surface. A SignalAdapter serves it and normalises what it watches.

- [MCPToolset and MCPConfig](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md#mcptoolset)
  {: .ao-icon-toolset}

  **What an agent may touch** — the tools it may use, and the servers behind
  them. Two walls: the server's own credentials, and the allowlist on top.

- [Channel](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md#channel)
  {: .ao-icon-channel}

  **Where an agent answers** — Telegram, the console, anything you write a
  ChannelAdapter for. Channels carry conversations. They never start one.

- [AgentRuntime](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md#agentruntime)
  {: .ao-icon-runtime}

  **What executes an agent** — the image, the credential it uses, the identity
  it runs as. One per vendor and trust level, shared by every route.

- [Conversation](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md#conversation)
  {: .ao-icon-conversation}

  **The running thing** — one incident, one task, one question. It holds the
  thread and the pod. Look here to see what happened.

- [Pipeline](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md#pipeline)
  {: .ao-icon-pipeline}

  **The wiring, and the only object that carries any** — the sources it listens
  on, the profile that answers, the tools that answer may use, the channels it
  answers on. To learn what an agent can do, read its Pipeline. There is nowhere
  else to look.

## Follow the guides

Guides are being written. **There are none yet.**
