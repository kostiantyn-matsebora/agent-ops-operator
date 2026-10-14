## Purpose

The role model agent-ops enforces for every identity it resolves: who may
continue a conversation that already belongs to someone else, who may see
every conversation rather than only their own, and who may grant that power
to others.

## ADDED Requirements

### Requirement: Three roles scope every identity's reach

Each `{channel, key}` identity pair SHALL hold exactly one role: `user`,
`responder`, or `administrator`.

The role SHALL be looked up per identity pair, never per bare identity
string. A Telegram sender and a console identity for the same person SHALL
hold independent roles unless an administrator links them.

The three roles SHALL carry this capability matrix:

| Capability | user | responder | administrator |
|---|---|---|---|
| Start a new conversation | yes | yes | yes |
| Reply to or address a conversation they own | yes | yes | yes |
| Reply to or address a conversation owned by someone else | no | yes | yes |
| Reply to or address a conversation with no owner | yes | yes | yes |
| See every conversation, not only their own | no | yes | yes |
| Bulk-close conversations | no | yes | yes |
| Bulk-delete conversations | no | no | yes |
| Assign roles | no | no | yes |

#### Scenario: A user is scoped to their own conversations

- **WHEN** a `user`-role identity is checked against this matrix
- **THEN** it may start conversations and act on its own, but not on one
  owned by a different identity

#### Scenario: Roles are independent per channel

- **WHEN** an administrator grants `administrator` to a console identity for
  a person
- **THEN** that person's Telegram identity keeps whatever role its own
  `{channel, key}` pair carries, unchanged

### Requirement: RoleGrant records one role binding per identity pair

A `RoleGrant` custom resource SHALL record exactly one `{channel, key}` to
role binding, namespaced like every other `agentops.dev` kind, following
the one-object-per-binding convention `Channel` and `SignalSource` already
use.

Its spec SHALL carry `channel` (matching the value space of
`Conversation.spec.originReader.channel`), `key` (the RAW identity string,
never hashed), and `role` (one of `user`, `responder`, `administrator`). Its
status SHALL carry `lastModifiedBy` and `lastModifiedAt`, recording who last
wrote it and when.

#### Scenario: A grant names one identity pair and one role

- **WHEN** a `RoleGrant` is created with `spec.channel: console`,
  `spec.key: administrator@example.com`, `spec.role: administrator`
- **THEN** exactly that identity pair resolves to `administrator`, and no
  other identity pair is affected

### Requirement: One RoleGrant per identity pair

At most one `RoleGrant` SHALL exist for any `{channel, key}` pair. The
console SHALL refuse to create a second `RoleGrant` for a pair that already
has one, and SHALL update the existing object instead.

#### Scenario: A second grant for the same pair is refused

- **WHEN** a `RoleGrant` already names `{console, administrator@example.com}`
  and another is created for the same pair
- **THEN** the create is refused and the role is changed by updating the
  existing object

### Requirement: An identity with no RoleGrant defaults to user

Any `{channel, key}` pair with no matching `RoleGrant` SHALL default to role
`user`. This SHALL require no provisioning step. An identity authenticating
for the first time SHALL be treated as `user` without an administrator
creating anything.

#### Scenario: A first-time identity is a user

- **WHEN** an identity pair with no `RoleGrant` object acts
- **THEN** it is treated as `user`, and no object is created on its behalf

### Requirement: Bootstrap administrators exist before any RoleGrant does

The console's shared-token identity (`identity == "token"`) SHALL be treated
as role `administrator` by construction, requiring no `RoleGrant` object.

Under external authentication, where no shared token is configured, the chart
value `console.rbac.bootstrapAdmins` SHALL name the initial administrators.

Each entry is a bare string, such as `administrator@example.com`. It is read
as the identity pair `{channel: console, key: <entry>}`, so the per-pair role
lookup stays uniform.

The list SHALL be read once. An install then has an administrator from first
boot, with no `RoleGrant` object created by hand.

#### Scenario: The shared-token identity is administrator

- **WHEN** a console write is authenticated by the shared token with no
  forward-auth identity
- **THEN** the resolved identity `token` is treated as role `administrator`

If external authentication is configured with no shared token and an empty
`console.rbac.bootstrapAdmins`, no identity is an administrator. Every
identity is `user` until an operator sets the value and upgrades the release.

#### Scenario: External authentication with no bootstrap administrators

- **WHEN** a release runs external authentication and the bootstrap-admins
  chart value is empty
- **THEN** no identity is treated as `administrator`, and the console
  offers no role assignment until the value is set

#### Scenario: An externally authenticated install names its first administrators

- **WHEN** a release runs external authentication and the bootstrap-admins
  chart value names an identity
- **THEN** that identity is treated as role `administrator` with no
  `RoleGrant` object required

### Requirement: Role and ownership are decided by rehashing the inbound sender

