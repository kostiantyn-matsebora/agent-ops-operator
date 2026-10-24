## Context

`AgentProfile.spec.agent` names a definition inside the profile's repository.
Two halves of that file matter: the `tools:` frontmatter, which every runtime
already reads and composes with the wiring's toolsets (`agent-definition-tools`),
and the body — the role prose. Today the body reaches the model only because
the manager's lane prompt asks the model to go and read
`.claude/agents/{{AGENT_NAME}}.md`. The copilot runtime, whose vendor keeps
definitions at `.github/agents/<name>.agent.md`, exposed the sentence as a
claude-ism: Copilot globbed for the wrong path on every run and titled its
answers with the miss, and a profile naming no agent rendered
`.claude/agents/.md`.

Constraints: the manager holds no checkout and reads no repository; the
composition of `tools:` stays in the runtime; `--agent` is never passed to
claude-code (it re-applies the definition as an availability intersection);
prompts carry no transport or vendor dialect.

## Goals / Non-Goals

**Goals:**
- The role reaches the model deterministically, from the component that holds
  the file, on every run including resumes.
- No prompt names a definition path; no vendor path is written where another
  vendor reads it.
- An unnamed agent costs nothing: no lookup, no rendered path, no mention.

**Non-Goals:**
- Moving the role onto the `AgentProfile`. A repository with several
  definitions still needs `spec.agent` to pick one, and the repo-less profile
  already has `role`/`systemPrompt`.
- Changing how `tools:` composes.
- A CRD or work-unit field. The unit already carries `agent`.

## Decisions

### D1 — The body is appended to the system message by the runtime

Each runtime's definition reader returns `{tools, body}`; the runtime appends
`body` to the system message it already controls, then the unit's
`systemPrompt` after it. Definition first because it is the agent's own
identity; the profile's inline role is the install's addendum to it.

| Runtime | Mechanism | Reader |
|---|---|---|
| claude | `--append-system-prompt` (one string: body + inline) | `tools.js` gains `agentDefinition()` |
| copilot | `systemMessage: {mode: append, content}` on create AND resume | same shape in its `tools.js` |
| ollama | `baseSystemPrompt + body + inline` in `agent.go` | `tools.go` gains the body |

*Alternative considered:* the manager reads the definition and ships it on the
unit. Rejected: the manager holds no checkout and must not — that is the
invariant that keeps the definition's `tools:` a runtime concern too.

*Alternative considered:* pass `--agent` / `customAgents` and let the vendor
adopt it. Rejected for the reason already on record: both re-apply the
definition's tools as an intersection and silently defeat `overwrite`.

### D2 — The prompt keeps the posture, loses the path

`task.md` and `investigate.md` keep one sentence of fallback posture — act as a
cautious SRE/platform advisor within your tools, observe before you act — and
lose everything that says where a role file is or what to do when it is
missing. The sentence "mention that no agent role file was found" goes with it:
there is nothing for the model to have looked for.

### D3 — Absent stays absent, at every layer

Empty `spec.agent`: the runtime looks up nothing (`agentDefinition('')` returns
`{tools: [], body: ''}`), the prompt renders no name-shaped text, and no line
in the transcript says a definition was not found. A NAMED agent whose file is
absent is logged once by the runtime — the operator's typo, visible in the
pod log — and contributes nothing, exactly as its frontmatter already does.

### D4 — Resume re-appends

A resumed session carries the body in its transcript already, but the
runtime supplies it again: `systemMessage.append` is not persisted by the
Copilot SDK, and `--append-system-prompt` is per invocation. Idempotent for the
model, and the only way "the role is always in effect" is a property of the
runtime rather than of the session's history.

## Risks / Trade-offs

- **Prompt size** → the body was already read into context by the model
  whenever it found the file; this moves the same bytes to the system message.
- **A body that contradicts `format.md`** → unchanged risk: it did the same
  when the model adopted it by instruction.
- **Four images move** → each is a tag; nothing in the manager or the chart
  changes shape, and an older runtime image against the new manager simply
  gets no role sentence in its prompt and reads the frontmatter as before — the
  degraded case is "no body adopted", never a broken run.

## Migration Plan

Additive. Manager first (the prompt loses the sentence), runtimes after (they
start appending). Between the two, a repo-backed profile's role is not adopted
on that runtime; bundle images move together in one chart minor so the window
exists only for an install pinning images by hand.

## Open Questions

- Should the body be capped, or a definition over some size logged? A
  definition is authored by the adopter, so the default is to trust it.
