## Context

`chart/templates/console.yaml` already renders an Ingress under `console.ingress.enabled`. It is the minimum that could work:

```yaml
ingress:
  enabled: false
  host: ""
  className: ""
  annotations: {}
  tls: []
```

One host, one hard-coded rule (`path: /`, `pathType: Prefix`), and `tls` spliced in verbatim with `toYaml`. The backend is already correct — `agentops-adapter-<name>`, the Service the `ChannelAdapter` reconciler owns, not one the chart ships.

Two things make this worth improving beyond "add more keys".

**The token is the boundary, and it is a header.** `console/auth.go` validates a bearer token; `docs/console.md` states that whoever holds it reads every conversation payload and can instruct any joined agent, which under `rbacMode: full` means an agent holding `cluster-admin`. Enabling an Ingress is precisely when that token starts crossing a network. The chart currently has nothing to say about it.

**`path` looks configurable but is not.** `console/ui/vite.config.ts` sets no `base`, so Vite's default `/` applies and the build emits absolute `/assets/…` URLs. `console/uiserve.go` serves those from the root of the listener. Hosting the console at `/console` therefore yields an Ingress that routes, an index.html that loads, and a blank page — the failure appears in the browser's network tab, far from the values file that caused it.

Constraints inherited from the chart: the console's connectivity belongs to the reconciler, never the chart; unguessable required fields fail the render rather than half-installing (the pattern `telegram-bundle` uses); and sub-maps are read defensively because `helm upgrade --reuse-values` keeps a previous release's map wholesale rather than merging new defaults into it.

## Goals / Non-Goals

**Goals:**

- An ingress surface an operator recognises from any other chart, with nothing renamed.
- TLS reachable in one value, including the cert-manager path, without restating hostnames.
- Configurations that cannot work refused at render time, not at first page load.
- Plaintext exposure visible as a decision.

**Non-Goals:**

- Sub-path hosting. See D3 — it needs SPA work, not chart work.
- Bundling or configuring oauth2-proxy, or any authenticating proxy. The docs point at it; the chart does not ship it.
- Issuing certificates. `clusterIssuer` names cert-manager's issuer; cert-manager does the work.
- Gateway API / `HTTPRoute`. A separate surface, not a variant of this one.
- Any change to the console module, the CRDs, the RBAC, or the trust model.

## Decisions

### D1: Extend the existing keys; rename nothing

Keep `enabled`, `host`, `className`, `annotations` exactly as they are, and add `extraHosts[]`, `path`, `pathType`, `labels`. The singular `host` stays singular.

*Why not the conventional `hosts[].paths[]` shape:* it is the familiar one, but it is built for charts serving several apps or several paths, and it renames the one key anyone has already set. The console serves one app at the root of one or more names — `host` plus `extraHosts` says that exactly, and the aliases fold into the TLS host list for free. Adopting `hosts[].paths[]` would also re-open the sub-path trap D3 closes.

*Alternative considered:* supporting both shapes (rejected: two spellings of one fact, and a values file that sets both has no defensible resolution).

### D2: `tls` becomes a map, with the list form still accepted

```yaml
tls:
  secretName: ""     # existing certificate Secret
  clusterIssuer: ""  # cert-manager issuer; derives secretName when unset
  existing: []       # verbatim tls[] entries; wins over the derived form
```

The rendered `tls[].hosts` is derived from `host` + `extraHosts`, so hostnames are declared once. A mismatch between the rule hosts and the certificate hosts becomes impossible to express rather than something to catch in review.

**Backward compatibility is not optional here.** `console.ingress.tls` is a list today, and `helm upgrade --reuse-values` carries a previous release's `console:` map wholesale. The template therefore branches on `kindIs "slice" $c.ingress.tls`: a list is rendered verbatim as the legacy form; a map takes the new path. This is the same defensive posture the template already uses for `console.write`, for the same reason.

*Alternatives considered:* a new key such as `tlsConfig` beside the old `tls` (rejected: two keys, one concept, forever); a clean break with a CHANGELOG note (rejected: `--reuse-values` would fail the upgrade rather than the thing being upgraded, which is the worst moment to discover a values change).

