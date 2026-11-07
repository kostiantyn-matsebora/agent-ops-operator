# pulse-signal-adapter (delta)

## ADDED Requirements

### Requirement: A firing alert carries a signalId

The adapter SHALL send a `signalId` on every signal it posts, per
`signal-adapter-contract`. A message here is one verified webhook delivery of a firing alert.

The `signalId` is `pulse:<sha256 of the verified request body, hex>`. Pulse retries a failed delivery with the identical body, so the retry repeats the id.

The id is stable across a re-send of the same delivery, including the
re-send after a `5xx`. It differs between two messages, even when they share
an `initiationId`.

#### Scenario: A retried delivery repeats its id

- **WHEN** Pulse re-delivers the same firing alert after a failed response
- **THEN** the signal carries the same `signalId` as the first delivery
