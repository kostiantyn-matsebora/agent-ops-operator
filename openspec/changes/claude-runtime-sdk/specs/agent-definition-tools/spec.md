## MODIFIED Requirements

### Requirement: An empty allowlist denies rather than defaulting or hanging
The runtime SHALL NOT substitute a tool the operator did not declare. When composition yields nothing, the agent SHALL have no tools, and a headless run SHALL never fall back to an interactive permission prompt, because a prompt no one can answer hangs the run until its idle timeout rather than reporting anything. An agent with no declared capabilities SHALL therefore start, find it can do nothing, and say so.

HOW the allowlist is enforced is the runtime's choice, and the rules bind every form of it. Passing the composed list to the vendor with a mode that denies unlisted tools satisfies this; so does deciding each invocation against the composed list in a permission callback, provided the callback is reached for EVERY call it is meant to govern and answers every one of them. A runtime taking the callback form SHALL NOT also pre-approve those tools by another mechanism — a pre-approval that resolves a call before the callback sees it makes the callback's rules unenforced while appearing to enforce them — and SHALL NOT select a mode that skips the callback, since a rule set that is never consulted denies for the wrong reason.

#### Scenario: Nothing declared grants nothing
- **WHEN** neither the agent definition nor the conversation's wiring declares any tools
- **THEN** the agent has no tools — no substituted default

#### Scenario: An empty allowlist does not hang the pod
- **WHEN** a work unit dispatches with an empty allowlist
- **THEN** the run completes and reports, rather than blocking on a permission prompt no one can answer

#### Scenario: A per-invocation decision satisfies the same rules
- **WHEN** a runtime enforces the composed allowlist by deciding each tool invocation rather than by passing a list
- **THEN** an unlisted tool is denied, a denial is answered immediately rather than prompting, and an empty composition denies everything

#### Scenario: No mechanism may shadow the decision point
- **WHEN** a runtime decides invocations in a callback
- **THEN** it does not additionally pre-approve those tools elsewhere, and does not run in a mode that resolves calls without consulting the callback
