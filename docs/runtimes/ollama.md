---
title: Ollama
permalink: /runtimes/ollama/
description: >-
  A local-model runtime. Your agents run on a model you host, over an Ollama
  endpoint you already run — and nothing they read leaves your cluster.

next:
  eyebrow: Next
  title: Put an agent to work
  body: >-
    A Pipeline names what starts work, what it may reach, where it answers —
    and, with `runtimeRef`, which runtime executes it.
  url: /agent-ops-operator/guides/pipeline/
---

**This is a runtime, not an integration.** It starts no work and answers on no
surface. It is what EXECUTES an agent — and the second one this chart ships.

**With this bundle, agent-ops runs a route on a model your own Ollama serves.**
Each conversation gets a `runtime-ollama` pod, and the manager hands it work.

The pod runs the agent loop, executes the tools and keeps the transcript. Your
Ollama server is asked for the next message, and nothing more.

Nothing an agent reads leaves your cluster.

A route opts in with `runtimeRef: ollama`. Every other route keeps running
where it did.

![The manager hands a work unit to runtime-ollama, which runs the agent loop and the tools itself, keeps one transcript per conversation, and asks Ollama only for the next message.]({{ '/assets/img/runtimes/ollama-light.svg' | relative_url }}){: .ao-diagram}

## What you get

| You get | Which means |
|---|---|
| **A model you host** | Every prompt, every tool result and every answer stays with your Ollama server. |
| **The same tools** | `Read`, `Grep`, `Glob`, `Edit`, `Write` and `Bash` are implemented in the runtime, so `agentops-observe`, `agentops-shell` and `agentops-edit` mean the same thing here as on the reference runtime. |
| **The same MCP servers** | Every `MCPConfig` a Pipeline binds is connected from the same `mcp.json`. |
| **Conversations that continue** | Each conversation keeps its transcript, so a follow-up sees what came before. |
| **A route-by-route choice** | `runtimeRef: ollama` on the Pipelines that suit a local model. The rest stay where they are. |

**The runtime is the harness.** Ollama is asked for the next message and
nothing else. The agent loop, tool dispatch and the transcript are the
runtime's own. That is why no manager or CRD change was needed to add it.

## Turn it on

**You need an Ollama server first, with a model pulled.** The bundle deploys
none. It points at one you run — in the cluster, on a GPU box, anywhere a pod
can reach.

```sh
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  -n agent-ops --create-namespace \
  --set ollama.enabled=true \
  --set ollama.endpoint=http://ollama.ollama.svc:11434 \
  --set ollama.model=qwen2.5:14b
```

```powershell
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
  -n agent-ops --create-namespace `
  --set ollama.enabled=true `
  --set ollama.endpoint=http://ollama.ollama.svc:11434 `
  --set ollama.model=qwen2.5:14b
```

**The endpoint is required.** Without it the render fails and names the key. A
runtime pointed at nothing would start, fail every run, and look like a broken
model.

**The model is optional when the server has one.** Ollama serves many models
and has no default of its own, so: one model pulled, and the runtime uses it.
Several pulled, and every run fails naming them until `ollama.model` says
which.

Then a route selects it:

```yaml
pipelines:
  - name: k8s-observe
    profile: k8s-engineer
    runtimeRef: ollama            # this route runs on the local model
    signalSources: [console]
    channels: [console]
    toolsets: [agentops-observe, k8s-observability]
    mcpConfigs: [k8s-api]
