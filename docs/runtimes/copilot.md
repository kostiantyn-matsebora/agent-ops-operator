---
title: GitHub Copilot
permalink: /runtimes/copilot/
description: >-
  A GitHub Copilot runtime. Your agents run on Copilot through its SDK — the
  same Pipelines, the same toolsets, a different vendor.

next:
  eyebrow: Next
  title: Put an agent to work
  body: >-
    A Pipeline names what starts work, what it may reach, where it answers —
    and, with `runtimeRef`, which runtime executes it.
  url: /agent-ops-operator/guides/pipeline/
---

**This is a runtime, not an integration.** It starts no work and answers on no
surface. It is what EXECUTES an agent — and the third one this chart ships.

**With this bundle, agent-ops runs a route on GitHub Copilot.** Each
conversation gets a `runtime-copilot` pod, and the manager hands it work.

The pod drives Copilot's own agent through the Copilot SDK, in process. Copilot
runs the loop and the tools. The runtime decides what it may reach, keeps the
context, and reports the run.

A route opts in with `runtimeRef: copilot`. Every other route keeps running
where it did.

![The manager hands a work unit to runtime-copilot, which translates the route's tools and drives the Copilot SDK, and Copilot runs the agent and its tools under the runtime's permission callback.]({{ '/assets/img/runtimes/copilot-light.svg' | relative_url }}){: .ao-diagram}

## What you get

| You get | Which means |
|---|---|
| **The same tools** | `Read`, `Grep`, `Glob`, `Edit`, `Write` and `Bash` are translated into Copilot's own, so `agentops-observe`, `agentops-shell` and `agentops-edit` mean the same thing here as on the other runtimes. |
| **The same MCP servers** | Every `MCPConfig` a Pipeline binds is registered with Copilot from the same `mcp.json`, secrets resolved in the pod. |
| **A scoped shell, enforced** | `Bash(kubectl:*)` approves `kubectl …` and denies everything else, per invocation. |
| **Conversations that continue** | Each conversation is one Copilot session, under an id the runtime chose, so a follow-up sees what came before. |
| **A route-by-route choice** | `runtimeRef: copilot` on the Pipelines that suit it. The rest stay where they are. |

**The vendor's vocabulary never reaches your wiring.** Copilot names its tools
`view`, `bash`, `mcp:<server>-<tool>`. A Pipeline goes on binding `Read`,
`Bash`, `mcp__<server>__<tool>`. The translation happens inside the runtime,
once. That is why no manager, CRD or toolset change was needed to add it.

## Turn it on

**You need a GitHub token with Copilot access.** That is the one value with no
default.

```sh
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  -n agent-ops --create-namespace \
  --set copilot.enabled=true \
  --set copilot.credentialsSecret.token=placeholder-token
```

```powershell
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
  -n agent-ops --create-namespace `
  --set copilot.enabled=true `
  --set copilot.credentialsSecret.token=placeholder-token
```

**Supply the token and the bundle creates the Secret**, release-managed. Leave
it empty to reference a Secret you manage yourself — the runtime reads it as
`COPILOT_GITHUB_TOKEN`, and exits at start naming the variable when it is
missing.

Then a route selects it:

```yaml
pipelines:
  - name: k8s-observe
    profile: k8s-engineer
    runtimeRef: copilot           # this route runs on Copilot
    signalSources: [console]
    channels: [console]
    toolsets: [agentops-observe, k8s-observability]
    mcpConfigs: [k8s-api]
```

**Nothing else moves.** `default` stays a copy of the first configured
runtime — the reference one — so every route naming no `runtimeRef` runs where
it did.

**To make Copilot the default**, flag it: `copilot.default=true`. Every route
naming no `runtimeRef` then runs here, and the reference runtime is still
selectable as `runtimeRef: claude`.

**Without Claude.** Turn the reference bundle off (`claude.enabled=false`) and
this is the only runtime — so it is the default without the flag.

## What it needs

| Value | Is |
|---|---|
| `copilot.credentialsSecret.token` | a GitHub token with Copilot access. Set, the Secret is created. Empty, the named Secret is yours to create |
| `copilot.model` | the model, by Copilot's id. Optional: empty means Copilot's default for the account |
| `copilot.maxAiCredits` | a hard ceiling on what one run may spend, in Copilot AI credits. Optional |
| `copilot.env` | further knobs, as env entries — the run timeout, the SDK log level, a BYOK provider |

