## Purpose
The contract between a voice surface — anything that hears or speaks — and agent-ops: audio in through one endpoint, spoken output out through one long-poll, with the surface exposing nothing.

## ADDED Requirements

### Requirement: Audio enters through one endpoint, as a clip or a stream

The receiver SHALL accept `POST /audio` carrying `surface`, `speaker`, an
optional `lang` hint, an optional `threadHint`, and audio either as one
finished clip (body with a declared media type) or as a stream (chunked, or a
WebSocket the surface opens). A surface that has already transcribed MAY send
`text` instead of audio, and the receiver SHALL then skip recognition. The
receiver SHALL answer with an utterance id, and the outcome of the utterance
SHALL reach the surface only through `/audio/ops`.

#### Scenario: A clip from a voice note
- **WHEN** a surface posts one finished OGG/Opus clip
- **THEN** it is accepted, recognised, analysed, and whatever follows arrives as ops

#### Scenario: A live microphone streams
- **WHEN** a surface opens a stream and sends audio as it is captured
- **THEN** recognition may begin before the stream ends, and the surface is not required to buffer

### Requirement: Spoken output leaves through one long-poll

The sender SHALL serve `GET /audio/ops` per surface adapter, long-poll,
at-least-once with stable op ids and an explicit completion report, exactly
as the channel contract does. Each op SHALL carry audio in the format the
adapter declared, the TEXT that was spoken, `lang`, the `kind` of what it
renders (`question`, `answer`, `notice`, `card`) and the conversation it
belongs to when there is one.

#### Scenario: A speaker that was off hears it once
- **WHEN** an out-half adapter reconnects after an outage
- **THEN** it receives the ops it never completed, once each, in order

#### Scenario: Text travels with the audio
- **WHEN** any op is delivered
- **THEN** its text is present, so a surface with a screen shows what was said and a test needs no ears

### Requirement: A surface implements either half, and exposes no port

A surface adapter SHALL implement the in-half, the out-half, or both, and
SHALL initiate every connection: it needs a URL and a token, never an inbound
port. Tokens SHALL be derived per surface adapter with context
`voice-surface:<name>` and validated by re-derivation.

#### Scenario: A room speaker
- **WHEN** an adapter only ever long-polls `/audio/ops`
- **THEN** it is a complete, conformant surface adapter

#### Scenario: Nothing reaches in
- **WHEN** a surface adapter runs outside the cluster
- **THEN** no component attempts a connection to it
