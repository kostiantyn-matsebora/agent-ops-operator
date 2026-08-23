## Context

See `proposal.md — Why`. The current state, in the four facts that shape the
design:

- **`ResolveFor(profile, ...)` in `internal/runtimepod/podspec.go`** is the ONE
  resolution point: `profile.Spec.RuntimeRef` → the `AgentRuntime` named
  `default` → the manager's bootstrap config. It returns the runtime, and the
  pod builder reads `rt.ServiceAccountName` off it.
- **The Conversation already materializes wiring** — `profileRef`,
  `channelRefs`, `toolsets`, `mcpConfigs` — snapshotted at creation, with
  CONTENT re-read at every use. Refs are frozen, content is not.
- **`ResolveFor(...).ContinuityPossible()`** is called by `/exit` and by
  dispatch to decide whether context survives. It takes a profile today.
- **The chart renders exactly one runtime SA**, `agentops-runtime`, and
  `runtime-rbac.yaml` binds mode-driven roles to it.

## Goals / Non-Goals

**Goals**

- ONE object states an agent's power: tools, servers, runtime, identity.
- A Pipeline edit never re-wires a running conversation's identity.
- An install that sets neither new field behaves exactly as it does today.

**Non-Goals**

- Per-conversation SA creation. The Pipeline NAMES an SA, and who may create
  one and what it is bound to stays an external grant — the same posture
  adapters already have.
- Validating that an SA exists or that its RBAC is sufficient. A missing SA
  fails at pod admission, visibly, and the manager holds no RBAC to check.
- Changing how the runtime image is selected per vendor. See D2.

## Decisions

### D1 — Two fields, not a composite ref

**Chosen:** `Pipeline.spec.runtimeRef` and `Pipeline.spec.serviceAccountName`,
independent.

**Alternative:** one `runtime: {ref, serviceAccountName}` stanza, or a set of
pre-baked `AgentRuntime` objects per trust level with no SA field at all.

**Why separate:** they answer different questions and vary independently. The
runtime is WHAT EXECUTES (a vendor image, a home volume, an idle TTL) and is
shared by every route using that vendor. The SA is HOW MUCH POWER this route
gets, and is the thing that differs between an observing route and an acting one
on the SAME image.

**This is what makes `runtimes are generic` stop being a workaround.** Today two
trust levels need two `AgentRuntime` objects — identical but for the SA — purely
because the SA has nowhere else to live. With the field on the Pipeline, one
runtime serves both and the difference is stated where the rest of the route's
power is stated.

**Cost, accepted:** two fields to read instead of one, and a Pipeline that names
an SA the runtime also names has to be understood as an override. D3 pins the
precedence so it is not guessed.

### D2 — The profile keeps NO runtime hint. A mis-wired Pipeline fails visibly

**Chosen:** `runtimeRef` leaves the profile entirely, and nothing replaces it.

**Alternative, from the proposal's open question:** a `runtimeClass`-style hint
on the profile that `Ready` validates the Pipeline's runtime against.

**Why not:** the coupling is real — a profile written against claude-code's
`.claude/agents/` layout cannot run on an arbitrary runtime — but a hint buys
little and costs a vocabulary.

- **It cannot be checked usefully.** The manager would compare two opaque
  strings an author typed. That catches a typo, not an incompatibility, and a
  correct pairing with mismatched strings would be REFUSED.
- **The failure is already loud and local.** A profile dispatched to a runtime
  that cannot read its prompt file fails its first run, with the runtime's own
  error, on a conversation that names both.
- **It re-creates the split this change closes.** A profile constraining which
  runtime a Pipeline may name is the profile influencing wiring again, one
  indirection later.

**When it stops being true, add it deliberately** — the moment there are two
vendor runtimes in one install, a `Ready` warning naming the mismatch is worth
having. Today there is one.

### D3 — Precedence is stated, not inferred

Resolution runs in this order and no other:

| Field | Order |
|---|---|
| runtime | `conversation.spec.runtimeRef` → `pipeline.spec.runtimeRef` → **`profile.spec.runtimeRef` (deprecated)** → `AgentRuntime/default` → bootstrap |
| service account | `conversation.spec.serviceAccountName` → `pipeline.spec.serviceAccountName` → `runtime.spec.serviceAccountName` → **THE MINIMUM-PRIVILEGE ACCOUNT** |

**THE BOTTOM OF THE IDENTITY CHAIN IS THE LEAST POWER, NOT THE MOST.** See D8.
A route that names no account gets an identity that can do NOTHING in the
cluster.

- **The conversation comes FIRST because it is the snapshot** (D4). Nothing
  reads the Pipeline at dispatch time.
- **The deprecated profile ref sits BELOW the Pipeline**, so an install that
  sets both moves to the new model immediately, and one that sets only the old
  keeps working.
- **An SA on the Pipeline OVERRIDES the runtime's.** That is the whole point of
  the field: one runtime, several trust levels.

### D4 — The Conversation snapshots both, or a Pipeline edit escalates