```

**Nothing else moves.** `default` stays a copy of the first configured
runtime — the reference one — so every route naming no `runtimeRef` runs where
it did.

**To make Ollama the default**, flag it: `ollama.default=true`. Every route
naming no `runtimeRef` then runs here, and the reference runtime is still
selectable as `runtimeRef: claude`.

**Without Claude.** Turn the reference bundle off (`claude.enabled=false`) and
this is the only runtime — so it is the default without the flag.

## What it needs

| Value | Is |
|---|---|
| `ollama.endpoint` | the server's URL, as a pod reaches it |
| `ollama.model` | a model already pulled there, by its Ollama name. Optional while the server has exactly one |
| `ollama.numCtx` | the context window, sent on every request. Default `8192` |
| `ollama.keepAlive` | how long the server keeps the model loaded after a run. Default `10m` |
| `ollama.env` | further knobs, as env entries — a bearer for a proxy in front of Ollama goes here, with `valueFrom` |

**Pick a model that can call tools.** Ollama reports it, and the runtime checks
at startup.

| The model | On a text-only route | On a route that grants tools |
|---|---|---|
| **can call tools** | runs | runs |
| **cannot** | runs | the run FAILS and names the model |

A Pipeline that grants tools to an agent that can never use them must not look
like a quiet agent.

**Sizes worth trying.** A 14B model with tool support handles the observe lane
well. 7B–8B models call tools less reliably and thrash more. Below that, keep
the route text-only.

## The tools it gives

Two sources, one allowlist:

| Source | Advertised as | From |
|---|---|---|
| **Built-in** | `Read`, `Grep`, `Glob`, `Edit`, `Write`, `Bash` | the runtime itself |
| **MCP** | `mcp__<server>__<tool>` | the servers the Pipeline's `mcpConfigs` bind |

**The gate is applied before the request.** Only allowed tools are advertised
to the model.

- A model that names one anyway gets a readable error, never an execution.
- An allowlist entry nothing here provides is logged as unavailable — visible
  in the run, not silently dropped.

**`agentops-shell` means the pod's shell.** With whatever the route's
`serviceAccountName` can reach. That is the same posture as the reference
runtime, and it is why the toolsets ship risk-split — see
[the agent's power]({{ '/installation/#the-agents-power' | relative_url }}).

## Where its context lives

One transcript per conversation, under the runtime's context directory. The
bundle declares it to
[context sync]({{ '/installation/#storage' | relative_url }}), so a run works
on pod-local storage and the durable volume holds a snapshot.

| The context | The run |
|---|---|
| **found** | continues, and says so |
| **slow to appear** | re-checked over a few seconds before it is believed gone |
| **gone** | FAILS with a readable reason. No answer is produced from an empty memory |

**Long conversations are trimmed, not lost.** When the transcript outgrows the
window, the oldest exchanges are left out of the request and the log says how
many. The transcript itself keeps everything.

## What it costs

| Fact | Consequence |
|---|---|
| **Ollama serialises work per model** | Five active conversations queue at the server. `OLLAMA_NUM_PARALLEL` on the server is the knob. |
| **A model unloads after `keepAlive`** | The first run after that pays a model load. Set it with `idleTtlMinutes` in mind. |
| **`numCtx` is memory on the server** | Raise it for a model that supports more. Never leave it to the server — Ollama's default drops the front of a long prompt silently. |
| **Small models answer less reliably** | The run is bounded by `maxTurns` and reports the limit. Choose the routes, per Pipeline. |
| **A shell is a shell** | Same as the reference runtime. Bind `agentops-shell` where a shell is wanted. |

## Choosing between the three

| | Claude Code | Ollama | GitHub Copilot |
|---|---|---|---|
| **Runs on** | Anthropic's API | a model you host | GitHub's API |
| **Data leaves the cluster** | yes | no | yes |
| **Agent quality** | the vendor's | the model's — an operator choice | the vendor's |
| **Tools** | claude-code's, plus MCP | the same six, natively, plus MCP | Copilot's, translated, plus MCP |
| **A scoped shell** | enforced by the CLI | grants nothing | enforced per call |
| **Context** | claude-code's transcripts | the runtime's own | Copilot's sessions |
| **Cost per run** | tokens | your hardware | credits |

**Any of them, on one install.** The hard lanes on a vendor, the routine lanes
local. Each Pipeline's `runtimeRef` is the whole decision.

### What the bundle renders

<!-- generated: renders bundle=ollama -->
```text
# Always
AgentRuntime/ollama
```
<!-- /generated -->

One `AgentRuntime`, named `ollama`, through the parent chart's shared renderer.
It inherits every release-wide default and carries only what names this vendor:
the image, the endpoint and model as environment, and its own context-sync
paths. No ServiceAccount, no volume, no credential.

## Going deeper

- [Run agents on your own backend]({{ '/guides/agent-runtime/' | relative_url }})
  — the contract this runtime implements, for writing a third.
- [GitHub Copilot]({{ '/runtimes/copilot/' | relative_url }}) — the vendor-SDK
  runtime, for the lanes a vendor's agent suits.
- [Installation]({{ '/installation/' | relative_url }}) — the runtime defaults
  every runtime inherits, and the storage they share.
- [`docs/claude.md`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/claude.md)
  — the reference runtime's page.
