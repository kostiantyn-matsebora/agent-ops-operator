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
- **Consequence: runtimes are generic** — one `AgentRuntime` per VENDOR. Trust
  level is no longer part of that product; see below.

#### EXECUTION, IDENTITY AND STORAGE ARE WIRING TOO

Three more optional fields, and they complete the object:

| Field | Selects | Absent |
|---|---|---|
| `spec.runtimeRef` | the `AgentRuntime` | the one named `default` |
| `spec.serviceAccountName` | the identity it executes under, OVERRIDING the runtime's | the FLOOR account, bound to NOTHING |
| `spec.persistence` | WHERE its conversations keep state — `context` and `workspace`, independently | the chart's RELEASE-WIDE claim, then EPHEMERAL |

**SILENCE MEANS NO POWER.** A route that names no account can do nothing in the
cluster. It shipped the other way once — the mode bound to the account every
unnamed route inherited — and three of four routes in the reference install held
pod-delete and node-patch because nobody typed a field, two of them routes that
reach no Kubernetes API at all. Shrinking the ROLE fixed the blast radius and
left the MODEL inverted.

**CAPABILITIES AND EXECUTION IDENTITY ARE THE SAME DECISION.** One says which
tools may be called, the other with whose credentials. Split across two objects,
no single object states an agent's power and no single reviewer can approve it.

**IT USED TO SAY "the SA stays runtime-level on purpose: a Pipeline choosing an
SA would make pipeline-edit rights a privilege escalation." THAT FAILED THE
SYMMETRY TEST**, and the history is kept because the argument is seductive:

- **It did not REMOVE the escalation, it moved it to the object you trust
  LESS.** `AgentProfile.spec.runtimeRef` selected an `AgentRuntime`, and an
  `AgentRuntime` carries the SA — so profile-edit rights were already
  SA-choice rights.
- **A profile is prompts, a repo ref and limits.** A Pipeline already grants
  tools and MCP servers. **Whoever is trusted to grant tools is more qualified
  to choose an execution identity, not less.**

**PRECEDENCE, and no other order:**

| | Chain |
|---|---|
| runtime | `conversation.spec.runtimeRef` → `profile.spec.runtimeRef` (DEPRECATED) → `AgentRuntime/default` → bootstrap |
| identity | `conversation.spec.serviceAccountName` → `runtime.spec.serviceAccountName` → THE FLOOR |

**THE IDENTITY CHAIN HAS NO MODE IN IT, AND NEVER DID HAVE ONE LEGITIMATELY.**
`global.agentops.runtime.rbacMode` is DELETED with no alias: it rendered a NAMED
account that granted nothing until a route typed its name, so it sat beside this
chain rather than in it while reading as though it were part of it.

- **The runtime's own account defaults to the FLOOR**, so an install that says
  nothing grants nothing.
- **Pointing `global.agentops.runtimeDefaults.serviceAccountName` at an account
  of your own moves the whole default**, and the chart still renders the floor —
  which is what keeps it NAMEABLE on one Pipeline to take that route back to
  nothing.
- **More than nothing is `rbac.runtime.serviceAccounts`** — a DECLARED account
  with its own posture. There is no preset.
| storage | `conversation.spec.contextClaimName` / `.workspaceClaimName` → the chart's release default (bootstrap `CONTEXT_PVC` / `WORKSPACE_PVC`) → EPHEMERAL |

- **THE RUNTIME IS IN NO PART OF THE STORAGE CHAIN**, and that is the whole of
  the move rather than an omission. `AgentRuntime.spec.home`, `spec.context` and
  `spec.workspace` are DELETED with no alias — see `terminology.md`. It keeps
  `spec.contextStorage`, which is BACKEND SHAPE (does this backend use a disk at
  all) rather than PLACEMENT.

- **THE PIPELINE IS IN NEITHER CHAIN.** Its fields are RESOLVED INTO the
  conversation at creation, and nothing reads a Pipeline at dispatch time —
  which is what stops an edit moving an INFLIGHT conversation onto different
  cluster power. See `terminology.md` for what the snapshot freezes.
- **`AgentProfile.spec.runtimeRef` is DEPRECATED**, dual-read for ONE release,
  and sits BELOW the Pipeline so adopting the new model needs no profile edit.
- **NAMING AN SA IS NOT CREATING ONE.** No reconciler makes one — TRUE NOW, and
  it was nearly true for a while: the adapter reconciler created one per
  workload, which is the one exception this rule lost. Adapters name
  `spec.serviceAccountName` and the CHART renders it. `Ready`
  does not check it — that would be an API read the manager holds no RBAC for,
  granted to produce a warning. The pod fails at admission naming the account.
