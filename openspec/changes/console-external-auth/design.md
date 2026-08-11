## Context

`console/auth.go` opens with three properties it says are "here rather than in a
doc because each is easy to erode". The first is the one this change must not
break:

> AN UNCONFIGURED TOKEN AUTHORIZES NOBODY, and is indistinguishable from a wrong
> one. "No token set" must never read as "no authentication required" — that is
> the failure mode where a fresh install is wide open.

The console already resolves identity from six forward-auth headers
(`X-Forwarded-Preferred-Username`, `X-Auth-Request-Email`, …), preferring them
over the shared token when attributing writes. Their trust is justified in the
code by deployment shape: the console is ClusterIP with no Ingress by default,
so reaching the port means being inside the cluster or past the proxy that sets
them.

That justification is exactly what changes when someone enables the Ingress —
which the reference install now does, on a public-ish hostname with TLS. At that
point the headers are attacker-settable unless a proxy strips and re-sets them,
and the token is doing the real work.

So this change is not "make auth optional". It is "let the operator move the
authentication boundary outward, and make the console state that it has moved".

## Goals / Non-Goals

**Goals:**

- A supported way to run with authentication performed upstream.
- No configuration in which an omitted or empty value results in open access.
- Writes remain attributable to a person.
- The deployed configuration itself records what is protecting the console.

**Non-Goals:**

- The console authenticating users itself (OIDC client, sessions per user).
- Per-user authorization. Whoever is in sees everything the ServiceAccount can
  read; that is unchanged and remains the documented trust boundary.
- Validating that the declared external authenticator exists or works. The chart
  cannot see what is in front of it.

## Decisions

### D1: A separate switch, never an inferred one

Authentication is disabled only by `console.auth.enabled: false`. An empty
`uiToken` continues to authorize nobody.

*Why:* the two states — "no credential configured" and "no credential required"
— must stay independent, because the entire failure this design guards against
is one being read as the other. Any scheme where absence implies openness (a
token of `""` meaning "off", or a `mode` that defaults from whether a token is
set) reintroduces it.

*Alternative:* a single `auth.mode: token | external | none` (rejected for the
default case — a three-valued setting has a wrong value that looks as ordinary
as the right one, whereas a boolean that must be paired with a named
authenticator makes the dangerous state require two deliberate edits).

### D2: Disabling requires NAMING the external authenticator

`console.auth.externalAuthenticator` must be non-empty when `auth.enabled` is
false; otherwise the render fails with a message saying what to set and why.

*Why:* it converts a dangerous configuration into a self-documenting one. The
value appears in `helm get values`, in the notes, and in review — so "what
protects the console?" is answerable from the release instead of from memory.
It also costs an operator one more deliberate act than flipping a boolean, which
is the point.

The chart already refuses configurations that cannot be right (a non-root
console ingress path, an MCP server sharing the runtime ServiceAccount), so this
is the established pattern rather than a new kind of strictness.

*Alternative:* a boolean acknowledgement (`iUnderstand: true`) — rejected as it
carries no information; the string names the thing and is useful later.

### D3: With external auth, writes need a resolved identity — no `token` fallback

`Identity()` currently returns the literal `token` when no forward-auth header is
present. Under external auth that string would assert something untrue: no token
was proven. So in that mode an unresolvable identity refuses the write rather
than inventing one; reads are unaffected.

*Why:* every write is logged with its identity, and the log is the only record
of who instructed an agent. An audit trail of `token` on an install where no
token exists is worse than no trail, because it looks like one.

The consequence is deliberate and should read as a feature: a proxy that
authenticates but forwards no identity gives a **read-only console**. The remedy
is to forward one, which is what a proxy in that position should do anyway.

*Alternatives:* fall back to `anonymous` (rejected — accepts unattributable
control-plane writes); a values flag to allow anonymous writes (rejected for now
— it is the flag someone sets to make an error message go away, and the correct
fix is proxy configuration).

### D4: Header trust is asserted by configuration, not inferred from deployment

The code's justification for trusting forward-auth headers is "you had to be
inside the cluster or past the proxy". With the Ingress enabled, that is only
true if the proxy strips inbound copies of those headers.

This change does not add header-stripping or a trusted-proxy allowlist — it
would be a second, weaker implementation of what the proxy already does. What it
adds is honesty: the notes state that identity headers are believed, so a proxy
that does not strip client-supplied copies lets a caller choose their own
identity. That is a real deployment requirement of every forward-auth setup and
belongs where the operator will read it.

*Open question below covers whether to go further.*

### D5: The token Secret is still rendered when auth is disabled

*Why:* the console Channel declares `credentialsSecretRef`, and the adapter pod
projects it with `envFrom`. A missing Secret makes the pod fail to start, so
removing it would turn "disable auth" into "console will not boot" — a failure
mode with no obvious cause. Keeping it also makes re-enabling auth a single
value change rather than a reinstall.

An unused credential sitting in a Secret is a small cost; a pod that will not
start because a credential was tidied away is not.

### D6: The SPA must not present a login it cannot satisfy

`/api/session` reports whether the console authenticates and, when it does not,
the resolved identity and its source. The SPA skips the login screen and shows
the identity in the masthead.

*Why:* a login form that accepts no token, on a console that requires none, is
the kind of dead end that makes people believe the install is broken. It also
gives the operator a way to confirm, from the UI, that the proxy is forwarding
identity — which is otherwise only visible in write logs.

## Risks / Trade-offs

- **`auth.enabled: false` with nothing actually in front** → the render refuses
  unless an authenticator is named, and the notes state plainly what is exposed.
  The chart cannot verify the claim; naming it is the strongest guarantee a chart
  can offer.
- **Forward-auth headers are attacker-settable if the proxy does not strip them**
  → documented as a requirement of the mode rather than silently assumed. See the
  open question on going further.
- **Someone disables auth to "fix" a lost token** → the notes and the values
  comment both point at re-issuing the token (one value) as the fix, and the
  required authenticator name is a speed bump in front of the wrong answer.
- **Read-only surprise when a proxy forwards no identity** → surfaced in the UI
  as an identity of "unknown" with writes disabled, and stated in the notes,
  rather than failing writes with an opaque 403.
- **Two auth mechanisms to keep correct** → the middleware becomes one decision
  ("did we authenticate, or did we declare that someone else did") rather than
  two code paths; tests cover both modes plus the "empty token, auth enabled"
  case that must stay closed.

## Migration Plan

1. Ship with `auth.enabled: true`. No existing install changes behaviour.
2. An install adopting external auth sets both values together; the render
   refuses if it sets only one.
3. Rollback is one value, and the token Secret still exists because D5 kept it.

## Open Questions

- **Should the console strip inbound forward-auth headers itself?** It cannot
  distinguish a header set by the proxy from one set by a client, since both
  arrive on the same connection — a trusted-proxy source allowlist would be the
  only way, and it duplicates what the proxy is for. Leaning to document, not
  implement, but this is the decision most worth challenging.
- **Should `auth.enabled: false` force `write.enabled: false` unless identity is
  proven at render time?** It cannot be proven at render time, which argues for
  the runtime rule in D3 alone.
- **Does the same switch belong on the manager's adapter contract?** No — those
  tokens are machine-to-machine and derived, with no proxy in the path. Naming
  it here so the question is closed rather than left open.
