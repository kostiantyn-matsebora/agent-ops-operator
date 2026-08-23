---
title: "Run your agent from a repository"
permalink: /guides/agent-from-a-repository/
description: >-
  What a profile's repository gives an agent, and how to wire the checkout, the
  agent definition and the deploy key without hitting the format trap.

next:
  eyebrow: Next
  title: "Give your agent tools"
  body: >-
    Bind toolsets and MCP servers to the route, and see how the allowlist is
    composed from two halves.
  url: /agent-ops-operator/guides/toolsets/
---

A profile with a `repository` gets that repository **checked out at
`/data/workspace` before the agent runs**, and the agent works there.

That checkout is where an agent's definition, its prompts and its project
context live once they outgrow an inline string. Its `CLAUDE.md`, its
`.claude/agents/`, its skills and every other file are simply there.

![An AgentProfile names your git repository, the runtime checks it out at /data/workspace, and the agent definition is read from there.]({{ '/assets/img/guides/agent-from-a-repository-light.svg' | relative_url }}){: .ao-diagram}

## Before you start

A repository is appropriate when:

- The role text is past a few lines, or you want it **reviewed and versioned**.
- The agent needs **project context** — runbooks, manifests, a codebase.
- You want several agents defined side by side and selected by name.

It is **not** what you want when a short inline `systemPrompt` does the job.
[Add your own agent]({{ '/guides/agent-profile/' | relative_url }})
covers that case, and it stays the simpler one.

You need a git repository the cluster can reach, and a deploy key for it.

{: .ao-callout}
> **The whole repository is readable to the agent.** A checkout is not a
> selective mount. Anything committed to it — a `.env`, a kubeconfig, a customer
> list — is reachable by any agent whose route grants `Read`.

| Get this wrong | And |
|---|---|
| A **read-write** deploy key | an agent whose route grants `Bash` can push to that repository |
| A key with **CRLF endings**, or flattened to one line | every run fails with `error in libcrypto` — a crypto error, not a permissions one |
| `agent:` naming a file that is not there | the run fails, naming the path it looked for |

## The overall shape

Three pieces:

1. **A repository** holding `.claude/agents/<name>.md` and whatever context you
   want the agent to have.
2. **A Secret** holding a read-only deploy key.
3. **`spec.repository` and `spec.agent`** on the profile, naming both.

The runtime performs the checkout — one clone into `/data/workspace`, with the
Secret projected as a file.

**The manager never reads that Secret.** It writes the name into the pod spec
and the kubelet resolves it, which is why the operator holds no `secrets`
permissions at all.

## Write the agent definition

`agent: my-agent` names `.claude/agents/my-agent.md` inside the repository:

```markdown
---
name: my-agent
description: What this agent is for.
tools: Read, Grep, Glob
---

The role text. What it is responsible for, how it decides, what it must
never do.
```

**A profile names one agent, and a Pipeline names one profile.** The agent comes
from the wiring, and no message may select another.

The `tools:` line is the definition's **own** declaration, and it is half of the
allowlist. The Pipeline's bound toolsets are the other half — see
[Give your agent tools]({{ '/guides/toolsets/' | relative_url }}).

## Create the deploy key

Build the Secret from the **file**, never from a shell string. Interpolating a
private key through a shell is what produces the flattened and CRLF forms that
fail as a crypto error.

```sh
ssh-keygen -t ed25519 -N '' -C agent-ops -f ./agentops_deploy_key
kubectl -n agent-ops create secret generic my-repo-key \
  --from-file=sshKey=./agentops_deploy_key
```

```powershell
ssh-keygen -t ed25519 -N '' -C agent-ops -f ./agentops_deploy_key
kubectl -n agent-ops create secret generic my-repo-key `
  --from-file=sshKey=./agentops_deploy_key
```

Add `agentops_deploy_key.pub` to the repository as a deploy key, **read-only**.

For HTTPS instead, set `type: https` and give the Secret a `token` key, plus an
optional `username`.

## Point the profile at it

<!-- generated: template kind=AgentProfile name=my-agent fields=repository.url,repository.ref,repository.auth,agent comments=off -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: AgentProfile
metadata:
  name: my-agent
spec:
  outputFormat: blocks   # blocks | none
  repository:
    url: <url>
    ref: main
    auth:
      secretRef:
        name: <name>
      type: ssh   # ssh | https
  agent: <agent>
```
<!-- /generated -->

`repository` is three fields and no more: `url`, `ref` and `auth`.

`outputFormat` is **required**. `blocks` tells the agent to write a title,
named sections and a `<details>` fold, which every surface renders its own way.
`none` appends nothing and leaves formatting to the agent's own definition in
`.claude/agents/<agent>.md` — which, for a repository-backed agent, is often
where you want it. See [Add your own
agent]({{ '/guides/agent-profile/' | relative_url }}).

## Replace the prompt wrapper, if you must

**The agent never receives your payload raw.** The manager wraps it in a
template of its own before sending.

That wrapper says the agent is running headless in a pod, tells it to adopt the
role from `.claude/agents/<agent>.md`, sets the rules for that kind of work, and
says how to finish.

There is one wrapper per kind of work. `contracts.md` calls these **lanes**:

| Kind of work | The wrapper says |
|---|---|
| a task, or a job tick | answer it, observe before acting, lead with the conclusion |
| an alert | **read-only triage** — gather evidence, name the likely cause, change nothing |
| a follow-up | continue the conversation you are already in |

Two profile fields replace that wrapper with a file from your checkout. **They
are not prose despite their names** — each is a path:

| Field | Replaces the wrapper for |
|---|---|
| `prompt` | the FIRST work unit of a conversation |
| `replyPrompt` | every follow-up in an existing one |

When either is set the manager stops sending rendered text and sends the path
plus a variable map instead. Your runtime reads the file out of the checkout and
substitutes:

| Variable | Carries |
|---|---|
| `AGENT_NAME` | the agent being run |
| `USER_TASK` | the request, for a task or a job tick |
| `SIGNAL_JSON` · `ALERTS_JSON` | the signal, for an alert |
| `DELIVERY_INSTRUCTIONS` | how the answer reaches its channels |

**Leave both empty unless you need a wrapper of your own.** The built-in ones
carry the delivery contract and the output format, and a replacement owns
getting both right.

{: .ao-callout}
> **`systemPrompt` is a different thing entirely**, despite the name. It is
> inline role text ADDED to the agent's system prompt. These two are files that
> REPLACE the wrapper around the task.

## Verify the checkout

```sh
kubectl -n agent-ops apply -f my-agent.yaml
kubectl -n agent-ops logs agentops-conv-<conversation> | head -20
```

```powershell
kubectl -n agent-ops apply -f my-agent.yaml
kubectl -n agent-ops logs agentops-conv-<conversation> | Select-Object -First 20
```

The first lines of a run show the clone. A failed key shows there, as
`error in libcrypto` rather than as a permissions message.

**The checkout path is fixed.** Sessions are keyed by working directory, so
`/data/workspace` is not configurable — a runtime that checked out elsewhere
could not resume one.

## What comes next

1. **[Give your agent tools]({{ '/guides/toolsets/' | relative_url }})**
   — and see how `toolsMode` composes against the `tools:` line you just wrote.
2. **[Every AgentProfile field](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/cr-reference.md#agentprofile)**
   — environment for the agent process, resources, a runtime override.
