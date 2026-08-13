---
title: Something happens. An agent takes care of it.
eyebrows:
  - Automation that thinks
  - Kubernetes-native, end to end
lede: >-
  A signal wakes it, your prompt tells it what to do, your YAML decides what it
  may touch — a crashlooping pod or the hallway lights.
description: >-
  agent-ops is a Kubernetes operator for agents you can address: a signal wakes
  one, your wiring decides what it may touch, and it answers in a thread you can
  reply to.

diagram: agent-ops
diagram_label: How it works
diagram_alt: >-
  Something happens — an alert fires, a pod crashloops, a schedule comes due, a
  room gets too warm, someone asks. You declare it as custom resources: a
  Pipeline for what wakes it, an AgentProfile for what it should do, an
  MCPToolset for what it may touch. The operator runs it: one conversation per
  incident in its own thread, its own isolated pod, picking up where it stopped.

stats:
  - value: 11
    icon: kinds
    label: custom resource kinds
    note: kubectl, GitOps and RBAC already work.
  - value: 3
    icon: contracts
    label: pluggable contracts
    note: Signals, runtimes, channels.
  - value: 0
    icon: secrets
    label: Secrets the operator reads
    note: Everything secret-shaped is resolved by the kubelet, never read here.
  - value: 3
    icon: bundles
    label: ready-made bundles
    note: Kubernetes, Telegram, VictoriaMetrics — switch one on.
stats_kicker: One helm install, in your own cluster.
---

## What "takes care of it" means

- **Investigates** — queries the system, reads state.
- **Explains** — in a thread you can reply to.
- **Acts** — only where your wiring granted it.
- **Asks** — when it needs you, it says so.

## Why it is built this way

- **Judgment, not a fixed sequence.** You describe the job in prose. Nobody
  enumerates the steps, and nothing has to have been predicted in advance.
- **Self-hosted, end to end.** It runs in your cluster on your credentials. No
  prompt, transcript or alert is sent to a vendor.
- **Bounded by construction.** One isolated pod per conversation. Tools come
  only from the wiring, and the operator itself never reads a Secret.

## Pluggable at three seams

Documented HTTP contracts, no fork.

- **Your own signal source** — Datadog, Dynatrace, Sentry, a sensor on your
  bench.
- **Your own agent runtime** — Codex, Gemini, Copilot or a script of your own.
  Swap the image; the work contract does not change.
- **Your own channel** — Slack, Teams, Discord, e-mail. The operator sends
  meaning; your adapter renders it.

## Where to start

- **[What it is and how to install it](https://github.com/kostiantyn-matsebora/agent-ops-operator#readme)** —
  the pitch, the architecture, and a five-minute demo that needs one credential
  and one flag.
- **[The kinds you will declare](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md)** —
  every CRD in full, and how a route's tool access is resolved.

## Understand the model

- [Concepts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md)
  — profiles, runtimes, conversations, channels, signal sources and the
  Pipeline that wires them together.
- [Contracts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/contracts.md)
  — the work contract a runtime implements, both adapter contracts, and the
  HTTP API.

## Run it

- [The console](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/console.md)
  — the wiring as a graph, live runs, and the channel it also is.
- [Cluster events](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/k8s-bundle.md)
  — the events lane, the agent that answers it, and Kubernetes MCP tooling.
- [Telegram](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/telegram-bundle.md)
  — the ingest stack and the chat surface.
- [Prometheus](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/prometheus-bundle.md)
  — the Alertmanager alert lane, its metrics tooling and the agent that answers.

## Keep it current

- [CHANGELOG](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/CHANGELOG.md)
  — every chart-version upgrade guide, newest first.
- [The repository](https://github.com/kostiantyn-matsebora/agent-ops-operator)
  — source, issues, and the working notes contributors read first.

> The reference pages above are read on GitHub today. Bringing them onto this
> site — with navigation, cross-page links and their own contents — is the next
> change; this one lands the site itself.
