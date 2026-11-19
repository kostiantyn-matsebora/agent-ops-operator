## MODIFIED Requirements

### Requirement: Agent session persistence is the shipped default
The chart SHALL provision the runtime context volume by default, so that an
agent's accumulated context survives a runtime pod restart out of the box. An
operator SHALL be able to opt out explicitly for clusters without a suitable
provisioner, and the opt-out SHALL be documented alongside the symptom it
avoids.

The volume SHALL be named for what it holds. It holds a conversation's
accumulated context — the thing `runtimeContextId` is a handle into and
`contextStorage` promises continuity on — and SHALL therefore be called the
CONTEXT volume everywhere it is named. The retired name said where it was
mounted, not what it holds. That it happens to be the runtime process's `$HOME`
is a property of one runtime image, not what the volume is for.

The MOUNT PATH SHALL move with the name, to `/data/context`. An earlier reading
held it load-bearing because the reference runtime resolves `${HOME}/.claude/`
off it. Checked against a live volume, it is not: a claim's contents appear AT
the mount path, so the same volume mounted elsewhere shows the same bytes, and
the stored transcript directory is named for the WORKING DIRECTORY rather than
for `$HOME`.

The workspace path moves to `/workspace` in the same release that retires
`/data/workspace`. The stored transcript directory is named for the working
directory, so the move strands every context stored under the old path. No
migration is provided, and an operator SHALL delete conversations before
upgrading. The path SHALL then stay at `/workspace`.

Turning persistence off SHALL NOT be silent at the moment it costs something. A
run whose context is gone SHALL FAIL with an explicit reason rather than
answering without context, and that reason SHALL reach both the thread, for the
person waiting on it, AND the `Conversation`, for anyone asking later why the
agent does not remember. A warning that scrolls past in a chat surface is not a
record, and the runtime pod that emitted it has usually exited by the time the
question is asked.

#### Scenario: Fresh install persists context
- **WHEN** the chart is installed with no persistence values supplied
- **THEN** a claim is provisioned and it reaches every conversation whose route binds none, with no runtime-side and no pipeline-side value set

#### Scenario: Cluster without a suitable provisioner
- **WHEN** an operator disables context persistence
- **THEN** the install succeeds, runtime pods use ephemeral storage, and the documentation states that context dies with each pod

#### Scenario: Lost session is explained, not hidden
- **WHEN** a run is asked to continue a context whose stored state no longer exists
- **THEN** the run fails with an explicit reason instead of answering without context, the thread is told what happened and that a new conversation is the remedy, and the conversation records that it can no longer be continued

#### Scenario: The mount path moves and stored context still resolves
- **WHEN** a runtime pod is built after the rename
- **THEN** the context volume is mounted at the context path, `$HOME` names it, and a conversation created before the rename resumes its context because nothing inside the volume moved

#### Scenario: The checkout path is untouched
- **WHEN** a runtime pod is rebuilt, or a later release ships, after the one move to `/workspace`
- **THEN** the workspace is at `/workspace` as before, because the stored transcript directory is named for it

#### Scenario: The workspace path is the new one
- **WHEN** a runtime pod is built after this change
- **THEN** the workspace is mounted at `/workspace` and nothing is mounted at `/data/workspace`

#### Scenario: A conversation from before the move does not resume
- **WHEN** a conversation created before the move continues its context after the upgrade
- **THEN** its stored transcript is not found under the new working directory and the run fails with the lost-context reason

### Requirement: The workspace volume is declared on the wiring, not the runtime
The workspace volume SHALL be declared where the context volume is — on the
`Pipeline` — and `AgentRuntime` SHALL declare neither. When a route binds one,
its runtime pods SHALL mount `/workspace` from that claim, one
subdirectory per conversation. When none resolves, the workspace SHALL
remain ephemeral.

The workspace path SHALL stay `/workspace` — claude-code keys its stored
transcripts by working directory, and relocating it breaks resume. When the
workspace survives a restart, the init container leaves a completed clone
untouched, as `workspace-sync` specifies.

#### Scenario: Workspace claim is mounted
- **WHEN** a route resolves a workspace claim
- **THEN** runtime pods mount `/workspace` from that claim

#### Scenario: Absent declaration stays ephemeral
- **WHEN** neither the route nor the release supplies a workspace volume
- **THEN** runtime pods use ephemeral storage for the checkout, as before

