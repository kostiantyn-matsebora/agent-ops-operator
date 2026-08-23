# Claude context — agent-ops-operator

**Go/controller-runtime Kubernetes operator.** `README.md` for the product
view, `docs/concepts.md` for the CRD detail.

**The context is `.claude/rules/`** — one topic per file, every one loaded at
launch except where a file declares `paths:` and loads only with the files it
names.

| Scoped rule | Loads when reading |
|---|---|
| `chart.md` | `chart/**` |
| `palette-and-mark.md` | the console theme, the site's css/js, the mark's three files |
| `signal-rules.md` | `signals/**`, the manager's `internal/ingest/`, their chart values |

**A DIRECTORY IS A COMPONENT** — `platform/` `runtimes/` `signals/` `channels/`
`gateways/`, one container each. The PATH is the published name: a plural group
lends its singular as a prefix (`signals/cron` → `agentops-signal-cron`), a
singular one lends nothing (`platform/console` → `agentops-console`). The
operator is `platform/manager/`. See `.claude/rules/structure.md`.

**DOCUMENTATION IS PART OF EVERY CHANGE, AND IT IS NOT OPTIONAL.** Both halves —
the reference docs AND the adopter site — are updated to match what the change
did, before the change is finished. **Every openspec change carries a DEDICATED
documentation task, and it is the LAST task before the change is done.** Not a
line inside another task, not a follow-up, not "docs later".

**IT IS ENFORCED, NOT TRUSTED** — `openspec/config.yaml` injects it when a tasks
file is written, and a `PreToolUse` hook REFUSES `openspec archive` while the
documentation task is unticked. See `.claude/rules/documentation.md`.

**The published site carries its own context** — `docs/CLAUDE.md` plus
`docs/.claude/`, loaded on demand when working under `docs/` and never at
launch.
