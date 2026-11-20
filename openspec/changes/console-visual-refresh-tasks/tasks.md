## 4. The workspace look: the other pages

Role: `frontend-developer`. Restyle only. Each page keeps the sections, order and fields it has in `classic`.

- [ ] 4.3 Configuration with its kind index, kind tables, object pages and the Overview and YAML tabs, with the same check
- [ ] 4.4 Topology with its real graph, filters and panels, and verify the `e2e/topology.spec.ts` assertions pass in both looks
- [ ] 4.5 Show a sign-out control only where authentication offers one, and verify a test under a shared token

## 5. Feedback and motion

Role: `frontend-developer`. Task 5.5 is checked against WCAG 2.3.3, animation from interactions.

- [ ] 5.1 Show command states on the row and in the thread, and verify a spec that a reply goes from sending to delivered on its event
- [ ] 5.2 Show the in-flight counter and the connection state in the header, and verify a spec that a dropped stream pauses and a return resumes
- [ ] 5.3 Make arrivals appear in place with no banner, toast or modal, and verify a spec on a signal-opened conversation
- [ ] 5.4 Show the working run as one fixed-size card that folds into `Run details` on the reply, and verify no per-event line remains
- [ ] 5.5 Honour reduced motion, and verify a spec with `prefers-reduced-motion` that no animation runs and state is still conveyed
- [ ] 5.6 Add the layout-shift check, observing `layout-shift` entries and polling header, tab bar and row rects through a simulated run, and verify it reports nothing beyond the one insertion

## 6. Viewports

Role: `testing-specialist`.

- [ ] 6.1 Define the six named viewports once (phone, tablet portrait, tablet landscape, laptop, desktop, wide) with Playwright device presets for touch, and verify the list matches `console-looks`
- [ ] 6.2 Add a mocked-tier spec that loads every page of every look at every viewport and asserts no sideways scroll, reachable controls and 44 pixel touch targets, and verify it fails on a planted overflow
- [ ] 6.3 Parametrise the `e2e-live` suite to run once per look at the desktop viewport, pinning each with `?look=`, and verify the nightly job runs both
- [ ] 6.4 Check the tablet layouts in both orientations by screenshot and read the images, per `visual-check.md`

## 7. The install default

Role: `frontend-developer`.

- [ ] 7.4 Flip the build default to `workspace` and verify the first-visit test

## 8. Unit tests

Role: `testing-specialist`.

- [ ] 8.3 Run the chart render tests and verify green
- [ ] 8.4 Verify coverage with the flags `build-test.md` names has no uncovered new behaviour
- [ ] 8.5 Run the mocked browser tier (`npm run test:e2e`, the `console-e2e` gate) for every look, including the viewport matrix from 6.2, and verify green

## 9. System tests

Role: `testing-specialist`.

- [ ] 9.1 State why the manager's system pack gains no lane, since nothing here is decided by a kubelet, RBAC or an informer
- [ ] 9.2 Run the `e2e-live` suite once per look against a real install (k3d or a local cluster, per `docs/testing.md`), and verify both runs pass with the specs reading only stable test identifiers

## 10. Documentation

Role: `frontend-developer`.

Reference docs:

- [ ] 10.1 Update `docs/console.md` for both looks and the switcher
- [ ] 10.2 Update `docs/console-guide.md` so the tour names the look it shows
- [ ] 10.3 Update `docs/testing.md` for the per-look and per-viewport browser tiers
- [ ] 10.4 Add `console.defaultLook` to `docs/configuration.md` and re-run `docs-generate.py`
- [ ] 10.5 Add a `docs/CHANGELOG.md` entry that the default look changes and how to keep `classic`
- [ ] 10.6 Update `.claude/rules/palette-and-mark.md` to say which tokens are shared with the site and which belong to a look

Adopter site:

- [ ] 10.7 Update the landing page, `docs/introduction.md` and `docs/getting-started.md` where they show the console
- [ ] 10.8 Update `docs/installation.md` where it shows the console's values
- [ ] 10.9 Re-run `npm run screenshots` and `npm run demo` for both looks and verify the assets changed
- [ ] 10.10 Run `publication-guard.py`, `retired-vocabulary-guard.py` and `docs-generate.py --check` and verify a clean verdict
