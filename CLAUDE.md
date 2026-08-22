# Claude context — agent-ops-operator

**Go/controller-runtime Kubernetes operator.** `README.md` for the product
view, `docs/concepts.md` for the CRD detail.

**The context is `.claude/rules/`** — one topic per file, every one loaded at
launch except where a file declares `paths:` and loads only with the files it
names.

| Scoped rule | Loads when reading |
|---|---|
| `chart.md` | `chart/**` |

**The published site carries its own context** — `docs/CLAUDE.md` plus
`docs/.claude/`, loaded on demand when working under `docs/` and never at
launch.
