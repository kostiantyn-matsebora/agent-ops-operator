# Claude context — agent-ops-operator

**Go/controller-runtime Kubernetes operator.** `README.md` for the product
view, `docs/concepts.md` for the CRD detail.

**The context is `.claude/rules/`** — one topic per file, every one loaded
at launch except where a file declares `paths:` and loads only with the
files it names.

| Scoped rule | Loads when reading |
|---|---|
| `chart.md` | `chart/**` |
| `palette-and-mark.md` | the console theme, the site's css/js, the mark's three files |
| `signal-rules.md` | `signals/**`, the manager's `internal/ingest/`, their chart values |
| `runtime-spec.md` | `runtimes/**`, the manager's `internal/dispatch/`/`internal/runtimepod/`/`internal/httpapi/`, `docs/contracts/runtime.md` |
| `authoring.md` | `.claude/rules/**`, `docs/.claude/**` |
| `writing.md` | any `*.md` file |
| `change-tests.md` | an openspec change's `tasks.md` or `proposal.md` |

**A DIRECTORY IS A COMPONENT** — `platform/` `runtimes/` `signals/`
`channels/` `gateways/`, one container each. The PATH is the published name
(`signals/cron` → `agentops-signal-cron`, `platform/console` →
`agentops-console`). Operator: `platform/manager/`. See
`.claude/rules/structure.md`.

**DOCUMENTATION IS PART OF EVERY CHANGE, NOT OPTIONAL** — both the reference
docs and the adopter site, updated before the change is finished.

Every openspec change carries a DEDICATED documentation task, LAST, with
UNIT then SYSTEM TESTS before it (or one ticked line saying why the tier
doesn't apply). Enforced by a `PreToolUse` hook refusing `openspec archive`
while unticked — see `.claude/rules/documentation.md`.

**The published site carries its own context** — `docs/CLAUDE.md` plus
`docs/.claude/`, loaded on demand under `docs/`, never at launch.
