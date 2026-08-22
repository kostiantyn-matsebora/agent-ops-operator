## Wiring (binding)

### `Pipeline`

**THE wiring, exclusively:** sources[] × channels[] + profile + TOOL ACCESS.

**No other CR carries wiring.** SignalSource has no profile or channel refs.
Channel has no default profile.

- **Sources no Ready Pipeline lists DROP signals** — `Wired=False` plus a
  response reason. For a CHAT source the reason also goes back to the surface
  the person typed on, because they are waiting.
- **Channels originate NOTHING**, so there is no "unwired channel" behavior to
  define. An unlisted chat source is the unwired case.

#### SOURCES ARE SHAREABLE, exactly as channels are

- **Any number of Ready Pipelines may list one**, of any kind, with NO conflict
  condition and no effect on `Ready`. Whether two agents watch one thing is the
  ADOPTER's call.
- **A signal admitted on a source N Pipelines serve opens N CONVERSATIONS**, one
  each, with their own profiles and capabilities.
- **Per-source policy is evaluated ONCE ABOVE the fan-out** — cooldown,
  signature grouping — or the first Pipeline spends the window and starves the
  rest.
- **`Wired` names EVERY server.** That count is how many conversations one
  signal opens. Ready pipelines only.
- **There is NO tiebreak left anywhere.** `sourceConflicts` and oldest-claimant
  are DELETED. Re-adding either is a regression, not a fix.

#### The ONE lane that does not fan out is a BARE chat message

A person asked one question and is owed one answer, and unlike an alert they CAN
name the agent.

| Ready Pipelines serving the chat source | What happens |
|---|---|
| one | it routes |
| several | ANSWER WITH THE CHOICES and the `/<pipeline> <task>` form |
| none | the unwired drop |

- **Several claimants is the EXPECTED shape on a shared surface** — see the
  many-to-many rule below. The choice list is the feature, not a degraded mode.
- **Addressed messages and thread replies are untouched.**
- **The lane is told apart by the ARRIVING SIGNAL's `kind` in ingest.** No
  `SignalSource` or `SignalAdapter` field declares "chat source", and no
  reconciler decides it. Adding such a handle buys one `if` at the price of a
  declaration every adapter author can get wrong.

#### Capabilities are wiring, exclusively

Two optional stanzas of ordered refs:

| Stanza | Points at | Is |
|---|---|---|
| `spec.toolsets` | `MCPToolset` | the allowlist |
| `spec.mcpConfigs` | `MCPConfig` | the MCP servers |

**`spec.toolsets.mode`** (`merge` | `overwrite`, default `merge`) composes
against the **AGENT DEFINITION** — the `tools:` frontmatter of
`.claude/agents/<agent>.md` in the profile's REPO.

- **Never against the profile**, which carries no capabilities. Mistaking the
  profile for the counterpart is what deleted this field once already.
- **`spec.mcpConfigs` has NO mode.** A definition declares no MCP servers, so
  there merge/overwrite really would be one behavior wearing two names.

**Refs apply in order.** Tools concatenate with dedup, server keys overlay
(later wins). Content stays in the referenced CRs, and Ready validates both ref
sets.

#### WIRING IS MANY-TO-MANY, IN EVERY DIRECTION. THIS IS THE MODEL, NOT A HAZARD

- A Pipeline claims MANY sources and delivers to MANY channels.
- A source is claimed by MANY Pipelines.
- A channel carries MANY Pipelines' conversations.

- **There is no exclusivity anywhere** — no conflict condition, no tiebreak,
  nothing to warn about. Two agents on one surface, or on one source, are
  ORDINARY CONFIGURATIONS an adopter chooses.
- **Any advice reading "prefer a source of its own" or "claiming this too would
  cost you X" is WRONG, and is deleted on sight.** Written three times in this
  repo, reverted three times.
