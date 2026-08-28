## ADDED Requirements

### Requirement: The console holds a voice surface identity beside its others

Alongside its channel and signal identities the console MAY hold a
`voice-surface:console` identity, used only to post audio to the receiver
and long-poll the sender on the browser's behalf. It SHALL be present only
when the console's voice surface is enabled, and the console SHALL make no
other use of it.

#### Scenario: One more token, one purpose
- **WHEN** the voice surface is enabled
- **THEN** the console's calls to the voice components carry that token and no other, and its manager calls are unchanged
