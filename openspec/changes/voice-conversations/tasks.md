Every build and test below runs INSIDE the worktree (`docker exec -w "$PWD"`
from `../agent-ops-worktrees/voice-conversations`), and every deploy uses
`--state-values-set chartPath=` naming this worktree's `chart/` — the
defaults resolve master and report success against it. Depends on
`message-addressing` on master. Phases follow design D-I.

## 1. Phase 1 — receiver, sender, the surface contract (D-A, D-B, D-F, D-H)

- [ ] 1.1 `signals/voice-receiver/` and `channels/voice-sender/`: two new
      standard-library modules on the shared recipe; `.github/components.sh`
      derives `signal-voice-receiver` and `channel-voice-sender`; build and vet green.
- [ ] 1.2 Receiver `POST /audio`: clip, chunked and WebSocket forms; `text`
      short-circuit; `voice-surface:<name>` token check; 429 past `MAX_JOBS`;
      tests for each form and each refusal.
- [ ] 1.3 Receiver reads its served SignalSources (`surface` → source) from
      the manager listing; unknown surface refused; test with a fake manager.
- [ ] 1.4 Receiver → recognizer client (`POST /recognize`, blocking with
      deadline) behind an interface with an in-process fixed-text stub used by
      tests until phase 2.
- [ ] 1.5 Receiver → analyzer: posts the utterance with the surface's
      `channel-reader` token; delivers `originate` on `/signal/inbound`,
      `reply`/`command` on `/channel/inbound`, `ask` to the sender, `refuse`
      as a `notice`; test asserts one manager call per decision and none for
      `ask`.
- [ ] 1.6 Sender: ChannelAdapter half — long-polls `/channel/ops`, serves
      `GET /audio/ops` per surface adapter at-least-once with stable ids and
      completion; `ensure-topic` answers the conversation name as thread id;
      tests for redelivery order and completion.
