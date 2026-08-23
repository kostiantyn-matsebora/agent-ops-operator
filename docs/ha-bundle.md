# Home Assistant bundle (subchart)

The Home Assistant subchart: log ingestion, house tooling, **two agents with
different privilege**, and their opt-in routes.

`chart/charts/ha-bundle/` packages the whole "the house has a problem, and
someone can ask about it or fix it" experience as four independently toggleable
components.

**Off by default and never enabled by demo mode.** Every component needs a Home
Assistant endpoint and a token no demo cluster has.

The shape of this bundle is a **privilege split**. Asking the house a question
and administering it are two jobs, and they get two agents.

| Agent | Profile | Reached by | Job |
|---|---|---|---|
| The house's user | `ha-user` | an ordinary chat message | **use** the house — services, lights, scenes, automations |
| The administrator | `ha-operator` | `/ha-ops <task>`, by name | **fix** the house — integrations, configuration, repairs |

**The split is use versus fix, not read versus act.** Home Assistant has no
read-only role, so neither credential is a safe one that merely looks. Both
agents act. What separates them is the job and the reach it needs.

**Neither route touches the Kubernetes API.** This bundle renders one
ServiceAccount per route — `agentops-ha-control` and `agentops-ha-ops` — with no
Kubernetes RBAC at all, and names each on its own Pipeline.

That is deliberate, and this bundle is the case that proves the rule.

Both routes used to run as the release's shared runtime account, so a Home
Assistant agent held whatever cluster power that account was granted, for no
capability it uses.

**The bundle knows what its routes do and the parent chart cannot.** That is why
the account is rendered here.

