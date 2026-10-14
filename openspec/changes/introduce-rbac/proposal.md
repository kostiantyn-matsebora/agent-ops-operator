## Why

Every identity the console or a channel resolves today can do everything the
install permits. It can start any conversation, reply to anyone's, and
bulk-close or bulk-delete across the whole install.

There is no distinction between an operator running the thing and a teammate
who should only watch and work their own requests.

There is no way to let someone send a task and follow up on it without also
handing them the ability to act on, or delete, everyone else's.

## What Changes

- Add three roles — `user`, `responder`, `administrator` — scoped per
  `{channel, key}` identity pair, the same shape
  `Conversation.spec.originReader` already uses. This is per identity pair,
  not per bare identity string. A Telegram sender and a console external-authentication
  identity for the same human hold independent grants unless linked later.
- Add a new CRD, one object per `{channel, key}` → role binding. This follows
  the existing one-object-per-binding convention (`Channel`, `SignalSource`,
  etc.) rather than a singleton holding a map.
- Any `{channel, key}` with no explicit binding defaults to `user`. This is
  what makes "an externally authenticated login gets `user` automatically" true with no
  provisioning step.
- **BREAKING (deliberate invariant reversal)**: the console gains its own,
  narrowly-scoped Kubernetes write RBAC: `create`/`update` on this one new
  CRD kind, added to its existing chart-rendered `ClusterRole`. This reverses
  the documented claim "there is no write path to the Kubernetes API
  anywhere in the module" (`docs/console.md`), for this one kind only. The
  console stays read-only on every other kind. The manager gets read-only
  watch on the new CRD and never writes it. Role assignment has exactly one
  writer.
- Add a **control-plane** route namespace on the console's own API
  (`/api/control-plane/...`), distinct from `/api/admin/...` naming so the
  word "admin" cannot mean both a role and a path. Role assignment is its
  first resident. Later administrator-only actions (such as the
  maintenance-mode toggle an in-flight, separate change is adding) are
  expected to land there too, but this change does not implement them.
- Console: filter conversation list/detail to the requester's own
  `{channel, key}` for `user`. `responder` and `administrator` see all.
  Hide, rather than anonymize, conversations the requester cannot see from
  Topology's Model view. `Conversation.metadata.name` is a deterministic
  word-chain derived from the conversation's own text, so it is not a safe
  label to show for a conversation someone cannot otherwise see.
- Manager: enforce role plus ownership on `POST /channel/inbound` (a reply)
  and on addressing an EXISTING conversation, for every channel uniformly.
  This reuses the `sender` field every channel's inbound traffic already
  carries. A `user`-role sender's reply or address to a conversation whose
  `originReader` is not theirs is refused. Starting a NEW conversation
  (`POST /signal/inbound`, or addressing a Pipeline fresh) stays open to
  every role. This change gates continuing someone else's conversation, not
  starting one's own. Both contracts are specified in this change: the reply
  contract and the addressing-by-name delta (`chat-addressing-discovery`).
- `responder` may bulk-close (reversible via reopen) but not bulk-delete.
  Bulk-delete and role assignment are `administrator`-only.
- Bootstrap: the console's shared-token identity (`identity == "token"`) is
  `administrator` by construction. An install running external
  authentication with no shared token names initial administrators in a new
  chart value, read once.
- Explicitly out of scope:
  - Unifying one human's identity across channels.
  - Gating who may start a conversation.
  - Hiding a Telegram topic's existence — Telegram's own group/forum
    permissions govern that. This change can only refuse a non-owner's input
    into it, never make the topic disappear.
  - Implementing the maintenance-mode or pause toggles the control-plane
    namespace is designed to later hold.

## Capabilities

### New Capabilities
- `access-control`: the role model, the `{channel, key}` identity scope, the
  role-binding CRD, default-to-`user`, bootstrap administrators, and the
  manager-side enforcement decision (role plus `originReader` ownership)
  used uniformly across every channel.

### Modified Capabilities
- `console-application`: the authentication decision gains a role
  dimension. Conversation list/detail reads are filtered by role and
  ownership. A new `/api/control-plane/...` route namespace carries
  `administrator`-only writes, including role assignment itself, with the
  console's own write RBAC extended to the new CRD.
- `console-topology`: the Model view's conversation nodes are omitted
  entirely, not anonymized, when the requester cannot see the underlying
  conversation.
- `channel-adapter-contract`: inbound reply routing (`POST
  /channel/inbound`) gains a role and ownership check against the target
  conversation's `originReader`, applied uniformly to every implementing
  adapter rather than console-specific.

## Impact

**Code**:
- A new CRD type under `platform/manager/api/v1alpha1/`, plus generated
  deepcopy and CRD YAML.
- A read-only manager-side watch and lookup for it.
- An authorization check in `internal/httpapi` (inbound) and
  `internal/chat` (addressing an existing conversation).
- Role-aware filtering in `platform/console/auth.go` and `conversations.go`.
- A new `/api/control-plane/...` route group and its first handler (role
  assignment) in `platform/console`.
- New write verbs on `chart/templates/console.yaml`'s `ClusterRole`:
  `create`/`update` on `rolegrants` and `update` on `rolegrants/status`.
- A new chart value for bootstrap administrators under external
  authentication.

**Reference docs**:
- `docs/concepts.md` — the new CRD and role semantics.
- `docs/contracts.md` — the inbound contract's new refusal case.
- `docs/CHANGELOG.md` — the breaking change and the `kubectl apply -f
  chart/crds/` upgrade step for the new CRD.

**Adopter site**:
- `docs/console-guide.md` — a role-assignment section alongside the
  existing authentication-decision section.
- `docs/console.md` — the "no write path to the Kubernetes API" claim must
  be corrected, as a stated reversal, not silently dropped.
- `docs/security.md` — a new trust-boundary entry: the console can now
  mutate who is an administrator, and what bounds that.
- `docs/installation.md` — the bootstrap-admins value. It is set once and
  not undoable by a later `helm upgrade`, so the irreversibility rule places
  it here and not on `configuration.md`.

**Not impacted**: the in-flight `pause-maintenance-mode` change is left
entirely as its own design, with its own manager `/admin/*` endpoint for its
own two callers (the console's UI and an agent's MCP tool).

This change only reserves the naming precedent (`control-plane`, not
`admin`) for whichever of that change's console-facing pieces land after
this one.
