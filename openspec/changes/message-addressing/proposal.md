# Proposal: message-addressing

## Why

Every message that reaches the manager today already carries its addressing in
its FORM — a slash command names a Pipeline, a topic id names a conversation, a
tap on a control picks from a list. A message that carries its addressing in
its WORDS — "ask k8s-ops to restart nginx", "reply to the ingress one: roll it
back" — has no way in, and the first surface that produces nothing but words is
speech (`voice-conversations` builds on this change). The decision is an LLM
judgement with a feedback loop, and it belongs to no existing component: not
the manager, which composes meaning and never guesses, and not a transport.

## What Changes

- **New component `platform/analyzer`** — the ANALYZER: takes one UTTERANCE
  (text, speaker, surface, language, an optional thread hint) and returns one
  DECISION — originate a conversation on a named Pipeline, reply to a named
  conversation, run a manager command, or ASK the speaker a question because it
  could not decide. It holds a short-lived, deliberately lossy pre-conversation
  per speaker so an answer to its question resolves the pending intent.
- **The analyzer holds no credential and posts nothing to the manager.** It
  reads the conversations a speaker could mean through the aops MCP server
  with the CALLER's `channel-reader:<channel>` token (`coordinated-agents`),
  and the caller — an adapter already holding its own tokens — delivers the
  decision through the ordinary `/signal/inbound` and `/channel/inbound`
  contracts. The manager sees a final message and nothing of the dialogue.
- **One contract, `POST /utterance`**, synchronous: the decision is the
  response. No callback, no queue, no port on the caller.
- **The LLM is replaceable behind an interface** — Ollama and any
  OpenAI-compatible chat endpoint on day one; one file per vendor.
- **Chart**: the analyzer's Deployment and Service, off by default
  (`analyzer.enabled`), behind the component wall, allowed out to its LLM
  endpoint and to `mcp-aops`.
- No CRD, no manager change: the reach class and `brief` it depends on land in
  `coordinated-agents`.

## Capabilities

### New Capabilities

- `utterance-addressing`: the analyzer — the utterance contract, the five
  decision kinds (`refuse` included), the question loop and its lossy state, what it reads and
  through whose token, and that it delivers nothing itself.

### Modified Capabilities

- `component-network-isolation`: the analyzer joins the wired set — reachable
  from the adapters that call it, allowed out to `mcp-aops` and to its
  configured LLM endpoint, and to nothing else.

## Impact

**Depends on** `coordinated-agents` phase 3 — `mcp-aops`, the
`channel-reader:<channel>` reach class and `status.brief`. Without them the
analyzer can resolve a Pipeline but never a conversation.

**Code**

- `platform/analyzer/`: new module, standard library only, shared Dockerfile
  recipe; `.github/components.sh` derives `analyzer`; multi-arch.
- `chart/`: Deployment, Service, NetworkPolicy, `analyzer.*` values, NOTES.txt.
- No manager, CRD or runtime change.

**Documents made untrue — reference docs**

- `docs/contracts.md`: the utterance contract and the decision shapes.
- `docs/concepts.md`: how a message with its addressing in its words reaches a
  Pipeline — beside the slash form and the choice list.
- `docs/installation.md`: `analyzer.*` values.
- `docs/security.md`: a new component that reads conversation briefs with a
  caller's token and holds none of its own; the threat-model diagram re-run.
- `docs/CHANGELOG.md`: the new component.
- `.claude/rules/structure.md`: `platform/analyzer`; `terminology.md`:
  utterance, decision, the analyzer is not the manager's and not a runtime.

**Documents made untrue — adopter site**

- `docs/introduction.md`: a third way a message reaches a Pipeline.
- `docs/installation.md`: the component list.
- `README.md`: one line under "Pluggable at three seams" — the seams become
  four; stays ≤ 215 lines.
