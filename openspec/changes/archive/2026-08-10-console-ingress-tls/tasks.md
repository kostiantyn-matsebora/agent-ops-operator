## 1. Values surface

- [x] 1.1 Extend `console.ingress` in `chart/values.yaml` with `extraHosts: []`, `path: /`, `pathType: Prefix`, `labels: {}`, keeping `enabled`, `host`, `className`, `annotations` unchanged
- [x] 1.2 Replace the `tls: []` default with the map form (`secretName`, `clusterIssuer`, `existing: []`)
- [x] 1.3 Write the block comment: what each key does, that the console must be served at the root of its hostname, that the legacy list form of `tls` still renders, and the plaintext-token consequence of enabling ingress without TLS

## 2. Ingress template

- [x] 2.1 Add a hostname helper in the Ingress block of `chart/templates/console.yaml` that resolves `host` + `extraHosts` into one list, used by both the rules and the TLS derivation
- [x] 2.2 Render one rule per hostname, all routing to `agentops-adapter-<name>` on `console.port` — the reconciler's Service, never a chart-shipped one
- [x] 2.3 Validate `path`: fail the render with an explicit message when it is not the root, explaining that the embedded SPA emits absolute asset URLs
- [x] 2.4 Keep `host` required when ingress is enabled, with the message naming the value
- [x] 2.5 Branch `tls` on `kindIs "slice"`: a list renders verbatim (legacy), a map takes the derivation path — comment the branch with the `--reuse-values` reason, matching the existing `console.write` comment
- [x] 2.6 Derive `tls[]` from `secretName` (or from `clusterIssuer` when `secretName` is unset), covering every resolved hostname; render `tls.existing` verbatim in preference to either
- [x] 2.7 Emit the cert-manager issuer annotation when `clusterIssuer` is set, merged with the operator's own `annotations`
- [x] 2.8 Merge `console.ingress.labels` onto the Ingress alongside `app.kubernetes.io/name: agentops-console`

## 3. Post-install notes

- [x] 3.1 Print the console URL from the configured hostname when ingress is enabled, alongside the existing port-forward instructions
- [x] 3.2 Print the plaintext-token warning when ingress is enabled and no TLS is configured, naming both remedies (terminate TLS, front it with an authenticating proxy)
- [x] 3.3 Suppress that warning when TLS is configured in any of its three forms

## 4. Render tests

- [x] 4.1 Default install renders no Ingress (`internal/integration/charttemplate_test.go`)
- [x] 4.2 `console.enabled=false` with ingress values still set renders no Ingress
- [x] 4.3 Enabled ingress routes to `agentops-adapter-console` on the configured port and ships no Service or Deployment
- [x] 4.4 `extraHosts` produces one rule per hostname
- [x] 4.5 `tls.secretName` derives a `tls[]` entry covering `host` and every `extraHosts` entry
- [x] 4.6 `tls.clusterIssuer` alone emits the issuer annotation and a derived Secret name
- [x] 4.7 `tls.existing` renders verbatim and suppresses derivation
- [x] 4.8 The legacy list form of `tls` renders as before
- [x] 4.9 Missing `host` fails the render, naming the value
- [x] 4.10 A non-root `path` fails the render with the root-hosting explanation

## 5. Documentation

- [x] 5.1 Add every new key to the `console.ingress.*` rows of the values table in `docs/console.md`
- [x] 5.2 Replace the one-line ingress pointer in the trust-boundary section with the concrete recipe: TLS plus a forward-auth proxy, and what an install without either exposes
- [x] 5.3 State the root-hosting constraint in `docs/console.md` with its cause, so it reads as a property of the embedded SPA rather than a chart limitation
- [x] 5.4 Add the `CHANGELOG.md` entry (newest first): new keys, the `tls` map form, the legacy list form that still works, and the non-root `path` failure
- [x] 5.5 Bump the chart version

## 6. Verification

- [x] 6.1 `helm template` the matrix by hand: disabled; enabled bare; `extraHosts`; each of the three TLS forms; and both failure cases
- [x] 6.2 Full test run with `KUBEBUILDER_ASSETS` (unit + envtest), confirming the new chart-render assertions pass
- [x] 6.3 Server-side dry-run against the live cluster before any apply
- [x] 6.4 Live check: apply with ingress enabled and TLS configured, reach the console over HTTPS, and confirm the SPA loads its assets and a deep link renders rather than 404s