`Conversation.spec` gains the resolved runtime and SA at creation, beside
`toolsets` and `mcpConfigs`, following `mcp-toolset-model`'s existing rule.

**REFS ARE SNAPSHOTTED, CONTENT IS NOT** — and here the distinction is sharper
than anywhere else it applies:

- **The snapshot must be the RESOLVED names**, not the Pipeline's raw fields, so
  a conversation created while the Pipeline named nothing keeps the default it
  actually ran with rather than picking up a later edit.
- **The AgentRuntime's CONTENT is still re-read** — image, TTL, volume — so
  fixing a runtime heals running conversations, exactly as editing a toolset
  does.
- **Without the snapshot, editing a Pipeline changes what identity an INFLIGHT
  conversation's next pod runs as.** That is not a re-wiring inconvenience, it
  is a privilege change applied to work already in progress, and it is the
  strongest instance of the rule this codebase already has.

### D5 — `ResolveFor` takes the CONVERSATION, not the profile

Its signature changes, and every caller with it — `/exit`, dispatch's
continuity check, the admission path.

**Why not add a parameter:** the profile is no longer an input to the answer at
all. Leaving it in the signature would leave a caller free to pass one and
believe it mattered, which is the state this change exists to end. The
deprecated profile ref is read INSIDE, from the conversation's own
`profileRef`, for the one release it survives.

**`ContinuityPossible()` is unaffected in meaning.** It reads the resolved
runtime's `contextStorage`, and the resolved runtime is now resolved differently.
Its callers ask the same question and get the same answer for every install that
has not adopted the new fields.

### D8 — SILENCE MEANS NO POWER. The fallback account is MINIMUM-PRIVILEGE

**Chosen:** the chart always renders an account with NO bindings at all, and a
Pipeline naming none runs as that. An agent whose route says nothing can do
nothing in the cluster — it reaches only what its MCP servers and toolsets give
it, which are declared on the same object.

**Rejected, and it is what this change first shipped:** absent → the runtime's
`serviceAccountName` → whatever `rbacMode` bound to it. That made SILENCE MEAN
MAXIMUM, which is the exact shape this change exists to end.

**The argument that killed it is the change's own.** D7 says a `cluster-admin`
default means "every Pipeline naming the same all-powerful account states
nothing". Shrinking that role's CONTENTS and leaving the FALLBACK pointed at it
fixes the blast radius and not the model: three of four routes in the reference
install held pod-delete and node-patch because nobody typed a field, and two of
them were Home Assistant routes that need no cluster access whatever.

**`rbacMode` therefore stops applying to the fallback account.** Acting power is
attached to a NAMED account a route opts into. A mode that silently widens
whatever a route inherits is the same defect one level up.

**Cost, accepted, and it is the third breaking half:** an install running
`rbacMode: full` loses cluster reach on every route until each names an account.
That is the point — the grant becomes something written down per route rather
than a release-wide default nobody re-reads.

- **FAILS CLOSED.** An agent is refused and says so, which is recoverable. The
  inverse failed OPEN and was silent.
- **The account still EXISTS** rather than being absent, so a pod spec always
  names one and a cluster-side audit can enumerate what each route holds.

### D9 — A BUNDLE RENDERS THE ACCOUNTS ITS OWN ROUTES NEED

**Chosen:** a bundle that ships Pipelines also ships the ServiceAccounts and
RBAC those Pipelines require, scoped to exactly what those routes do.

**Rejected, and it is the rule this change first restated:** "the parent owns
every runtime SA, a bundle naming one creates nothing."

**Why it reverses:** the bundle is the only scope that KNOWS what its routes
need. `k8s-bundle` knows `k8s-operate` deletes pods and `k8s-observe` does not.
`ha-bundle` knows neither of its routes touches Kubernetes at all. Forcing every
account through the parent means the parent must restate each bundle's needs,
which is the two-spellings-of-one-fact problem, and in practice means one shared
account sized to the most demanding route.

**THE OLD RULE IS NOT WRONG ABOUT WHAT IT WAS PROTECTING.** `invariants.md` says
putting the runtime in a bundle "made TWO runtime SAs exist, one granted
everything". That failure was a bundle rendering THE SUBSTRATE — the runtime,
its credential, an account sized for everything. This is the opposite: an
account sized DOWN to one route.

**So the invariant splits rather than falls:**

| Still the parent's, exclusively | Now the bundle's |
|---|---|
| `AgentRuntime`, the model credential, the home volume, the idle TTL | the ServiceAccount + RBAC for each Pipeline the bundle itself renders |

- **A bundle that ships NO Pipeline ships no account.** `telegram-bundle` stays
  exactly as it is.
- **A bundle's account is named on the bundle's own route**, never inherited by
  anything else.
- **The parent keeps the MINIMUM-PRIVILEGE fallback** (D8) and any accounts the
  operator declares.

### D6 — MANY ACCOUNTS PER RELEASE, and the parent's is the FLOOR

