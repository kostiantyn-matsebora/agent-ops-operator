## Context

See proposal.md — Why.

- `agentops.runtimePodExecutionAllowed` in the parent's `_helpers.tpl` reads
  `global.agentops.runtimeDefaults.allowPodExecution` and is what
  `agentops.runtimeWriteRules` gates on. The kubernetes bundle's
  `mcp-server.yaml` already includes that helper for the server's ClusterRole,
  so a subchart calling a parent helper over `.Values.global` is the
  established pattern here (see `chart.md`: a subchart reads no parent scope
  but `global`).
- `profile.yaml` renders `spec.systemPrompt` from `profile.systemPrompt`,
  verbatim, trimmed and indented. The shipped text lives in the bundle's
  `values.yaml` and is quoted in `docs/guides/agent-profile.md` (hand-written
  prose; check for a `generated:` marker before editing — if the block is
  generated, the generator owns it).
- The chart render tests are Go, in
  `platform/manager/internal/integration/charttemplate_test.go`, driving
  `helm template` with `--set` and asserting on the documents by kind and name.
  They skip silently without `helm` in the build container (memory:
  `go-125-build-container`).

## Goals / Non-Goals

**Goals:**

- The agent is told the posture by the same value that enforces it, so the two
  cannot disagree.
- The text is readable and tunable in values, not buried in a template.
- An operator who wrote their own `systemPrompt` still gets the posture.

**Non-Goals:**

- Reflecting `kubernetes.allowMutations` or `mcpServers.readOnly` in the
  prompt. Those move the TOOL LIST, which the existing "not in your allowlist,
  say so" sentence already covers; the tool list is visible to the agent, the
  RBAC gate is not.
- Reflecting the posture in other bundles' profiles (`home-assistant`,
  `prometheus`). Neither reaches the Kubernetes API through its route.
- Any change to what is granted. RBAC, toolsets and the MCP server are
  untouched.
- A general "describe your RBAC to the agent" mechanism. One paragraph for one
  gate; a rendered inventory of verbs would be a second source of truth for the
  role.

## Decisions

**One profile with a conditional paragraph, not two profiles.**
The user's first framing was two profile versions. Rejected: a profile chosen
by hand to match a value is a copy of that value, and it drifts the first time
someone flips `allowPodExecution` without re-selecting the profile. The chart
knows the fact; the chart tells the profile. This is `wiring.md`'s "both walls
move together" argument applied to a third wall.

**The paragraph is a bundle value, `profile.podExecutionWithheldPrompt`,
rendered only when the gate is off.**
Alternatives: (a) hardcode it in the template — unreadable and untunable, and
the shipped prompt is already a value; (b) two whole prompts,
`systemPrompt` and `systemPromptPodExecutionWithheld` — doubles the text to
maintain and reintroduces the drift under another name. A value holding only
the delta keeps one prompt and one fact.

**Appended after `systemPrompt`, whatever `systemPrompt` is.**
The operator's prompt is the agent's judgement; the posture is the install's
fact. Concatenating rather than substituting means overriding the role never
silently drops the posture. An operator who wants no paragraph sets the value
to an empty string, which `with` then skips — no separate boolean.

**Wording names what remains possible, scoped by the tool list.**
The paragraph says scaling, restarting, evicting, cordoning, deleting workloads
and editing ConfigMaps/Services/Ingresses stay available "where your tools
allow", because on the observe-only route none of them is. The tool-list
sentence already in the role resolves that, and a paragraph claiming those
actions unconditionally would be wrong under `allowMutations=false`.

**The template calls the parent helper, not `.Values.global` directly.**
`agentops.runtimePodExecutionAllowed` is the one place the default and the
`dig` path live; `mcp-server.yaml` calls it the same way. A second reading of
the path is a second place for the default to disagree.

**Tests are chart render tests, no e2e lane.**
Whether a paragraph is in a rendered string is decided by `helm template`.
Whether the agent then obeys it is a model's behaviour, which `docs/testing.md`
puts in no tier this repository can pin; the real-runtime e2e lane would spend
a credential to measure a prompt.

## Risks / Trade-offs

- [The agent still tries, despite the paragraph] → the RBAC refusal remains
  the backstop; nothing is granted. The paragraph reduces confusion, it does
  not replace the wall. Verify by hand on the live install after deploy.
- [The paragraph goes stale against the verbs the helper gates] → the
  enumerated kinds in the default text mirror `runtimeWriteRules`' gated block;
  a render test asserts the paragraph names each workload kind the helper
  gates, so widening the helper fails the test.
- [An operator on an older bundle version with a hand-copied `systemPrompt`
  values file] → appending never removes their text; they gain a paragraph
  on upgrade, stated in the CHANGELOG.
- [`docs/guides/agent-profile.md` quotes the shipped prompt] → if the block is
  generated, `docs-generate.py` re-renders it; if hand-written, the docs task
  updates it. Either way `--check` in CI catches a generated drift.

## Migration Plan

Chart-only. `helm upgrade` re-renders the `AgentProfile`; conversations
re-read profile CONTENT at every use (`terminology.md`: refs are snapshotted,
content is not), so running conversations pick up the paragraph on their next
work unit. Rollback is the previous chart version. No CRD change, so no
`kubectl apply -f chart/crds/` step.
