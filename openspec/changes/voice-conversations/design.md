# Design: voice-conversations

## Context

- Both Telegram adapters drop a message with no `text`
  (`signals/telegram/main.go:304`, `channels/telegram/main.go:537`); the
  console has no audio path. Every manager contract is text.
- `message-addressing` supplies the analyzer: `POST /utterance` → one
  decision, credential-free, reading briefs through the caller's
  `channel-reader` token.
- The channel adapter contract is the shape to copy: long-poll out,
  at-least-once with stable op ids, push in, per-adapter derived tokens,
  no port on the adapter. The console adapter already proxies a browser and
  holds a channel and a signal identity.
- The reference VM runs Ollama CPU-only; there is no headroom for a local STT
  or TTS model beside it. Google Cloud Speech is streaming, detects
  language, and speaks 50+ languages.

## Goals / Non-Goals

**Goals:**
- Four components with one responsibility, one contract and one dependency
  class each, testable end to end with stubs and no vendor.
- Vendors and surfaces both out-of-process, both polling, both exposing
  nothing.
- The console loop feels live: audio streams while spoken, a question comes
  back in seconds.

**Non-Goals:**
- Telegram voice notes (`voices/telegram`) — follow-up, the gateway branch is
  designed here and not built.
- Local vendors (`recognizers/whisper`, `synthesizers/piper`) — follow-ups;
  the contract is designed against them.
- Translation. Wake words. Speaker identification beyond what the surface
  reports.
- Co-locating the four owners in one image (allowed by `repository-layout`,
  not done).

## Decisions

### D-A — Four owners, four directories, four containers, poll everywhere

- `platform/voice-receiver`, `platform/voice-recognizer`,
  `platform/voice-synthesizer`, `platform/voice-sender`: standard-library
  Go, shared recipe, derived names `voice-*`.
- Every edge between a vendor or surface and an owner is initiated by the
  vendor or surface; owners are the only listeners. Alternative rejected:
  owners pushing to adapters' Services — a port per adapter and an ingress
  the day one runs outside the cluster.
- The two internal owner→owner edges (receiver→recognizer,
  sender→synthesizer) are owner-initiated HTTP: `POST /recognize` and
  `POST /speak` enqueue a job and block on its result with a deadline. They
  are in-cluster and could become in-process calls under co-location.

### D-B — Job queues are in-memory, at-least-once, bounded

- A job is `{id, payload, claimedBy, claimedAt, deadline}`; a claim expires
  after `CLAIM_WINDOW` (default 30 s) and the job is re-offered; a result
  for an unclaimed or completed id is refused. Queue depth is bounded
  (`MAX_JOBS`, default 100); beyond it `/audio` answers 429.
- Lossy on restart on purpose: an utterance mid-recognition when the
  recognizer restarts is gone, and the surface hears nothing. Speech is
  ephemeral; a person repeats themselves. Nothing here is state the manager
  would want (`state-durability`: declared lossy, and the gap is reported to
  the surface as a `notice` op where the receiver can still name the
  utterance).

### D-C — Streaming is a WebSocket the adapter opens

- A streaming job's claim carries the id; the adapter opens
  `WS /recognize/jobs/{id}/stream`; the owner pipes the surface's stream in
  and forwards partials nowhere (the analyzer needs the final text) but
  keeps the socket so a long utterance is not buffered whole. The console
  surface streams the same way into `/audio`.
- Alternative rejected: chunked HTTP in both directions — no partial
  transcripts back, and the console's proxy would buffer.

### D-D — Google adapters, one file knows the API

- `recognizers/google`: Cloud Speech-to-Text v2 streaming recognize, model
  `chirp_2` where available, `languageCodes` from the hint list, `auto` when
  none. `synthesizers/google`: Text-to-Speech, voice chosen per `lang` from a
  small preference map (Neural2 first, Standard fallback), formats OGG/Opus
  and LINEAR16.
- Credential: a Secret holding a service-account key, projected as a file
  and `GOOGLE_APPLICATION_CREDENTIALS`; the adapter mints its own OAuth
  token from it with the standard library (JWT bearer grant) — no SDK, zero
  requires, per `structure.md`.
- Egress: the adapter's NetworkPolicy allows `oauth2.googleapis.com`,
  `speech.googleapis.com` / `texttospeech.googleapis.com` by FQDN where the
  CNI supports it, else the documented address ranges.

