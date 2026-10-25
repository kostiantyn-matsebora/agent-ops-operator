---
title: Claude
permalink: /runtimes/claude/
description: >-
  The reference runtime, and the one a fresh install executes on — claude-code
  driven through the Agent SDK, one session held open per pod.

next:
  eyebrow: Next
  title: Put an agent to work
  body: >-
    A Pipeline names what starts work, what it may reach, where it answers —
    and, with `runtimeRef`, which runtime executes it.
  url: /agent-ops-operator/guides/pipeline/
---

**This is a runtime, not an integration.** It starts no work and answers on no
surface. It is what EXECUTES an agent — the reference runtime, and the one a
fresh install runs on.

It conforms to the [runtime specification]({{ '/contracts/runtime/' | relative_url }}), version
2.0.0.

**With this bundle, agent-ops runs a route on claude-code.** Each conversation
gets a `runtime-claude` pod, and the manager hands it work. The pod drives
claude-code through `@anthropic-ai/claude-agent-sdk`, in process, holding one
session open across the pod's work units.

**ON by default, unlike every other bundle.** A route that names no
`runtimeRef` runs here, and nothing has to be enabled first — the one thing an
install supplies is the credential.

![The manager hands a work unit to runtime-claude, which drives claude-code through the Agent SDK in one session held open across units, keeps one transcript per conversation, and asks Claude only for the next message.]({{ '/assets/img/runtimes/claude-light.svg' | relative_url }}){: .ao-diagram}

**The same vendor ships twice.** This bundle holds a session open. The
[Claude CLI]({{ '/runtimes/claude-cli/' | relative_url }}) bundle is the
earlier behavior kept for compatibility — a fresh `claude` CLI process spawned
per work unit — and its page carries the side-by-side comparison.

## What you get

| You get | Which means |
|---|---|
| **A session held open** | the startup cost — replaying the transcript, launching every bound MCP server — is paid once per pod, then skipped, instead of on every work unit. |
| **The tools** | `Read`, `Grep`, `Glob`, `Edit`, `Write` and `Bash` are claude-code's own, plus every MCP server a Pipeline's `mcpConfigs` bind. |
| **Permissions enforced per call** | the route's allowlist is a `canUseTool` callback every tool call passes through, re-read fresh each time. |
| **Conversations that continue** | each conversation keeps its transcript, and a follow-up unit reuses the live session where nothing session-affecting changed. |
| **The default, by construction** | a Pipeline naming no `runtimeRef` resolves to `default`, which the parent renders as a copy of this runtime on a fresh install. |

## Turn it on

It already is. Supply the credential — a `claude setup-token` output or an
Anthropic API key — and the bundle renders its `Secret` beside the runtime:

```sh
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  -n agent-ops --create-namespace \
  --set claude.credentialsSecret.token=placeholder-token
```

```powershell
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
  -n agent-ops --create-namespace `
  --set claude.credentialsSecret.token=placeholder-token
```

**Everything else is inherited from `global.agentops.runtimeDefaults`.** Any
key those defaults accept is accepted under `claude:` and overrides them for
this runtime only. The ordinary install sets nothing else.

## Turning it off

```yaml
claude:
  enabled: false
```

**The render then FAILS if anything still resolves to `default`**, naming the
missing runtime and the routes that needed it. That is deliberate — the
alternative is conversations held `Pending` forever, with the reason in the
manager's log and nowhere an operator looks.

Two ways to satisfy it: declare a replacement runtime answering to the name
`default` under the parent's `runtimes:`, or give every route a `runtimeRef`
of its own. The check reads no cluster, so it protects a GitOps render exactly
as it protects an interactive one.

## What it needs

A Claude credential, and nothing else. The token lands in
`CLAUDE_CODE_OAUTH_TOKEN`, it has no default, and the bundle renders the
`Secret` only when one is supplied — see
[installation]({{ '/installation/' | relative_url }}).

## Where its context lives

claude-code's own transcripts, under the runtime's context directory. The
sidecar keeps the live context pod-local and snapshots it to the durable
volume, so a replacement pod resumes from disk — the same persistence
contract every runtime here uses.

## A tool call the model cannot FORM

A model writes its tool arguments as text, and that text is not always valid
JSON. When it is not, the call is **discarded before anything runs** — and
from outside, the run just looks busy.

- No MCP server sees the call, and no allowlist refuses it.
- No request leaves the pod.

The runtime therefore counts them:

| Situation | What happens |
|---|---|
| a call whose arguments do not parse | logged, and counted |
| the run ANSWERS having made some | the answer carries a line saying how many never ran, and which tool |
| the SAME tool, the SAME arguments, `RUNTIME_UNPARSED_REPEAT_LIMIT` times in a row (default 5) | the run is ended and reported **failed**, naming the tool and quoting what was written |

**Failing is the point of the breaker.** Without one, a spin ends as a
successful-looking run answered from whatever the session already held.

- Only consecutive, identical calls trip it — a model that varies its
  arguments is trying something.
- Set the limit to `0` to disable the breaker without disabling the counting.

## Deriving your own image

What an agent may REACH is wiring, so adding tooling to an image never grants
it anything. Point `claude.image` at your derived image, or declare a second
runtime beside this one under the parent's `runtimes:` and select it per
route.

### What the bundle renders

<!-- generated: renders bundle=claude -->

```text
# Always
AgentRuntime/claude
AgentRuntime/default
```

<!-- /generated -->

One `AgentRuntime`, named `claude`, through the parent chart's shared
renderer. Beside it: the credential `Secret` when a token is supplied, and
the `default` copy the parent renders.

The substrate stays the parent's: the floor ServiceAccount, the runtime
defaults every runtime inherits, the context volume. No RBAC.

## Going deeper

- [Claude CLI]({{ '/runtimes/claude-cli/' | relative_url }}) — the same
  vendor spawned per work unit, kept for compatibility.
- [Ollama]({{ '/runtimes/ollama/' | relative_url }}) — the local-model
  runtime, for the lanes nothing may leave.
- [GitHub Copilot]({{ '/runtimes/copilot/' | relative_url }}) — the
  vendor-SDK runtime, for the lanes a vendor's agent suits.
- [Run agents on your own backend]({{ '/guides/agent-runtime/' | relative_url }})
  — the contract this runtime implements, for writing your own.
- [Installation]({{ '/installation/' | relative_url }}) — the runtime
  defaults every runtime inherits, and the storage they share.
