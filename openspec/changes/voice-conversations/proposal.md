# Proposal: voice-conversations

## Why

A person can start and continue a conversation by typing, tapping and posting
a signal, and by nothing else. Speech is the surface where a person is not at
a keyboard — a room, a headset, a phone — and today a voice note or a
microphone reaches nothing: both Telegram adapters drop a message with no
text, and the console has no microphone. Every contract carries text, so
speech must become text before agent-ops, and the answer must become speech
after it, with the addressing decided in between by `message-addressing`.

## What Changes

- **Four new voice components, each one responsibility, one contract, one
  dependency class**, in `platform/`:

  | component | owns | talks to | depends on |
  |---|---|---|---|
  | `voice-receiver` | `POST /audio` — audio from surfaces, clip or stream | the recognizer, then the analyzer; delivers decisions to the manager | nothing |
  | `voice-recognizer` | `/recognize` — a job queue recognizer ADAPTERS long-poll | its adapters | nothing |
  | `voice-synthesizer` | `/speak` — a job queue synthesizer ADAPTERS long-poll | its adapters | nothing |
  | `voice-sender` | `GET /audio/ops` — spoken output surfaces long-poll; the voice Channel's adapter | the synthesizer, the manager's channel ops | nothing |

- **Speech vendors are out-of-process adapters of the recognizer and
  synthesizer contracts**, polling, holding their own credential, exposing no
  port — so one may run on a GPU host outside the cluster. Day one:
  `recognizers/google`, `synthesizers/google` (Cloud Speech-to-Text and
  Text-to-Speech) and a `stub` of each for tests. `recognizers/whisper` and
  `synthesizers/piper` are named follow-ups the interface is designed
  against.
- **A voice SURFACE is a Channel plus a SignalSource** — served by
  `voice-sender` and `voice-receiver` — and a surface adapter that hears and/or
  speaks, posting to `/audio` and long-polling `/audio/ops`. The two halves are
  independent: a room microphone is in-only, a speaker out-only.
- **The console is the first surface**: a push-to-talk microphone and a
  speaker in the UI, proxied by the console adapter so the browser holds no
  token. `voices/telegram` (voice notes through the gateway's has-audio
  branch) is a named follow-up.
- **Language lives on the utterance**: detected by the recognizer (or hinted
  by the surface), carried to the analyzer, whose questions come back in it,
  and stamped on every spoken op so the synthesizer picks the voice. No
  translation stage; the agent answers in the language it was asked in.
- **Spoken rendering is the sender's presentation**: an `answer` is spoken as
  its title and body; `<details>` blocks and code are announced, not read;
  every spoken op also carries its text.
- **`repository-layout` clarified**: a directory is a CONTAINER; a container
  MAY hold several components each with its own contract. The four owners
  ship as four containers now and may be co-located later without moving.
- **Chart**: the four components, the two Google adapters and the two stubs,
  all under `voice.enabled` (default false), behind the wall; the console's
  voice proxy under `console.voice.enabled`.

## Capabilities

### New Capabilities

- `voice-surface-contract`: `/audio` in (clip and stream, hints, thread hint),
  `/audio/ops` out (audio + text + language, at-least-once), surface tokens,
  and that a surface implements either half.
- `speech-recognition-contract`: the recognizer owner's job queue and the
  adapter's poll/claim/result/stream protocol, capability declaration,
  language detection, tokens.
- `speech-synthesis-contract`: the synthesizer owner's job queue and adapter
  protocol, voices by language, formats, tokens.
- `voice-conversation-flow`: end to end — audio to utterance to decision to
  manager; questions spoken back; answers and cards spoken; the voice Channel
  and SignalSource; language on the utterance; what is spoken of a long answer.
- `console-voice-surface`: the console as a surface — push-to-talk, playback,
  the proxy, the transcript beside the spoken word.

### Modified Capabilities

- `repository-layout`: one directory is one container; components within it
  are packages with stated contracts.
- `component-network-isolation`: the four owners join the wall; the Google
  adapters are allowed out to their API and nothing else.
- `console-adapter`: the console holds a voice surface identity beside its
  channel and signal identities, and proxies audio both ways.

## Impact

**Depends on** `message-addressing` (the analyzer) and through it on
`coordinated-agents` (`mcp-aops`, `channel-reader`, `brief`).

**Code**

- `platform/voice-receiver/`, `platform/voice-recognizer/`,
  `platform/voice-synthesizer/`, `platform/voice-sender/`: new standard-library
  modules on the shared recipe.
- `recognizers/google/`, `recognizers/stub/`, `synthesizers/google/`,
  `synthesizers/stub/`: new groups, derived names `recognizer-google` etc.
- `platform/console/`: the proxy endpoints and the UI's microphone and
  playback; `ui/src` gains the push-to-talk control.
- `chart/`: workloads, NetworkPolicies, values, the rendered voice Channel and
  SignalSource under demo mode, NOTES.txt; `.github/components.sh` unchanged
  in code, eight more derived components.

**Documents made untrue — reference docs**

- `docs/contracts.md`: the surface, recognition and synthesis contracts, the
  spoken rendering of an op.
- `docs/concepts.md`: a voice surface as Channel + SignalSource; language on
  the utterance.
- `docs/console.md`, `docs/console-guide.md`: the microphone and playback,
  the proxy endpoints.
- `docs/security.md`: audio leaves the cluster to a vendor under
  `recognizers/google`; the vendor credential's projection; re-run
  `python3 docs/diagrams/threat-model.py`.
- `docs/installation.md`: `voice.*`, `console.voice.*`, the Google
  credential Secret.
- `docs/CHANGELOG.md`: the components.
- `.claude/rules/structure.md`, `adapters.md` (two more adapter kinds),
  `terminology.md` (surface, utterance, recognizer, synthesizer),
  `invariants.md` (no adapter exposes a port; the receiver delivers under the
  surface's identity).

**Documents made untrue — adopter site**

- `docs/index.md`: the claims strip and the "when it runs" list gain speech;
  the console recording shows the microphone.
- `docs/introduction.md`: a spoken message as a way in.
- `docs/getting-started.md`: unchanged unless demo mode wires the console
  surface — decided in design.
- `docs/installation.md`: the component list, the vendor credential.
- `docs/guides/talk-to-an-agent.md`: new guide; `_data/nav.yml` line.
- `README.md`: the seams and the "works with" line; stays ≤ 215 lines.