### D3: A non-root `path` fails the render

`path` and `pathType` are accepted and defaulted, and a `path` other than the root fails with a message saying the console must be served at the root of its hostname.

*Why offer `path` at all if only one value is legal:* because operators will look for it, and finding it absent produces a values file that omits it and an assumption that the default is configurable elsewhere. Present-and-validated teaches the constraint at the moment it is being violated. `pathType` stays genuinely free — `Prefix`, `ImplementationSpecific` and `Exact` all work at the root, and ingress controllers differ on which they prefer.

*What sub-path hosting would actually require:* Vite `base: './'` plus a runtime-injected `<base href>` (the base is a deploy-time fact and Vite's `base` is a build-time constant), plus a router basename, plus API paths made relative. That is a console-module change with its own tests — a separate proposal, named here so the constraint reads as a decision rather than an oversight.

### D4: Plaintext exposure warns; it does not fail

Enabling ingress with no TLS renders successfully, and `NOTES.txt` states that the bearer token crosses the network in clear text, naming both remedies (terminate TLS; front it with an authenticating proxy). The warning is suppressed when TLS is configured.

*Why not fail the render:* the chart cannot see what sits in front of it. TLS terminated at a cloud load balancer or in a service mesh is a normal, correct deployment, and failing it would push operators toward `--set` workarounds that also silence the honest cases. This differs from D3 deliberately: a sub-path is a configuration that *cannot* work, while plaintext behind an upstream terminator *can*.

*Why not default TLS on with a derived Secret name:* it would render an Ingress referencing a Secret that does not exist, which most controllers serve with a self-signed fallback certificate. A browser warning on every visit is worse than an honest note in the install output.

### D5: Render assertions pin both the positive and negative cases

`internal/integration/charttemplate_test.go` already pins the console's chart contract — that it renders nothing when disabled, and that the chart ships no Service or Deployment for it. Ingress assertions belong in the same place: default renders no Ingress; the backend stays `agentops-adapter-<name>`; TLS derives its host list; the legacy list form still renders; and the two failure cases (missing `host`, non-root `path`) fail rather than render.

## Risks / Trade-offs

- **`kindIs` branching on `tls` is subtle, and a subtle template is a template that breaks quietly** → both branches get an explicit render test, and the legacy branch is commented with the `--reuse-values` reason it exists, matching the comment already on `console.write`.
- **Deriving `tls[].hosts` removes the ability to certify a subset of hostnames** → `tls.existing` is the escape hatch and takes precedence; the derived form is the convenience, not the only path.
- **`clusterIssuer` assumes cert-manager's annotation vocabulary** → it is the near-universal one, and an install using something else uses `annotations` plus `secretName` directly, which is unchanged.
- **A NOTES.txt warning is easy to scroll past** → it is the honest ceiling for something the chart cannot verify; `docs/console.md` carries the same statement where it will be read a second time.
- **Failing the render on a non-root path could surprise an operator mid-upgrade** → only reachable by setting a value that never worked, so nobody has a working release to break.

## Migration Plan

1. Values-only change; no CRD, image, or operator change. `helm upgrade` on a release with ingress disabled is a no-op for these objects.
2. Releases using `console.ingress.tls` as a list keep rendering unchanged via the legacy branch — no action required.
3. `CHANGELOG.md` entry, newest first: the new keys, the `tls` map form, the legacy list form that still works, and the non-root `path` failure.
4. Rollback is a chart revert; nothing outside the Ingress object changes.
5. Verify with `helm template` across the matrix in the tasks, then a server-side dry-run before applying to the live cluster.

## Open Questions

- **Should `pathType` default to `Prefix` or `ImplementationSpecific`?** `Prefix` is what the template hard-codes today and is the safer default for an SPA that owns its own routes. `ImplementationSpecific` is what the upstream chart scaffold generates. Keeping today's `Prefix` unless a controller-specific problem surfaces.
- **Is `labels` worth adding?** Included for parity with other objects, but the console's Ingress is a single object and the chart already stamps `app.kubernetes.io/name`. Cheap to drop if it reads as surface for its own sake.
