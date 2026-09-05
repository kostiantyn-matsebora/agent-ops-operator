<div align="center">

<img src="docs/assets/img/logos/agent-ops.svg" width="72" alt="">

# agent-ops

**A Kubernetes operator for LLM agents within boundaries you declare, cluster you control and conversations you stay in.**

Something happens — an alert fires, a pod crashloops, someone asks. A
conversation opens, an agent works in its own pod, and answers in a thread you
can reply to.

<img src="docs/assets/img/claim-thinks.svg" width="18" alt=""> **Automation that thinks** &nbsp;·&nbsp;
<img src="docs/assets/img/logos/kubernetes.svg" width="18" alt=""> **Kubernetes native** &nbsp;·&nbsp;
<img src="docs/assets/img/claim-gitops.svg" width="18" alt=""> **GitOps ready**

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Docs](https://img.shields.io/badge/docs-agent--ops-informational)](https://kostiantyn-matsebora.github.io/agent-ops-operator/)
[![API](https://img.shields.io/badge/API-v1alpha1-blueviolet)](docs/cr-reference.md)
[![Chart](https://img.shields.io/github/v/tag/kostiantyn-matsebora/agent-ops-operator?filter=chart-v*&label=chart)](docs/CHANGELOG.md)
[![CI](https://github.com/kostiantyn-matsebora/agent-ops-operator/actions/workflows/ci.yml/badge.svg?branch=master)](https://github.com/kostiantyn-matsebora/agent-ops-operator/actions/workflows/ci.yml)
[![Published images](https://github.com/kostiantyn-matsebora/agent-ops-operator/actions/workflows/image-scan.yml/badge.svg?branch=master)](https://github.com/kostiantyn-matsebora/agent-ops-operator/actions/workflows/image-scan.yml)
[![E2E nightly](https://github.com/kostiantyn-matsebora/agent-ops-operator/actions/workflows/e2e-full.yml/badge.svg)](https://github.com/kostiantyn-matsebora/agent-ops-operator/actions/workflows/e2e-full.yml)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=kostiantyn-matsebora_agent-ops-operator_manager&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=kostiantyn-matsebora_agent-ops-operator_manager)

### **→ [The documentation site](https://kostiantyn-matsebora.github.io/agent-ops-operator/) is the main source of information.**

This page is the short version of it.

</div>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/img/readme-flow-dark.svg">
  <img alt="Left to right. SOMETHING HAPPENS: an alert fires (Alertmanager), a pod crashloops (cluster events), a schedule comes due (cron), someone asks (chat). YOU DECLARE IT, ONE PIPELINE — the only place wiring lives, naming what starts it (signalSourceRefs), what it should do (profileRef), what it may touch (toolsets and mcpConfigs) and where it answers (channelRefs). THE OPERATOR RUNS IT: a Conversation, one per incident, resumable, with its own thread; and its own agent pod — isolated, serial, capped — which investigates, explains and acts ONLY where your wiring granted it. YOU ANSWER on your channels: Telegram, the console, your own adapter; and when you reply, the same conversation continues." src="docs/assets/img/readme-flow-light.svg" width="1000">
</picture>

<sup>The whole story at page scale, as one drawing: [light](docs/assets/img/agent-ops-light.svg) ·
[dark](docs/assets/img/agent-ops-dark.svg) — or [watch it happen](https://kostiantyn-matsebora.github.io/agent-ops-operator/#tour).</sup>

**Works with**
<img src="docs/assets/img/logos/kubernetes.svg" width="18" alt=""> Kubernetes &nbsp;·&nbsp;
<img src="docs/assets/img/logos/prometheus.svg" width="18" alt=""> Prometheus &nbsp;·&nbsp;
cron schedules &nbsp;·&nbsp;
<img src="docs/assets/img/logos/home-assistant.svg" width="18" alt=""> Home Assistant &nbsp;·&nbsp;
<img src="docs/assets/img/logos/telegram.svg" width="18" alt=""> Telegram &nbsp;·&nbsp;
<img src="docs/assets/img/logos/agent-ops.svg" width="18" alt=""> the console &nbsp;·&nbsp;
<picture><source media="(prefers-color-scheme: dark)" srcset="docs/assets/img/logos/ollama-dark.svg"><img src="docs/assets/img/logos/ollama-light.svg" width="18" alt=""></picture> Ollama &nbsp;·&nbsp;
<picture><source media="(prefers-color-scheme: dark)" srcset="docs/assets/img/logos/github-dark.svg"><img src="docs/assets/img/logos/github-light.svg" width="18" alt=""></picture> GitHub Copilot &nbsp;·&nbsp;
any MCP server &nbsp;·&nbsp; your own

## How it works

1. **Something happens.** An alert fires, a pod crashloops, a schedule comes due,
   a room gets too warm, someone asks a question in chat.
2. **One Helm install puts an agent in the path** — your cluster, your credentials.
3. **You declare the route. One `Pipeline`** — what starts it, what it should
   do, what it may touch, which servers those tools come from, and where you
   talk to it.
4. **Then it runs.** One conversation per incident, in its own isolated pod,
   strictly serial and capped. A restart loses nothing.
5. **Every part of it is a Kubernetes object.** `kubectl get conversations`.

## What you write

One `Pipeline`. It is the whole route, and it is the only place wiring lives.

```yaml
apiVersion: agentops.dev/v1alpha1
kind: Pipeline
metadata:
  name: k8s-ops
spec:
  signalSourceRefs:
    - name: cluster-events      # what starts it
  profileRef:
    name: k8s-engineer          # what it should do
  toolsets:
    refs:
      - name: agentops-observe  # what it may touch
  mcpConfigs:
    refs:
      - name: k8s-api           # where those tools live
  channelRefs:
    - name: telegram            # where you talk to it
```

## What agent-ops is

agent-ops gives you a platform that puts an LLM agent behind the systems you
already run. One Helm install and your cluster grows a brain and a pair of hands.

agent-ops is more than a model and a prompt in a pod: grouping and cooldown, a
capacity cap with a queue, context that survives a restart, per-route cluster
identity, mediated egress and at-least-once delivery are already in it.

- **Kubernetes native** — eleven custom resources, validated by the API server
  like anything else you deploy.
- **GitOps ready** — every route is text. It reviews as a diff and deploys
  through the pipeline you already run.
- **Open at three seams** — your own signal source, runtime or channel.
  Documented HTTP contracts, no fork.
- **Any model you can run** — Claude Code, Ollama and GitHub Copilot ship with
  it. Point it at your own image and the work contract is unchanged.
- **A pod per conversation** — isolated, serial and capped.
- **Tools that come from the wiring** — not from the prompt. An agent holds the
  cluster identity its route named and nothing else.

## What it gives you

| | |
|---|---|
| **A cluster that explains itself** | It reads the events, the pods and the logs, names the cause, and fixes what you allowed it to. |
| **Alerts that arrive investigated** | Every firing alert turns up with the work already done. |
| **The checks nobody gets to** | Certificates, drift and capacity, on a schedule. |
| **An assistant for your home** | Its logs, its devices, its config. Not everything is a cluster. |
| **An answer where your team already talks** | Ask it in chat and reply in the thread it answers in. |
| **Routine work on a model you host** | Route the lanes that run all day to a local model. Nothing they read ever leaves. |

**Console — all in one place.** The [console](docs/console.md) ships enabled and
[read-only on your cluster](docs/console.md) — every conversation as it happens,
what is queued, what is stuck and why, and the whole wiring as a graph. It is a
channel too, so you answer the agent right there.
**[Watch one signal, start to finish](https://kostiantyn-matsebora.github.io/agent-ops-operator/#tour)** — a minute, no sound.

## The kinds you declare

Eleven, one line each. [Every field, in full](docs/concepts.md).

| Kind | What it defines |
|---|---|
| [`AgentProfile`](docs/concepts.md#agentprofile) | Who the agent is — repo, role file, credentials, limits. Carries no capabilities and selects no runtime. |
| [`AgentRuntime`](docs/concepts.md#agentruntime) | What executes it — image, idle TTL, default identity. One per VENDOR, declared by the install or by a bundle. |
| [`Conversation`](docs/concepts.md#conversation) | One incident or task: chat topic + agent session + a serial queue of inputs. |
| [`ConversationInput`](docs/concepts.md#conversationinput) | Out-of-line payloads, so Conversation objects stay small in etcd. |
| [`Channel`](docs/concepts.md#channel) | A chat surface: where output goes. Type-agnostic metadata plus opaque config. |
| [`ChannelAdapter`](docs/concepts.md#channeladapter) | A channel implementation, plugged in as a CR whose name is the type key. |
| [`SignalSource`](docs/concepts.md#signalsource) | An ingest lane. Inert until a Pipeline claims it. |
| [`SignalAdapter`](docs/concepts.md#signaladapter) | A signal implementation — the inbound-only sibling of ChannelAdapter. |
| [`Pipeline`](docs/concepts.md#pipeline) | **The wiring**: sources × channels + profile + capabilities + what executes it and under whose identity. The only place any of them is declared. |
| [`MCPConfig`](docs/concepts.md#mcpconfig) | Reusable MCP server sets, bound per wiring. |
| [`MCPToolset`](docs/concepts.md#mcptoolset) | A named list of tool patterns — the allowlist half of a route's tools. |

## Try it

A read-only **k8s-engineer** agent — no chat, no repository, no MCP setup. One
credential, one flag, no clone:

```sh
kubectl create namespace agent-ops
kubectl -n agent-ops create secret generic agentops-claude \
  --from-literal=oauthToken=$(claude setup-token)   # or an Anthropic API key

helm install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  -n agent-ops --set global.demo.enabled=true
```

**[Getting started](https://kostiantyn-matsebora.github.io/agent-ops-operator/getting-started/)**
is the walkthrough: how to ask it something, what a good run looks like, and what
to check when nothing happens.

> **Demo mode also watches your cluster**, ingesting `Warning` events and
> answering them itself — LLM credits on a noisy cluster, bounded by cooldown,
> grouping and the conversation cap.

## Where to go next

**The [site](https://kostiantyn-matsebora.github.io/agent-ops-operator/) first**:
the [Introduction](https://kostiantyn-matsebora.github.io/agent-ops-operator/introduction/),
the [console tour](https://kostiantyn-matsebora.github.io/agent-ops-operator/console/),
[Installation](https://kostiantyn-matsebora.github.io/agent-ops-operator/installation/)
and [seven guides](https://kostiantyn-matsebora.github.io/agent-ops-operator/introduction/#follow-the-guides)
in learning order. The reference pages are read here:

| | |
|---|---|
| [docs/concepts.md](docs/concepts.md) | Every CRD in full, capacity and queueing, and how a route's tools resolve |
| [docs/cr-reference.md](docs/cr-reference.md) | Every field of every kind, generated from the CRDs the chart ships |
| [docs/contracts.md](docs/contracts.md) | The work contract, both adapter contracts, and the HTTP API |
| [docs/console.md](docs/console.md) | Console reference: its endpoints, RBAC grant, values and internals |
| [kubernetes](https://kostiantyn-matsebora.github.io/agent-ops-operator/integrations/kubernetes/) · [prometheus](https://kostiantyn-matsebora.github.io/agent-ops-operator/integrations/prometheus/) · [home-assistant](https://kostiantyn-matsebora.github.io/agent-ops-operator/integrations/home-assistant/) · [telegram](https://kostiantyn-matsebora.github.io/agent-ops-operator/integrations/telegram/) · [claude](docs/claude.md) · [ollama](https://kostiantyn-matsebora.github.io/agent-ops-operator/runtimes/ollama/) · [copilot](https://kostiantyn-matsebora.github.io/agent-ops-operator/runtimes/copilot/) | Each integration: configure it, tune it, and bind its parts into your own pipelines — and the three runtimes |
| [Security](https://kostiantyn-matsebora.github.io/agent-ops-operator/security/) | What a default install grants, what each of the three walls bounds, what is not addressed |
| [docs/CHANGELOG.md](docs/CHANGELOG.md) · [docs/adr/](docs/adr/) | Upgrade guides newest first, and decisions with the alternatives that were built |

## Contributing

**[CONTRIBUTING.md](CONTRIBUTING.md)** states the workflow, which cannot be
inferred from the tree: changes are planned as specifications in `openspec/`, and
documentation is part of a change rather than a follow-up.
[`.claude/rules/`](.claude/rules/) is the working context, one topic per file.
**One directory per container** — `platform/` `runtimes/` `signals/` `channels/`
`gateways/` — and the path is the published image name; the operator is
`platform/manager/`.

[SECURITY.md](SECURITY.md): report privately, never in an issue.
[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md): the Contributor Covenant.

## Status

**`v1alpha1` — young, and running in production for its author.** The API group is
provisional and may be renamed before 1.0; breaking chart changes carry an upgrade
entry in [docs/CHANGELOG.md](docs/CHANGELOG.md).

© 2026 [kostiantyn-matsebora](https://github.com/kostiantyn-matsebora). Licensed
under the [Apache License 2.0](LICENSE).
