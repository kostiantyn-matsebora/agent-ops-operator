## Purpose
The analyzer turns an utterance whose addressing is in its words into one decision a surface adapter can deliver to the manager — or into a question back to the speaker when it cannot.

## ADDED Requirements

### Requirement: One utterance yields one decision, synchronously

The analyzer SHALL accept `POST /utterance` carrying `text`, `surface` (the
Channel the speaker is on), `speaker`, `lang`, an optional `threadHint` (a
thread on that surface the utterance arrived in) and the caller's reader
token, and SHALL answer within the request with exactly one decision:

| decision | carries |
|---|---|
| `originate` | the Pipeline or Coordinator name and the task text |
| `reply` | the conversation name, its thread on the surface, and the text |
| `command` | one manager command in its typed form (`/close`, `/exit`, `/pipelines`) and the thread it applies to |
| `ask` | a question in the speaker's language |
| `refuse` | a reason, when the utterance addresses nothing reachable from the surface |

The text of `originate` and `reply` SHALL be the speaker's instruction with the
addressing removed, in the language it was spoken, and SHALL NOT be rewritten
otherwise.

#### Scenario: A named pipeline resolves without a slash
- **WHEN** a speaker on a surface two Ready Pipelines serve says "ask k8s-ops to restart the nginx deployment"
- **THEN** the decision is `originate` naming `k8s-ops` with the task "restart the nginx deployment"

#### Scenario: A thread hint is a hint
- **WHEN** an utterance arrives with a `threadHint` and its words name a different conversation
- **THEN** the words decide, and the decision names the conversation the words meant

### Requirement: A conversation is resolved from briefs, never from transcripts

To resolve which conversation a speaker means, the analyzer SHALL read only
the `{name, title, brief, phase, pipeline}` projection of conversations bound
to the utterance's surface, obtained from the aops MCP server with the
CALLER-SUPPLIED `channel-reader:<channel>` token. The analyzer SHALL hold no
token of its own for the manager or the MCP server, and SHALL read no run,
input or transcript.

#### Scenario: The ingress one
- **WHEN** two open conversations on the surface have briefs about an ingress in `web` and a CronJob in `batch`, and the speaker says "on the ingress one, roll back the deploy"
- **THEN** the decision is `reply` naming the ingress conversation with the text "roll back the deploy"

#### Scenario: A stranger's conversation is invisible
- **WHEN** a conversation exists with no thread on the speaker's surface
- **THEN** it is absent from what the analyzer reads and can never be a decision

### Requirement: Ambiguity is a question, not a guess

When the utterance could mean more than one Pipeline or conversation, or names
neither, the analyzer SHALL answer `ask` with a question naming the
candidates in the speaker's language, and SHALL remember the pending intent
for that `(surface, speaker)` for a bounded time so the next utterance from
the same speaker on the same surface is read as the answer. The pending
intent SHALL expire unanswered and SHALL NOT survive a restart; an utterance
after expiry is read fresh.

#### Scenario: Two candidates, one question
- **WHEN** two open conversations both concern nginx and the speaker says "close the nginx one"
- **THEN** the decision is `ask`, naming both by title, and the speaker's next utterance "the ingress" yields `command` `/close` on that conversation

#### Scenario: Forgotten on purpose
- **WHEN** a question goes unanswered past the configured window
- **THEN** the next utterance from that speaker is analysed with no pending intent, and the earlier question is never answered late

### Requirement: The analyzer delivers nothing

The analyzer SHALL NOT call `/signal/inbound`, `/channel/inbound` or any
manager verb. The caller SHALL deliver the decision under its own adapter
identity, so the manager records a message from the surface's adapter exactly
as it would a typed one, and nothing of the question loop reaches the manager.

#### Scenario: The manager sees one message
- **WHEN** an utterance takes two questions to resolve into `originate`
- **THEN** the manager receives one chat signal from the surface's signal adapter, and no record of the questions exists on any Conversation

### Requirement: The language model is a replaceable backend

The analyzer SHALL reach its language model through one interface with one
implementation file per vendor, selected by configuration, with Ollama and an
OpenAI-compatible chat endpoint available on day one. No decision shape SHALL
depend on which backend answered.

#### Scenario: Swapping the model changes no contract
- **WHEN** the configured backend changes from Ollama to an OpenAI-compatible endpoint
- **THEN** every caller's requests and the decision shapes are unchanged
