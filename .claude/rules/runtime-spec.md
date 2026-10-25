---
paths:
  - "runtimes/**"
  - "platform/manager/internal/dispatch/**"
  - "platform/manager/internal/runtimepod/**"
  - "platform/manager/internal/httpapi/**"
  - "docs/contracts/runtime.md"
---

## The runtime specification governs this surface

**`docs/contracts/runtime.md` is the one law for this surface** — the three
shipped runtimes plus the manager's dispatch, runtimepod and httpapi
packages.

It states what the manager owes a runtime and what a runtime owes the
manager. Build and review against it, never against a shipped runtime's
source.

| Obligation | Rule |
|---|---|
| Verify before done | A change on this surface is verified against `docs/contracts/runtime.md` before it is claimed done |
| No unapproved spec edit | `docs/contracts/runtime.md` is NEVER edited without the user's explicit approval |
| A spec change carries conformance | An approved specification change brings every shipped runtime — `runtimes/claude`, `runtimes/ollama`, `runtimes/copilot` — into conformance in the SAME change, and moves every vendor page's claimed version. The change is not archivable before that |

**Reaching for the specification itself without approval is a STOP, not an
edit.** Approval is a conversation fact, not something a hook can read — stop,
ask, and report the conflict instead of writing it.

**`runtime-claude` and `runtime-claude-cli` are two separate runtimes, not one
image with a switch.** `runtimes/claude` drives the SDK in-process and holds a
session open per pod, default-eligible. `runtimes/claude-cli` spawns the CLI
per unit, off by default. The composed allowlist is a `canUseTool` callback
rule set on `runtimes/claude` only — `runtimes/claude-cli` keeps
`--allowedTools`/`--permission-mode dontAsk`.
