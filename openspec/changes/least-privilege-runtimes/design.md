## Context

See `proposal.md` — Why. Four facts constrain the approach, and the second is
the one that decides the shape.

**A SUBCHART CAN READ NO PARENT SCOPE EXCEPT `global.`** That is Helm's rule, not
a convention this chart chose, and it is the entire reason there are three
`runtime` blocks today.

**A PARENT HELPER CALLED FROM A SUBCHART SEES ONLY `.Values.global` TOO**, and
this is the part nobody remembers. Named templates are global in Helm, so
`k8s-bundle/templates/mcp-server.yaml` calls the parent's
`agentops.runtimeWriteRules` to build its MCP server's RBAC — and inside that
call, `dig "agentops" "runtime" "allowPodExecution" false $g` resolves against
the SUBCHART's `.Values.global`. Move `allowPodExecution` to a parent-scope key
and the MCP server's write rules silently lose their gate.

That is also what makes `wiring.md`'s "BOTH WALLS MOVE TOGETHER" true rather
than aspirational: one value gates the runtime role and the MCP server's role
through one shared helper.

**`rbacMode` GRANTS NOTHING.** It renders an extra, named ServiceAccount; the
account does nothing until a `Pipeline` names it. Verified in
`runtime-rbac.yaml`, which refuses outright to bind anything to the floor.

**THE SA TOKEN IS MOUNTED AND THE IMAGE SHIPS `curl`.** `podspec.go` sets no
`automountServiceAccountToken: false`, and `runtimes/claude/Dockerfile` installs
curl. So whatever an account holds is callable from `Bash` directly, not only
through MCP. This does not change what the chart grants; it decides how much the
default matters.

## Goals / Non-Goals

**Goals:**

- One default posture, and it is NOTHING. No setting can widen it.
- Two runtime values blocks, with a stated rule for which is which.
- Defaults that are SUFFICIENT — `runtimes: []` yields a working install.
- A runtime shippable by a bundle, without losing the guarantee that an install
  can execute.
- Subchart keys named for the system they integrate.
- Every retired key failing the render rather than being ignored.

**Non-Goals:**

- **`automountServiceAccountToken`.** Turning it off for routes that name no
  account is a real tightening and a separate change — folding it in here would
  put a runtime-behaviour change inside a values restructure.
- **Changing what the acting rules ENUMERATE.** The verbs are unchanged; only
  who selects them moves.
- **A migration that preserves `rbacMode` semantics.** There is no alias: the
  concept is deleted, not renamed.
- **Renaming published images.** Subchart keys move; image names do not.

## Decisions

### `global.agentops.runtimeDefaults`, and why it is `global.` rather than tidy

Everything a runtime inherits goes in one block under `global.`, including the
keys the parent alone reads.

- **Two independent forcings, not a preference.** `serviceAccountName` is read by
  three bundles' MCP server templates, and `allowPodExecution` by the shared
  write-rules helper in subchart context. Either alone requires `global.`
- **A bundle-shipped runtime has no other scope to inherit from.** Once a bundle
  may render an `AgentRuntime`, the defaults must be reachable from inside that
  bundle — and `global.` is the only place it can look.
- **The parent-only keys ride along**, which costs nothing: a subchart that does
  not read `image` is not harmed by being able to.
- **Alternative rejected — keep `runtimeDefaults` at parent scope and leave the
  two keys under `global.agentops.runtime`.** That preserves three blocks to
  avoid exposing values nobody reads, and leaves the rule separating them exactly
  as unstateable as it is today.

### "Defaults" means SUFFICIENT, and the resources come with it

The defaults block SHALL render a working runtime on its own. `runtimes: []` is a
valid install.

- **The credential is the single exception.** A secret has no defensible default,
  so it stays the one value an install must supply.
- **`resources` is written out**, replacing `resources: {}`. The numbers already
  exist — 100m/256Mi requests and 1/1536Mi limits, in `podspec.go` — so this
  moves them from Go to where an operator can read and tune them. Behaviour is
  unchanged on every install.
- **The Go constants stay** as the fallback for a deployment with no
  `AgentRuntime` at all. That is not duplication: one is the chart's default,
  the other is the operator's floor when the chart is not what rendered the CR.

### The default account is a REFERENCE; the floor is always rendered

