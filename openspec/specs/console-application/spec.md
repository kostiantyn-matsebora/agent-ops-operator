# console-application Specification

## Purpose
TBD - created by archiving change rich-console-ui. Update Purpose after archive.

## Requirements

### Requirement: One service fans in every source the browser must not touch
The console SHALL be a single Go service that aggregates Kubernetes CR list/watch, the manager's activity stream, install facts (Deployments and pods), and the manager's channel and signal contracts, and serves a browser API plus the frontend. The browser SHALL NOT hold Kubernetes credentials or reach the Kubernetes API, the manager, or the activity stream directly.

The service SHALL make no write to the Kubernetes API. Its only writes SHALL be `POST /channel/inbound` (reply) and `POST /signal/inbound` (originate), both through existing contracts.

#### Scenario: One upstream stream serves many browsers
- **WHEN** several browsers are connected
- **THEN** the console holds one activity-stream connection to the manager and multiplexes it, rather than one upstream connection per browser

#### Scenario: No write path to the cluster
- **WHEN** any console endpoint is exercised, including origination and chat
- **THEN** no Kubernetes object is created, updated or deleted by the console

### Requirement: Snapshots are authoritative and the stream carries cursors
The console SHALL serve a snapshot endpoint per view and a single SSE stream carrying CR deltas, activity events and transcript appends, each with a monotonic cursor. The browser SHALL treat snapshots as authoritative and re-fetch on RESYNC; a missed event SHALL cost staleness, never a wrong screen. First connect and reconnect SHALL follow the same path.

**A DELTA CARRIES ITS OBJECT, AND IS APPLIED.** An event SHALL carry the changed object as the console would have fetched it, and the browser SHALL apply it to what it already holds. Re-fetching on every delta was the mechanism, and it cost a request and a blank page per change while the answer was already on the wire.

A snapshot stays AUTHORITATIVE, which is what makes applying safe: a resync replaces applied state wholesale, so an applier that is ever wrong is corrected by the next reconnect rather than persisting.

#### Scenario: A sleeping tab converges
- **WHEN** a browser is disconnected while many changes occur, then reconnects
- **THEN** it receives a resync, re-fetches snapshots, and its rendered state equals a cold load

#### Scenario: A delta updates the view without a request
- **WHEN** a watched object changes while a view holding it is open
- **THEN** the view updates from the event, and no snapshot is re-fetched

#### Scenario: The wire format survives CRD evolution
- **WHEN** a CRD gains fields
- **THEN** the browser renders the fields it knows and ignores the rest, exactly as it does from a snapshot

### Requirement: The overview page reports the installation and what is wrong with it
The console SHALL serve an overview covering: chart version and `appVersion`; the manager's image, readiness, replica state and uptime; every adapter with its image, readiness, port and served-CR count; every runtime image; `MAX_RUNTIMES` against runtime pods in use; active conversations and queued inputs; and a rollup of EVERY condition across every watched kind that is not `True`, newest first, each linking to its object.

#### Scenario: Health is answerable on one page
- **WHEN** any component reports a failing condition, or a pod is not ready
- **THEN** it appears in the overview's problem rollup with its reason, without the operator visiting another page

#### Scenario: Versions are concrete
- **WHEN** the overview is loaded
- **THEN** it names the image and version of the manager, every adapter and every runtime present in the namespace

### Requirement: Queue state is a first-class view that separates queued from stalled
The console SHALL present work and delivery queues as their own view, keeping the two distinct:

- **Work queue** — conversations waiting for a runtime slot against `MAX_RUNTIMES`, and inputs waiting behind an inflight run because dispatch is strictly serial per conversation.
- **Delivery queue** — channel ops queued for an adapter to claim, and ops claimed but not completed, per adapter.

Every entry SHALL carry an age, and the view SHALL flag stuck items explicitly — ops queued with nothing claiming them, ops claimed and never completed beyond a threshold, and conversations inflight far beyond typical run duration — rather than leaving the operator to compare timestamps. Active cooldowns SHALL be shown here, because a suppressed signal lane is otherwise indistinguishable from an idle one.

Each row SHALL link to the conversation, adapter or pipeline it concerns.

#### Scenario: A backlog is distinguishable from a stall
- **WHEN** ops are queued and being claimed and completed with steady turnover
- **THEN** the view shows depth without flagging a fault
- **WHEN** ops are queued and nothing claims them, or claimed items age past the threshold
- **THEN** the view flags the responsible adapter as stalled

#### Scenario: Capacity exhaustion is named as such
- **WHEN** runtime slots in use equal `MAX_RUNTIMES` and conversations are waiting
- **THEN** the view reports the ceiling as the cause, distinguishing it from a hung runtime

#### Scenario: Suppression is visible
- **WHEN** a signal lane is in cooldown
- **THEN** the view shows the active cooldown, so an idle-looking lane is not mistaken for a healthy one

#### Scenario: Queue state stays live without a new browser connection
- **WHEN** queue depths change
- **THEN** the view updates over the stream the browser already holds, and per-conversation queueing derives from the CR watch rather than polling

