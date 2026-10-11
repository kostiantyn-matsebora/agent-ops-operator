## Context

See `proposal.md` for the motivation. This covers how the pieces fit
together.

Two existing mechanisms this design reuses rather than reinvents:

- `Conversation.spec.originReader` (`{channel, key}`, key a salted hash)
  already records who started a conversation, for every channel, not only
  the console.
- The manager's inbound contract already carries a `sender` string on every
  reply and origination, for every channel, because the console's own
  `handleSend` forwards `Identity(r)` as that field exactly as Telegram's
  adapter forwards its own sender.

Neither needs to change. This design adds a role lookup beside them.

## Goals / Non-Goals

**Goals:**
- One role per `{channel, key}` identity, enforced the same way for every
  channel.
- Exactly one writer of role data (the console), exactly one reader besides
  it (the manager), both narrowly scoped.
- No change to how `originReader` is stamped or hashed.

**Non-Goals:**
- Cross-channel identity linking (see proposal's explicit out-of-scope
  list).
- Any change to who may START a conversation. This design only gates
  continuing one.
- Implementing the control-plane route namespace's future residents
  (maintenance mode, pause/resume). This design only reserves the name and
  ships its first resident, role assignment.

## Decisions

### A new CRD, `RoleGrant`, not `RoleBinding`

Kubernetes already has `rbac.authorization.k8s.io/v1.RoleBinding`. Naming
this kind `RoleBinding` would read as a Kubernetes RBAC object from `kubectl
get`, when it grants nothing at the Kubernetes API level at all. `RoleGrant`
avoids the collision, matching this project's existing care around reusing a
word for two things (`terminology.md`: "agent," "operator").

One object per `{channel, key}` grant, namespaced like every other
`agentops.dev` kind. Fields:

```yaml
apiVersion: agentops.dev/v1alpha1
kind: RoleGrant
spec:
  channel: console          # or telegram, etc — matches OriginReader.Channel
  key: administrator@example.com    # RAW identity, see below — never hashed
  role: administrator       # user | responder | administrator
status:
  lastModifiedBy: operator@example.com
  lastModifiedAt: "2026-10-11T12:00:00Z"
```

No `delete` verb is granted to the console. Demoting someone back to
`user` is an explicit `update` to `role: user`, not object removal — this
keeps a record of who was demoted and when, and keeps the console's RBAC
grant to exactly two verbs (`create`, `update`) rather than three.

The console also stamps `status.lastModifiedBy`/`lastModifiedAt`, so it holds
`update` on `rolegrants/status`.

### `RoleGrant.spec.key` is RAW. `OriginReader.key` stays a SALTED HASH

These are two different needs on the same identity:

- An administrator must be able to type or pick a specific person
  (`administrator@example.com`). That requires the raw string.
- `OriginReader` deliberately stores only a salted hash, so a conversation
  object never carries a raw identity — a privacy property this design does
  not touch.

Enforcement reconciles the two without changing either. The manager hashes
the inbound `sender` string with the SAME function used when `OriginReader`
is stamped, to check ownership. It uses the same `sender` string RAW to
look up `RoleGrant` by `{channel, key}`.

One string, hashed once for one comparison and used plain for the other,
both derived fresh from the request. Nothing is persisted that was not
already being persisted.

### A conversation with no `originReader` has no owner, and is not restricted

Alert- and job-originated conversations, and any conversation predating the
field, carry no `originReader` (`invariants.md`).

A strict reading of "a `user` may only act on their own conversation"
would refuse every `user` reply on these.

That is wrong for this product's own domain. An alert opens an incident
conversation so whoever is on call can jump in — not only whoever the
system happens to attribute it to.

**Decision**: an absent `originReader` means there is no owner to violate.
Every role may reply. The restriction applies only when `originReader`
names someone else.

This needs to be a stated requirement, not an implementation accident. The
alternative — restrictive by default — is the one that would ship if
nobody decided it on purpose.

### Enforcement lives in the manager, not duplicated per channel

