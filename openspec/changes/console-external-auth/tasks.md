## 1. The auth decision

- [ ] 1.1 Add an auth mode to the console's config (`AUTH_ENABLED` + `EXTERNAL_AUTHENTICATOR`), read alongside the existing token config in `console/auth.go`
- [ ] 1.2 Make the middleware ONE decision — "did we authenticate this, or did the release declare that someone else did" — rather than two parallel paths; an empty token with auth enabled MUST stay closed
- [ ] 1.3 Drop the `token` identity fallback when authentication is external: `Identity()` reports the forward-auth identity or nothing
- [ ] 1.4 Refuse writes when authentication is external and no identity resolves, leaving reads served
- [ ] 1.5 Update the file's opening comment — it states the invariants deliberately, and one of them now has a named exception that must be written down with it

## 2. Session and SPA

- [ ] 2.1 Report the auth mode, resolved identity and its source from `GET /api/session`
- [ ] 2.2 Skip the login screen when the console authenticates nobody, and show the identity (or "unknown") in the masthead
- [ ] 2.3 Show why writes are unavailable when identity is unresolved, rather than letting the composer fail with an opaque refusal

## 3. Chart

- [ ] 3.1 Add `console.auth.enabled` (default `true`) and `console.auth.externalAuthenticator` (default empty) with comments that state the exposure, not just the syntax
- [ ] 3.2 Fail the render when `auth.enabled: false` and no `externalAuthenticator` is named, in the style of the existing ingress-path and MCP-ServiceAccount guards
- [ ] 3.3 Keep rendering the token Secret when auth is disabled, so the Channel's credential projection still resolves
- [ ] 3.4 Pass both settings into the adapter workload
- [ ] 3.5 Extend `NOTES.txt`: when auth is disabled, name the external authenticator and state what is reachable by anyone who bypasses it

## 4. Tests

- [ ] 4.1 Console: auth enabled + empty token stays closed (the case that must never regress)
- [ ] 4.2 Console: auth enabled + correct token and session behave exactly as today
- [ ] 4.3 Console: auth external + no token serves reads
- [ ] 4.4 Console: auth external + forward-auth header allows a write, and the identity is what gets logged
- [ ] 4.5 Console: auth external + no identity header refuses the write and still serves reads
- [ ] 4.6 Chart render: default is unchanged and requires a token
- [ ] 4.7 Chart render: `auth.enabled=false` alone FAILS; with an authenticator named it succeeds and still renders the Secret

## 5. Documentation

- [ ] 5.1 `docs/console.md` — the trust boundary section gains the external-authentication recipe, and states that forward-auth identity headers are believed, so the fronting proxy must strip client-supplied copies
- [ ] 5.2 `CHANGELOG.md` — the new values, the render guard, and the read-only consequence of a proxy that forwards no identity

## 6. Verification

- [ ] 6.1 `go build ./... && go vet ./...` at the root and in the console module
- [ ] 6.2 Console module tests, plus `helm lint` and renders of all three configurations
- [ ] 6.3 Live: with auth external behind the existing ingress, confirm a request carrying a forward-auth identity can write and that the identity appears in the console's write log
- [ ] 6.4 Live: confirm a request with no identity header is read-only rather than silently attributed
- [ ] 6.5 Live: re-enable auth with one value and confirm the token still works — proving the Secret survived the switch
