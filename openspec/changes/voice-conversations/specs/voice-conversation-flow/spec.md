## Purpose
How a spoken utterance starts or continues a conversation and how the conversation speaks back — the chain from audio to the manager's contracts and out again, with the manager unaware of speech.

## ADDED Requirements

### Requirement: A voice surface is a Channel and a SignalSource

A voice surface SHALL be declared as one `Channel` served by `voice-sender`
and one `SignalSource` served by `voice-receiver`, both naming the surface in
their `config`. A Pipeline claiming the source and listing the channel is what
makes an agent reachable by voice and its conversations addressable by it;
without the listing, no conversation is in the analyzer's reach for that
surface.

#### Scenario: Wiring makes it speakable
- **WHEN** a Pipeline claims surface `desk`'s source and lists its channel
- **THEN** an utterance on `desk` may originate a conversation on that Pipeline, and its conversations may be replied to by voice

### Requirement: The receiver delivers the analyzer's decision under the surface's identity

The receiver SHALL post each utterance's transcript, speaker, surface,
language and thread hint to the analyzer with the surface's
`channel-reader` token, and SHALL deliver the decision itself: `originate` as
a `kind: chat` signal on the surface's SignalSource, `reply` and `command` as
`/channel/inbound` on the surface's Channel. The manager SHALL see one
message, from the surface's adapters, and nothing of recognition or of the
question loop.

#### Scenario: A spoken origination
- **WHEN** a speaker says "ask k8s-ops to restart nginx"
- **THEN** the manager receives one chat signal on the surface's source with the text "restart nginx" addressed to `k8s-ops`, and a conversation opens

#### Scenario: A spoken reply
- **WHEN** a speaker says "on the ingress one, roll back the deploy"
- **THEN** the manager receives one `/channel/inbound` on that conversation's thread with "roll back the deploy"

### Requirement: The analyzer's questions are spoken

An `ask` decision SHALL be handed to the sender as a `question` op for the
speaker's surface, in the utterance's language, and SHALL reach the manager
at no point.

#### Scenario: Which one
- **WHEN** the analyzer cannot choose between two conversations
- **THEN** the surface hears the question naming both, and the speaker's next utterance is read as the answer

### Requirement: Answers and cards are spoken as presentation

The sender SHALL render the manager's channel ops to speech: an `answer` as
its `<title>` then its body; a `notice` as its text; a `signal` card as its
title and source. `<details>` blocks and fenced code SHALL NOT be read aloud
and SHALL be announced as present, with the text of the op still carrying
them in full. The language of a spoken op SHALL be the language of the last
utterance on that thread from that surface, or the surface's configured
default when there is none.

#### Scenario: A long answer is not read for a minute
- **WHEN** an answer carries a title, two paragraphs and a `<details>` block
- **THEN** the surface hears the title and the paragraphs, then that details are available on screen

#### Scenario: The person's language is kept
- **WHEN** the last utterance on a thread was in Ukrainian
- **THEN** the answer is spoken in a Ukrainian voice, whatever language the agent's text is in

### Requirement: Speech is invisible to the manager

No manager contract, CRD field or reconciler SHALL change for speech. The
transcript recorded on the conversation SHALL be the text delivered, marked by
nothing as spoken.

#### Scenario: The record is text
- **WHEN** a spoken reply is recorded in `status.runs[].inputs[]`
- **THEN** it is indistinguishable from a typed one
