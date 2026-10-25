# control-plane-api Specification

## Purpose
The control-plane API is the manager's dedicated surface for operational
configuration — a caller asking the manager to change state on purpose,
distinct from the manager's own reconcile loop and from every adapter or
coordination surface it already serves.

## Requirements

### Requirement: A dedicated bearer token authenticates every request

`/control-plane/*` SHALL require `Authorization: Bearer <token>`, checked
constant-time against a token read once from the `CONTROL_PLANE_TOKEN`
environment variable at process start. The token SHALL be distinct from `ADAPTER_TOKEN` and from
the coordinator token.

#### Scenario: A correct token reaches the handler
- **WHEN** a caller presents the configured control-plane token on a
  `/control-plane/*` request
- **THEN** the handler runs

#### Scenario: A missing or wrong token is refused
- **WHEN** a caller presents no token, or a token that does not match the
  configured one
- **THEN** the manager responds 401 and the handler does not run

### Requirement: An unconfigured token authorizes nobody

An unset control-plane token SHALL NOT be treated as "no authentication
required". It SHALL behave as a surface no caller can reach.

#### Scenario: The surface is unconfigured
- **WHEN** the control-plane token's environment variable is unset or empty
- **THEN** every `/control-plane/*` request is refused with 503, whatever
  token it carries or omits

### Requirement: The token reaches only the control-plane surface

A caller holding the control-plane token SHALL reach `/control-plane/*` and
no other route. A caller holding `ADAPTER_TOKEN`, a derived adapter token,
or a coordinator token SHALL NOT reach `/control-plane/*`.

#### Scenario: The adapter token does not open the control plane
- **WHEN** a caller presents `ADAPTER_TOKEN` (or a token derived from it) on
  a `/control-plane/*` request
- **THEN** the manager responds 401

#### Scenario: The control-plane token does not open other surfaces
- **WHEN** a caller presents the control-plane token on `/signal/*`,
  `/channel/*`, `/work`, or `/coordinate/*`
- **THEN** the manager responds 401

### Requirement: The placeholder kind proves a full write-then-read round trip

The control-plane surface SHALL let an authenticated caller create or update
a placeholder object by name and key/value pair, and read one back by name.
A read SHALL return exactly the key/value pair last written for that name.

| Verb | Route | Success |
|---|---|---|
| Create or update | `PUT /control-plane/probes/{name}` | 200, body `{"ok": true}` |
| Read | `GET /control-plane/probes/{name}` | 200 with the stored object |

Request body of the `PUT`:

```json
{"name": "example", "key": "greeting", "value": "hello"}
```

Response body of the `GET`:

```json
{"name": "example", "key": "greeting", "value": "hello"}
```

`name` in the `PUT` body is optional. When present it SHALL match the
route's `{name}`, and when omitted the route's `{name}` applies.

Every error response (400, 401, 404, 503) carries a JSON body with one
`error` string:

```json
{"error": "probe \"example\" not found"}
```

#### Scenario: Writing then reading returns what was written
- **WHEN** a caller creates a placeholder object with a given name, key, and
  value, then reads it back by that name
- **THEN** the response carries the same key and value

#### Scenario: A body name that differs from the route name is refused
- **WHEN** a caller `PUT`s a body whose `name` differs from the route's
  `{name}`
- **THEN** the manager responds 400 and stores nothing

#### Scenario: A body without a name uses the route name
- **WHEN** a caller `PUT`s a body that omits `name`
- **THEN** the manager stores the object under the route's `{name}` and
  the response carries that name

#### Scenario: Reading an unknown name fails explicitly
- **WHEN** a caller reads a placeholder object by a name that was never
  created
- **THEN** the manager responds 404, naming the object as not found

#### Scenario: Writing again updates the same object
- **WHEN** a caller writes a new key/value pair to a name that already
  exists
- **THEN** a later read by that name returns the new pair, not the old one

### Requirement: The placeholder kind is disposable scaffolding, not a feature

The placeholder kind SHALL carry no meaning beyond exercising the round
trip. Its documentation SHALL state that it is scaffolding to be replaced,
not extended, once a real first resident of `/control-plane/*` is proposed.

#### Scenario: No behavior beyond the round trip is promised
- **WHEN** a caller inspects the placeholder kind's contract
- **THEN** it finds only create/update-by-name and read-by-name, and no
  other guarantee

### Requirement: The surface writes only the kinds it hosts

A caller authenticated on `/control-plane/*` SHALL be able to act only on
the kinds this surface exposes handlers for. It SHALL NOT act as a general
proxy to any other kind the manager's own identity can reach.

#### Scenario: The control plane is not a general API proxy
- **WHEN** a caller holding the control-plane token requests a kind
  `/control-plane/*` has no handler for
- **THEN** the manager responds 404, and no other kind's object is created,
  read, or modified
