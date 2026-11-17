## Purpose

The generic HTTP request signal adapter in `signals/http/`. A dependency-free module that turns any authenticated HTTP request into one signal, with every normalized field mapped per source by template or constant. It owns no grouping policy.

## ADDED Requirements

### Requirement: The adapter serves HTTP requests as signals through the signal adapter contract
A signal adapter in `signals/http/` (own dependency-free Go module and image `agentops-signal-http`, listed by `.github/components.sh images` as `signal-http`) SHALL serve `SignalSource`s whose `spec.adapter` names its `SignalAdapter`, through the signal adapter contract. It SHALL accept `POST /request/{source}` on the port declared by `SignalAdapter.spec.port` (`LISTEN_ADDR`).

The `{source}` segment SHALL route to a source learned from `GET /signal/sources?adapter=<key>`, where `<key>` is the adapter's own `SignalAdapter` name, which is its routing key. One request SHALL produce exactly one signal, and the response SHALL be `202` once the manager accepts it.

The adapter SHALL NOT deduplicate, apply cooldown, group or route (`ADP-3`).

#### Scenario: A request becomes one signal
- **WHEN** an authenticated request is posted to `/request/<source>` for a served source
- **THEN** exactly one signal is pushed to `/signal/inbound` and the response is `202`

#### Scenario: Unknown source is refused
- **WHEN** a request names a source no served `SignalSource` matches
- **THEN** the adapter returns `404` and pushes nothing

#### Scenario: A manager refusal is relayed
- **WHEN** the manager answers `5xx` to the push
- **THEN** the adapter answers the sender `5xx` so it can retry

### Requirement: A bearer token is mandatory and belongs to one source
Every request SHALL carry `Authorization: Bearer <token>` matching the token projected from that source's own `credentialsSecretRef` key `TOKEN` (`AGENTOPS_CRED_<SOURCE>_TOKEN`), compared in constant time. A mismatch or absence SHALL return `401` and push nothing.

A token SHALL be valid for its own source only. There SHALL be no adapter-wide token, no anonymous mode, and no token accepted in the URL.

A source with no `credentialsSecretRef` or no `TOKEN` key SHALL report `Ready=False` and refuse its requests.

#### Scenario: Wrong token is refused
- **WHEN** a request carries a token that matches another source's credentials
- **THEN** the adapter returns `401` and pushes nothing

#### Scenario: A source without a token is not ready
- **WHEN** a served source has no `TOKEN` credential
- **THEN** the adapter reports `Ready=False` with a reason naming the missing token and answers its requests `401`

#### Scenario: Token in the URL is ignored
- **WHEN** a request carries a token only in the query string
- **THEN** the adapter returns `401`

### Requirement: Each signal field is a per-source template or constant
A source's `config` SHALL map the normalized fields below to a Go `text/template` string. A string with no actions is a constant. Templates SHALL read the request through exactly two functions, `header "<name>"` and `body "<dotted.path>"`, over a JSON body. `header` SHALL match the name case-insensitively, as HTTP header names are. `body` paths SHALL be case-sensitive. A non-JSON body, an empty body and a path the JSON does not contain SHALL each leave `body` unresolved, while headers stay available.

For example, a source mapping a sender's delivery id and severity:

```yaml
config:
  initiationId: '{{ body "alert.group" }}'
  signalId: '{{ header "X-Delivery-Id" }}'
  kind: task
  title: '{{ body "alert.name" }}'
  severity: '{{ body "alert.level" }}'
  labels:
    env: '{{ header "X-Env" }}'
```

| Field | Default | When unresolved or invalid |
|---|---|---|
| `initiationId` | fresh random id per request | fresh random id |
| `signalId` | fresh random id per request | fresh random id |
| `kind` | none, not sent (the manager then reads `alert`) | none, not sent |
| `title` | none | none |
| `labels` | none | the key is dropped |
| `payload` | the raw request body | the raw request body |
| `severity` | none | none |

`reader` SHALL NOT be configurable and SHALL NOT be sent.

A reference that does not resolve SHALL never fail the request. It SHALL degrade only its own field as the table says.

#### Scenario: A configured initiationId groups requests
- **WHEN** two requests render the same `initiationId` from their headers and body
- **THEN** both signals carry that id and the manager routes the second as a recurrence

#### Scenario: No initiationId template means no grouping
- **WHEN** the source configures no `initiationId`
- **THEN** each request carries a different random id and opens its own conversation