- **The ONLY consequence of several claimants:** an UNADDRESSED chat message is
  answered with the list of agents serving the surface, so the person names one.
  A teaching moment, not a cost, and the whole of it.

#### CLAIMING AND ADDRESSING ARE INDEPENDENT MECHANISMS

| Mechanism | Is | Checks |
|---|---|---|
| **CLAIM** (`signalSourceRefs`) | who answers an UNADDRESSED message | read from Ready pipelines only |
| **ADDRESSING** (`/<pipeline> <task>`, `router.go` `HandleCommand`) | reaching one by name | a plain `Get` BY NAME — no claim check, no Ready check |

**`boundChannels` folds the originating channel in.** The reply lands in the
thread it was asked from, whatever the addressed Pipeline declares.

Two consequences that decide how bundles wire themselves:

1. **Several pipelines share ONE surface without sharing its source.**
2. **Listing a chat source on a Pipeline that is only ever addressed grants that
   Pipeline NOTHING**, while making every unaddressed message on that surface
   ambiguous — which the bare-chat lane answers by REFUSING.

`/pipelines` lists Ready pipelines only, so an addressable Pipeline stays
discoverable whether or not it claims anything.

#### REACHED, NEVER NAMED

A Pipeline is reached two ways and no others:

1. A signal posted to a source it CLAIMS.
2. A `/<pipeline>` chat command on a wired surface.

- **There is NO HTTP form that names a Pipeline.** `POST /task` was deleted, not
  renamed, because a caller selecting its own wiring is the shape this CRD
  exists to prevent.
- **There is likewise no profile-addressed form and no per-profile default.** A
  Pipeline declaring no bindings grants nothing, and that is a configuration,
  not a defect to warn about.
- **Every Pipeline the CHART ships must therefore declare its own tools.**
  Forgetting that is what made every signal-driven conversation toolless once.
- **Consequence: runtimes are generic** — one `AgentRuntime` per vendor × trust
  level. The SA stays runtime-level on purpose: a Pipeline choosing an SA would
  make pipeline-edit rights a privilege escalation.

### `MCPToolset`

**A pure LIST of tool patterns** (`spec.tools`).

- **No servers, no status.** Patterns are opaque, passed through like
  `allowedTools`. Servers live ONLY in `MCPConfig`.
- **Manager RBAC on it is read-only.**
- **Bound from `Pipeline.spec.toolsets` ONLY** — capabilities are wiring, never
  profile fields.

**What the pipeline binds is HALF the allowlist.** The RUNTIME composes it with
the agent definition's own `tools:` per the unit's `toolsMode`, since it alone
holds the checkout.

Verified against the CLI:

- **`--allowedTools` is the sole permission authority**, and a definition's
  `tools:` neither widens nor narrows the main session. The union must be built
  here or it does not happen.
- **Never pass `--agent <name>`.** That re-applies the definition as an
  availability INTERSECTION and silently defeats `overwrite`.
- **No `|| 'Read'` fallback.** Empty is passed as empty, with
  `--permission-mode dontAsk` — a permission prompt in a pod is a hang.

**The chart ships the built-in vocabulary risk-split** under
`global.builtinToolsets` (`agentops-observe` / `-shell` / `-edit`). `global.`
because subcharts read no other parent scope.

**A `kind: task` signal posted to a source X claims carries X's bindings** —
channels AND tooling both. Reaching a pipeline gets its wiring, not half of it.

**Multi-channel conversations.** The manager:

- Fans replies and acks to every bound thread.
- Delivers a user message to every bound channel EXCEPT the surface that
  displayed it (attributed text, per DESTINATION — not "siblings").
- Dispatches once ≥1 thread binding exists.

**The OPERATOR delivers.** Agent output reaches every bound thread through the
manager's adapters, single- and multi-channel alike.

- **Agents never post to a transport**, and Channel carries no delivery mode.
- **So prompts carry no transport steps**, and runtimes hold no channel
  credentials.