The manager SHALL decide whether a `user`-role identity may act on an
existing conversation by rehashing the inbound `sender` string with the SAME
function already used to stamp `Conversation.spec.originReader.key`, then
comparing that hash against the conversation's own `originReader.key` for
the matching channel.

The manager SHALL look up the identity's role by its RAW `sender` string
against `RoleGrant.spec.key`, scoped to the matching `channel`. One request
therefore hashes `sender` once for the ownership comparison, and uses it
unhashed, once, for the role lookup. Neither derived value is persisted.

#### Scenario: One sender string serves two comparisons

- **WHEN** an inbound reply carries a `sender` string
- **THEN** the manager hashes it once to compare against `originReader.key`,
  and uses it unhashed, once, to look up the sender's `RoleGrant`

#### Scenario: OriginReader's hash function is unchanged

- **WHEN** a role is enforced against a conversation's ownership
- **THEN** the hash applied to `sender` is the exact function
  `OriginReader.Key` is already stamped with, introducing no second hashing
  scheme

### Requirement: A conversation with no originReader has no owner

A conversation whose `spec.originReader` is absent SHALL be treated as
having no owner. Every role, including `user`, SHALL be permitted to reply
to or address such a conversation. The ownership restriction SHALL apply
ONLY when `originReader` names an identity pair different from the
requester's own.

#### Scenario: An alert-originated conversation admits any role

- **WHEN** a `user`-role identity replies to a conversation carrying no
  `originReader`
- **THEN** the reply is admitted, exactly as for `responder` or
  `administrator`

#### Scenario: Restriction fires only on a named mismatch

- **WHEN** a conversation's `originReader` names an identity pair different
  from the requester's own
- **THEN** a `user`-role requester is refused, and a `responder` or
  `administrator` requester is not

### Requirement: Ownership is enforced at two named points

The manager SHALL enforce ownership at exactly two points.

| Point | Gates |
|---|---|
| `POST /channel/inbound` in `internal/httpapi` | a reply on a thread |
| the router's `HandleCommand` in `internal/chat` | addressing an existing conversation by name |

Only the first is specified by the `channel-adapter-contract` delta. The
contract delta for addressing by name is deferred, and task 2.3 implements it
against this matrix in the meantime.

#### Scenario: Addressing is gated at the router

- **WHEN** a `user`-role sender addresses a conversation owned by a
  different identity pair
- **THEN** the router's `HandleCommand` refuses it under this matrix

### Requirement: Starting a new conversation is open to every role

Role SHALL gate only CONTINUING an existing conversation — a reply, or
addressing one by name. Starting a NEW conversation, through `POST
/signal/inbound` or by addressing a Pipeline or Coordinator fresh, SHALL
remain open to every role, including `user`.

#### Scenario: A user starts a new conversation freely

- **WHEN** a `user`-role identity posts a signal that opens a new
  conversation
- **THEN** it is admitted exactly as for any other role

#### Scenario: Gating does not reach origination

- **WHEN** role and ownership enforcement is applied
- **THEN** it is evaluated only against an EXISTING conversation's
  `originReader`, never against the act of starting one

### Requirement: Responder may bulk-close but never bulk-delete

`responder` SHALL be permitted to bulk-close conversations, a reversible
action since a closed conversation may be reopened. `responder` SHALL be
refused bulk-delete. Both bulk-delete and role assignment SHALL be
restricted to `administrator`.

#### Scenario: A responder's bulk-close succeeds

- **WHEN** a `responder`-role identity bulk-closes a set of conversations
- **THEN** every targeted conversation transitions to `Closed`

#### Scenario: A responder's bulk-delete is refused

- **WHEN** a `responder`-role identity attempts a bulk-delete
- **THEN** the request is refused, naming `administrator` as the role it
  requires

### Requirement: The console is RoleGrant's sole writer, and never deletes one

The console SHALL hold `get`, `list` and `watch` on `RoleGrant`, since role
lookup reads it, and SHALL be the only component granted `create` and `update` on
`RoleGrant` and `update` on `rolegrants/status`. The console SHALL stamp
`status.lastModifiedBy` and `status.lastModifiedAt` on every write. The manager SHALL hold read-only `list`/`watch`/`get` on
`RoleGrant` and SHALL NEVER write one.

Neither the console nor any other component SHALL be granted `delete` on
`RoleGrant`. Demoting an identity back to `user` SHALL be an explicit
`update` setting `spec.role: user`, never an object removal, so a record of
who was demoted and when survives.

#### Scenario: Only the console writes a role

- **WHEN** any component other than the console attempts to create or
  update a `RoleGrant`
- **THEN** its RBAC grant carries no such verb on that kind

#### Scenario: Demotion updates rather than deletes

- **WHEN** an administrator demotes an identity back to `user`
- **THEN** the existing `RoleGrant` object is updated to `spec.role: user`,
  and no `RoleGrant` object is deleted
