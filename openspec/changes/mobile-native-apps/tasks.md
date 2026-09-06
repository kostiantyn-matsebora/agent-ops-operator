## 1. Web shell (responsive foundation)

- [ ] 1.1 In `platform/console/ui/src/App.tsx`, switch `Page` to `isManagedSidebar`, add a `PageToggleButton` inside `MastheadToggle`, and delete the `navOpen` state and `onPageResize` handler; verify with `npm run build` in the worktree's `platform/console/ui` and a Playwright shot at 390×844 (dev server per `visual-check.md`, PNG to the scratchpad) showing the sidebar hidden and the toggle present.

## 2. Web tables (responsive foundation)

- [ ] 2.1 Add `gridBreakPoint="grid-md"` and a `dataLabel` on every `Td` (matching its `Th` text) to the three tables in `Overview.tsx`, hiding Image and Source per design.md via `visibility`; verify `npm test` passes and a 390px shot shows labelled cards.
- [ ] 2.2 The same for `Config.tsx`'s kind list, conditions and findings tables (hide Labels, Age, Since); verify as 2.1.
- [ ] 2.3 The same for `Queues.tsx`'s three tables and `Vocabulary.tsx`'s two; verify as 2.1.
- [ ] 2.4 `Conversations.tsx`: add `gridBreakPoint`, hide Runs, Queued and Console below `lg`, give the select and reopen cells explicit narrow handling, and collapse the toolbar's filters behind `collapseListedFiltersBreakpoint="md"` with the search box kept visible; verify `Conversations.test.tsx` still passes and a 390px shot shows one card per conversation with labels and the search box.
- [ ] 2.5 `Conversation.tsx`: add `gridBreakPoint` to the run timeline table keeping Timeline visible; verify at 390px that the transcript, the composer with its command menu open, and the timeline each fit without sideways scroll.
- [ ] 2.6 Add `platform/console/ui/e2e/phone.spec.ts` at a 390×844 viewport asserting, for every route, `scrollWidth <= innerWidth`, the sidebar hidden with the toggle present, and the conversations list rendering cards with data labels; verify it passes with `npx playwright test e2e/phone.spec.ts` in the worktree and that the existing e2e specs still pass.

## 3. Native shell scaffolding

- [ ] 3.1 Create `platform/console-mobile/` as a Capacitor project pointing at `platform/console/ui`'s build output (`npm install @capacitor/core @capacitor/cli @capacitor/android @capacitor/ios`, `npx cap init`); verify `npx cap sync` completes with no errors and generates `android/` and `ios/` native project directories.
- [ ] 3.2 Build the connect screen (console base URL + bearer token fields, a verification call against a known-cheap endpoint before storing) using `@capacitor/preferences` for the non-secret URL and a native secure-storage plugin (Keychain/Keystore) for the token; verify a unit test on the connection-state logic (connect, reject, forget) and a manual run in the Android/iOS simulator per 4.1/5.1.
- [ ] 3.3 Wire the frontend's API client to call through `@capacitor/http` when running inside the native shell (feature-detected, so the same bundle still runs unmodified in a browser) and route the stored token as its bearer credential; verify with a unit test that the browser build's fetch path is untouched when the Capacitor bridge is absent.
- [ ] 3.4 Add offline/unreachable handling: mark the active view stale on a failed request and resume the console's existing snapshot-then-stream reconnect once reachable; verify with a unit test that simulates a request failure followed by a recovered one.

## 4. Android app

- [ ] 4.1 Configure `platform/console-mobile/android/` (app id, icon, splash screen, minimum SDK) and produce a debug build; verify it installs and runs in the Android Emulator at both a phone and a tablet skin, reaching a real console through the connect screen.
- [ ] 4.2 Add a release build config (signing left to a keystore the operator supplies, not committed) and a CI job building the debug APK on every change to `platform/console-mobile/`; verify the job succeeds and uploads the APK as a build artifact.

## 5. iOS app

- [ ] 5.1 Configure `platform/console-mobile/ios/` (bundle id, icon, splash screen, minimum iOS version) and produce a debug build; verify it installs and runs in the iOS Simulator at both an iPhone and an iPad simulator, reaching a real console through the connect screen.
- [ ] 5.2 Add a CI job on a `macos-latest` runner building the unsigned iOS app on every change to `platform/console-mobile/`; verify the job succeeds and uploads the build as a CI artifact. Code signing and TestFlight/App Store submission are left to whoever holds the Apple Developer Program account — not automated here.

## 6. Unit tests

- [ ] 6.1 `cd platform/console/ui && npm test` passes with the responsive table and shell changes covered (2.1–2.5's per-task assertions).
- [ ] 6.2 `cd platform/console-mobile && npm test` covers the connection-state logic (connect, reject, forget) and the native-vs-browser HTTP client selection from 3.2–3.3.

## 7. E2E tests

- [ ] 7.1 `npx playwright test e2e/phone.spec.ts` (from 2.6) passes against the worktree's build, covering the responsive layout across every route.
- [ ] 7.2 Nothing in this change is decided by a Kubernetes cluster — the native apps are new HTTP clients of the console's existing, unchanged API, and the console's own deployment, RBAC and CRDs do not move — so `platform/manager/test/e2e/` gets no new lane. Manual device verification (4.1, 5.1) stands in for what a cluster would otherwise decide, since the property under test is "does this app run on a real phone/tablet OS", which no envtest or e2e cluster lane can answer.

## 8. Documentation

### 8.1 Reference docs

- [ ] 8.1.1 `docs/console.md`: add a section describing the native apps as a third way to reach the console, alongside the browser — what they require (a console base URL and a bearer token; the external-authenticator auth mode is not supported by the app, per design.md's risk), and where CI publishes their build artifacts. No endpoint, RBAC grant or chart value changes, so no other reference doc needs an entry.
- [ ] 8.1.2 Confirm `docs/CHANGELOG.md` needs no entry (nothing breaking) and record that in the pull request description.

### 8.2 Adopter site

- [ ] 8.2.1 `docs/console-guide.md`: add the responsive tour content (navigation opens from the masthead, lists become cards on a phone) and a new section on the native apps — the connect-by-URL-and-token flow, and that tablet is the same layout as desktop at that width.
- [ ] 8.2.2 Add where to get the apps (a new `docs/integrations/`-style page or a section of `docs/console.md`, whichever this change lands closer to at the time — the artifacts are CI build output until someone with the store accounts publishes them, so state that plainly rather than implying a store listing exists).
- [ ] 8.2.3 Re-run BOTH `npm run screenshots` and `npm run demo` in `platform/console/ui` from the worktree and commit the regenerated site assets; verify `git status` shows the console PNGs and the landing recording updated and the masthead toggle visible in them.
