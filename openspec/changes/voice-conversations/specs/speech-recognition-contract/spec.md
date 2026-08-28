## Purpose
The recognizer owns the recognition contract; vendor implementations are adapters that poll it, so a vendor may run anywhere and is swapped by configuration.

## ADDED Requirements

### Requirement: Recognition is a job an adapter claims

The recognizer SHALL queue one recognition job per utterance and SHALL serve
`GET /recognize/jobs` as a long-poll adapters claim from, with a claim
window, retry on an expired claim, and at-least-once delivery. An adapter
SHALL post `POST /recognize/jobs/{id}/result` carrying the transcript, the
detected or assumed `lang`, and a confidence. For a streaming job an adapter
SHALL open `WS /recognize/jobs/{id}/stream` itself; the recognizer pipes audio
in and partial transcripts return on the same socket. No adapter SHALL expose
a port.

#### Scenario: An adapter outside the cluster
- **WHEN** a recognizer adapter runs on a host with no route into the cluster
- **THEN** it recognises every job, because every connection is one it opened

#### Scenario: A claim that dies is reclaimed
- **WHEN** an adapter claims a job and reports nothing within the window
- **THEN** the job is offered again, and one result is accepted

### Requirement: An adapter declares what it can do, and the owner adapts

On each claim an adapter SHALL declare whether it streams, whether it detects
language, and which languages it accepts. The recognizer SHALL hand a
streaming job only to a streaming adapter, otherwise the buffered clip; SHALL
pass the surface's `lang` hint as the candidate list to a detecting adapter
and as the answer to a non-detecting one; and SHALL offer a job only to an
adapter accepting its language when one is known.

#### Scenario: A non-detecting adapter is told
- **WHEN** the only adapter does not detect language and the surface hinted `uk`
- **THEN** the job carries `uk` as the language, and the result reports it

#### Scenario: Two adapters, one fits
- **WHEN** a streaming job arrives and both a streaming and a clip-only adapter are polling
- **THEN** the streaming adapter gets it

### Requirement: Vendors are adapters, and the stub is one

Each vendor SHALL be its own adapter component holding its own credential,
projected into its pod, and the recognizer SHALL hold none. A stub adapter
returning a fixed transcript SHALL exist and SHALL be indistinguishable to
the recognizer from a vendor.

#### Scenario: Swapping the vendor
- **WHEN** the Google adapter is replaced by a whisper adapter
- **THEN** the recognizer, the receiver and every surface are unchanged

#### Scenario: Tokens are per adapter
- **WHEN** an adapter polls
- **THEN** it authenticates with a token derived with context `recognizer-adapter:<name>`, validated by re-derivation
