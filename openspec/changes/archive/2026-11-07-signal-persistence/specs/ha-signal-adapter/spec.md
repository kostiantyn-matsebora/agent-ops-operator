# ha-signal-adapter (delta)

## ADDED Requirements

### Requirement: An emitted condition carries a signalId

The adapter SHALL send a `signalId` on every signal it posts, per
`signal-adapter-contract`. A message here is one emission of the dwell queue for one condition.

The `signalId` is `<initiationId>#<unix seconds the dwell cycle opened>`. Re-posting the same emission repeats it, and the next dwell cycle opens at a new time.

The id is stable across a re-send of the same delivery, including the
re-send after a `5xx`. It differs between two messages, even when they share
an `initiationId`.

#### Scenario: A re-posted emission repeats its id

- **WHEN** the adapter's post of an emission fails and is retried
- **THEN** the retry carries the same `signalId`, while the condition's next dwell cycle carries a different one