`global.agentops.runtime.serviceAccountName` stays and still defaults to
`agentops-runtime` — but what it names changes meaning under D8. It is the
MINIMUM-PRIVILEGE floor a Pipeline naming nothing gets, not a default posture
that `rbacMode` widens.

`runtime-rbac.yaml` renders bindings for MORE THAN ONE account, because a second
trust level is now expressible without a second runtime.

- **"Exactly one runtime ServiceAccount per release" is DEAD**, not weakened to
  "one default". A release has as many as its routes need: the floor, whatever
  its bundles render for their own routes (D9), and whatever the operator
  declares.
- **The floor account NEVER receives `rbacMode` bindings.** Widening it would
  re-create the fail-open default D8 exists to remove.
- **A Pipeline may name an account the chart did not create.** That is an
  external grant, the same posture adapters already have, and neither the render
  nor a reconciler refuses it.

### D7 — The acting role is ENUMERATED, and `cluster-admin` goes

`rbacMode: full` binds the runtime ServiceAccount to `cluster-admin` today. It
is replaced by a ClusterRole the chart writes out.

**Why this belongs in THIS change and not a later one:** the whole change is
about making an agent's power reviewable on one object. Landing that while the
default identity is `cluster-admin` would move a decision that has no content —
every Pipeline naming the same all-powerful account states nothing.

**`--allowedTools` IS NOT A CONTROL HERE.** The allowlist configures a
COOPERATING agent. A ServiceAccount binding is what an uncooperative one
actually has, and an agent has a shell. This is the same argument
`platform/egress-proxy/` already makes for network reach, applied to RBAC.

**The manager already keeps the stricter rule.** It holds no `secrets` verbs at
all — everything secret-shaped compiles to `valueFrom` and the kubelet resolves
it. The component running untrusted model output must not out-rank the component
orchestrating it.

**Writing the role is the work, and it is a judgement call, not a mechanical
one.** The starting point is what `k8s-bundle`'s MCP server is already permitted
to do, because that enumeration exists and has been in use — the difference is
that the MCP server's grant is read-only under `readonly` and this one is the
acting half.

- **No wildcard on resources or apiGroups.** `*` reaches Secrets without naming
  them, which is how a role passes review and fails its purpose.
- **No `escalate` / `bind` on RBAC**, or the role can widen itself.
- **Enumerate the workload verbs an agent genuinely fixes with** — scale, patch,
  delete a pod, roll a deployment, cordon — and nothing else.

**An install needing more adds its OWN ClusterRole and names the account on a
Pipeline.** That is the shape this change creates: a grant that is deliberate,
separately reviewable, and attached to the route it serves. A mode name that
silently means everything is what it replaces.

**AND THE ROLE IS NEVER BOUND TO THE FLOOR ACCOUNT** (D8). `rbacMode: full`
declares that an acting account EXISTS and what it may do. It does not decide
what an unnamed route inherits, because an unnamed route inherits nothing.

## Risks / Trade-offs

- **A Pipeline author can now escalate to any SA in the namespace** → true, and
  it is the intended trade: that authority already existed, spread across
  profile-edit and runtime-edit rights, where nobody could see it. Concentrating
  it makes it reviewable. Installs that care restrict who may write Pipelines,
  which is the same control that already governs tool grants.
- **Two fields where an install previously had one** → mitigated by both being
  optional with today's behaviour as the default. An install that adopts
  neither sees no change.
- **The deprecated dual-read is a real branch** → time-boxed to one release and
  deleted with the field, the same as `sessionId`. The task list names the
  deletion rather than leaving it to a later sweep.
- **Dropping `cluster-admin` BREAKS installs that relied on it** → intentionally,
  and it is the second breaking half. Mitigated by the enumerated role covering
  what the shipped profiles actually do, and by the escape hatch being an
  explicit ClusterRole rather than a values flag that re-widens the default.
- **The enumerated role will be wrong at first** → likely, and it fails CLOSED:
  an agent is refused an action and says so, rather than quietly holding power
  nobody reviewed. Add verbs on evidence, one at a time.
- **A conversation created before the snapshot has neither field** → it falls
  through to the same chain it always used, because the deprecated profile ref
  is still in it. Nothing backfills.

## Migration Plan

1. **Add the Pipeline fields and the Conversation snapshot**, with resolution
   preferring them and falling back to the profile. Nothing sets them, so every
   install behaves identically. Verifiable in production as a no-op.
2. **Chart exposes both on `pipelines:` values**, unset by default.
3. **Move installs one Pipeline at a time**, watching that the pod comes up
   under the intended SA.
4. **Delete the profile field** in the following release, once no profile in the
   install sets it.

**Rollback:** clear the Pipeline fields. Resolution falls through to the profile
ref, which is still read. Only conversations created while the fields were set
carry the snapshot, and they carry the identity they already ran under.

## Open Questions

- **Should `Ready` refuse a Pipeline naming an SA that does not exist?** It
  would be a read the manager currently has no RBAC for, and adding one to
  validate a name is a permission granted for a warning. Leaning no — the pod
  fails at admission with a message naming the SA — but worth deciding before
  the reconciler work.
