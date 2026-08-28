## ADDED Requirements

### Requirement: The definition's body is adopted by the runtime, and no prompt names the file
When a work unit names an agent and the runtime finds that agent's definition at the path its vendor defines, the runtime SHALL read the definition's body — everything below the frontmatter — and append it to the system message it controls, before any inline `systemPrompt` the unit carries. It SHALL do so on every run, a resumed one included, because a system-message addition is not part of the session's stored state on every vendor.

The manager's lane prompts SHALL name no definition file, no directory and no vendor convention. They MAY state a fallback posture for an agent with no definition, and SHALL NOT instruct the model to look for a file or to report one missing.

A work unit naming NO agent SHALL cause no lookup, render no path-shaped text, and produce no mention of a definition. A work unit naming an agent whose definition is absent SHALL be logged once by the runtime and SHALL contribute nothing, exactly as its frontmatter already does.

#### Scenario: A named definition's role reaches the model without being asked for
- **WHEN** a profile names `k8s-engineer` and its checkout holds that definition with a body
- **THEN** the body is in the system message of every run, and the prompt the manager rendered contains no file path

#### Scenario: A resumed run has the role again
- **WHEN** a conversation on any runtime is continued
- **THEN** the definition's body is supplied to that run as it was to the first

#### Scenario: An unnamed agent is silent
- **WHEN** a profile names no agent
- **THEN** no runtime looks for a definition, the prompt contains no `agents/` text, and the answer says nothing about a role file

#### Scenario: A vendor's path is the runtime's alone
- **WHEN** the same profile executes on the claude and on the copilot runtime
- **THEN** each reads its own vendor's file, and neither the prompt nor the transcript names the other's path
