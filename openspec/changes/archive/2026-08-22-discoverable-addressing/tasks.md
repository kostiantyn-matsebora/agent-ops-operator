## 1. Terminology and the addressed form

- [x] 1.1 Remove the `:<agent>` capture from `internal/addressing` and drop `Command.Agent` — and verify `addressing_test.go` asserts `/devops:node-doctor drain agent-2` parses as Pipeline `devops` with the whole remainder as the task
- [x] 1.2 Stop writing `InputItem.Agent` in `CreateTaskConversation`, keep `dispatch` reading it for one release, and mark the API field deprecated in `api/v1alpha1/conversation_types.go` — and verify a dispatch test asserts a pre-existing input with an agent still dispatches while a newly created one carries none
- [x] 1.3 Regenerate deepcopy and CRDs with `controller-gen` and verify `chart/files/crds/` reflects the deprecation and nothing else moved
- [x] 1.4 Rename the listing command to `/pipelines`, keep `/agents` accepted but never emitted, and add `pipelines` to the reserved set — and verify `router_test.go` covers both names returning the same listing and a Pipeline named `pipelines` being unreachable by command
- [x] 1.5 Rewrite the listing reply and the ambiguity refusal to say Pipeline throughout, dropping the `:<role>` line — and verify no user-visible manager string uses "agent" for a Pipeline

## 2. Manager: the vocabulary and its revision

- [x] 2.1 Add the vocabulary model in `internal/chat` — entry `{kind, name, description, position, profile}`, `kind` in `builtin|pipeline`, `position` in `general|thread` — and verify a unit test pins the built-ins with their positions (`pipelines`/`help` general, `exit`/`close` thread) and asserts `agents` is absent
- [x] 2.2 Build the vocabulary from Ready Pipelines, description derived from the answering profile, deterministically ordered — and verify a unit test asserts unready Pipelines are absent and ordering is stable
- [x] 2.3 Derive the revision as a hash over `(kind, name, description, position)` only — and verify a unit test mutates an unrelated Pipeline field and asserts the revision is unchanged, then flips Ready and asserts it changes
- [x] 2.4 Serve `GET /channel/vocabulary` returning `{revision, entries}` under existing adapter authentication — and verify a handler test covers an authenticated read and an unauthenticated refusal
- [x] 2.5 Carry the revision as a response header on `GET /channel/ops` for both the `200` and the `204` — and verify a handler test asserts the header on both and that the `204` still has no body
- [x] 2.6 Verify with an envtest case in `internal/integration` that a Pipeline becoming Ready changes the header a polling adapter observes, with no manager-initiated connection

## 3. Manager: choices and reply linkage

- [x] 3.1 Add `Choices []Choice{Label, Command}` to `chat.Message` as an optional structured field — and verify tests assert it is omitted when empty and that nothing under `internal/` renders it to text
- [x] 3.2 Add `InReplyTo` to `chat.Message` as an opaque handle — and verify a test asserts nothing under `internal/` parses, compares or constructs one, matching the `threadId` treatment
- [x] 3.3 Accept an opaque origin-message label on a chat signal in `internal/httpapi/signals.go` and thread it onto messages sent back to the originating surface — and verify a test asserts a refusal carries it and a signal without the label still sends
- [x] 3.4 Populate `Choices` on the ambiguity refusal and the Pipeline listing, keeping the prose — and verify existing refusal tests pass and new assertions cover one choice per serving Pipeline
- [x] 3.5 Verify `dispatch` and `ingest` fixtures are unchanged except where task 1.2 intends it — those tests are pinned deliberately, so any other diff is a decision, not a side effect

## 4. Telegram ingest: selections reach the right adapter

- [x] 4.1 Add the origin-message label in `signal-telegram` beside `agentops.dev/channel` and `agentops.dev/sender` — and verify `main_test.go` asserts it is set from the update and that the adapter still contacts Telegram nowhere
- [x] 4.2 Widen `allowed_updates` to include the selection update kind in `telegram-router` — and verify `telegram_test.go` asserts the request body carries both kinds
- [x] 4.3 Classify a selection by reading `is_topic_message` from the message it was attached to, reusing the existing binary rule — and verify `classifyUpdate` tests cover a general-surface selection routing to the signal target and an in-topic one routing to the channel target
- [x] 4.4 Acknowledge every selection from the router, content-free, before forwarding — and verify a test asserts exactly one acknowledgement per selection and that forwarding is still verbatim
- [x] 4.5 Normalize a general-surface selection into an addressed chat signal in `signal-telegram`, recovering the original text from the tapped message's reply linkage — and verify tests cover a successful reconstruction and an unrecoverable original

## 5. Telegram channel adapter: menu, spelling map, controls

