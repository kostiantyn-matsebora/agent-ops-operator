## ADDED Requirements

### Requirement: Voice owners are inside the wall; vendor adapters reach only their vendor

`voice-receiver`, `voice-recognizer`, `voice-synthesizer` and `voice-sender`
SHALL be reachable only from the components wired to call them and SHALL be
allowed out only to each other, the analyzer and the manager as the flow
requires. A vendor adapter SHALL be allowed out to its owner and to its
vendor's API, and to nothing else; a stub adapter to its owner alone.

#### Scenario: Audio leaves for exactly one place
- **WHEN** `recognizer-google` runs
- **THEN** its only external destination is the Google Speech API, and a connection elsewhere is refused

#### Scenario: A surface cannot reach a vendor
- **WHEN** a surface adapter attempts a connection to a recognizer adapter
- **THEN** the network refuses it
