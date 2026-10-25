---
title: AgentProfile
permalink: /crds/agent-profile/
description: >-
  Every spec field of the AgentProfile kind — who the agent is and how it
  behaves, never what it may reach.
---

An `AgentProfile` is who the agent is and how it behaves — repository, role,
prompts and limits. It selects no execution and grants no reach: tools,
channels and identity come from the `Pipeline` or `Coordinator` routing it.

## Spec, at a glance

<!-- generated: crd-example kind=AgentProfile name=k8s-engineer -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: AgentProfile
metadata:
  name: k8s-engineer
spec:
  outputFormat: blocks   # blocks | none
  agent: <agent>                # the agent to adopt: name of `.claude/agents/<agent>.md` in the …
  env:                          # extra environment for the agent process
  - name: <name>
  maxTurns: 60                  # maxTurns bounds the agent's own turns within ONE work unit
  prompt: <prompt>              # prompt / ReplyPrompt are repo-relative template paths (job-style lanes)
  replyPrompt: <replyPrompt>    # replyPrompt wraps a follow-up in an existing conversation, where …
  repository:                   # repositorySpec identifies the git repository an agent runs from
    auth:
      secretRef:
        name: <name>
      type: ssh   # ssh | https
    ref: main
    url: <url>
  resources:                    # resources for the runtime pod running this profile
    claims:
    - name: <name>
    limits:
      cpu: "100m"
    requests:
      cpu: "100m"
  runtimeRef:                   # runtimeRef selects the AgentRuntime (execution backend) for this profile
    name: <name>
  systemPrompt: <systemPrompt>  # INLINE role text appended to the agent's system prompt, for profiles …
```
<!-- /generated -->

## Every field

<!-- generated: crd-fields kind=AgentProfile -->

| Field | Type | Required | Meaning |
|---|---|---|---|
| `outputFormat` | `string` | **yes** | OutputFormat declares this agent's OUTPUT CONTRACT. |
| `agent` | `string` |  | The agent to adopt: name of `.claude/agents/<agent>.md` in the repository. |
| `env` | `[]object` |  | Extra environment for the agent process. |
| `maxTurns` | `integer` |  | MaxTurns bounds the agent's own turns within ONE work unit. |
| `prompt` | `string` |  | Prompt / ReplyPrompt are repo-relative template paths (job-style lanes). |
| `replyPrompt` | `string` |  | ReplyPrompt wraps a follow-up in an existing conversation, where Prompt wraps the first unit. |
| `repository` | `object` |  | RepositorySpec identifies the git repository an agent runs from. |
| `resources` | `object` |  | Resources for the runtime pod running this profile. |
| `runtimeRef` | `object` |  | RuntimeRef selects the AgentRuntime (execution backend) for this profile. |
| `systemPrompt` | `string` |  | INLINE role text appended to the agent's system prompt, for profiles with no repository — where `agent` can name no `.claude/agents/<agent>.md` because nothing is checked out. |

<!-- /generated -->

One row per field and nothing unrolled — `outputFormat`'s full rationale
stays in
[Concepts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md).