| Component | Flag | What it renders |
|---|---|---|
| Ingest lane | `logsAdapter.enabled` (**on**) | The `SignalAdapter` (`home-assistant`, adapter `signals/ha/`) and — under `source.create` — a `SignalSource` (`ha-logs`). **Not the claim on it** |
| MCP tooling | `mcp.enabled` (**on**) | An `MCPConfig` (`ha-api`, server key `homeassistant`) and two `MCPToolset`s |
| Admin MCP | `adminMcp.enabled` (**off**) | A second `MCPConfig` (`ha-admin-api`, server key `homeassistant_admin`) and the `ha-admin` toolset — see [Repairing the house](#repairing-the-house-adminmcp) |
| Admin MCP server | `adminMcpServer.enabled` (**off**) | That server's workload: `Deployment` + `Service` (`agentops-mcp-ha`) and **its own `ServiceAccount`** |
| Profiles | `profiles.user.enabled` / `profiles.ops.enabled` (**on**) | Up to two `AgentProfile`s, behaviour only, each with an inline `systemPrompt` |
| Wiring | `pipelines.enabled` (**off**) | Up to two `Pipeline`s — see [The bundle's own wiring](#the-bundles-own-wiring) |

**The bundle ships no substrate.** No `AgentRuntime`, no runtime
ServiceAccount, no LLM credential and no runtime RBAC: those are release-wide
facts in the parent chart's `runtime:` block and `global.agentops.runtime.*`
([concepts](concepts.md#the-substrate-runtime-and-globalagentopsruntime)).

**There is no MCP server workload either**, and that is the difference from
`k8s-bundle` and `prometheus-bundle`. Home Assistant serves its own MCP endpoint.

## Prerequisites

Three things must exist before the bundle does anything.

1. **A reachable Home Assistant instance.** Every component connects to the same
   `homeAssistant.endpoint`.
2. **The MCP Server integration enabled** in Home Assistant, if you want the MCP
   path. Settings → Devices & Services → Add Integration → **Model Context
   Protocol Server**. It serves SSE at `/mcp_server/sse` and exposes the Assist
   intents your instance has exposed to voice assistants.
3. **Two Home Assistant tokens.** Give them to the chart directly and it
   creates the Secrets, or create the Secrets yourself and name them.

### Letting the chart hold them

Hand it the token and it creates the Secret, deriving both keys from the one
value. This is the form a secret manager wants — the value can be a reference
your tooling resolves:

```yaml
ha-bundle:
  homeAssistant:
    endpoint: http://ha.ha.svc.cluster.local:8123
    credentials:
      controlToken: ref+vault://secret/ha?#controlToken
      operatorToken: ref+vault://secret/ha?#operatorToken
```

### Or creating them yourself

Name an existing Secret instead (`controlSecret` / `operatorSecret`). It carries
the token under **two keys**:

```sh
TOKEN=<paste from Home Assistant → your profile → Long-lived access tokens>
kubectl -n <ns> create secret generic ha-operator \
  --from-literal=token="$TOKEN" \
  --from-literal=authorization="Bearer $TOKEN"
```

```powershell
$TOKEN = "<paste from Home Assistant>"
kubectl -n <ns> create secret generic ha-operator `
  --from-literal=token="$TOKEN" `
  --from-literal=authorization="Bearer $TOKEN"
```

Setting both forms of one credential **fails the render**. Two sources for one
token is ambiguous.

Two keys for one token, because two consumers want it in two shapes.

- The signal adapter authenticates with the raw `token`.
- The MCP path sends it as an HTTP header. A header value that references a
  Secret is substituted **whole**, so nothing can prepend `Bearer ` for you.

### The two tokens are two Home Assistant users

| Value | Holds | Mint it as |
|---|---|---|
| `credentials.controlToken` / `.controlSecret` | the everyday agent's credential, and the MCP path's | the account that **uses** the house |
| `credentials.operatorToken` / `.operatorSecret` | the fixing agent's credential, and the ingest lane's | an **admin** Home Assistant user |
| `credentials.ingestSecret` | the ingest lane's own, when it should differ | an **admin** user — see below |

**This is where the split is actually enforced.** A token inherits its user's
rights, and no toolset can narrow a credential.

The operator credential is a **prerequisite** twice over.

- Without it, neither `ha-operator` nor the `ha-ops` route renders. That is a
  house you can use and question but never repair.
- **The ingest lane needs it too.** Home Assistant's `subscribe_events` is
  admin-only, and so are the `system_log/list` backfill and the
  `config_entries/get` the dwell re-check verifies with. A control token
  connects, passes auth, and is then refused the subscription — which surfaces
  as `Ready=False, reason=Unreachable` and reads like a network problem.

## The ingest lane (`logsAdapter`)

The chart renders the `SignalAdapter` CR. The reconciler owns the workload.

`kubernetesAccess: false` — this adapter's data source is Home Assistant, so it
holds no Kubernetes client and mounts no ServiceAccount token.

**What it watches** is the `system_log_event` stream over Home Assistant's
WebSocket API.

That event is a structured record — logger, level, message, source location,
occurrence count. It is the closest thing Home Assistant has to a Kubernetes
Warning event.

The plain-text `/api/error_log` would make every one of those fields a regex, and
its cursor a byte offset that log rotation invalidates.

| Label | From |
|---|---|
| `integration` | the logger's domain (`homeassistant.components.<domain>`, `custom_components.<domain>`), else the logger itself |
| `logger` | the Python logger name |
| `level` | `ERROR`, `CRITICAL`, … |
| `location` | `file.py:line` |

The **fingerprint** keys on logger plus source location — Home Assistant's own
deduplication identity — never on the occurrence. A recurring error therefore
collapses into one conversation under the manager's cooldown instead of opening
one per repeat.

**Grouping is by integration.** One broken hub logs from several code paths, and
the useful output is one conversation about the hub.

Default levels are `ERROR` and `CRITICAL`. `WARNING` is Home Assistant's
background hum — deprecation notices, slow updates, sleepy devices — and watching
it would bury real problems and spend credits doing it.

### Cursor and restarts

The read position lives in the manager (`/signal/state`), not in the pod. A
restart resumes where it stopped, and `backfill` reports what was logged while
the adapter was down.

A cursor that sits minutes **ahead** of everything Home Assistant still holds
names a record that is gone. The log was cleared, or the cursor belongs to
another instance.

The adapter re-reads the whole log rather than stalling. A lane that goes quiet
forever and looks healthy is the failure this avoids.

## Suppression (`logsAdapter.source.rules` / `.route`)

The configuration is the **same vocabulary** the cluster Events lane uses, and
this page does not restate it. Read
[Event suppression](k8s-bundle.md#event-suppression-eventsadaptersourcerules)
for what `rules`, `for`, `action: drop`, `escalateAfterObjects` and
`route.inhibitRules` mean, and
[What `for` actually does](k8s-bundle.md#what-for-actually-does) for the
verification ladder.

Four differences are specific to this adapter.

**Matchers can read the message.** Home Assistant records carry no `reason`
field, so level and logger alone cannot tell "will retry" from "credentials
rejected".

`message` is available to matchers as the record's text. It is **match-only** and
never becomes a label: a label carrying free text would key conversation grouping
on the exact wording.

**The verification ladder is Home Assistant's.**

```
record matches a rule ──► hold `for` ──► re-check
                                            │
                     config entries loaded ─┤ the integration recovered
                     record never recurred ─┤ it stopped
                                            │       → DROP
                     count rose / entry     │
                     still failing ─────────┴─────► EMIT once, with evidence
```

Rung 1 is the integration's **config entry state** (`setup_error`,
`setup_retry`). Integrations configured in YAML have no entries, and core loggers
have no integration at all, so both fall to rung 2 — **did the record occur
again during the window**. Rung 3 is fail-open.

**There is no time axis.** `timeIntervals` and `muteTimeIntervals` exist on the
cluster Events adapter and are not implemented here. An unknown key is a config
error, not a window that silently never fires.

**Self-exclusion is about the loop this adapter can create.** See below.

### Self-exclusion: never answer a log the agent itself wrote

The loop is short and complete. The ops agent calls a Home Assistant service, the
call fails, Home Assistant logs the failure, the record becomes a signal, the
signal opens a conversation with the ops agent, which calls the service again.

Three independent mechanisms, and **none of them is configurable**:

| # | Drops a record when | Needs a read |
|---|---|---|
| 1 | the logger, location or text names agent-ops | no |
| 2 | the logger is a surface agent-ops calls Home Assistant through (`mcp_server`, `api`, `websocket_api`) | no |
| 3 | the record names the Home Assistant user backing this adapter's token | yes, `auth/current_user` at connect |

Mechanism 2 is the one that breaks the loop, and it needs no read, so it holds
before the session has authenticated. Mechanism 3 is precise and simply off until
that read succeeds.

`homeassistant.components.conversation` is deliberately **not** in the surface
list. That is Home Assistant's own voice assistant, used by people, and silencing
it would cost real errors to buy nothing.

What this does not catch is a rejected token: `http.ban` logs that against an IP
address with no user to attribute it to. Silencing intrusion attempts to cover
that case is the worse trade.

## The house as MCP tools (`mcp`)

Two halves, and both matter — a config without a toolset gives an agent a server
it may not call.

The `MCPConfig` server key is **fixed** at `homeassistant`. It has no values
path, because that key IS the `mcp__homeassistant__*` namespace named in every
allowlist, and a rename would silently strip an agent's tools instead of failing.

An empty `mcp.url` defaults onto `<endpoint>/mcp_server/sse`, which is where the
built-in integration serves. An explicit URL still wins.

The component needs a **credential** as well as an endpoint. An `MCPConfig` that
Home Assistant answers `401` to costs an agent its tools and looks installed
doing it.

It authenticates as the **control** user, and that costs nothing.

Every Assist intent is within that user's rights. The operator token's extra
power is configuration, which no MCP tool reaches, so defaulting to it would
widen the shared path and buy no capability.

### Two toolsets, split by risk

| Toolset | Grants | Renders when |
|---|---|---|
| `ha-observability` | reading state, weather, time, list contents | the MCP component renders |
| `ha-actions` | turning things on and off, lights, climate, media, vacuums, broadcasts | the MCP component renders |

**Both routes bind both toolsets.** Invoking services is the everyday agent's
whole job, so withholding `ha-actions` from it would leave it unable to do the
thing it exists for. Set `mcp.toolsets.actions.enabled: false` for an install
that may look and never touch.

Tools are **enumerated, never `mcp__homeassistant__*`**. A wildcard spans both
halves and defeats the split, which is exactly what the split replaces.

`ha-actions` is **not** gated on a credential, because there is no credential
fact to follow: Home Assistant registers the same Assist intents for any user,
and has no read-only server mode to detect.

**Which tools exist is decided by your house.** Home Assistant's MCP server
registers the Assist intents your instance exposes.

The shipped lists are a starting point to check against your own `/mcp_server`,
not a contract. A name that resolves to nothing is inert. A tool you need that is
missing is a values edit.

## Repairing the house (`adminMcp`)

**Home Assistant's built-in MCP server exposes Assist intents only.** It turns
things on and off. It cannot read a log, reload an integration, or disable an
entity — those live in the entity and config-entry registries, which Home
Assistant serves over its **WebSocket API**.

Without this component the ops agent reads its own tool list, finds device
controls and no way to reach a registry, and hands the job back. That is not a
prompt problem. The capability is genuinely absent.

`adminMcp` adds a **second MCP server**, bound to `ha-ops` alone.

| Route | Reaches | Can it repair? |
|---|---|---|
| `ha-control` | `ha-api` — intents, as the control user | no, and not by allowlist: its server has no such tools |
| `ha-ops` | `ha-api` **and** `ha-admin-api` | yes — registries, integrations, logs, services |

That is two walls rather than one. The everyday agent cannot reach configuration
even if its allowlist were wrong, because the server it talks to does not expose
it.

**That wall is the server's capability, so it survives a shell** — unlike a
toolset, which the agent's own CLI applies and an agent with a shell can simply
go around.

What does not survive is the SEPARATION, if both servers are deployed. The admin
server authenticates nobody, so a `ha-control` agent that can run commands can
reach it directly and hold the operator token's power. Two things prevent that,
both off by default:

- `global.agentops.networkPolicy` restricts the admin server to runtime pods.
- `runtime.egressMediation` enforces each conversation's bound toolsets, so a
  control conversation is refused the admin server's tools.

### Two ways to have a server

Both are off by default. Enabling `adminMcp` with **neither** fails the render:
an `MCPConfig` pointing nowhere costs the agent its tools and looks installed.

1. **Let the chart deploy one.** `adminMcpServer.enabled: true` runs
   [ha-mcp](https://github.com/homeassistant-ai/ha-mcp) in-cluster, talking to
   Home Assistant over its API. Nothing is installed inside Home Assistant.
2. **Point at one you run.** Set `adminMcp.url`. That covers a HACS custom
   integration serving MCP inside Home Assistant, or any other server.

> **Add-ons are not an option on Home Assistant Core.** Add-ons are
> Supervisor-managed, so a Core install — the usual shape in Kubernetes — has no
> add-on store. The deployed container and a HACS custom integration are the two
> paths that exist there.

### What the toolset grants, and what it withholds

The `ha-admin` toolset is **enumerated from the server's real tool list**, never
wildcarded. `mcp__homeassistant_admin__*` would grant restarting Home Assistant,
deleting registry objects and installing HACS packages in one line.

Of the 78 tools ha-mcp 8.3.0 registers, **52 ship**. The four that answer the
cases this component exists for:

| Task | Tool |
|---|---|
| disable or rename an entity | `ha_set_entity` |
| enable, disable or reconfigure an integration | `ha_set_integration` |
| read the logs | `ha_get_logs` |
| reload configuration without a restart | `ha_reload_core` |

**26 are withheld**, and each for a reason: they restart Home Assistant
(`ha_restart`), manage backups (`ha_manage_backup`), delete registry objects
(`ha_remove_entity`, `ha_remove_device`, the `ha_config_remove_*` family), or
install software (`ha_manage_hacs`, `ha_manage_app`). Add the ones you want by
restating `adminMcp.toolset.tools` — Helm replaces lists rather than merging.

**The pinned tag is load-bearing.** The toolset names tools, so a server that
renames or adds one changes what the allowlist means with nothing failing.

### Two things about the deployed server

**It runs under its own ServiceAccount**, never the runtime's — setting them
equal fails the render.

It needs no Kubernetes permissions at all, since it reads an HTTP API rather
than the API server. The separate identity is where a grant would go if one were
ever needed.

**It authenticates by URL-path secrecy.** There is no token on the MCP endpoint
itself, so anything that can reach the Service and knows the path can drive the
house.

The Service is ClusterIP, which bounds that to the cluster. On a shared cluster
set `adminMcpServer.path` to a high-entropy value and the URL follows it.

## The two profiles (`profiles`)

Two objects, behaviour only. No repository, no `allowedTools`, no `mcp`. What each
agent may DO comes from the Pipeline routing it.

Because neither has a repository, no `.claude/agents/<name>.md` can resolve, so
the inline `systemPrompt` is not decoration. Without it a log record would reach a
personality-free agent whose only inputs are an allowlist and a payload.

- **`ha-user`** uses the house and answers questions about it. Its role tells it
  to refuse repairs — reloading an integration, editing configuration, fixing
  what Home Assistant is reporting as broken — and to point at `/ha-ops`.
- **`ha-operator`** repairs it. Its role tells it to investigate before acting,
  to say so when an error resolved itself, and to **describe and stop** for
  anything hard to undo, anything that removes a device or deletes history, and
  anything where the integration is unclear.

**The ops role spells out the REST path, and that is not decoration.** Home
Assistant's MCP server exposes Assist intents only.

An agent reading its own tool list therefore concludes it cannot read a log or
reload an integration, and says so instead of doing the job.

The prompt names `$HA_URL` and `$HA_TOKEN` and gives the calls it needs most:
the error log, entity states, the config-entries listing with its `entry_id`s,
a reload, a service call, and a config check.

Both are told to describe and stop for a lock, an alarm, a garage door, or
heating in a way someone could be harmed by.

**Both are also told to quote `domain`.** Home Assistant advertises
`GetLiveContext`'s domain filter with an `anyOf` whose first branch is an empty
schema, so that parameter carries no declared type — and a model writes
`{"domain": sensor}` where its neighbours, typed `string`, come out quoted.

That text is not valid JSON, so the call is discarded before it runs. Measured
on one install, **59 of 110 calls to that tool never executed**, and the runs
answered from readings they already held.

The prompt line is a workaround for a schema this chart does not own. The
runtime's own breaker is the backstop —
[contracts](contracts.md#a-tool-call-the-model-cannot-form).

Both carry connectivity `env`: `HA_URL`, and `HA_TOKEN` via `valueFrom`, resolved
in the runtime pod. The manager reads no Secrets.

`ha-user` renders when there is **some** way to reach the house — the MCP path or
a control credential. `ha-operator` renders **only** with an operator credential.

## The bundle's own wiring

`pipelines.enabled` defaults **false**, and nothing forces it on — no turnkey
mode enables this bundle at all. Turning it on renders up to two routes.

```sh
helm upgrade --install agentops \
  oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator -n agentops \
  --set ha-bundle.enabled=true \
  --set ha-bundle.homeAssistant.endpoint=https://ha.example.org \
  --set ha-bundle.homeAssistant.credentials.controlToken="$CONTROL_TOKEN" \
  --set ha-bundle.homeAssistant.credentials.operatorToken="$OPERATOR_TOKEN" \
  --set ha-bundle.pipelines.enabled=true \
  --set ha-bundle.pipelines.chatSources={console-ha} \
  --set ha-bundle.pipelines.channels={console}
```

```powershell
helm upgrade --install agentops `
  oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator -n agentops `
  --set ha-bundle.enabled=true `
  --set ha-bundle.homeAssistant.endpoint=https://ha.example.org `
  --set ha-bundle.homeAssistant.credentials.controlToken="$CONTROL_TOKEN" `
  --set ha-bundle.homeAssistant.credentials.operatorToken="$OPERATOR_TOKEN" `
  --set ha-bundle.pipelines.enabled=true `
  --set ha-bundle.pipelines.chatSources={console-ha} `
  --set ha-bundle.pipelines.channels={console}
```

| Route | Profile | Claims | Binds |
|---|---|---|---|
| `ha-control` | `ha-user` | `pipelines.chatSources` | `ha-observability` + `ha-actions` |
| `ha-ops` | `ha-operator` | the same, **plus `ha-logs`** | the same, **plus a shell** |

The shell is the whole difference in reach, and it is deliberate. Home
Assistant's MCP server exposes Assist intents only, so **no MCP tool here
reaches configuration**. Repairing an integration means REST, and REST means a
shell tool plus the token in the profile's env.

**Every admitted log record then opens a conversation and spends LLM credits**,
where before it dropped at `Wired=False`.

### Both agents are offered on the surface

**Wiring is many-to-many.** A source is claimed by as many pipelines as you
want, a pipeline claims as many sources, and a channel carries as many
pipelines. Nothing here is exclusive.

Both routes claim every chat source you name, so both agents are offered there.
An unaddressed message is answered with the list, and you name the one you meant:

```
/ha-control turn the kitchen lights off
/ha-ops     zwave has been throwing errors since the reboot
```

Addressing works from **any** wired surface regardless of claims —
`/<pipeline> <task>` resolves by name, with no claim check and no Ready check,
and the reply lands in the thread you asked from. `/agents` lists them any time.

### Channels, and what happens without one

Both routes deliver to `pipelines.channels`, a values-supplied list omitted
entirely when empty. With none bound the conversation dispatches immediately and
the answer is read from `status.runs[].result`.

## The credential path is wider than the toolset path

The toolsets are a real boundary **for the MCP path**. They are not the whole
boundary.

Each agent's token sits in its profile's `env`. A route that also binds a shell
toolset reaches the entire Home Assistant REST API **for that user** — every
service, every entity, and for the operator, configuration too.

| Path | Reach | Can it repair? |
|---|---|---|
| MCP | the tools the bound toolset names, as the control user | no — Assist intents control devices and touch no configuration |
| REST, via a shell tool | everything that route's credential may do | yes — logs, config entries, reloads, service calls |

That is not a leak to engineer away. It is the mechanism by which `ha-ops` does
its job, and the reason `ha-control` is given no shell.

`pipelines.restAccess` decides who gets that second path:

- **`null` (default)** — on for `ha-ops`, off for `ha-control`. The asymmetry is
  the design.
- **`true`** — both routes. The everyday agent then holds its own token's whole
  surface, for no capability it needs.
- **`false`** — neither. `ha-operator` can still call services and can no longer
  reconfigure anything, which makes the fixing lane mostly decorative.

This is the same asymmetry `k8s-bundle` documents for a CLI versus the MCP
server's own identity. Writing it down is the point. Pretending the toolset split
is the whole boundary would be the defect.

## Adopting a hand-applied install

Already running the same objects by hand? Enabling the bundle with matching names
hits server-side-apply ownership conflicts. Three options, in order of
preference:

1. **Leave the bundle disabled.** Nothing changes, and an upgrade never forces
   the choice.
2. **Adopt.** Match the live names in values and upgrade once with
   `--force-conflicts`.
3. **Install side by side** under fresh names, then retire the old CRs.

Rolling back is disabling the bundle. Helm removes the objects it owns, and
hand-applied ones are untouched.