**`maxTurns` is not a Copilot control.** A Pipeline's profile bounds a claude
run and an ollama loop by turns. Copilot's nearest control is a credit budget.
The runtime logs the turns it was asked for and enforces `maxAiCredits`
instead. Without it, a run is bounded by its timeout (`COPILOT_RUN_TIMEOUT_S`,
default an hour) and the pod's idle TTL.

## The tools it gives

One allowlist, translated into two Copilot layers:

| The route binds | Copilot gets | And each call is |
|---|---|---|
| `Read`, `Grep`, `Glob` | `view`, `grep`, `glob` | approved |
| `Edit`, `Write` | `edit`, `create` | approved |
| `Bash` | `bash` | approved, any command |
| `Bash(kubectl:*)` | `bash` | approved only when the command is `kubectl …` |
| `mcp__<server>__<tool>` | `mcp:<server>-<tool>` | approved |
| anything else | nothing | denied, and logged |

**An empty allowlist stays empty.** Copilot reads an agent definition with no
`tools:` as "every tool". This runtime passes the composed list explicitly,
even when it is empty, so the vendor's default never applies.

**A denial is an answer, not a hang.** Every permission request is decided
from the allowlist in the pod, and the model is told why — nothing waits on a
prompt nobody can answer.

- A pattern with no Copilot equivalent is withheld and logged. It is never
  passed through as a string Copilot might read as some other tool.
- `mcp__<server>__*` is refused. Copilot admits every MCP server or an exact
  tool name, never one server's tools — and "every server" would grant more
  than the route bound.
- A scoped shell pattern is enforced per call: a command outside it, or one
  carrying `;`, `|`, `&&` or a substitution, is denied.

**The agent definition is `.github/agents/<agent>.agent.md`.** That is where
Copilot keeps one, so that is what this runtime reads — the same frontmatter
shapes, the same `merge` / `overwrite` composition as the reference runtime,
and an absent or unreadable file declares nothing.

**`agentops-shell` means the pod's shell.** With whatever the route's
`serviceAccountName` can reach. That is the same posture as the other
runtimes, and it is why the toolsets ship risk-split — see
[the agent's power]({{ '/installation/#the-agents-power' | relative_url }}).

## Where its context lives

One Copilot session per conversation, under an id the runtime minted, in the
SDK's own session store beneath the runtime's context directory. The bundle
declares it to [context sync]({{ '/installation/#storage' | relative_url }}),
so a run works on pod-local storage and the durable volume holds a snapshot.

| The context | The run |
|---|---|
| **found** | continues, and says so |
| **slow to appear** | re-checked over a few seconds before it is believed gone |
| **gone** | FAILS with a readable reason. No answer is produced from an empty memory |

**The handle is minted, never derived.** Encoding the conversation's name into
it would make it reproducible — and then a lost context could never be replaced
by a fresh one, which is the failure the manager's latest-wins rule exists to
undo.

## What it costs

| Fact | Consequence |
|---|---|
| **Copilot bills in credits** | `maxAiCredits` is the ceiling per run. Without it, the run timeout is. |
| **Every prompt reaches GitHub** | The checkout, the tool results and the answer leave the cluster. Route accordingly. |
| **The session store is the SDK's** | A SQLite database and an event log per session, snapshotted by context sync. The runtime interprets none of it. |
| **A shell is a shell** | Same as the other runtimes. Bind `agentops-shell` where a shell is wanted, and scope it where a scope is enough. |

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

**Any of them, on one install.** Each Pipeline's `runtimeRef` is the whole
decision.

### What the bundle renders

<!-- generated: renders bundle=copilot -->
```text
# Always
AgentRuntime/copilot
```
<!-- /generated -->

One `AgentRuntime`, named `copilot`, through the parent chart's shared
renderer. It inherits every release-wide default and carries only what names
this vendor: the image, the credential as environment, the model and the
credit ceiling as environment, and its own context-sync paths. No
ServiceAccount, no volume. The credential Secret renders only when a token was
supplied.

## Going deeper

- [Run agents on your own backend]({{ '/guides/agent-runtime/' | relative_url }})
  — the contract this runtime implements, for writing a fourth.
- [Ollama]({{ '/runtimes/ollama/' | relative_url }}) — the local-model
  runtime, for the lanes that must not leave the cluster.
- [Installation]({{ '/installation/' | relative_url }}) — the runtime defaults
  every runtime inherits, and the storage they share.
- [`docs/claude.md`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/claude.md)
  — the reference runtime's page.
