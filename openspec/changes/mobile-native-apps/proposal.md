## Why

A phone or tablet reaching the console today means a browser tab: no home
screen icon, no push notification when a conversation needs a reply, and no
offline-tolerant reconnect after the OS suspends the tab. `console-responsive`
set out to fix the *browser* half of that and, in its own design, rejected a
separate app — the responsive fixes cost nothing extra to ship and the one
page a phone user wants already fit. That rejection is reversed here: the
console is now asked for as installable apps on Android phones, Android/iOS
tablets and iPhones, which a browser tab cannot be. `console-responsive` is
folded into this change rather than shipping twice — the same responsive
layout work is now a dependency of every target, not an alternative to one.

## What Changes

- The console frontend adopts the responsive layout `console-responsive`
  designed: a managed collapsible sidebar, tables that stack into labelled
  cards below the tablet breakpoint, a collapsing filter toolbar, and no page
  that scrolls sideways — verified at a phone viewport. This is a
  precondition for the native shell below, not a separate deliverable.
- A new `console-mobile/` directory wraps that same frontend bundle in a
  native shell (Capacitor), producing two installable artifacts from one
  UI covering four device classes: an Android app (phone and tablet, one
  APK/AAB — the layout is what tells them apart), and an iOS app (iPhone and
  iPad, one universal build).
  It sits at the repository ROOT, not under `platform/` — `platform/` is a
  component group whose directories are each exactly one published container
  image, and this produces app-store artifacts instead, which
  `repository-layout`'s existing rule already names as not belonging in a
  component group.
- The app adds one screen the browser has no need for: connect to a console
  by URL and store a bearer token in the OS keychain/keystore, since a native
  app has no origin session to inherit.
- Native HTTP is used for API calls (bypassing the WebView's CORS
  restrictions), so no CORS, contract or backend change is needed to serve a
  native client — it is the same read-mostly browser API and `/channel/inbound`
  write path `console-application` already serves.
- **Not in this change**: push notifications, offline caching beyond
  reconnect, and app-store account/publishing setup (the build artifacts are
  produced; who submits them to App Store Connect / Play Console is an
  operational decision for later).

## Capabilities

### New Capabilities
- `console-responsive`: the console frontend renders on a narrow viewport —
  collapsible navigation, tables that stack, no page that scrolls sideways.
  (Design and requirements carried over from the retired `console-responsive`
  change, now a foundation of this one rather than its own change.)
- `console-native-shell`: the console frontend is packaged as installable
  Android and iOS apps via a native wrapper, with device-local connection
  settings and secure token storage standing in for the browser's origin and
  session.

### Modified Capabilities

(none — `console-application` states the API contract, which does not change;
`console-adapter` and `console-deployment` state how the console is deployed
and served, which a native client added on top does not change)

## Impact

- `platform/console/ui/src/App.tsx` — shell and masthead (managed sidebar).
- `platform/console/ui/src/pages/{Overview,Config,Queues,Vocabulary,Conversations,Conversation}.tsx`
  — table props, column visibility, toolbar collapse.
- `platform/console/ui/e2e/` — a new phone-viewport spec.
- `console-mobile/` — new top-level directory (sibling of `platform/`,
  `runtimes/`, `docs/`): the Capacitor project, its Android and iOS native
  projects, the connection-settings screen, and its own build/test tooling.
  Not a container image, and not placed in a component group — `structure.md`'s
  own component table gains a row calling it out, shaped like the existing
  `test/` row's **NOT a component** verdict. `.github/components.sh` discovers components by
  `go.mod`/`Dockerfile`, and this directory has neither, so it is naturally
  invisible to that discovery (no exclusion rule needed, unlike `test/`).
- No Go, CRD, chart value or manager contract change.

Documents this makes untrue, both halves:

- Reference docs: `docs/console.md` gains a section describing the native
  apps as a third way to reach the console (endpoints, RBAC and values it
  documents do not move). `docs/CHANGELOG.md`: no entry — nothing breaking.
- Adopter site: `docs/console-guide.md` — the tour describes only the browser
  today and needs the native apps as a documented path, including the
  connect-by-URL-and-token flow the browser has no equivalent of.
  `docs/installation.md` or a new `docs/integrations/`-style page needs where
  to get the apps once published. The landing page and Introduction currently
  make no claim about how the console is reached and may want one line noting
  a phone/tablet app exists, once shipped.
