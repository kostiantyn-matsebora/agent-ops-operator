# alertmanager-signal-adapter (delta)

## ADDED Requirements

### Requirement: An alert notification carries a signalId

The adapter SHALL send a `signalId` on every signal it posts, per
`signal-adapter-contract`. A message here is one alert of one webhook delivery.

The `signalId` is `am:<sha256 of the raw request body, hex>:<alert fingerprint or index>`. Alertmanager retries a failed delivery with the identical body, so the retry repeats the id.

The id is stable across a re-send of the same delivery, including the
re-send after a `5xx`. It differs between two messages, even when they share
an `initiationId`.

#### Scenario: A retried webhook repeats its ids

- **WHEN** Alertmanager re-delivers the same webhook body after a failed response
- **THEN** each alert carries the same `signalId` as in the first delivery