### D-E — The console is a surface adapter inside the console adapter

- New handlers: `POST /voice/utterance` (browser → adapter; WebSocket,
  proxied to the receiver's stream with `surface: console`, `speaker` = the
  console's user key, `threadHint` = the open conversation) and spoken ops
  forwarded on the existing live stream as a new event kind; audio bytes
  base64 in the event, played via `<audio>`. The adapter long-polls
  `/audio/ops` as a goroutine beside its channel ops loop.
- UI: a push-to-talk button (`pointerdown`/`pointerup`, space bar held), a
  level meter, the question/answer text beside it; `MediaRecorder` at
  `audio/webm;codecs=opus` 16 kHz mono. Google accepts WebM/Opus directly.
- Prototype: none. The control is one button; composition is settled in
  code and by the screenshot in tasks 5.4.

### D-F — Language rides the op

- Receiver: `lang` hint from the surface → recognizer candidates → detected
  `lang` on the utterance → analyzer. Sender: keeps `lastLang[(surface,
  thread)]` in memory from utterances the receiver reports to it
  (`POST /sender/heard {surface, thread, lang}`), stamps it on every op for
  that thread, falls back to the surface's `config.lang`. Lossy on restart:
  the fallback is a wrong voice once.

### D-G — Spoken rendering lives in the sender

- The sender parses the block grammar (a third parser beside
  `channels/telegram/blocks.go` and `blocks.ts`, written against the same
  capability spec): speaks `<title>` then prose paragraphs; `<details>` and
  fenced code are replaced by one spoken sentence naming them; tables are
  read row by row up to five rows then announced. The op's `text` carries
  the whole body untouched.
- Alternative rejected: asking the analyzer or an LLM to condense. A second
  model turn per answer, and a paraphrase of what the agent said.

### D-H — Surfaces as CRs: a Channel and a SignalSource, nothing new

- `Channel{adapter: voice-sender, config: {surface, lang, format}}`,
  `SignalSource{adapter: voice-receiver, config: {surface, lang}}`. The
  owners read their served CRs through the adapter listing endpoints as every
  adapter does, keyed by `surface`. Under `global.demo.enabled` the chart
  renders the `console` surface pair and lists the channel on the demo
  Pipeline.
- Alternative rejected: a `VoiceSurface` CRD. It would carry what two existing
  kinds already carry, and its reconciler would decide nothing.

### D-I — Delivery is phased, one PR, each phase green

| Phase | Ships | Green because |
|---|---|---|
| 1 | receiver, sender, the surface contract, stubs for recognition and synthesis in-process | nothing enabled by default |
| 2 | recognizer + synthesizer owners, `recognizers/stub`, `synthesizers/stub` | stubs only |
| 3 | `recognizers/google`, `synthesizers/google` | opt-in credential |
| 4 | console surface | `console.voice.enabled` false |
| 5 | chart demo wiring, rules | render tests |
| 6 | docs, screenshots, recording | last |

## Risks / Trade-offs

- **Audio leaves the cluster** under the Google adapters → stated on the
  security page; the stub and the named local follow-ups are the answer for
  installs that cannot.
- **Latency budget**: capture → stream STT (~0.5 s after release) → analyzer
  (1–3 s CPU) → manager → agent (seconds to minutes) → TTS (~0.3 s). The
  analyzer's question loop is the only part a person waits on in silence;
  the sender speaks a short "working on it" `notice` when an origination is
  delivered, so the loop closes audibly.
- **Three block-grammar parsers** → the capability spec is the reference;
  the sender's is a port of `blocks.go`.
- **Eight new derived components** → CI matrix cost; all standard-library,
  seconds each.

## Migration Plan

1. `coordinated-agents` and `message-addressing` on master and released.
2. `helm upgrade` with `voice.enabled: true`, the Google credential Secret
   (or `voice.recognizer.adapter: stub` / `voice.synthesizer.adapter: stub`),
   `console.voice.enabled: true`; the surface pair rendered by demo mode or
   written by hand and listed on a Pipeline.

Rollback: disable the flags; no CRD, no manager state.

## Open Questions

- Whether the sender's "working on it" notice should be configurable per
  surface. Default on.
- The Google voice preference map's initial languages — the reference
  install's, measured in 3.3.
