## Why

The console's bearer token is currently the only way in, and there is no way to
say "something else already authenticated this user". An install that fronts the
console with oauth2-proxy, an Istio/Envoy filter, Cloudflare Access or an
authenticating sidecar ends up with two authentications stacked: the operator
signs in to the proxy, then signs in again with a shared token that identifies
nobody. The second one adds no security — the request already got past the
proxy — and costs a shared credential that must be distributed and rotated.

The console already reads six forward-auth identity headers and prefers them for
attributing writes, so it half-supports this today: it will believe the proxy
about **who** you are, while still demanding a token to decide **whether** you
get in.

The reason this has not simply been made optional is stated in the code, in
capitals, and is worth preserving exactly: *"AN UNCONFIGURED TOKEN AUTHORIZES
NOBODY … 'No token set' must never read as 'no authentication required' — that
is the failure mode where a fresh install is wide open."* The console reads every
conversation payload in the namespace and can instruct an agent that, in a
`rbacMode: full` install, holds cluster-admin.

So the goal is not to weaken that. It is to add a **second, explicit** way to be
authenticated, without ever letting an absent value become an open door.

## What Changes

- **`console.auth.enabled`, default `true`.** Set it to `false` and the console
  stops requiring its own token. Nothing else opens the door: an unconfigured or
  empty token with `auth.enabled: true` still authorizes nobody, exactly as now.
  The two conditions stay independent, because the whole hazard is one being
  mistaken for the other.
- **Disabling auth REQUIRES naming what authenticates instead.**
  `console.auth.externalAuthenticator` must be a non-empty string (e.g.
  `oauth2-proxy`, `cloudflare-access`, `envoy-ext-authz`) or the render FAILS.
  This is the chart's established way of refusing a configuration that cannot be
  right — the same posture as the non-root ingress path and the MCP
  ServiceAccount collision — and it makes the answer to "what is protecting
  this?" a value in the release rather than something in an operator's head.
- **Writes still require a real identity.** With token auth off, a write is
  accepted only when a forward-auth header resolves an identity. Falling back to
  the literal `token` would be a lie once no token was proven, and a control
  plane that can instruct a cluster-admin agent should not record its actions
  against nobody. A proxy that authenticates but forwards no identity therefore
  yields a **read-only** console — which is a correct, legible outcome and is
  fixed by configuring the proxy to forward one.
- **The SPA stops asking for a password it does not need.** With auth disabled,
  the session endpoint reports the request as authenticated and names the
  identity source, so no login form appears and the masthead shows who the
  console thinks you are.
- **The notes say it out loud.** When auth is disabled, `NOTES.txt` states that
  the console performs no authentication of its own, names the declared external
  authenticator, and says what is reachable by anyone who bypasses it.
- **The token Secret is still created** when auth is disabled, so the Channel's
  credential projection keeps resolving and re-enabling auth is one value rather
  than a reinstall.

## Capabilities

### New Capabilities

<!-- none — this modifies how the existing authentication behaves rather than adding a surface -->

### Modified Capabilities

- `console-application`: reads may be authenticated by the console OR asserted to
  have been authenticated upstream; writes gain the rule that an unresolvable
  identity refuses the write when the console is not the authenticator.
- `console-deployment`: the chart gains the switch, the mandatory
  `externalAuthenticator` declaration and its render guard, and the notes
  requirement for a console that authenticates nobody.

## Impact

- **Console**: `console/auth.go` (the middleware becomes "authenticated by us OR
  declared external", identity resolution loses its `token` fallback in external
  mode), `console/api.go` (session endpoint reports mode and identity), SPA login
  and masthead.
- **Chart**: `console.auth.enabled`, `console.auth.externalAuthenticator`, the
  render guard, `NOTES.txt`.
- **Tests**: console unit tests for each mode; chart render tests that the guard
  fires and that the default is unchanged.
- **Docs**: `docs/console.md` — the trust boundary section gains the
  external-authentication recipe alongside the existing forward-auth text.
- **Sequencing**: touches `chart/templates/console.yaml`, as do the in-flight
  `console-ingress-tls` and the proposed `stable-generated-secrets`.
- **Out of scope**: the console authenticating users itself (OIDC client,
  user accounts, per-user RBAC). The console's authorization stays coarse —
  whoever is in, sees everything its ServiceAccount can read.
