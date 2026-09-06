## Context

See proposal.md — Why. Two things already true about `platform/console/`
shape everything below:

- **It is one PatternFly 6 React SPA**, built to a static bundle and served
  embedded via `go:embed` — no server-side rendering, no per-client build.
  `App.tsx` holds `navOpen` state and `onPageResize={() => setNavOpen(true)}`;
  the masthead renders no toggle. Eleven `@patternfly/react-table` tables lack
  `gridBreakPoint`. The transcript is already a narrow-safe grid; the
  composer's command menu is absolutely positioned above the field.
  `index.html` already ships `width=device-width`. This is the state
  `console-responsive`'s tasks fix — see that spec for the full detail; it is
  carried here unchanged.
- **The browser talks to the console over a documented, credential-agnostic
  HTTP+SSE API** (`console-application`): snapshot endpoints per view, one SSE
  stream of cursored deltas, and two writes — `POST /channel/inbound` (reply)
  and `POST /signal/inbound` (originate). Auth is a bearer token (or an
  external forward-auth proxy) on reads, plus a resolved identity on writes.
  Nothing about that contract assumes a browser origin or a cookie jar; it is
  already the shape a non-browser client needs.

No prototype was made. Composition for the responsive half is PatternFly's
own stacked-table and managed-sidebar layouts as shipped; composition for the
native half is a wrapped WebView, whose chrome is the OS's, not a drawn
mockup.

## Goals / Non-Goals

**Goals:**
- One frontend implementation serves the browser, the Android app and the
  iOS app — no second UI, no second `blocks.ts` parser, no per-platform
  layout code.
- Phone, tablet and desktop are one responsive layout, not three.
- The native apps need no console-side change beyond what `console-responsive`
  already delivers.

**Non-Goals:**
- Push notifications. The console has no outbound-to-device mechanism today
  (delivery is per bound *channel*, and a device is not one); adding one is a
  separate change once this ships and the delivery model is designed for it.
- Support for the external-authenticator auth mode (`console-application`'s
  "declared external" path) — see Risks.
- Offline data entry or caching beyond an ordinary reconnect.
- App-store account setup, signing certificate provisioning, or submitting
  to review. This change produces installable build artifacts; publishing
  them is an operational step outside the repository.
- A tablet-specific layout distinct from the browser's own tablet breakpoint.

## Decisions

**Responsive rendering is a precondition, not a parallel track.** Carried
over from the retired `console-responsive` change with no revision:
`Page isManagedSidebar` plus a `PageToggleButton`, `gridBreakPoint="grid-md"`
on every table with matching `dataLabel`s, hiding low-value columns via
`visibility` rather than stacking them, and `Toolbar
collapseListedFiltersBreakpoint="md"` on the conversations toolbar. See that
capability's spec for the full requirement-and-scenario detail; it is not
repeated here.

**A native wrapper (Capacitor) around the existing bundle, not a rewrite in
React Native or Flutter, and not three separate native codebases.**

| Option | Cost |
|---|---|
| **Capacitor wrapper (chosen)** | one UI, reuses `blocks.ts` unchanged, reuses every page and test as-is; native code is limited to the connect screen and secure-storage/HTTP plugins |
| React Native rewrite | a second UI implementation of every page (PatternFly has no React Native equivalent); `blocks.ts` itself is portable, but nothing that renders its output is |
| Flutter | a second UI *and* a second parser — Dart cannot import `blocks.ts`, reintroducing exactly the two-parsers cost `adapters.md` already accepts once and warns against a third |
| Separate native codebases per platform (Swift + Kotlin) | two more UIs and two more parsers, for a page count that does not justify it |

The console is a small number of data-dense pages, not a page count or an
interaction style (swipe, native gestures) that would justify a from-scratch
native UI. Wrapping is the option that costs nothing already paid for.

**`platform/console-mobile/` is a new component, but not a container
image.** It holds the Capacitor project plus the Android and iOS native
projects it generates into. `.github/components.sh` discovers components by
`go.mod` or `Dockerfile` presence; this directory has neither, so it needs no
exclusion rule (unlike `test/`, which carries both and had to be excluded
explicitly) — it is simply invisible to image-based discovery, which is
correct, since it publishes to app stores rather than GHCR. `structure.md`'s
table gains a row noting this is the first non-image component; the
path-to-image-name rule does not apply to it.

**Native HTTP for every API call, never the WebView's fetch.** Capacitor's
HTTP plugin routes requests through the native platform's networking stack,
which is not subject to the WebView's CORS enforcement. This is what keeps
`console-application`'s contract, including its CORS posture, entirely
unchanged — the alternative (relaxing CORS to accept the app's origin) would
be a server-side change for every operator, not just the ones who install the
app.

**Connection is URL + bearer token, stored in platform secure storage.** The
app has no browser origin and no session cookie, so it needs what the console
already supports as its primary auth mode: a bearer token. Entered once on a
connect screen, verified against the console, then held in Keychain
(iOS) / an encrypted Keystore-backed store (Android) — never in
`localStorage`-equivalent or a plain file, which either platform's backup
mechanism could otherwise carry off-device unencrypted.

**Tablet is not a fourth layout.** Both an Android tablet and an iPad render
the same responsive breakpoints as a desktop browser at that width, produced
already by `console-responsive`. A tablet is a viewport width, not a distinct
product surface.

## Risks / Trade-offs

- [The app only works when the console authenticates via bearer token; a
  console declaring `console.auth.external` (fronted by an oauth2-proxy) has
  no forward-auth identity for a native client to present] → documented as a
  requirement for the app path (`docs/console-guide.md`), not solved here;
  presenting that login flow to a native app is a real design problem (an
  in-app browser tab running the proxy's OAuth flow) sized for its own
  change once the token path has shipped and is in use.
- [iOS builds need a macOS toolchain, which this repository's CI does not
  have (`build-test.md`'s container is Linux-only)] → the iOS build step
  runs on a `macos-latest` GitHub Actions runner, added only for this
  component's job; nothing else in the matrix needs one.
- [A WebView-hosted bundle behaves slightly differently from a desktop
  browser one — viewport units, safe-area insets around notches/home
  indicators] → the phone e2e spec from `console-responsive` covers layout
  correctness; safe-area insets are handled with the standard
  `env(safe-area-inset-*)` CSS, verified with a real-device screenshot per
  platform before this change is called done, per `visual-check.md`.
- [Publishing needs an Apple Developer Program membership and a Google Play
  Console account, both outside this repository] → out of scope by design
  (see Non-Goals); tasks record where the built artifacts land so a person
  holding those accounts can submit them by hand.
