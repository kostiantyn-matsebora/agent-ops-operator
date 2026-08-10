## Context

Two chart-generated credentials, both following the same idiom:

```
{{- $existing := lookup "v1" "Secret" .Release.Namespace $name }}
{{- if and $existing $existing.data $existing.data.token }}
  token: {{ $existing.data.token }}
{{- else }}
  token: {{ randAlphaNum 32 | b64enc }}
{{- end }}
```

Measured behaviour on the reference cluster rather than assumed:

- `helmfile apply` **preserves** both tokens — the Secret is not even written
  (resourceVersion unchanged across a no-op apply).
- `helmfile diff` reports **both as changed on every run**, because the renderer
  it uses has no cluster and `lookup` returns empty.

So the mechanism works on the path that matters most and misreports on the path
people read. The failure is not theoretical: on any renderer without cluster
access the same empty `lookup` produces a *fresh* value that then gets applied.

Two further facts shape the design:

- **The adapter master token is the root of every adapter's credential.**
  Per-adapter tokens are `HMAC(master, adapter name)`, validated by
  re-derivation. Rotating the master invalidates every adapter at once.
- **The console's explicit `uiToken` is unreachable.** The existing-Secret branch
  is tested first, so on any install that already has a Secret the setting is
  silently ignored.

## Goals / Non-Goals

**Goals:**

- A deploy never changes a credential the operator did not ask to change,
  whatever renders it.
- Pinning a token works, including on an install that already has one.
- A diff of an unchanged install is empty.

**Non-Goals:**

- Scheduled rotation. Changing a value and upgrading is the rotation story.
- Managing the runtime's Claude credential differently — the chart never
  generates that one; it is explicit or referenced.
- Encrypting values at rest. A token in values goes wherever values go, which is
  the same posture the bot token and Claude token already have.

## Decisions

### D1: Precedence is explicit → existing → generate

The explicit value is checked FIRST, then the live Secret, then generation.

*Why:* an operator who sets a value has stated an intent, and the current order
overrides that intent with history. The present behaviour is the worst shape a
setting can have — it is accepted, documented, and ignored, with no error and no
log line. Reordering also makes rotation an ordinary values edit instead of
"delete the Secret, then upgrade".

*Alternative:* keep existing-wins and document that pinning only applies to fresh
installs (rejected — that is a setting that means different things depending on
history, and nothing in the chart tells you which you are getting).

### D2: A generated credential is rendered on install only, and kept

When no explicit value is supplied, the Secret renders under `.Release.IsInstall`
and carries `helm.sh/resource-policy: keep`. Upgrades do not render it at all.

*Why:* this removes the random value from the upgrade path entirely, rather than
relying on `lookup` succeeding to neutralise it. The churn and the
rotate-on-dry-run hazard have the same root — a template that *can* emit a fresh
credential during an upgrade — and stopping it from being emitted is the fix that
does not depend on which renderer runs.

`keep` is what makes not-rendering safe: without it, dropping a resource from the
manifest deletes it.

*Alternatives:*
- **Keep `lookup` and accept the diff noise** (rejected — leaves the real
  rotation hazard on cluster-less renderers, which is the case that actually
  costs sessions).
- **A `pre-install` hook Secret** (rejected — hook resources sit outside the
  release, complicating uninstall, for the same effect `IsInstall` + `keep`
  achieves inside it).
- **Have the console mint its own token on first start** (rejected outright — the
  console has no write path to the Kubernetes API, by design and by RBAC, and
  adding one to store a credential is the wrong direction).
- **Derive the token deterministically** from release name/namespace (rejected —
  a predictable credential is not a credential).

### D3: An explicit value is always rendered, on install and upgrade

The install-only rule applies only to the *generated* case. When a value is
supplied the Secret renders on every upgrade, because that is the path by which
changing the value takes effect.

This also gives an operator a way back if the Secret is deleted by hand: set the
value. Stated in the notes, because the recovery is otherwise non-obvious once
the chart has stopped managing the object.

### D4: `lookup` stays, as the upgrade-path safety net for the explicit case only

With D2 the generated branch no longer needs `lookup`. It is retained where it
still earns its place — deciding whether an install-time generation is even
needed when a Secret already exists (a `helm install` over a namespace that has
one, e.g. after `helm uninstall` left it behind under `keep`). Without that
check, reinstalling would overwrite a credential every adapter is still using.

### D5: The notes say which source is in effect

`NOTES.txt` currently prints the `kubectl get secret … uiToken` recipe
unconditionally, which reads as "fetch your new token" after every deploy even
when nothing rotated. It becomes conditional: the fetch recipe when the token is
generated, and a line naming the source when it is pinned or brought.

*Why this is not cosmetic:* the instruction is where the belief that the token
changes every deploy comes from. Behaviour and the message about it are being
fixed together, or the next operator draws the same conclusion.

## Risks / Trade-offs

- **A generated Secret becomes unmanaged after the first upgrade** → same trade
  already accepted for the CRDs and the persistence claims, and the alternative
  is a template that can rotate a live credential. Documented, with the explicit
  value as the supported way back.
- **Deleting the Secret by hand then upgrading leaves nothing to restore it** →
  D3's explicit-value path, plus D4's install-time lookup, cover the realistic
  recoveries; the notes name them.
- **A pinned token lives in values** → identical to the existing bot-token and
  Claude-token posture; the comment points at a secret store rather than
  pretending values are private.
- **The first upgrade after this change shows the Secret leaving the release** →
  one-time, and it is a removal from the manifest, not from the cluster. Worth
  stating in the CHANGELOG so it is not read as deletion.
- **An operator sets `adapterAuth.token` on a running install** → every adapter's
  derived token changes at once and adapters 401 until their pods restart with
  the new env. That is inherent to rotating a master credential; the values
  comment must say it plainly rather than leave it to be discovered.

## Migration Plan

1. Ship the precedence fix and the new value. No install changes behaviour on
   upgrade: an existing generated Secret is left exactly as it is.
2. First upgrade drops the generated Secret from the release manifest while
   `keep` retains the object. Verify with `helm get manifest` and by reading the
   Secret — the value must be identical before and after.
3. Verify the diff is empty on a second no-op upgrade. That is the acceptance
   test for the whole change.
4. Rollback: revert the chart. The Secret is still present and still holds the
   same value, so the old template's `lookup` finds it and continues as before.

## Open Questions

- **Should `helm uninstall` remove a generated credential?** `keep` means it
  survives, so a reinstall adopts it — convenient, and arguably surprising for
  someone expecting uninstall to be complete. Leaning to keep-and-document,
  matching the claims.
- **Should the chart refuse an obviously weak pinned token?** A length floor is
  cheap, but a chart that validates credential strength is a chart with an
  opinion it cannot enforce anywhere else.
