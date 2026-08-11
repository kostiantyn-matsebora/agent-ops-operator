## 1. Precedence

- [x] 1.1 Reorder `chart/templates/console.yaml` so an explicit `console.auth.uiToken` is checked BEFORE the existing Secret, and rewrite the comment that currently describes the old order
- [x] 1.2 Add `adapterAuth.token` to `chart/values.yaml` with the same shape as `console.auth.uiToken`, and state at the setting that changing it invalidates every derived adapter token until pods restart
- [x] 1.3 Apply the same explicit-first order in `chart/templates/adapter-auth.yaml`

## 2. Stability across renderers

- [x] 2.1 Render a generated Secret under `.Release.IsInstall` only, in both templates
- [x] 2.2 Add `helm.sh/resource-policy: keep` to both generated Secrets, so leaving the manifest does not delete the object
- [x] 2.3 Keep the install-time `lookup` and adopt an existing value rather than generating over it, so reinstalling into a namespace that retained a Secret does not mint a new credential
- [x] 2.4 Render the Secret on every upgrade when — and only when — an explicit value is configured
- [x] 2.5 Guard the one-time migration (design D5): `agentops.generatedSecretGuard` in `_helpers.tpl` FAILS the render when an upgrade would drop a live generated Secret that carries no keep annotation, printing the `kubectl annotate` command — helm reads the policy off the LIVE object, so without this the first upgrade DELETES both credentials
- [x] 2.6 Wire the guard into both templates
- [x] 2.7 Correct the stale advice that deleting the Secret re-issues the token — it no longer does

## 3. Notes and values

- [x] 3.1 Make the console token recipe in `NOTES.txt` conditional: print the fetch command only when the token was generated, and name the source otherwise
- [x] 3.2 Say in the values comments that a generated credential stops being managed after the first upgrade, and that an explicit value is the supported way to change or restore one — one line per setting, per the standing correction on chart comment length; the full account is in `docs/console.md`

## 4. Tests

- [x] 4.1 Chart render test: an explicit `console.auth.uiToken` appears in the rendered Secret (the case that is silently ignored today)
- [x] 4.2 Chart render test: an upgrade render with no explicit value emits no Secret for either credential
- [x] 4.3 Chart render test: an install render emits both, each carrying the keep policy
- [x] 4.4 Chart render test: an explicit `adapterAuth.token` renders on upgrade
- [x] 4.5 Chart render test: `console.auth.existingSecret` still renders no Secret and is referenced by name
- [x] 4.6 The guard is NOT render-testable — it depends on `lookup`, which returns empty under `helm template`. Verified live instead (6.0)

## 5. Documentation

- [x] 5.1 Document the three sources and their order in `docs/console.md`, including that a redeploy does not sign anyone out
- [x] 5.2 `CHANGELOG.md`: the precedence fix, the new `adapterAuth.token`, the one-time manifest removal that is not a deletion, and the required `kubectl annotate` step — chart 5.8.0

## 6. Verification

- [x] 6.0 Live: confirmed on the reference cluster that neither generated Secret carries the keep annotation, and that `helm upgrade --dry-run=server` is refused with the exact annotate command
- [x] 6.1 `helm lint` and render the chart at defaults, with each token pinned, and with `existingSecret`
- [x] 6.2 Full test run with `KUBEBUILDER_ASSETS` (unit + envtest)
- [x] 6.3 Live: annotated both Secrets, then upgraded the reference install (revision 54, chart 5.8.0, via `_home-data-center` helmfile) — both keep their exact values AND their resourceVersion, so helm did not write them at all. Both left the release manifest; only the explicit-value Secrets (`home-ops-telegram`, `agentops-claude`) remain in it
- [x] 6.4 Live: `helmfile diff` twice on the now-unchanged install — BOTH COMPLETELY EMPTY. Previously every run reported both tokens as changed. The acceptance test for the whole change
- [x] 6.5 Live: precedence verified by `helm upgrade --dry-run=server` against the real cluster — an explicit `console.auth.uiToken`/`adapterAuth.token` renders over the live existing Secret, which is exactly the case that was silently ignored before. The sign-in half is N/A on this install: `console.auth.enabled: false` behind oauth2-proxy, so no token sign-in exists to exercise, and flipping it back is a settled no

Post-deploy state: every pod Running with no restart from the upgrade, pipeline
`k8s-ops` Ready, all three sources Wired+Served.
