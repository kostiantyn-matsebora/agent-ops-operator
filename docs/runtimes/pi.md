---
title: pi
permalink: /runtimes/pi/
description: >-
  A pi runtime. Your agents run on the pi coding agent SDK, through whichever
  model provider you choose — the same Pipelines, the same toolsets, a
  different vendor.

next:
  eyebrow: Next
  title: Put an agent to work
  body: >-
    A Pipeline names what starts work, what it may reach, where it answers —
    and, with `runtimeRef`, which runtime executes it.
  url: /agent-ops-operator/guides/pipeline/
---

**This is a runtime, not an integration.** It starts no work and answers on no
surface. It is what EXECUTES an agent — one of the runtimes this chart
ships.

It conforms to the [runtime specification]({{ '/contracts/runtime/' | relative_url }}), version
2.0.2.

**With this bundle, agent-ops runs a route on pi.** Each conversation gets a
`runtime-pi` pod, and the manager hands it work.

The pod drives the [`@earendil-works/pi-coding-agent`](https://www.npmjs.com/package/@earendil-works/pi-coding-agent)
SDK, in process. Pi runs the loop and the tools, against whichever model
provider you configured — unlike every other runtime here, pi is not tied to
one vendor's account.

A route opts in with `runtimeRef: pi`. Every other route keeps running where
it did.

![The manager hands a work unit to runtime-pi, which resumes a pi SDK session by its own minted id, checks every tool call against a deny-by-default hook, and reaches bound MCP servers through its own pi-mcp client.]({{ '/assets/img/runtimes/pi-light.svg' | relative_url }}){: .ao-diagram}

## What you get

| You get | Which means |
|---|---|
| **A choice of model provider** | `pi.provider` + `pi.model` pick pi's own provider and model id. One SDK, many backends. |
| **The same tools** | `Read`, `Grep`, `Glob`, `Edit`, `Write` and `Bash` are translated into pi's own built-ins, so `agentops-observe`, `agentops-shell` and `agentops-edit` mean the same thing here as on the other runtimes. |
| **The same MCP servers** | Every `MCPConfig` a Pipeline binds is connected from the same `mcp.json`, through pi's own MCP client, secrets resolved in the pod. |
| **Permissions enforced per call** | the route's allowlist is a `tool_call` hook every call passes through, deny by default. |
| **Conversations that continue** | each context gets its own minted session id, and a follow-up unit resumes the same session from disk. |
| **A route-by-route choice** | `runtimeRef: pi` on the Pipelines that suit it. The rest stay where they are. |

**The vendor's vocabulary never reaches your wiring.** Pi names its built-in
tools `read`, `write`, `edit`, `bash`, `grep`, `find`. A Pipeline goes on
binding `Read`, `Bash`, `mcp__<server>__<tool>`. The translation happens
inside the runtime, once.

## Turn it on

**You need a model provider and a credential it accepts.** Those are the
values with no default.

```sh
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  -n agent-ops --create-namespace \
  --set pi.enabled=true \
  --set pi.provider=anthropic \
  --set pi.model=claude-sonnet-5 \
  --set pi.credentialsSecret.token=placeholder-token
```

```powershell
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
  -n agent-ops --create-namespace `
  --set pi.enabled=true `
  --set pi.provider=anthropic `
  --set pi.model=claude-sonnet-5 `
  --set pi.credentialsSecret.token=placeholder-token
```

**Supply the token and the bundle creates the Secret**, release-managed. Leave
it empty to reference a Secret you manage yourself — the runtime reads it as
`PI_API_KEY`, and exits at start naming the variable when it is missing.

Then a route selects it:

```yaml
pipelines:
  - name: release-notes
    profile: k8s-engineer
    runtimeRef: pi              # this route runs on pi
    signalSources: [console]
    channels: [console]
    toolsets: [agentops-observe, k8s-observability]
    mcpConfigs: [k8s-api]
```

**Nothing else moves.** `default` stays a copy of the first configured
runtime — the reference one — so every route naming no `runtimeRef` runs where
it did.

**To make pi the default**, flag it: `pi.default=true`. Every route naming no
`runtimeRef` then runs here, and the reference runtime is still selectable as
`runtimeRef: claude`.

**Without Claude.** Turn the reference bundle off (`claude.enabled=false`) and
this is the only runtime — so it is the default without the flag.

## What it needs

| Value | Is |
|---|---|
| `pi.provider` | the model provider, by pi's own provider id (e.g. `anthropic`). REQUIRED once enabled — the render fails naming this key otherwise |
| `pi.model` | the model, by the chosen provider's own id (e.g. `claude-sonnet-5`). REQUIRED once enabled, for the same reason |
| `pi.credentialsSecret.token` | the provider's credential. Set, the Secret is created. Empty, the named Secret is yours to create |
| `pi.env` | further knobs, as env entries — `PI_RUN_TIMEOUT_S` (default 3600), and `PI_PROVIDER_API` / `PI_BASE_URL` for a self-hosted or BYOK endpoint |

**`PI_BASE_URL` points at a self-hosted or BYOK endpoint for the chosen
provider.** `PI_PROVIDER_API` names the API shape it speaks, when that
differs from `pi.provider`'s own default — an endpoint speaking
`openai-completions`, say.

This is the mechanism behind "whichever model provider you configure." The
provider id picks the SDK's defaults, and these two knobs override them.

**`maxTurns` is not a pi control.** A Pipeline's profile bounds a claude run
and an ollama loop by turns.

The runtime logs the turns a route asked for, but enforces none of them. A
run is bounded by `PI_RUN_TIMEOUT_S` (default an hour) and the pod's idle TTL
instead.

## The tools it gives

One allowlist, translated into pi's own built-in names:

| The route binds | Pi gets | And each call is |
|---|---|---|
| `Read` | `read` | approved |
| `Write` | `write` | approved |
| `Edit` | `edit` | approved |
| `Bash` | `bash` | approved, any command |
| `Grep` | `grep` | approved |
| `Glob` | `find` | approved |
| `mcp__<server>__<tool>` | the matching MCP tool | approved, once the server is connected |
| `Bash(<prefix>:*)` | nothing | withheld and logged — pi hands the whole command to its own `bash` tool, with no per-command hook to narrow it against |
| anything else | nothing | denied, and logged |

**A scoped shell pattern grants nothing here.** Unlike the CLI-driven
runtimes, pi offers no prefix-level narrowing on its `bash` tool — a route
that needs an enforced scope belongs on a runtime that can enforce one.

**The hook is deny by default, the same posture as the reference runtime.**
Every `tool_call` is checked against the composed allowlist before it runs,
and an empty allowlist denies everything rather than falling back to pi's
own default of "every tool."

**The agent definition is `.claude/agents/<agent>.md`**, the same path and
composition rules the reference runtime reads.

**`agentops-shell` means the pod's shell.** With whatever the route's
`serviceAccountName` can reach. That is the same posture as the other
runtimes, and it is why the toolsets ship risk-split — see
[the agent's power]({{ '/configuration/#the-agents-power' | relative_url }}).

## Where its context lives

One pi session per conversation, under an id the runtime minted, in the SDK's
own session store beneath `.pi/agent/sessions/**` — the path the bundle
declares to [context sync]({{ '/installation/#storage' | relative_url }}), so
a run works on pod-local storage and the durable volume holds a snapshot.

| The context | The run |
|---|---|
| **found** | resumes, and says so |
| **slow to appear** | re-checked over a few seconds before it is believed gone |
| **gone** | FAILS with a readable reason. No answer is produced from an empty memory |

**The handle is minted, never derived.** Encoding the conversation's name into
it would make it reproducible — and then a lost context could never be
replaced by a fresh one, which is the failure the manager's latest-wins rule
exists to undo.

**Only the session subtree syncs.** Pi's own `auth.json` and
`models-store.json` sit beside the sessions directory and stay empty under
this design, since the credential is env, never OAuth.

## What it costs

| Fact | Consequence |
|---|---|
| **Billing follows the provider** | pi has no billing of its own — tokens, credits or hardware, whatever the chosen `pi.provider` charges. |
| **Every prompt reaches that provider** | the checkout, the tool results and the answer leave the cluster unless the provider is one you host yourself. |
| **The session store is the SDK's** | one directory per session id, snapshotted by context sync. The runtime interprets none of it. |
| **A shell is a shell** | same as the other runtimes. Bind `agentops-shell` where a shell is wanted. |

## Choosing between the runtimes

| | Claude Code | Ollama | GitHub Copilot | pi |
|---|---|---|---|---|
| **Runs on** | Anthropic's API | a model you host | GitHub's API | whichever provider you configure |
| **Data leaves the cluster** | yes | no | yes | depends on the provider |
| **Agent quality** | the vendor's | the model's — an operator choice | the vendor's | the chosen provider's |
| **Tools** | claude-code's, plus MCP | the same six, natively, plus MCP | Copilot's, translated, plus MCP | pi's own, translated, plus MCP |
| **A scoped shell** | enforced by the CLI | grants nothing | enforced per call | grants nothing |
| **Context** | claude-code's transcripts | the runtime's own | Copilot's context store | pi's own session store |
| **Cost per run** | tokens | your hardware | credits | whatever the provider charges |

**Any of them, on one install.** Each Pipeline's `runtimeRef` is the whole
decision. Pi is the one to reach for when the provider itself — not just the
model — is the choice a route needs to make.

### What the bundle renders

<!-- generated: renders bundle=pi -->

```text
# Always
AgentRuntime/pi
```

<!-- /generated -->

One `AgentRuntime`, named `pi`, through the parent chart's shared renderer. It
inherits every release-wide default and carries only what names this vendor:
the image, the credential as environment, the provider and model as
environment, and its own context-sync paths.

No ServiceAccount, no volume. The credential Secret renders only when a token
was supplied.

## Going deeper

- [Run agents on your own backend]({{ '/guides/agent-runtime/' | relative_url }})
  — the contract this runtime implements, for writing your own.
- [GitHub Copilot]({{ '/runtimes/copilot/' | relative_url }}) — the other
  vendor-SDK runtime, for the lanes a vendor's agent suits.
- [Installation]({{ '/installation/' | relative_url }}) — the runtime defaults
  every runtime inherits, and the storage they share.
- [Claude]({{ '/runtimes/claude/' | relative_url }}) — the reference runtime,
  and the one a fresh install runs on.
