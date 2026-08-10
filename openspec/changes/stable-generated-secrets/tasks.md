## 1. Precedence

- [ ] 1.1 Reorder `chart/templates/console.yaml` so an explicit `console.auth.uiToken` is checked BEFORE the existing Secret, and rewrite the comment that currently describes the old order
- [ ] 1.2 Add `adapterAuth.token` to `chart/values.yaml` with the same shape as `console.auth.uiToken`, and state at the setting that changing it invalidates every derived adapter token until pods restart
- [ ] 1.3 Apply the same explicit-first order in `chart/templates/adapter-auth.yaml`

## 2. Stability across renderers

- [ ] 2.1 Render a generated Secret under `.Release.IsInstall` only, in both templates
- [ ] 2.2 Add `helm.sh/resource-policy: keep` to both generated Secrets, so leaving the manifest does not delete the object
- [ ] 2.3 Keep the install-time `lookup` and adopt an existing value rather than generating over it, so reinstalling into a namespace that retained a Secret does not mint a new credential
- [ ] 2.4 Render the Secret on every upgrade when — and only when — an explicit value is configured

## 3. Notes and values

- [ ] 3.1 Make the console token recipe in `NOTES.txt` conditional: print the fetch command only when the token was generated, and name the source otherwise
- [ ] 3.2 Say in the values comments that a generated credential stops being managed after the first upgrade, and that an explicit value is the supported way to change or restore one

## 4. Tests

- [ ] 4.1 Chart render test: an explicit `console.auth.uiToken` appears in the rendered Secret (the case that is silently ignored today)
- [ ] 4.2 Chart render test: an upgrade render with no explicit value emits no Secret for either credential
- [ ] 4.3 Chart render test: an install render emits both, each carrying the keep policy
- [ ] 4.4 Chart render test: an explicit `adapterAuth.token` renders on upgrade
- [ ] 4.5 Chart render test: `console.auth.existingSecret` still renders no Secret and is referenced by name

## 5. Documentation

- [ ] 5.1 Document the three sources and their order in `docs/console.md`, including that a redeploy does not sign anyone out
- [ ] 5.2 `CHANGELOG.md`: the precedence fix, the new `adapterAuth.token`, and the one-time manifest removal that is not a deletion

## 6. Verification

- [ ] 6.1 `helm lint` and render the chart at defaults, with each token pinned, and with `existingSecret`
- [ ] 6.2 Full test run with `KUBEBUILDER_ASSETS` (unit + envtest)
- [ ] 6.3 Live: upgrade the reference install and confirm both Secrets keep their exact values (compare base64 before and after, and the resourceVersion)
- [ ] 6.4 Live: run `helmfile diff` twice on an unchanged install and confirm it reports NO credential change — the acceptance test for the whole change
- [ ] 6.5 Live: pin `console.auth.uiToken` on the running install and confirm the new value takes effect and signs in
