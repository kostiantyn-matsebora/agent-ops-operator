# console-origination Specification

## Purpose

How the console STARTS a conversation: by emitting a `kind: chat` signal to
`/signal/inbound` for a SignalSource it serves, exactly as any other chat surface
does. It names no pipeline, no profile and no toolset — which Pipeline answers is
decided by the claim on that source.

The originating channel is folded into the new conversation's bindings, so a
console-started conversation is immediately joined and repliable without editing
any Pipeline's `channelRefs`.
## Requirements
### Requirement: The console originates conversations from a claimed SignalSource
The console SHALL start conversations by emitting a `kind: chat` signal to `POST /signal/inbound` naming a SignalSource it serves, authenticated with its signal-adapter identity. It SHALL NOT use `POST /task` and SHALL NOT create Conversation objects directly.

Each signal SHALL carry `agentops.dev/channel` set to the console's Channel and `agentops.dev/sender` set to the resolved user identity. Which agent answers SHALL be determined solely by the Pipeline that claimed the source — the console SHALL NOT name a pipeline, profile, or set of capabilities in the request.

#### Scenario: Starting work goes through the sanctioned lane
- **WHEN** a user submits a task against a wired console source
- **THEN** a `chat` signal is posted to `/signal/inbound` for that source, and the Pipeline claiming it creates the Conversation with the profile and tooling that Pipeline declares

#### Scenario: The console cannot choose the agent
- **WHEN** a request attempts to name a pipeline, profile or toolset
- **THEN** the field is ignored or rejected, and routing remains whatever the claiming Pipeline declares

#### Scenario: Conversation objects are never created directly
- **WHEN** the console originates any number of conversations
- **THEN** every resulting Conversation was created by the manager's ingest path, and the console has made no write to the Kubernetes API

### Requirement: Console-started conversations are joined without pipeline edits
Because the originating channel is appended to the conversation's channels, a conversation started from the console SHALL already carry a console thread binding, so its transcript is live and its composer usable without adding the console Channel to the Pipeline's `channelRefs[]`.

#### Scenario: Start then chat, with no wiring change
- **WHEN** a user starts a conversation from the console against a pipeline that does not list the console channel
- **THEN** the resulting conversation has a console thread binding, and the user can reply in it immediately

#### Scenario: Observing others' work still requires joining
- **WHEN** a conversation was started by a different signal source on a pipeline that does not list the console channel
- **THEN** it is fully visible but has no composer, and the UI states why and shows the patch that would join it

### Requirement: What can be started is what is wired
The console SHALL offer as origination targets exactly those console SignalSources reporting `Wired=True`, each labeled with the Pipelines serving it and — when exactly one does — that Pipeline's profile. Sources no Ready Pipeline serves SHALL be shown as unavailable with their `Wired=False` reason, not hidden.

A SignalSource is SHAREABLE, so several Pipelines MAY serve one console source. Where they do, an unaddressed task is refused rather than sent to an arbitrary one, and the console SHALL say so before the task is typed. Reaching a specific agent on such a source is ADDRESSING it by name, and the composer SHALL offer the addressable Pipelines rather than requiring the name be recalled (see `chat-addressing-discovery`). Separate console SignalSources remain a valid way to express separate destinations, but are no longer the only one.

#### Scenario: No wiring, honest empty state
- **WHEN** no Pipeline claims any console SignalSource
- **THEN** origination is unavailable, and the UI states that no pipeline claims the console source and shows the patch that would claim it — rather than offering a control that fails

#### Scenario: Several destinations
- **WHEN** two console SignalSources are served by two different Pipelines
- **THEN** both appear as targets, each labeled with its serving pipeline and profile

#### Scenario: One source, several answerers
- **WHEN** two Ready Pipelines serve one console SignalSource
- **THEN** the target names both, offers no single profile, and states that a
  task must address one of them

#### Scenario: Origination is refused when unclaimed
- **WHEN** a start request names a source no Pipeline claims
- **THEN** it is refused with the source's `Wired=False` reason, and no Conversation is created

### Requirement: Origination is a write, gated and attributed
Origination SHALL be available only when writes are enabled and the caller is authenticated, and every origination SHALL be logged with the resolved identity and the target source.

#### Scenario: Writes disabled
- **WHEN** `console.write.enabled` is false
- **THEN** the origination endpoint is rejected server-side and the UI does not present the action

#### Scenario: Attribution is recorded
- **WHEN** an authenticated user starts a conversation
- **THEN** the console logs who started it and against which source, and the signal carries that identity in `agentops.dev/sender`

