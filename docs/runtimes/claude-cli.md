---
title: Claude CLI
permalink: /runtimes/claude-cli/
description: >-
  The spawn-per-unit alternative to the default Claude runtime — one `claude`
  CLI process per work unit, no live session held open.

next:
  eyebrow: Next
  title: Put an agent to work
  body: >-
    A Pipeline names what starts work, what it may reach, where it answers —
    and, with `runtimeRef`, which runtime executes it.
  url: /agent-ops-operator/guides/pipeline/
---

**This is a runtime, not an integration.** It starts no work and answers on no
surface. It is what EXECUTES an agent — the same vendor as the default
runtime, run a different way.

It conforms to the [runtime specification]({{ '/contracts/runtime/' | relative_url }}), version
2.0.0.

**With this bundle, agent-ops spawns a fresh `claude` CLI process for every
work unit**, instead of holding one session open across a pod's life. Each
conversation still gets a `runtime-claude-cli` pod, and the manager hands it
work exactly as it does any other runtime.

A route opts in with `runtimeRef: claude-cli`. Every other route keeps running
where it did — including on `claude`, the default, which holds its session
open instead.

## Why you would pick this over the default

`runtimes/claude` holds one `@anthropic-ai/claude-agent-sdk` session open
in-process across a pod's work units, re-opening it only when the
session-affecting configuration changes. That is the normal path now.

This bundle keeps the earlier behavior: resume from disk, every unit, with no
live session in memory between units.

| | `claude` (default) | `claude-cli` |
|---|---|---|
| **Process per work unit** | one session, reused | one CLI spawn |
| **Startup cost on each unit** | paid once, then skipped | paid every unit |
| **Permissions** | a `canUseTool` callback | `--allowedTools` / `--permission-mode dontAsk` |
| **Session reuse** | yes, `runtime-session-reuse` | no |

**A rollback, not a second product.** Pick it where a pod-per-unit process
model is what you want to keep operating, or while qualifying the default
bundle's session-reuse behavior against your own workload.

## Turn it on

```sh
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  -n agent-ops --create-namespace \
  --set claude-cli.enabled=true \
  --set claude-cli.credentialsSecret.token=placeholder-token
```

```powershell
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
  -n agent-ops --create-namespace `
  --set claude-cli.enabled=true `
  --set claude-cli.credentialsSecret.token=placeholder-token
```

Then a route selects it:

```yaml
pipelines:
  - name: house-ops
    profile: house-agent
    runtimeRef: claude-cli          # this route spawns the CLI per unit
    signalSources: [console]
    channels: [console]
```

**Nothing else moves.** `default` stays a copy of the first configured
runtime, and every route naming no `runtimeRef` keeps running there.

## What it needs

The same credential as the `claude` bundle — a token for
`CLAUDE_CODE_OAUTH_TOKEN`, with no default. See
[installation]({{ '/installation/#install' | relative_url }}).

## The tools it gives

Composed the same way `runtimes/claude` composes them — `Read`, `Grep`,
`Glob`, `Edit`, `Write`, `Bash`, plus every MCP server a Pipeline's
`mcpConfigs` bind — but enforced as `--allowedTools` with
`--permission-mode dontAsk`, passed to the spawned CLI process, instead of a
per-invocation callback.

## Where its context lives

claude-code's own transcripts on disk, under the runtime's context directory
— the same persistence contract `runtimes/claude` uses. A run resumes from
there on every unit, since no live session is kept.

## What it costs

| Fact | Consequence |
|---|---|
| **Every unit re-establishes the session** | the conversation transcript replays off disk and every bound MCP server relaunches, on each work unit. The cost grows with the conversation. |
| **No long-lived process** | nothing to leak across units — a safer default while qualifying a long `idleTtlMinutes`. |

### What the bundle renders

<!-- generated: renders bundle=claude-cli -->
```text
# Always
AgentRuntime/claude-cli
```
<!-- /generated -->
