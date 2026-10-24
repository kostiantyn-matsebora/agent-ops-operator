## Purpose

The console frontend is packaged as installable Android and iOS apps, wrapping
the same responsive bundle `console-responsive` renders in a browser, with a
device-local connection and a securely stored credential standing in for the
browser's origin and session.

## ADDED Requirements

### Requirement: One frontend bundle, two installable targets
The native apps SHALL wrap the identical frontend bundle the browser is
served, built once and packaged two ways: an Android app usable on phone and
tablet, and an iOS app usable on iPhone and iPad. No app-specific fork of the
frontend SHALL exist; a page or component available in the browser SHALL be
available, unchanged, in the app.

#### Scenario: A page renders identically in the app and the browser
- **WHEN** the same console deployment is viewed through the native app and through a browser at an equivalent viewport
- **THEN** the rendered page is the same markup and behavior, save for the connection screen the app alone has

#### Scenario: Tablet layout comes from the same breakpoints
- **WHEN** the app is opened on a tablet-sized device
- **THEN** the layout renders exactly as the browser does at that viewport width, with no app-specific tablet layout

### Requirement: The app connects to an operator-named console over a stored credential
On first launch, and whenever no working connection is stored, the app SHALL
present a screen to enter a console base URL and a bearer token. On a
successful connection the token SHALL be stored in the platform's secure
credential storage (Keychain on iOS, Keystore-backed storage on Android) and
SHALL NOT be written to any other location on the device. The app SHALL offer
a way to forget the stored connection and return to this screen.

#### Scenario: First launch
- **WHEN** the app is opened with no connection stored
- **THEN** it shows the connect screen and makes no API call until a URL and token are entered

#### Scenario: A wrong token is refused the same way the browser is refused
- **WHEN** the entered token is rejected by the console
- **THEN** the app reports the failure without storing the token and remains on the connect screen

#### Scenario: Forgetting a connection
- **WHEN** the operator chooses to forget the stored connection
- **THEN** the token is removed from secure storage and the app returns to the connect screen on next launch

### Requirement: The app calls the console's existing API with no server-side change
The app SHALL call the same snapshot, stream and write endpoints the browser
calls, using the platform's native HTTP stack rather than the WebView's
fetch/XHR path, so the console's CORS posture SHALL NOT need to change to
serve it. The app SHALL NOT require a new endpoint, a new CRD field, a new
chart value or a change to the manager's channel or signal contracts.

#### Scenario: Reading and replying from the app
- **WHEN** an operator views the overview, browses a conversation, and replies from the app
- **THEN** every request reaches the same endpoints `console-application` already serves, and the console requires no configuration change to answer them

### Requirement: A lost connection is reported, not silently retried into staleness
When the app cannot reach the configured console — network loss, the console
unreachable, or a rejected token after having connected before — it SHALL show
that state rather than rendering stale data as if it were current, and SHALL
resume the normal snapshot-then-stream reconnect path once reachable again.

#### Scenario: The device goes offline mid-session
- **WHEN** the app loses network reachability while a view is open
- **THEN** the view is marked stale rather than left silently unchanged, and it resynchronizes automatically once reachability returns