### Requirement: A metrics backend extends history and is optional
The console SHALL support an optional metrics backend query URL. When configured, it SHALL serve time ranges beyond the activity buffer as aggregates read from that backend, and SHALL label them as aggregate — rates, percentiles and depths carrying no per-item identity — so a long window is never mistaken for the exact per-hop record.

The console SHALL store no time series of its own, and SHALL remain fully functional with no backend configured, offering buffer-length windows and stating that longer ones are unavailable rather than rendering empty charts.

#### Scenario: Optional, not required
- **WHEN** no metrics backend is configured
- **THEN** every view works, and only windows longer than the buffer are unavailable, with the reason stated

#### Scenario: History without storage
- **WHEN** a backend is configured and a long window is requested
- **THEN** the aggregates are read from it, and the console persists nothing

#### Scenario: Aggregate is not mistaken for detail
- **WHEN** a historical window is displayed
- **THEN** it is labeled as aggregate, and per-item drill-down is offered only within the range the activity buffer covers

### Requirement: The configuration browser renders and cross-checks every CR
The console SHALL list every `agentops.dev` kind with a per-kind summary, and offer a detail view carrying conditions, the full spec, the raw YAML, and inbound references. It SHALL surface cross-object findings — references resolving to nothing, SignalSources no Pipeline claims, Channels whose adapter is absent, `configSchema` violations, and Pipelines whose profile has no runtime.

Findings sourced from a reconciler condition SHALL be presented as such; findings the console derives by cross-reference SHALL be marked as the console's own. Configuration SHALL be read-only.

#### Scenario: YAML matches the cluster
- **WHEN** a CR's detail view is opened
- **THEN** the YAML shown is equivalent to `kubectl get <kind> <name> -o yaml`

#### Scenario: Silent misconfiguration becomes visible
- **WHEN** a SignalSource is claimed by no Pipeline, or a Channel names a nonexistent adapter
- **THEN** the configuration view reports it with the reason, distinguishing reported conditions from console-derived findings

#### Scenario: No editing
- **WHEN** any configuration view is used
- **THEN** no endpoint exists to modify a CR

### Requirement: Reads are authenticated; writes require identity and can be disabled
Read access SHALL require a bearer token or a session established from it, UNLESS the deployment declares that authentication is performed upstream. Write actions — replying in a conversation and originating one — SHALL additionally require `console.write.enabled` and a resolved identity, taken from a trusted forward-auth header when present and recorded as the token identity otherwise. An unconfigured token SHALL authorize nobody, and SHALL be indistinguishable from a wrong token.

The two states SHALL remain independent: an absent or empty token SHALL NEVER be interpreted as "authentication not required". Only an explicit declaration that an external authenticator fronts the console SHALL relax the token requirement.

When authentication is declared external, writes SHALL require an identity resolved from a forward-auth header and SHALL be refused when none resolves. The token identity SHALL NOT be used as a fallback in that mode, because no token was proven and a write log naming one would assert something untrue.

Every write SHALL be logged with the resolved identity.

#### Scenario: Unconfigured is closed, not open
- **WHEN** no token is configured
- **THEN** every authenticated route is refused, and login failures do not reveal whether a token exists

#### Scenario: Viewer-only deployment
- **WHEN** `console.write.enabled` is false
- **THEN** the composer and the new-conversation action are absent, and both endpoints reject requests server-side

#### Scenario: Identity is carried, not assumed
- **WHEN** a trusted forward-auth header is present
- **THEN** that identity is recorded on the write and carried as the signal's sender

#### Scenario: External authentication admits the request
- **WHEN** the deployment declares authentication external and a request arrives with no bearer token
- **THEN** reads are served, because the declaration — not the absence of a credential — is what opened the door

#### Scenario: External authentication without identity is read-only
- **WHEN** authentication is declared external, writes are enabled, and a request carries no forward-auth identity header
- **THEN** the write is refused and no identity is invented, while reads continue to be served

#### Scenario: An empty token still closes a console that authenticates
- **WHEN** authentication is NOT declared external and the configured token is empty
- **THEN** every authenticated route is refused, exactly as when a token is set and a wrong one is presented

### Requirement: The set of trusted identity headers is a published interface

The identity headers the console reads from a fronting proxy are the interface
between the console and whatever authenticates in front of it. That set SHALL be
treated as published: it SHALL be documented for an operator in full, in the
order the console prefers them, on the adopter-facing page that tells an operator
how to expose the console.

Changing the set — adding a header, removing one, or reordering preference —
SHALL be accompanied by the documentation change in the same change, because an
operator's proxy configuration is written from that list. A header the console
trusts but the documentation omits is a header an install will not strip, and a
client-supplied copy of it then reaches the console unopposed.

The console SHALL NOT gain a second, undocumented way to assert an identity.

#### Scenario: An operator configures a forward-auth proxy

- **WHEN** an operator configures a proxy to front the console
- **THEN** the documentation names every header the console will trust, so all
  of them can be set by the proxy and none accepted from a client

#### Scenario: A header is added to the trusted set

- **WHEN** the console is changed to read an additional identity header
- **THEN** the adopter-facing documentation of the set is updated in the same
  change

#### Scenario: The documented list is checked against the code

- **WHEN** the published list is compared to the console's own source
- **THEN** the two contain the same headers in the same order