- [x] 5.1 Fetch `/channel/vocabulary` on the existing refresh path and on an observed revision change — and verify tests cover the fetch and that an unchanged revision triggers none
- [x] 5.2 Implement the transport-local spelling in `channel-telegram` (`-` → `_`, injective because a Kubernetes name cannot contain `_`) and its reverse in `signal-telegram`, each independently and with no shared state — and verify unit tests cover a hyphenated name round-tripping both ways, and that a name Telegram cannot express is skipped rather than refused, reported or conditioned on
- [x] 5.3 Register built-ins and Pipelines with `setMyCommands` per `BotCommandScopeChat`, only when the adapted list differs from the last registered one — and verify a test asserts an inconsequential revision change produces zero Bot API registration calls
- [x] 5.4 Use the registered spelling wherever the adapter names a Pipeline to a person — and verify a test asserts the listing and the menu print the same string for one Pipeline
- [x] 5.5 Render `Message.Choices` as an inline keyboard on the message, falling back to text for any choice whose callback payload exceeds 64 bytes — and verify `render_test.go` covers both branches
- [x] 5.6 Send with `reply_to_message_id` when `Message.InReplyTo` is set — and verify a test asserts it is passed through opaquely and omitted when absent
- [x] 5.7 Handle a forwarded selection: resolve the Pipeline through the spelling map, recover the original message, post the addressed form — and verify tests cover the happy path and the expired-offer refusal that delivers nothing

## 6. Console: one source, two composers

- [x] 6.1 Fetch the vocabulary over the channel contract rather than deriving it from the Pipeline cache — and verify a BFF test asserts the fetch and that the Pipeline watch still serves only topology and config
- [x] 6.2 Reproject `/api/agents` onto the vocabulary and rename its payload for what it carries — and verify `agents_test.go` assertions hold and a new one asserts entries carry `position`
- [x] 6.3 Offer thread-position entries in the conversation reply composer with the same typeahead behaviour as `NewConversation` — and verify a `Conversation.test.tsx` case covers the prefix opening the list, narrowing, and insertion
- [x] 6.4 Present `/exit` and `/close` together with the difference stated, never one without the other — and verify a test asserts both appear whenever either does
- [x] 6.5 Exclude Pipeline entries from the thread composer — and verify a test asserts no Pipeline is offered there
- [x] 6.6 Render `choices` as buttons in the console transcript — and verify a test covers a message carrying choices
- [x] 6.7 Replace "agent" with "Pipeline" in every console string naming an addressable thing — and verify a test or grep asserts no user-visible console string calls a Pipeline an agent
- [x] 6.8 Re-run `npm run screenshots` in `console/ui` and verify the twelve site PNGs under `docs/assets/img/console/` match the changed UI

## 7. Documentation

- [x] 7.1 Update `docs/concepts.md` for the single-segment addressed form and the deprecated `InputItem.agent` — and verify no page still documents a per-message agent override
- [x] 7.2 Document the vocabulary endpoint, the revision header, `choices` and `inReplyTo` in `docs/contracts.md` — and verify the endpoint list, the message-kind table and the adapter obligations all reflect the additions
- [x] 7.3 Document the Telegram command menu and the transport-local spelling in `docs/telegram-bundle.md` — and verify the page states why the menu may complete a different string than the CR carries
- [x] 7.4 Update `docs/console-guide.md` for the reply-composer assistance — and verify it says what the composer offers and where, without restating reference detail owned by `docs/console.md`
- [x] 7.5 Add a `CHANGELOG.md` entry, newest first, covering the listing rename, the removed override and the deprecated field — and verify it names what an operator must do, which is nothing
- [x] 7.6 Update `CLAUDE.md`: the reserved command set gains `pipelines`, the `addressing/` map line loses `[:<agent>]`, and the terminology section states that a Pipeline is what a message addresses — and verify no line in the file still shows the two-segment form
- [x] 7.7 Run the adopter-prose lint from `CLAUDE.md` over `docs/*.md` and verify it is silent

## 8. Whole-change verification

- [x] 8.1 Build and vet the root module and all nine submodules per the `CLAUDE.md` container recipe, and verify every module passes
- [x] 8.2 Run the full suite with `KUBEBUILDER_ASSETS` and verify unit and envtest cases pass
- [x] 8.3 Verify backward compatibility end to end: run an adapter that ignores the header and the endpoint against the upgraded manager, assert its behaviour is unchanged and the contract version is still `2`
- [x] 8.4 Smoke the real thing on a cluster — a rendered pod is not a running one: confirm the `/` control appears in a Telegram chat and completes both a built-in and a hyphenated Pipeline, that the completed Pipeline opens a conversation recorded under its real name, and that selecting a control on an ambiguity refusal delivers the original message
