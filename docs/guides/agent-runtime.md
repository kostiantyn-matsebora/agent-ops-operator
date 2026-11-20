---
title: "Run agents on your own backend"
permalink: /guides/agent-runtime/
description: >-
  What an agent runtime is, the three parts it is built from, and how to
  implement one for a backend agent-ops does not ship.

next:
  eyebrow: Reference
  title: Every AgentRuntime field
  body: >-
    The kind's own spec page — a commented example and every field, in the
    sidebar's CRDs group.
  url: /agent-ops-operator/crds/agent-runtime/
---

An **agent runtime** is what executes an agent: a container image plus an
`AgentRuntime` resource describing it. The image implements a small HTTP
contract — poll for work, execute it, report the outcome, exit when idle.

![The manager hands a work unit to your runtime inside the conversation's pod, which runs the agent and reports the result back.]({{ '/assets/img/guides/agent-runtime-light.svg' | relative_url }}){: .ao-diagram}

## Before you start

Write a runtime when the backend differs or you need behaviour `runtimes/claude` doesn't implement. Review [the runtime contract]({{ '/contracts/runtime/' | relative_url }}) first.

{: .ao-callout}
> **Your runtime applies the tool allowlist, and nothing checks that it did.**

## The overall shape

1. **A container image** implementing the work contract — poll, execute, report, exit.
2. **An `AgentRuntime` resource** declaring the image, its ServiceAccount, and what continuity it promises.
3. **A `Pipeline` naming it** through `runtimeRef`.

## 1. Poll for work

```sh
curl "$CONTROL_URL/work?convo=$CONVO_ID&pod=$POD_NAME&wait=25"
```

```powershell
curl "$env:CONTROL_URL/work?convo=$env:CONVO_ID&pod=$env:POD_NAME&wait=25"
```

- `204`: nothing queued; `200`: returns a unit (see [`GET /work`]({{ '/contracts/runtime/' | relative_url }}#get-work))

## 2. Clone the repository

```sh
if [ -n "$REPO_URL" ]; then
  git clone --branch "$REPO_REF" "$REPO_URL" /data/workspace
fi
```

```powershell
if ($env:REPO_URL) {
  git clone --branch $env:REPO_REF $env:REPO_URL /data/workspace
}
```

- Empty `REPO_URL`: skip clone
- Authenticate via `GIT_AUTH_TYPE` with `GIT_SSH_KEY` or `GIT_TOKEN`

## 3. Compose the tool allowlist

`allowedTools` from the work unit plus `tools:` from `.claude/agents/<agent>.md` — see [Composing the allowlist]({{ '/contracts/runtime/' | relative_url }}#composing-the-allowlist).

| `toolsMode` | Pass to the model |
|---|---|
| `merge` | union, definition's entries keep position |
| `overwrite` | `allowedTools` alone |

## 4. Run the unit and report the outcome

```sh
curl -X POST "$CONTROL_URL/work/done" -d '{
  "convo": "task-kvb55", "runId": "r-7", "status": "succeeded",
  "result": "the answer, as markdown",
  "runtimeContextId": "abc123", "continuity": "continued"
}'
```

```powershell
curl -X POST "$env:CONTROL_URL/work/done" -d '{
  "convo": "task-kvb55", "runId": "r-7", "status": "succeeded",
  "result": "the answer, as markdown",
  "runtimeContextId": "abc123", "continuity": "continued"
}'
```

- `status`: `succeeded`/`failed`; `result`: the deliverable
- `runtimeContextId`: your handle for context continuation; `continuity`: `continued`/`new`/`unavailable`
- Trim `turns[]`/`toolCalls[]` to [bounds]({{ '/contracts/runtime/' | relative_url }}#bounds) (≤200 entries, ≤200 bytes per string)

## 5. Exit when idle

Exit `0` after `RUNTIME_IDLE_TTL_M` minutes with no work — see [Lifecycle]({{ '/contracts/runtime/' | relative_url }}#lifecycle).

## 6. Declare the AgentRuntime

```yaml
apiVersion: agentops.dev/v1alpha1
kind: AgentRuntime
metadata:
  name: my-runtime
spec:
  image: registry.example.com/my-runtime:1.0.0
  contextStorage: volume   # volume | external | none
  serviceAccountName: agentops-runtime
  idleTtlMinutes: 10
```

`contextStorage`: `volume` (PVC), `external` (elsewhere), `none` (stateless).

{: .ao-callout}
> **Pod exec grants every Secret in the namespace.**

## 7. Point a Pipeline at it

```sh
kubectl -n agent-ops patch pipeline my-route --type=merge \
  -p '{"spec":{"runtimeRef":{"name":"my-runtime"}}}'
```

```powershell
kubectl -n agent-ops patch pipeline my-route --type=merge `
  -p '{\"spec\":{\"runtimeRef\":{\"name\":\"my-runtime\"}}}'
```

A typo in `runtimeRef` turns `Ready=False`. Existing conversations keep their runtime.

## Verify

- Poll returns `204` when idle, `200` with a work unit
- Composed allowlist appears in logs before model execution
- `POST /work/done` returns `200`; posting 201 `turns[]` entries returns `400`
- Pipeline shows `Ready=True`; conversation logs show the unit, allowlist, and answer

## What comes next

1. [The runtime contract]({{ '/contracts/runtime/' | relative_url }}) — every field, enums, bounds, and the conformance checklist
2. [`runtimes/claude`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/runtimes/claude/) — the reference implementation (~200 lines)
3. [The CRD reference]({{ '/crds/agent-runtime/' | relative_url }}) — every `AgentRuntime` field with examples
