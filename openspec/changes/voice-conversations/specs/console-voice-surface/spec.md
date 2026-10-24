## Purpose
The console is the first voice surface: a push-to-talk microphone and playback in the UI, proxied by the console adapter so the browser holds no token.

## ADDED Requirements

### Requirement: Push-to-talk on a conversation and on the start view

The console UI SHALL offer a push-to-talk control on a conversation view and
on the origination view. While held it SHALL stream captured audio to the
console adapter; on release the utterance ends. On a conversation view the
utterance SHALL carry that conversation's thread as `threadHint`.

#### Scenario: Speaking into a conversation
- **WHEN** a person holds the control on a conversation and speaks
- **THEN** audio streams as captured, and the spoken reply appears in the transcript as text when the manager records it

### Requirement: The adapter proxies both halves and holds the tokens

The console adapter SHALL post audio to the receiver and long-poll the sender
under a `voice-surface:console` token, and SHALL forward spoken ops to the
browser over its existing live stream, where they are played and their text
shown. The browser SHALL hold no token for any voice component.

#### Scenario: A spoken question is heard and seen
- **WHEN** the analyzer asks which conversation was meant
- **THEN** the browser plays the question and shows its text beside the control

### Requirement: Off by default, and a page without a microphone still works

The console's voice surface SHALL render only under `console.voice.enabled`,
and a browser denying microphone access SHALL keep every other console
function.

#### Scenario: Disabled
- **WHEN** `console.voice.enabled` is false
- **THEN** no control renders and no proxy endpoint is served
