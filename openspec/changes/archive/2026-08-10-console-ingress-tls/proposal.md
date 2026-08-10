## Why

The console's Ingress template exists but is the minimum that could render: one `host`, a hard-coded `/` `Prefix` path, and `tls` as a raw list the operator must hand-write in full. Exposing the console is exactly the moment its trust boundary matters — whoever holds the UI token can read every conversation payload and instruct any joined agent, which in a `rbacMode: full` install means an agent holding `cluster-admin` — and that token travels in an `Authorization` header. Today nothing in the chart makes serving it over plaintext HTTP feel like the decision it is, and nothing helps an operator get a certificate onto it.

The other half is a trap the current shape invites. `path` looks like it should be configurable, but the SPA is built with Vite's default `base: /` and emits absolute `/assets/…` URLs, so hosting the console under `/console` returns a blank page with 404s in the console. A "configurable ingress" that lets someone configure that is worse than one that refuses to.

## What Changes

- **A conventional, fuller ingress values surface** on `console.ingress`, keeping the existing `enabled` / `host` / `className` / `annotations` keys so no working values file breaks:
  - `extraHosts[]` — additional names serving the same console, folded into the rules and (when TLS renders) into the certificate's host list.
  - `path` / `pathType` — present, defaulted to `/` and `Prefix`, and **validated**: a non-root path FAILS the render with the reason, because the embedded SPA is root-only. Configurability that cannot work is not offered.
  - `labels` merged onto the Ingress alongside the chart's own.
- **TLS becomes a supported path rather than a passthrough.** `console.ingress.tls` becomes a map:
  - `secretName` — serve HTTPS from an existing certificate Secret; the `tls[]` block is derived from `host` + `extraHosts` so hostnames are never restated.
  - `clusterIssuer` — the cert-manager path most installs actually want: emits the issuer annotation and derives `secretName` when unset.
  - `existing[]` — the raw `tls:` list, used verbatim, for anything the above does not cover.
  - **Backward compatible**: a `console.ingress.tls` supplied as a LIST (today's shape) is detected and used verbatim, so `--reuse-values` upgrades and existing values files keep working. Documented as the legacy form.
- **Plaintext exposure is announced, not silently allowed.** Enabling the Ingress with no TLS configured renders successfully — TLS is often terminated upstream at a load balancer or mesh — but `NOTES.txt` states plainly that the UI token crosses the network in clear text and what to do about it. The render never fails on this; an operator with upstream termination is not wrong.
- **Docs carry the exposure story in one place**: the values table gains every new key, and the trust-boundary section gains the concrete recipe — TLS plus a forward-auth proxy — rather than the current one-line pointer.
- **Chart render tests pin the behaviour**: default renders no Ingress, a non-root path fails, TLS derives its hosts, the legacy list form still works, and the backend service name stays the reconciler-owned `agentops-adapter-<name>`.

## Capabilities

### New Capabilities

- `console-ingress`: the console's browser-facing exposure — the ingress values surface, TLS derivation and the cert-manager path, the root-path constraint, and how plaintext exposure is reported.

### Modified Capabilities

- `console-deployment`: the "Browser access is authenticated" requirement currently ends at "optional Ingress template"; it gains what that template must guarantee about the token in transit.

## Impact

- **Chart**: `chart/templates/console.yaml` (Ingress block), `chart/values.yaml` (`console.ingress.*`), `chart/templates/NOTES.txt` (exposure and plaintext warning), chart minor bump.
- **Tests**: `internal/integration/charttemplate_test.go` gains ingress render assertions, including the negative cases (non-root path fails, disabled renders nothing).
- **Docs**: `docs/console.md` — values table and trust boundary; `CHANGELOG.md` — the `tls` shape change and the legacy list form that keeps working.
- **No operator, API, contract or console-module changes.** Nothing here touches Go code, CRDs, RBAC or the console's trust model — the token, the read-only Role and the absence of a Kubernetes write path are all unchanged.
- **Non-goals**: sub-path hosting (needs a runtime-injected SPA base — a separate change, named in the design so the constraint is not mistaken for an oversight); bundling oauth2-proxy or any authenticating proxy; issuing certificates; `Gateway API` / `HTTPRoute` support.