- [ ] 1.7 Sender `POST /say` (the receiver's questions) and
      `POST /sender/heard` (language per thread); `lastLang` fallback to
      `config.lang`; tests.
- [ ] 1.8 Sender spoken rendering (D-G): port `channels/telegram/blocks.go`,
      then the reading rules — title, prose, announced details/code, five
      table rows; golden tests against the `structured-agent-output`
      fixtures; `text` on the op is untouched.
- [ ] 1.9 Sender → synthesizer client behind an interface with an in-process
      silence stub; the op carries audio, text, lang, kind, conversation.

## 2. Phase 2 — recognizer, synthesizer, stubs (D-A, D-B, D-C)

- [ ] 2.1 `platform/voice-recognizer/`: `POST /recognize` (owner-initiated
      enqueue, blocks with deadline), `GET /recognize/jobs` long-poll with
      capability declaration, `POST …/result`, `WS …/stream`; claim window
      and re-offer; routing by streams/detects/languages; tests for each
      routing rule and the claim expiry.
- [ ] 2.2 `platform/voice-synthesizer/`: the same shape for `/speak`; routing
      by language and format; tests.
- [ ] 2.3 `recognizers/stub/` and `synthesizers/stub/`: polling adapters
      returning a fixed transcript (from env) and a silent clip; derived
      names `recognizer-stub`, `synthesizer-stub`; an integration test runs
      owner + stub in-process end to end.
- [ ] 2.4 Receiver and sender switch from their in-process stubs to the
      owners; the phase-1 tests pass unchanged against a fake owner.

## 3. Phase 3 — Google adapters (D-D)

- [ ] 3.1 `recognizers/google/`: streaming recognize over WebSocket-fed
      audio, language candidates, detection; OAuth from a projected
      service-account key with the standard library; recorded-response tests
      and a skippable live test gated on the credential being present.
- [ ] 3.2 `synthesizers/google/`: voice map per language, OGG/Opus and
      LINEAR16; same test shape.
- [ ] 3.3 Live check from the worktree: speak three short utterances in two
      languages through the stub surface (curl a clip); record detected
      language and round-trip time as verdicts; pick the default voice map.

## 4. Phase 4 — console surface (D-E)

- [ ] 4.1 Console adapter: `voice-surface:console` token, the `/audio/ops`
      poll goroutine, spoken ops as a new live-stream event; `POST
      /voice/utterance` WebSocket proxy to the receiver with surface, speaker
      and threadHint; all under `console.voice.enabled`; tests with a fake
      receiver and sender.
- [ ] 4.2 UI: push-to-talk control on the conversation and origination
      views, MediaRecorder WebM/Opus, level meter, question/answer text,
      playback; microphone denied leaves the rest working; unit tests for
      the state machine.
- [ ] 4.3 Playwright end to end against the dev server with a fake media
      stream feeding a `.wav` and the stub adapters: hold, speak, see the
      transcript, hear (assert the event) the reply.
- [ ] 4.4 Screenshot the control per `visual-check.md` into the scratchpad
      and READ the PNG before ticking.

## 5. Phase 5 — chart, wall, rules

- [ ] 5.1 `chart/`: Deployments and Services for the four owners, Deployments
      for the four adapters, `voice.*` values (owner flags, adapter selection,
      Google credential Secret ref, claim window, max jobs), NetworkPolicies
      per `component-network-isolation`, demo-mode surface pair and the
      Pipeline listing; `helm template` with every combination;
      `serviceaccount-guard.py` passes.
- [ ] 5.2 Derived tokens rendered for `voice-surface:console`,
      `recognizer-adapter:*`, `synthesizer-adapter:*`, `channel-reader:` for
      the console surface; render tests.
- [ ] 5.3 Deploy from the worktree chart with stubs; speak from the console;
      a conversation opens on the demo Pipeline; record the verdict.
- [ ] 5.4 `.claude/rules/structure.md` (four owners, two adapter groups, the
      container/component sentence), `adapters.md` (recognizer and
      synthesizer adapter kinds, surface adapters), `terminology.md`,
      `invariants.md` (no adapter listens; speech is invisible to the
      manager); `gotchas.md` if phase 3 or 4 paid for one.
- [ ] 5.5 `publication-guard.py` and `retired-vocabulary-guard.py` pass;
      verdict only.

## 6. Documentation — THE LAST TASK, and it is not optional

### 6a. Reference docs

- [ ] 6a.1 `docs/contracts.md`: surface, recognition and synthesis contracts;
      spoken rendering; the token contexts.
- [ ] 6a.2 `docs/concepts.md`: a voice surface as Channel + SignalSource;
      language on the utterance.
- [ ] 6a.3 `docs/console.md`, `docs/console-guide.md`: the microphone, the
      proxy endpoints, the values.
- [ ] 6a.4 `docs/security.md`: audio egress under the Google adapters, the
      credential projection; re-run `python3 docs/diagrams/threat-model.py`.
- [ ] 6a.5 `docs/installation.md`: `voice.*`, `console.voice.*`, the
      credential Secret with a placeholder from the allowlist;
      `docs/CHANGELOG.md`.
- [ ] 6a.6 Re-run `python3 .github/scripts/docs-generate.py`; commit every
      regenerated block; `--check` passes.

### 6b. Adopter site

- [ ] 6b.1 `docs/index.md`: the claims strip and "when it runs" mention
      speech.
- [ ] 6b.2 `docs/introduction.md`: a spoken message as a way in.
- [ ] 6b.3 `docs/installation.md`: the component list and the credential.
- [ ] 6b.4 `docs/guides/talk-to-an-agent.md`: new guide with generated CR
      blocks for the surface pair; `_data/nav.yml` line.
- [ ] 6b.5 `README.md`: seams and "works with"; `wc -l README.md` ≤ 215.
- [ ] 6b.6 `platform/console/ui`: re-run BOTH `npm run screenshots` and
      `npm run demo` with the control visible; commit the assets.