- **A BUNDLE RENDERS THE ACCOUNTS ITS OWN ROUTES NEED.** It is the only scope
  that knows what its routes do. `kubernetes` renders one per route,
  `home-assistant` two with NO Kubernetes RBAC (neither route touches that API),
  `telegram` none because it ships no Pipeline. The SUBSTRATE stays the
  parent's — runtime, credential, context volume, floor account.
  - **"No subchart renders a runtime SA" is REVERSED, not weakened.** That rule
    guarded against a bundle rendering the substrate, including an account
    sized for everything. An account sized DOWN to one route is the opposite.

#### PERSISTENCE: A BINDING NAMES A CLAIM OR A VOLUME, AND THAT DECIDES WHO CREATES WHAT

`claimName` names a PVC that EXISTS. `volumeName` names a `PersistentVolume` —
and **a pod cannot mount one**, only a claim on it. So supporting a PV at all
means something renders the claim, and which thing follows from where the
binding was declared:

| Declared on | Renders the claim |
|---|---|
| a Pipeline | the MANAGER |
| the chart | the CHART, and a chart-shipped Pipeline's too, under the SAME derived name |

- **The two are mutually exclusive, enforced by CEL at the API server**, not by
  a reconciler reporting it after the object is stored. They are two answers to
  "who creates this", not a preference.
- **The derived name is `agentops-<pipeline>-<volume>`**, spelled identically in
  `runtimepod.PipelineClaimName` and in `pvc.yaml`. Both sides must agree or a
  route gets two claims and mounts the wrong one.
- **THIS IS THE ONE PLACE `NAMING IS NOT CREATING` DOES NOT HOLD**, and it is
  stated rather than smuggled. Elsewhere a name is a reference and nothing is
  provisioned from it.
- **THE CREATED CLAIM CARRIES NO OWNERREF ON THE PIPELINE, and the manager holds
  no `delete` verb.** Deleting a Pipeline must NEVER delete the accumulated
  context of the conversations it started — storage is the one thing here whose
  loss cannot be repaired by reconciling again. Guarded twice, deliberately.
- **The rendered claim's storage class is an EXPLICIT empty string**, never an
  absent field: absent is filled in by admission with the cluster's default
  class, which provisions a SECOND volume beside the one that was named.

**WHY IT IS HERE AND NOT ON THE RUNTIME.** A runtime is an ENGINE. Two Pipelines
sharing one must be able to keep their conversations on different volumes
without cloning it — exactly what expressing a second trust level used to
require, and fixed the same way. Whoever is trusted to grant an agent tools and
a cluster identity is more qualified to say where its context lives, not less.

**NO BINDING USES A BUILT-IN ROLE.** Not `cluster-admin` under `full`, not
`view` under `readonly` — the two postures a DECLARED account may state. Every grant is a role the chart writes out
(`agentops.runtimeReadRules` / `runtimeWriteRules`
in `chart/templates/_helpers.tpl`), so an operator can read it without resolving
an aggregated role.

**GRANTS ARE CLUSTER-WIDE, AND WHAT MAKES THAT SAFE IS OMISSION.**
`agentops.dev` is NEVER granted in any rule, so Conversations and Pipelines are
unreadable everywhere. Nor is `secrets`, nor `clusterroles`.

- **NAMESPACED ROLES WERE BUILT AND REVERTED.** RBAC cannot say "everywhere
  except", so bounding an agent meant an allow-list: one binding per namespace
  per account — 224 objects on a 28-namespace cluster — and a new namespace
  invisible to the agent until someone edited values. The maintenance was worse
  than the exposure. Re-adding it needs a better argument than the first one.
- **THE COST, NAMED:** under `full` an agent can restart or delete pods in the
  operator's own namespace, the manager and adapters included.

**`allowPodExecution` IS THE SECRETS BOUNDARY, AND IT DEFAULTS OFF.**

- **No `secrets` verb is NOT the same as cannot read Secrets.** The KUBELET
  resolves a Secret when it builds a pod, so an agent that can create a pod
  mounting one — or exec into one that has one — reads it having never asked the
  API server. `secrets: get` is never evaluated. PROVEN on a live cluster
  against this role: pod created, log read, value returned, all seven verbs
  denied.
- **It gates every write that PRODUCES OR ENTERS a pod**, not `pods: create`
  alone — a Job, a Deployment, a patch to a pod template are the same path.
  Gating one verb would be a flag that reads as a boundary and is not one.
- **`--allowedTools` IS NOT A CONTROL HERE.** An allowlist configures a
  COOPERATING agent; a ServiceAccount binding is what an uncooperative one with
  a shell actually has. Same argument `platform/egress-proxy/` makes for
  network reach.
- **BOTH WALLS MOVE TOGETHER.** kubernetes's MCP server is the other wall on the
  same path — an agent reaches the cluster THROUGH it — so it carries the same
  split from the same values. Fixing one leaves the hole one indirection along.
- **The manager keeps the stricter rule already**, holding no `secrets` verbs at
  all. The component running untrusted model output must not out-rank the one
  orchestrating it.

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
