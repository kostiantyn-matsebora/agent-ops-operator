## MODIFIED Requirements

### Requirement: Publication stores and acknowledges, nothing else

`POST /signal/inbound` SHALL validate the batch, apply the source's
`attributeFilters`, durably enqueue one `ingest-signal` task per admitted
signal, and then respond. The `in.signal` consumer SHALL create ONE `Signal`
per admitted signal afterwards (`async-signal-ingest`). "Publication" below
names the consumer's create.

- A batch of N admitted signals yields N `Signal`s, even when several share
  an `initiationId`.
- The one exception is a signal whose `signalId` names a `Signal` that
  already exists. The consumer's create is answered `AlreadyExists`, so it
  changes nothing, records a duplicate activity event and succeeds.

`POST /signal/inbound` is no longer leader-gated. Any replica enqueues the
task, because the `in.signal` queue is bus-backed and any replica may accept.

Publication SHALL look up no existing `Signal`, not even by `signalId`: the
deterministic `sig-<h>` name makes a create answered `AlreadyExists` the
duplicate. It SHALL make NO routing decision:

- no claim lookup, no `Wired` check, no ambiguity check
- no cooldown consulted or spent
- no capacity check, no addressing parse
- no conversation created or appended, no chat surface told

`attributeFilters` stay SOURCE ADMISSION: a filtered-out signal is refused
at the request, enqueues nothing, persists nothing, and is reported on the
response as it is today.

Every admitted signal that can open or continue a conversation SHALL be
published this way — machine kinds, an unaddressed chat message, and an
addressed `/<pipeline> <task>`.

A chat COMMAND that only answers opens and continues nothing, so it is not a signal:

- The commands are the listing command, a bare `/<pipeline>` asking for its task, and `/close` or `/exit` sent on a general surface.
- The consumer answers it in place and persists no `Signal`.
- The request no longer answers it.

The response SHALL be written only after every admitted signal's task is
durably enqueued. A signal whose task cannot be enqueued SHALL NOT be
acknowledged. The response is an error, and the adapter's at-least-once
re-send posts the batch again.

A `Signal` therefore MAY not exist yet when the response is written. It
exists once its task has run, and a task that exhausts its attempts is dead
and visible, never silently lost.

- A re-send of a signal carrying a `signalId` is deduplicated at
  publication. The consumer creates it under its deterministic name, and
  `AlreadyExists` means it creates nothing.
- A re-send of a signal without one yields a further `Signal`. The
  consumer makes that safe — by cooldown and by equality reuse.
- A redelivery of ONE task SHALL NOT yield a second `Signal`.

Each `Signal` SHALL carry an ownerRef to its `SignalSource`.

#### Scenario: The CR exists before the acknowledgement

- **WHEN** an adapter posts an admitted signal to the inbound endpoint
- **THEN** an `ingest-signal` task for it is durably recorded before the
  `202` response is written, and a `Signal` CR owned by the named
  `SignalSource` exists once that task has run

#### Scenario: A signal that cannot be stored is not acknowledged

- **WHEN** enqueueing the task fails
- **THEN** the response is a `5xx` error instead of `202`, and a re-send of
  the batch is accepted: signals with a `signalId` that did land are
  deduplicated, the rest are enqueued

#### Scenario: Publication routes nothing

- **WHEN** an adapter posts a signal for a source no Ready Pipeline claims
- **THEN** the response acknowledges it as queued, a `Signal` exists once
  its task has run with no `status.phase`, and no conversation, cooldown
  entry or chat notice came from the call

#### Scenario: An addressed chat message is published like any other

- **WHEN** a chat signal whose payload is `/k8s-ops check nodes` is posted
- **THEN** a `Signal` is created with that payload verbatim, and no
  conversation exists until a consumer processes it

#### Scenario: Every admitted signal is its own Signal

- **WHEN** one batch carries three signals, two of them with the same
  `initiationId`
- **THEN** three `Signal`s exist once their tasks have run, none absorbed
  into another

#### Scenario: A filtered signal is refused at publication

- **WHEN** a signal fails one of its source's `attributeFilters`
- **THEN** no task and no `Signal` is created for it, and the response
  reports the filtered drop

#### Scenario: A dead task leaves no silent loss

- **WHEN** a task's attempts are exhausted before its `Signal` is created
- **THEN** the task is dead with its last error in the task stream, and no
  `Signal` exists for it