`POST /channel/inbound` and addressing an existing conversation
(`internal/chat` `HandleCommand`) are the two places every channel's
"continue this conversation" traffic already converges, regardless of which
adapter sent it. One check here covers Telegram and the console both, with
no per-adapter code.

The console's own `a.write()` gate on `handleSend` stays, but only as a
fail-fast UX nicety — the manager's check is the actual authority, since
the console's own reply already flows through the same endpoint. Removing
the console-side check would not create a hole, only a slower error.

### The manager gets read-only RBAC on `RoleGrant`. Never write

Considered giving the manager write access too, for its own `/admin/roles`
endpoint alongside the console's.

Rejected:

- Every additional writer of "who is an administrator" is one more code
  path that could grant it wrongly.
- The console already resolves end-user identity and already logs every
  write (`console write: action=%s identity=%s`).
- A second, independent writer buys nothing and doubles the review surface
  for the one piece of data that controls everything else.

### Console's new write RBAC is scoped to one kind, not "console can write now"

The chart-rendered `ClusterRole` in `chart/templates/console.yaml` already
carries a dedicated `apiGroups: ["agentops.dev"]` block for console's
read-only grants. This adds `create`/`update` on `RoleGrant` to that same
block. Every other kind, and the Configuration page's existing read-only
position, is untouched.

This is a stated reversal of `docs/console.md`'s "there is no write path to
the Kubernetes API anywhere in the module," not a silent one — see
`proposal.md`'s Impact section for the docs this touches.

## Risks / Trade-offs

- **[Risk]** Console ServiceAccount token theft bypasses every
  application-level check. A compromised console pod with its own SA
  token could write `RoleGrant` directly against the API server, with no
  HTTP-layer identity or logging in between.
  → **Mitigation**: scope stays to exactly one kind and two verbs, never a
  general write grant. This is a real, accepted residual risk of giving
  any component its own write RBAC, named rather than hidden — the same
  risk the manager's own broad write RBAC already carries today, now
  shared with one more component for one narrow kind.

- **[Risk]** `OriginReader`'s salt rotating would silently break every
  existing ownership match. This is an existing property of
  `OriginReader`, not new here. Today a broken match only mislabels a UI
  tab ("mine"). After this change it also refuses a legitimate reply.
  → **Mitigation**: out of scope to fix the salt-rotation story in this
  change. Flagged here so it is not rediscovered as a surprise later.

- **[Risk]** Two independent grants per human (console email, Telegram
  username) is confusing for an administrator managing a small team.
  → **Mitigation**: explicitly out of scope per the proposal. The
  console-guide documentation must say this plainly, rather than let
  someone discover it by promoting the wrong identity.

**[Trade-off] No `delete` verb for the console on `RoleGrant`.** Keeps the
grant narrower and keeps a demotion's history, at the cost of
`RoleGrant` objects persisting for every identity ever explicitly set,
rather than reverting to "no object" on demotion.

## Migration Plan

1. Ship the CRD. Zero existing objects — every identity defaults to `user`
   from the moment this lands, until an administrator is bootstrapped. The
   shared-token identity is `administrator` by construction. Under external
   authentication the chart value `console.rbac.bootstrapAdmins` names the
   first administrators (see `specs/access-control/spec.md`).
2. `helm upgrade` re-renders the console's `ClusterRole` with the new verbs.
   `kubectl apply -f chart/crds/` is required on the upgrade path, same as
   any new CRD (`gotchas.md`: Helm installs a CRD only when absent).
3. Rollback: reverting the console/manager images and chart drops the new
   route and the new RBAC checks. Any `RoleGrant` objects left in the
   cluster are simply unread by the older binaries — a safe no-op, not a
   dangling reference anything depends on.

## Open Questions

- Exact JSON shape of `status.lastModifiedBy`/`lastModifiedAt` (string
  format, omitempty) — pin during implementation, does not change the
  approach.
- Whether the control-plane route namespace needs its own auth middleware
  function or can wrap `a.write()` with an added role check — an
  implementation choice inside `platform/console`, not a behavior change.
