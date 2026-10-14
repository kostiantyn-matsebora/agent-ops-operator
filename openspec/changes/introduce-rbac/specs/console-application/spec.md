## MODIFIED Requirements

### Requirement: Reads are authenticated; writes require identity and can be disabled
Read access SHALL require a bearer token or a session established from it, UNLESS the deployment declares that authentication is performed upstream. Write actions — replying in a conversation and originating one — SHALL additionally require `console.write.enabled` and a resolved identity, taken from a trusted forward-auth header when present and recorded as the token identity otherwise. An unconfigured token SHALL authorize nobody, and SHALL be indistinguishable from a wrong token.

The two states SHALL remain independent: an absent or empty token SHALL NEVER be interpreted as "authentication not required". Only an explicit declaration that an external authenticator fronts the console SHALL relax the token requirement.

When authentication is declared external, writes SHALL require an identity resolved from a forward-auth header and SHALL be refused when none resolves. The token identity SHALL NOT be used as a fallback in that mode, because no token was proven and a write log naming one would assert something untrue.

Every write SHALL be logged with the resolved identity.

The resolved identity SHALL additionally resolve to a role — `user`,
`responder`, or `administrator` — looked up by `{channel: "console", key:
identity}` against `RoleGrant` (access-control), defaulting to `user` when
none exists.

Under external authentication with no shared token, an identity named in
`console.rbac.bootstrapAdmins` SHALL resolve to
`administrator` even when a `RoleGrant` says otherwise (access-control).

The shared-token identity (`identity == "token"`) SHALL resolve to
`administrator` by construction, with no `RoleGrant` (access-control).

A console-handled write that a role beyond `user` restricts SHALL be
checked for role before being forwarded, as a fail-fast courtesy.

The manager's own enforcement is authoritative for reply and originate
writes only. It never gates role-management writes.

Removing the console's own check on a reply or originate write would only
delay a refusal, never open a hole.

#### Scenario: Unconfigured is closed, not open
- **WHEN** no token is configured
- **THEN** every authenticated route is refused, and login failures do not reveal whether a token exists

#### Scenario: Viewer-only deployment
- **WHEN** `console.write.enabled` is false
- **THEN** the composer and the new-conversation action are absent, and both endpoints reject requests server-side

#### Scenario: Identity is carried, not assumed
- **WHEN** a trusted forward-auth header is present
- **THEN** that identity is recorded on the write and carried as the signal's sender

#### Scenario: External authentication admits the request
- **WHEN** the deployment declares authentication external and a request arrives with no bearer token
- **THEN** reads are served, because the declaration — not the absence of a credential — is what opened the door

#### Scenario: External authentication without identity is read-only
- **WHEN** authentication is declared external, writes are enabled, and a request carries no forward-auth identity header
- **THEN** the write is refused and no identity is invented, while reads continue to be served

#### Scenario: An empty token still closes a console that authenticates
- **WHEN** authentication is NOT declared external and the configured token is empty
- **THEN** every authenticated route is refused, exactly as when a token is set and a wrong one is presented

#### Scenario: Identity resolves to a role
- **WHEN** a request is admitted with a resolved identity
- **THEN** the console looks up that identity's role under channel
  `console`, defaulting to `user` when no `RoleGrant` names it, except a
  `bootstrapAdmins` entry (always `administrator`, overriding any `RoleGrant`)
  and the shared-token identity, which resolves to `administrator`

#### Scenario: The session response carries the role
- **WHEN** an authenticated client calls `GET /api/session`
- **THEN** the response includes a `role` field holding the resolved role,
  so the UI can read it without a second lookup

#### Scenario: The console's own role check is a courtesy, not the authority
- **WHEN** the console's own role check would refuse a reply or originate write
- **THEN** removing that check would only replace the console's refusal
  with the manager's own, never open a hole
- **AND** a bulk or role-assignment refusal has no manager backstop, so the
  console's check alone is its authority

## ADDED Requirements

### Requirement: The control-plane route namespace is administrator-only

The console SHALL expose a `/api/control-plane/...` route namespace,
distinct from `/api/admin/...`, so the word "admin" cannot simultaneously
name a role and a path.

Every route under `/api/control-plane/...` SHALL require the resolved
identity's role to be `administrator`, refusing every other role including
`responder`.

Role assignment — creating and updating `RoleGrant` objects — SHALL be this
namespace's first resident. A later administrator-only action MAY land
there too, but this requirement governs only what exists today.

#### Scenario: A responder is refused a control-plane route

- **WHEN** a `responder`-role identity requests a `/api/control-plane/...`
  route
- **THEN** the request is refused, naming `administrator` as the role it
  requires

#### Scenario: An administrator assigns a role through control-plane

- **WHEN** an `administrator`-role identity creates or updates a
  `RoleGrant` through `/api/control-plane/...`
- **THEN** the request succeeds and the write is logged with that identity

### Requirement: Conversation list and detail are filtered by role and ownership

For a requester whose resolved role is `user`, the console SHALL filter
conversation list and detail reads to conversations whose
`spec.originReader` names that requester's own `{channel: "console", key}`
pair. This is the same comparison `ConversationSummary.Mine` already makes.

A conversation carrying no `originReader` has no owner (access-control).
The console SHALL also serve it to a `user`, who may reply to it.

A conversation owned by a different identity SHALL be absent from a
`user`'s list, and SHALL read as not found on direct detail access.

For a requester whose resolved role is `responder` or `administrator`, the
console SHALL serve every conversation, unfiltered, exactly as it does
today.

Read siloing is enforced by the console alone. The manager exposes no
conversation read route to a human requester, so the console's own watch is
the only read path and a manager-side read check has nothing to guard.

#### Scenario: A user sees only their own conversations

- **WHEN** a `user`-role requester lists conversations
- **THEN** only conversations whose `originReader` names that requester's
  own identity pair, or that carry no `originReader`, are returned

#### Scenario: A user sees unowned conversations

- **WHEN** a `user`-role requester lists conversations and one carries no
  `originReader`
- **THEN** that conversation is returned and its detail is readable

#### Scenario: A responder sees everything

- **WHEN** a `responder`-role requester lists conversations
- **THEN** every conversation is returned, including ones owned by other
  identities and ones with no owner

#### Scenario: Direct access to someone else's conversation is refused

- **WHEN** a `user`-role requester requests the detail of a conversation
  owned by a different identity
- **THEN** the console responds as if the conversation does not exist

### Requirement: Bulk routes are gated by the resolved role

The console's bulk-close route SHALL require `responder` or `administrator`.
The bulk-delete route SHALL require `administrator`. Each check SHALL run
against the resolved role before the request is forwarded or written, and a
refusal SHALL name the role it requires.

#### Scenario: A user's bulk-close is refused

- **WHEN** a `user`-role requester calls the bulk-close route
- **THEN** the console refuses it, naming `responder` as the required role

#### Scenario: A responder's bulk-delete is refused

- **WHEN** a `responder`-role requester calls the bulk-delete route
- **THEN** the console refuses it, naming `administrator` as the required role

#### Scenario: An administrator may bulk-close and bulk-delete

- **WHEN** an `administrator`-role requester calls either bulk route
- **THEN** the console forwards the request