#### Scenario: A missing reference falls back
- **WHEN** the `initiationId` template reads a header the request did not send
- **THEN** the adapter sends a fresh random id and the request succeeds

#### Scenario: The body is the default payload
- **WHEN** the source configures no `payload`
- **THEN** the signal's `payload` is the raw request body

### Requirement: Every emitted field respects the contract's vocabulary and bounds
The adapter SHALL validate each rendered value before pushing, so one stray value never causes the manager to refuse the batch.

| Field | Rule |
|---|---|
| `kind` | `alert`, `job` or `task`. Any other value, `chat` included, is sent as none. |
| `severity` | `critical`, `error`, `warning` or `info`, after the optional value map. Any other value is sent as none. |
| `signalId` | 1 to 256 bytes of printable ASCII with no spaces. Otherwise a fresh random id. |
| `title` | truncated to 500 runes |
| `labels` | at most 32 keys. A key under the `agentops.dev/` prefix is dropped (`LANE-2`, `LANE-3`). |
| request body | capped below the manager's 1 MiB limit. A larger request is refused `413`. |

#### Scenario: An unknown severity is sent as unrated
- **WHEN** a `severity` template renders `fatal` and the source's value map has no entry for it
- **THEN** the signal carries no `severity` and the manager accepts the batch

#### Scenario: chat is never sent
- **WHEN** a `kind` template renders `chat`
- **THEN** the signal carries no `kind`

#### Scenario: A reserved label is dropped
- **WHEN** a `labels` entry names `agentops.dev/channel`
- **THEN** that label is not sent

#### Scenario: An oversized request is refused
- **WHEN** a request body exceeds the adapter's cap
- **THEN** the adapter returns `413` and pushes nothing

### Requirement: Configuration faults surface on the source, request-level misses do not
The adapter SHALL validate a source's config when it loads the source. A template that fails to parse, an unknown function, or a constant `kind` or `severity` outside its vocabulary SHALL report `Ready=False` through `POST /signal/sources/{name}/status`, with a reason and message naming the field. A source that loads cleanly SHALL report `Ready=True`.

A fault on one source SHALL NOT affect another served source.

#### Scenario: A bad template is reported
- **WHEN** a source's `title` template has a syntax error
- **THEN** the source reports `Ready=False` naming `title` and its requests are refused

#### Scenario: A constant chat kind is reported
- **WHEN** a source sets `kind: chat`
- **THEN** the source reports `Ready=False` naming `kind`

#### Scenario: One bad source does not stop another
- **WHEN** two sources are served and only one has a bad template
- **THEN** the other keeps accepting requests

### Requirement: The adapter's page states what a message is and who chooses the agent's prompt
The adapter's documentation SHALL state that one request is one message, so `signalId` is random unless the source maps a sender-supplied delivery id (`ADP-9`). It SHALL state that a templated `kind` lets a token holder choose the agent's prompt, and that a constant is the safer choice. It SHALL state that an unset `kind` is not sent and the manager then reads it as `alert`. It SHALL warn that a sender pointed at agent-ops' own events can feed a loop.

#### Scenario: The adapter page carries the required statements
- **WHEN** `docs/integrations/http.md` is read
- **THEN** it states the message definition, the `kind` consequences and the loop warning

### Requirement: The adapter ships as an embedded subchart gated by `http.enabled`, off by default
The adapter SHALL ship as its own subchart at `chart/charts/http/`, like every other bundle. The parent `chart/Chart.yaml` SHALL declare it as a dependency with `condition: http.enabled`, and the subchart's templates SHALL be additionally self-gated on `http.enabled`.

The parent `chart/values.yaml` SHALL document an `http:` block with `enabled: false` and a typical enablement snippet. The subchart SHALL read no parent scope but `global.`.

By default the bundle SHALL render the `SignalAdapter` with its port, workload knobs and the adapter's own floor service account, and nothing else. It SHALL NOT be enabled by demo mode.

#### Scenario: Default install renders nothing
- **WHEN** the chart is installed with default values
- **THEN** no object from the bundle renders

#### Scenario: Demo mode does not enable the bundle
- **WHEN** the chart is installed with `global.demo.enabled=true` and default bundle values
- **THEN** no object from the bundle renders

#### Scenario: Enabling renders the adapter
- **WHEN** `http.enabled=true`
- **THEN** a `SignalAdapter` with `spec.port` set renders and the reconciler creates its Service