Two accounts, and the second is the useful half.

| Account | Rendered by | Bound to |
|---|---|---|
| the chart's floor | the chart, ALWAYS | nothing |
| whatever the default names | the operator | whatever they chose |

- **Naming is not creating**, the posture adapters already have. It is also what
  makes reusing an account you own possible at all: the chart creating it would
  collide on Helm's ownership check.
- **Always rendering the floor is what buys the RESTRICT case.** On an install
  whose inherited default carries rights, naming the floor on one Pipeline is the
  only way to take that route back to nothing.
- **Alternative rejected — a `createServiceAccount: false` flag.** It answers the
  same need with a toggle, and gives no way to restrict a single route.

### The `default` runtime guard replaces an invariant

"The parent always renders `default`" cannot survive the runtime shipping in a
disableable bundle. The replacement is a render-time check: FAIL when no declared
runtime answers to `default` and any Pipeline resolves to it.

- **It needs no cluster**, so it protects a GitOps install exactly as it protects
  an interactive one — unlike the claim-rename guard, whose `lookup` is blind
  without one.
- **Failing is the point.** The alternative is conversations reaching `Pending`
  forever with the reason in the manager's log and nowhere an operator looks.
- **Routes naming their own runtime need no default**, so the check is
  conditional on something actually resolving to that name.

### Subchart names, and why not just dropping the suffix

Each subchart is named for the SYSTEM it integrates: `kubernetes`,
`home-assistant`, `telegram`, `prometheus`, `claude`, `ollama`.

- **Dropping `-bundle` mechanically gives `k8s` and `ha`**, which are not
  descriptive, and both collide in reading with PIPELINE names an install already
  declares — `k8s-ops`, `k8s-observe`, `ha-ops`, `ha-control`. The same string on
  two kinds of object is what the terminology rules exist to prevent.
- **It is the widest values break this chart has shipped**, so every retired key
  fails the render naming its replacement. Helm reports no unread values key,
  and the quiet outcome — a bundle silently not rendering — is indistinguishable
  from an operator who meant to leave it off.

### `k8s-bundle`'s derivation becomes three stated settings

Widening one value moved the MCP server's read-only flag, that server's RBAC and
which route rendered. The grouping was right; the trigger was not.

- **They may still move together**, stated as such. What is refused is a setting
  whose name describes none of what it changes.
- **The walls must not drift apart.** `wiring.md` records why: an agent reaches
  the cluster THROUGH the MCP server, so fixing one wall leaves the hole one
  indirection along. Whatever replaces the trigger has to move both.

### Egress mediation on by default

- **The cost is real and lands late.** A privileged `NET_ADMIN` init container is
  refused by a `restricted` Pod Security namespace — at POD ADMISSION, not at
  render, so the failure appears far from the setting.
- **The notes name it**, rather than the render refusing. The chart cannot see
  the namespace's Pod Security level, so a guard here would be a guess.

## Risks / Trade-offs

- **THE WIDEST VALUES BREAK THIS CHART HAS SHIPPED.** Every operator has set at
  least one renamed key. → Every retired key fails the render naming its
  replacement, with no cluster needed, and the CHANGELOG is written first.
- **Demo mode may need something this change assumes it does not.** The claim
  that demo's cluster reads come from the MCP server's own account is reasoned,
  not measured — the local demo run that would have proved it inherited
  `rbacMode: full` from a production values block. → Verify it on a clean install
  before the mode is deleted, not after.
- **A bundle rendering a runtime re-opens a failure mode.** → The `default` guard
  is what closes it, and it must land in the same change as the capability, not
  after.
- **Egress on by default will break an install under `restricted` Pod Security**,
  and it will look like an unrelated admission error. → Named in the notes and in
  the CHANGELOG's upgrade steps.
- **The floor account holding nothing is now the ONLY thing between an unnamed
  route and the API**, because the token is mounted and `curl` is present. →
  Unchanged by this design, and the reason the default must stay at zero rather
  than "minimal". It also argues for the deferred
  `automountServiceAccountToken` change.
- **Two accounts may confuse where one did.** An install could name a default AND
  see the chart render a floor it does not use. → It is the mechanism that makes
  a single route restrictable, and the values comment has to say so where the
  default is set.
