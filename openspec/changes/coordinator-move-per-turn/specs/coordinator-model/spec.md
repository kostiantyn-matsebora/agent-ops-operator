## ADDED Requirements

### Requirement: A root's work unit is framed as coordination

The work unit the manager hands a runtime for a conversation carrying
`coordinatorRef` SHALL frame the run as coordination, not as a repository
agent's task.

The framing SHALL carry:

| Part | Content |
|---|---|
| the input | the signal, the person's message, or the member's result, verbatim |
| who it came from | a claimed source, a person on a channel, a member by its `agents[]` entry name, or the manager |
| the delivery section | the same output-format section every work unit carries |

The framing SHALL NOT carry:

- a repository checkout to read
- an agent definition file to adopt
- a fallback role to act in "within your tools"

The AgentProfile's `systemPrompt` SHALL still be appended, unchanged. That
text, and the chart's `systemPromptPrefix` that feeds it, remain the
adopter's surface for instructions, guardrails and install specifics.

A conversation carrying `causedBy` and no `coordinatorRef` SHALL keep the
work unit it has today. A member is a domain agent, and a domain agent may
have a repository.

#### Scenario: A signal opens a root
- **WHEN** a signal opens a Coordinator's root and the first run is dispatched
- **THEN** the prompt names the signal and its source, says nothing of a checkout or a role file, and carries the profile's `systemPrompt`

#### Scenario: A member's result reaches the root
- **WHEN** a member's result is dispatched to its parent root
- **THEN** the prompt names the result and the `agents[]` entry it came from

#### Scenario: A person addresses the root
- **WHEN** a person's message on a bound channel is dispatched to a root
- **THEN** the prompt names the message and the channel it came from

#### Scenario: A member keeps its own framing
- **WHEN** a member with a repository is dispatched its task
- **THEN** its prompt is the task framing it receives today, checkout and role file included
