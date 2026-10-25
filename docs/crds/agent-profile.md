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
  runtimeRef:                   # runtimeRef selects the AgentRuntime (execution backend) for this …
    name: <name>
  systemPrompt: <systemPrompt>  # INLINE role text appended to the agent's system prompt, for profiles …
```
<!-- /generated -->

## Every field

<!-- generated: crd-fields kind=AgentProfile -->

| Field | Type | Required | Meaning |
|---|---|---|---|
| `outputFormat` | `string` | **yes** | OutputFormat declares this agent's OUTPUT CONTRACT. One of `blocks`, `none`. |
| `agent` | `string` |  | The agent to adopt: name of `.claude/agents/<agent>.md` in the repository. |
| `env` | `[]object` |  | Extra environment for the agent process. |
| `env[].name` | `string` | **yes** | Name of the environment variable. |
| `env[].value` | `string` |  | Variable references $(VAR_NAME) are expanded using the previously defined environment variables in the container and any service environment variables. |
| `env[].valueFrom` | `object` |  | Source for the environment variable's value. The standard Kubernetes `EnvVarSource` -- set exactly one of `configMapKeyRef`, `secretKeyRef`, `fieldRef` or `resourceFieldRef`, each naming its source object and key. |
| `maxTurns` | `integer` |  | MaxTurns bounds the agent's own turns within ONE work unit. Defaults to `60`. |
| `prompt` | `string` |  | Prompt / ReplyPrompt are repo-relative template paths (job-style lanes). |
| `replyPrompt` | `string` |  | ReplyPrompt wraps a follow-up in an existing conversation, where Prompt wraps the first unit. |
| `repository` | `object` |  | RepositorySpec identifies the git repository an agent runs from. |
| `repository.auth` | `object` |  | RepoAuth references credentials for a (possibly private) git repository. |
| `repository.auth.secretRef` | `object` | **yes** | SecretRef points to a Secret holding either key `sshKey` (type=ssh, private deploy key) or key `token` (type=https, PAT. |
| `repository.auth.secretRef.name` | `string` | **yes** | Name of the referenced object. |
| `repository.auth.type` | `string` | **yes** | RepoAuthType selects the git auth mechanism. One of `ssh`, `https`. |
| `repository.ref` | `string` |  | Branch or ref to check out. Defaults to `main`. |
| `repository.url` | `string` |  | URL of the repository to check out, in any form git understands (ssh://, git@host:path, https://). |
| `resources` | `object` |  | Resources for the runtime pod running this profile. |
| `resources.claims` | `[]object` |  | Claims lists the names of resources, defined in spec.resourceClaims, that are used by this container. |
| `resources.claims[].name` | `string` | **yes** | Name must match the name of one entry in pod.spec.resourceClaims of the Pod where this field is used. |
| `resources.claims[].request` | `string` |  | The name chosen for a request in the referenced claim. |
| `resources.limits` | `map[string]object` |  | Limits describes the maximum amount of compute resources allowed. |
| `resources.requests` | `map[string]object` |  | Requests describes the minimum amount of compute resources required. |
| `runtimeRef` | `object` |  | RuntimeRef selects the AgentRuntime (execution backend) for this profile — deprecated, use `Pipeline.spec.runtimeRef`. |
| `runtimeRef.name` | `string` | **yes** | Name of the referenced object. |
| `systemPrompt` | `string` |  | INLINE role text appended to the agent's system prompt, for profiles with no repository — where `agent` can name no `.claude/agents/<agent>.md` because nothing is checked out. |

<!-- /generated -->

Behaviour only, never reach — `outputFormat`'s full rationale, the blocks
contract, stays in
[Concepts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md).
