# Agent Ops Operator

A Kubernetes operator for **agents you can address**: monitoring signals and
direct chat tasks become **Conversations** — each pinned to its own chat topic,
executed by an isolated per-conversation agent pod, resumable across restarts,
approvable from your phone.

> Working name. API group `agentops.dev/v1alpha1` (provisional pre-1.0).

```
  Alertmanager ─┐                                        ┌─▶ Telegram topic per
  cron          ├─▶ SignalSource ─┐                      │   conversation
  k8s events ───┤                 │                      │   (reply = continue,
  kind: task ───┘                 ▼                      │    approve = act)
  from a script              Conversation CR ◀───────────┤
  /<agent> <task> ──▶       (queue of inputs)            │
  in chat                         │                      │
                                  ▼                      │
                          manager (this operator)        │
                          dispatches work units          │
                                  │ long-poll /work      │
                                  ▼                      │
                       agent runtime pod (per convo) ────┘
                       runs the agent CLI, streams
                       transcript to pod logs
```

## Concepts (CRDs)

Eleven kinds, one line each; the full reference is [docs/concepts.md](docs/concepts.md).

| Kind | What it defines |
|---|---|
| [`AgentProfile`](docs/concepts.md#agentprofile) | Who the agent is — repo, role file, credentials, limits. Carries no capabilities and selects no runtime. |
| [`AgentRuntime`](docs/concepts.md#agentruntime) | What executes it — image, idle TTL, home volume, service account. |
| [`Conversation`](docs/concepts.md#conversation) | One incident or task: chat topic + agent session + a serial queue of inputs. |
| [`ConversationInput`](docs/concepts.md#conversationinput) | Out-of-line payloads, so Conversation objects stay small in etcd. |
| [`Channel`](docs/concepts.md#channel) | A chat surface: where output goes. Type-agnostic metadata plus opaque config. |
| [`ChannelAdapter`](docs/concepts.md#channeladapter) | A channel implementation, plugged in as a CR whose name is the type key. |
| [`SignalSource`](docs/concepts.md#signalsource) | An ingest lane. Inert until a Pipeline claims it. |
| [`SignalAdapter`](docs/concepts.md#signaladapter) | A signal implementation — the inbound-only sibling of ChannelAdapter. |
| [`Pipeline`](docs/concepts.md#pipeline) | **The wiring**: sources × channels + profile + capabilities + what executes it and under whose identity. The only place any of them is declared. |
| [`MCPConfig`](docs/concepts.md#mcpconfig) | Reusable MCP server sets, bound per wiring. |
| [`MCPToolset`](docs/concepts.md#mcptoolset) | A named list of tool patterns — the allowlist half of a route's tools. |

## Behaviors that matter

- **One workflow: a signal originates, a channel carries.** Every Conversation
  starts from a signal on a `SignalSource` some Ready `Pipeline` claims — an
  alert, a cron job, or a person typing on chat, all one path.
- **Conversation = topic = session.** Replying in a topic resumes the same agent
  session; a repeat alert signature within its window reuses the conversation.
- **Per-conversation pods, on demand.** A pod spawns when work arrives, exits
  after its idle TTL, respawns with full context;
  `kubectl logs agentops-conv-<name> -f` streams the transcript.
- **[Bounded concurrency, queued not dropped](docs/concepts.md#capacity-how-many-run-at-once)** —
  over-cap work waits in phase `Pending` with nothing provisioned, admitted
  oldest-first.
- **Least privilege by construction.** The manager holds no cluster powers
  beyond its own CRDs plus pod lifecycle in its namespace, and never reads a
  Secret. No cluster CLI in the runtime image — reach is
  [wiring](docs/concepts.md#runtime-images-are-generic).
- **[See it, and answer it, on one screen](docs/console.md)** — the optional
  console draws the wiring as a graph and is itself a channel, so you can reply
  from the run you are watching.
- **Threads open with the event**, so a thread reads event → work → answer. The
  manager sends MEANING; each adapter renders its own surface.
- **Structured chat.** Built-in lane prompts embed a six-template message format
  spec — no stream-of-consciousness walls.

## Get started

A project-agnostic, **read-only k8s-engineer** agent — no chat, no repository,
no MCP setup. One credential, one flag:

```sh
kubectl create namespace agent-ops
kubectl -n agent-ops create secret generic agentops-claude \
  --from-literal=oauthToken=$(claude setup-token)   # or an Anthropic API key
helm install agent-ops ./chart -n agent-ops --create-namespace --set global.demo.enabled=true

# ask something — a task is an ordinary signal to a source a Pipeline claims
TOKEN=$(kubectl -n agent-ops get secret agentops-adapter-token -o jsonpath='{.data.token}' | base64 -d)
kubectl -n agent-ops run q --rm -i --image=curlimages/curl --restart=Never -- \
  curl -s -X POST http://agentops-manager.agent-ops.svc:8080/signal/inbound -H "Authorization: Bearer $TOKEN" \
  -d '{"source":"cluster-events","signals":[{"fingerprint":"ask-1","kind":"task","payload":"any pods crashlooping?"}]}'

kubectl -n agent-ops get conversations                  # watch it work
```

**[Getting started](https://kostiantyn-matsebora.github.io/agent-ops-operator/getting-started/)**
is the walkthrough: what each command is for, what a good run looks like, what to
check when nothing happens, and how to write your first route. Demo mode is
exactly [the k8s bundle with its defaults](docs/k8s-bundle.md) plus the read-only
RBAC it resolves — nothing demo-specific exists.

> **It also watches your cluster**, ingesting `Warning` events and answering them
> itself — LLM credits on a noisy cluster, with cooldown, grouping and the
> conversation cap as the bounds.

**The chart owns the substrate; bundles contribute domain** — `runtime:` renders
the `AgentRuntime` a Pipeline naming none executes on, so an install with no
bundle still works.
[Full reference](docs/concepts.md#the-substrate-runtime-and-globalagentopsruntime).
For alert ingestion, enable the [Prometheus bundle](docs/prometheus-bundle.md).

## Documentation

| | |
|---|---|
| [Documentation site](https://kostiantyn-matsebora.github.io/agent-ops-operator/) | The adopter hub: what this is, where to start, and every path onward |
| [Getting started](https://kostiantyn-matsebora.github.io/agent-ops-operator/getting-started/) | The read-only demo: install it and ask an agent about your cluster |
| [The console](https://kostiantyn-matsebora.github.io/agent-ops-operator/console/) | A tour of its six views, and how to decide who may reach it |
| [Installation](https://kostiantyn-matsebora.github.io/agent-ops-operator/installation/) | The real install: what to decide, what to configure, how to wire a route |
| [Guides](https://kostiantyn-matsebora.github.io/agent-ops-operator/introduction/#follow-the-guides) | Seven, in learning order: the wiring, an agent, its tools, your own ingest, chat surface and backend |
| [docs/concepts.md](docs/concepts.md) | Every CRD in full, and how a route's tools are resolved |
| [docs/cr-reference.md](docs/cr-reference.md) | Every field of every kind, generated from the CRDs the chart ships |
| [docs/contracts.md](docs/contracts.md) | The work contract, both adapter contracts, and the HTTP API |
| [docs/console.md](docs/console.md) | Console reference: its endpoints, RBAC grant, values and internals |
| [docs/k8s-bundle.md](docs/k8s-bundle.md) | Cluster events, the agent that answers them, Kubernetes MCP tooling |
| [docs/telegram-bundle.md](docs/telegram-bundle.md) | The Telegram ingest stack and chat surface |
| [docs/prometheus-bundle.md](docs/prometheus-bundle.md) | The Alertmanager alert lane and its metrics tooling |
| [docs/ha-bundle.md](docs/ha-bundle.md) | The Home Assistant lane, and its two agents split by privilege |
| [docs/CHANGELOG.md](docs/CHANGELOG.md) | Every chart-version upgrade guide, newest first |
| [.claude/rules/](.claude/rules/) | Working notes, one topic per file: terminology, wiring, invariants, and the gotchas that cost debugging |

## Development

**One directory per container, grouped by what it is at runtime** —
`platform/` `runtimes/` `signals/` `channels/` `gateways/`, with the operator at
`platform/manager/`. A component's published name comes from its path:
`signals/cron` is `agentops-signal-cron`, `platform/console` is
`agentops-console`.

See [.claude/rules/build-test.md](.claude/rules/build-test.md) for the
build/test workflow. In `platform/manager`, `go test ./...` covers unit
semantics (grouping, cooldown, dispatch, addressing, MCP compilation) and
envtest integration (real API server: lifecycle, alert routing, runtime
selection). Every other module is a `go build ./... && go vet ./... && go test
./...` of its own, discovered by `.github/components.sh modules`.

## Status

`v1alpha1` — young but running in production for its author. Roadmap: approve
buttons (inline keyboards), cron + k8s Events signal sources, custom metrics,
Helm chart. License TBD.
