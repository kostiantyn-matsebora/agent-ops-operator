# k8s-events-signal-adapter (delta)

## ADDED Requirements

### Requirement: An event observation carries a signalId

The adapter SHALL send a `signalId` on every signal it posts, per
`signal-adapter-contract`. A message here is one observation of a Kubernetes Event at one `count`.

The `signalId` is `<source>/<event uid>/<count>`. A watch resync that replays an unchanged Event repeats it, and an aggregated update increments `count`.

The id is stable across a re-send of the same delivery, including the
re-send after a `5xx`. It differs between two messages, even when they share
an `initiationId`.

#### Scenario: A resync replay repeats the id

- **WHEN** a watch resync delivers an Event whose `count` has not changed
- **THEN** the signal carries the same `signalId` as before

#### Scenario: An aggregated update is a new message

- **WHEN** the Event's `count` rises from 3 to 4
- **THEN** the signal carries a different `signalId` from the one it carried at 3
