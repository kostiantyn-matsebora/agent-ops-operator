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
| [`AgentProfile`](docs/concepts.md#agentprofile) | Who the agent is — repo, role file, credentials, limits. Carries no capabilities. |
| [`AgentRuntime`](docs/concepts.md#agentruntime) | What executes it — image, idle TTL, home volume, service account. |
| [`Conversation`](docs/concepts.md#conversation) | One incident or task: chat topic + agent session + a serial queue of inputs. |
| [`ConversationInput`](docs/concepts.md#conversationinput) | Out-of-line payloads, so Conversation objects stay small in etcd. |
| [`Channel`](docs/concepts.md#channel) | A chat surface: where output goes. Type-agnostic metadata plus opaque config. |
| [`ChannelAdapter`](docs/concepts.md#channeladapter) | A channel implementation, plugged in as a CR whose name is the type key. |
| [`SignalSource`](docs/concepts.md#signalsource) | An ingest lane. Inert until a Pipeline claims it. |
| [`SignalAdapter`](docs/concepts.md#signaladapter) | A signal implementation — the inbound-only sibling of ChannelAdapter. |
| [`Pipeline`](docs/concepts.md#pipeline) | **The wiring**: sources × channels + profile + capabilities. The only place capabilities are declared. |
| [`MCPConfig`](docs/concepts.md#mcpconfig) | Reusable MCP server sets, bound per wiring. |
| [`MCPToolset`](docs/concepts.md#mcptoolset) | A named list of tool patterns — the allowlist half of a route's tools. |

## Behaviors that matter

- **One workflow: a signal originates, a channel carries.** Every Conversation
  starts from a signal routed against a `SignalSource` some Ready `Pipeline`
  claims — an alert, a cron job, or a person typing on chat, all one path.
  Channels never start conversations, so "who answers this?" is always declared
  by a claim, never inferred.
- **Conversation = topic = session.** Replying in a topic resumes the same agent
  session; a repeat alert signature within its window reuses the conversation.
- **Per-conversation pods, on demand.** A runtime pod spawns when work arrives,
  stays warm across turns, exits after its idle TTL (default 1 min), respawns with
  full context. `kubectl logs agentops-conv-<name> -f` streams the transcript.
- **[Bounded concurrency, queued not dropped](docs/concepts.md#capacity-how-many-run-at-once).**
  At most `maxActiveConversations` (default 5) conversations hold a pod at once;
  over-cap work waits in phase `Pending` — nothing provisioned, not even the chat
  topic — and is admitted oldest-first. `/close` in a thread deletes the CR,
  archives every bound thread, and frees the slot.
- **Least privilege by construction.** The manager holds no cluster powers
  beyond its own CRDs + pod lifecycle in its namespace, and never touches agent
  credentials (all `valueFrom`). Agent powers are the runtime SA's RBAC.
- **See it, and answer it, on one screen.** The optional
  [console](docs/console.md) draws the wiring as a graph coloured by the
  conditions the reconcilers already write, plus live runs — and is itself a
  channel, so a pipeline listing it lets you reply from the run you are watching.
  Read-only SA; its token sees all that SA can read.
- **Structured chat.** Built-in lane prompts embed a six-template message format
  spec — no stream-of-consciousness walls. Custom prompts set their own.

## Try it in five minutes (demo advisor)

A project-agnostic, **read-only k8s-engineer** agent — no chat, no repository,
no MCP setup. One credential, one flag:

```sh
kubectl -n agent-ops create secret generic agentops-claude \
  --from-literal=oauthToken=$(claude setup-token)   # or an Anthropic API key
helm install agent-ops ./chart -n agent-ops --create-namespace --set global.demo.enabled=true

# ask something — a task is an ordinary signal to a source a Pipeline claims
TOKEN=$(kubectl -n agent-ops get secret agentops-adapter-token -o jsonpath='{.data.token}' | base64 -d)
kubectl -n agent-ops run q --rm -i --image=curlimages/curl --restart=Never -- \
  curl -s -X POST http://agentops-manager.agent-ops.svc:8080/signal/inbound -H "Authorization: Bearer $TOKEN" \
  -d '{"source":"cluster-events","signals":[{"fingerprint":"ask-1","kind":"task","payload":"any pods crashlooping?"}]}'

kubectl -n agent-ops get conversations                  # watch it work
kubectl -n agent-ops logs -f agentops-conv-<name>       # live agent transcript
kubectl -n agent-ops get conversation <name> -o jsonpath='{.status.runs[0].result}'
```

Demo mode is exactly **the k8s bundle with its defaults** plus the read-only
RBAC it resolves — nothing demo-specific exists any more. It binds only `view`
(+ node/namespace/metrics reads) to the release's one runtime SA: a pure
advisor, gated by `global.demo.enabled` and removable by flipping it off.

> **It also watches your cluster.** Unlike the pre-2.0 demo, this ingests
> `Warning` events and answers them itself (LLM credits on a noisy cluster;
> cooldown, grouping and the cap bound it). `--set
> k8s-bundle.eventsAdapter.enabled=false` restores ask-only.

## Install (current state)

```sh
helm install agent-ops ./chart -n agent-ops --create-namespace \
  --set persistence.enabled=true   # agent session continuity (PVC, RWX recommended;
                                   # off = sessions are ephemeral per runtime pod)
# CRDs install/upgrade with the release and carry helm.sh/resource-policy: keep —
# uninstall deletes neither your CRs nor the session PVC.
# Then your site config: profiles, Channel, SignalSource, pipelines (config/samples/)
```

**The chart owns the substrate; bundles contribute domain.** `runtime:` renders
the one `AgentRuntime` named `default` (image, credential, idle TTL, `home.pvcRef`
wired from `persistence`), so an install with no bundle — or only a chat bundle —
still executes conversations; `global.agentops.runtime.rbacMode` (`none` |
`readonly` | `full`, empty = readonly under demo) is the one knob for the agent's
in-cluster power. [Full reference](docs/concepts.md#the-substrate-runtime-and-globalagentopsruntime).

For alert ingestion, enable the [VictoriaMetrics bundle](docs/vm-bundle.md) and
point an Alertmanager-compatible sender at the adapter's webhook Service
(`continue: true` route).

## Documentation

| | |
|---|---|
| [docs/concepts.md](docs/concepts.md) | Every CRD in full, and how a route's tools are resolved |
| [docs/contracts.md](docs/contracts.md) | The work contract, both adapter contracts, and the HTTP API |
| [docs/console.md](docs/console.md) | The console: topology, live runs, and the channel it also is |
| [docs/k8s-bundle.md](docs/k8s-bundle.md) | Cluster events, the agent that answers them, Kubernetes MCP tooling |
| [docs/telegram-bundle.md](docs/telegram-bundle.md) | The Telegram ingest stack and chat surface |
| [docs/vm-bundle.md](docs/vm-bundle.md) | The VictoriaMetrics alert lane |
| [CHANGELOG.md](CHANGELOG.md) | Every chart-version upgrade guide, newest first |
| [CLAUDE.md](CLAUDE.md) | Working notes: terminology, invariants, the gotchas that cost debugging |

## Development

See [CLAUDE.md](CLAUDE.md) for build/test workflow. `go test ./...` covers unit
semantics (grouping, cooldown, dispatch, addressing, MCP compilation) and
envtest integration (real API server: lifecycle, alert routing, runtime selection).

## Status

`v1alpha1` — young but running in production for its author. Roadmap: approve
buttons (inline keyboards), cron + k8s Events signal sources, custom metrics,
Helm chart. License TBD.
