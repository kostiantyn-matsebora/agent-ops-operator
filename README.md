<div align="center">

<img src="docs/assets/img/logos/agent-ops.svg" width="72" alt="">

# agent-ops

**A Kubernetes operator for agents you can address.**

Something happens — an alert fires, a pod crashloops, someone asks. A
conversation opens, an agent works in its own pod, and answers in a thread you
can reply to.

<img src="docs/assets/img/claim-thinks.svg" width="18" alt=""> **Automation that thinks** &nbsp;·&nbsp;
<img src="docs/assets/img/logos/kubernetes.svg" width="18" alt=""> **Kubernetes-native** &nbsp;·&nbsp;
<img src="docs/assets/img/claim-gitops.svg" width="18" alt=""> **GitOps-ready** &nbsp;·&nbsp;
<img src="docs/assets/img/logos/claude.svg" width="18" alt=""> **Runs Claude Code** &nbsp;·&nbsp;
<picture><source media="(prefers-color-scheme: dark)" srcset="docs/assets/img/logos/ollama-dark.svg"><img src="docs/assets/img/logos/ollama-light.svg" width="18" alt=""></picture> **Runs Ollama**

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Docs](https://img.shields.io/badge/docs-agent--ops-informational)](https://kostiantyn-matsebora.github.io/agent-ops-operator/)
[![API](https://img.shields.io/badge/API-v1alpha1-blueviolet)](docs/cr-reference.md)

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

## When it runs

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

- **Your own signal source** — Datadog, Dynatrace, Sentry, a sensor on your bench.
- **Your own agent runtime** — swap the image; the work contract does not change.
- **Your own channel** — Slack, Teams, Discord, e-mail. The operator sends meaning,
  your adapter renders it.

## Why agent-ops?

The same wiring, wherever something needs looking at.

| Where | What happens |
|---|---|
| **Watch and fix your cluster** | It reads the events, the pods and the logs, and names the cause. |
| **Answer your alerts** | Every firing alert arrives with the investigation already done. |
| **Run the checks nobody gets to** | Certificates, drift and capacity, on a schedule. |
| **An assistant for your home** | Its logs, its devices, its config. Not everything is a cluster. |
| **Ask it from chat** | It answers in the thread where your team already talks. |
| **Plug in your own** | Three HTTP contracts: your source, your runtime, your channel. |

**And all of it in one place.** The [console](docs/console.md) ships enabled and
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
| [kubernetes](https://kostiantyn-matsebora.github.io/agent-ops-operator/integrations/kubernetes/) · [prometheus](https://kostiantyn-matsebora.github.io/agent-ops-operator/integrations/prometheus/) · [home-assistant](https://kostiantyn-matsebora.github.io/agent-ops-operator/integrations/home-assistant/) · [telegram](https://kostiantyn-matsebora.github.io/agent-ops-operator/integrations/telegram/) · [claude](docs/claude.md) · [ollama](https://kostiantyn-matsebora.github.io/agent-ops-operator/runtimes/ollama/) | Each integration: configure it, tune it, and bind its parts into your own pipelines — and the two runtimes |
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
