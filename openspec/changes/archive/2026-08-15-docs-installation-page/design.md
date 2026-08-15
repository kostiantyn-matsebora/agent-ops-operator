## Context

The site has three pages: the landing pitch, the Introduction (the model) and
Getting started (a read-only demo that says outright it is not a deployment).
The next question an adopter asks has no page.

`chart/values.yaml` is ~620 lines. Its comments are excellent and are the reason
the chart is usable at all, but they are only reachable by opening the file.
Meanwhile the routing table has an owner for every kind of documentation EXCEPT
the parent chart's values.

Constraints inherited, not renegotiated:

- **Adopter documentation is structure over prose** — numbered steps, tables,
  short sentences, emphasis on the load-bearing phrase, no semicolons.
- **The theme holds no prose and the pages hold no theme.** A page is markdown
  plus one line in `_data/nav.yml`.
- **Reference pages under `docs/` are still untreated** — no front matter, no
  navigation entry, linked where they live.
- **A page declares its own `permalink`.**

## Goals / Non-Goals

**Goals:**

- An operator finishes with a release installed on their own terms, one bundle
  enabled, and one route wired — and knows which decisions are expensive to
  reverse.
- The parent chart's values have exactly one home.
- Bundle values stay with their bundles. This page is the hub, not the store.

**Non-Goals:**

- Not a values reference. `helm show values` is exhaustive and always current.
- Not a second `concepts.md`. Where a value's semantics need explaining, the
  page links to the document that owns them.
- Not a production-hardening guide. Ingress, TLS, oauth2-proxy and per-surface
  credentials belong to `console.md` and the bundle pages.
- Not the demo. Getting started keeps that, and the two link to each other.

## Decisions

### Values are grouped by DECISION, not by key

The obvious page is a table of every value. It is also the wrong one: it
duplicates `values.yaml`, goes stale on the first key added, and answers
"what can I set" when the operator is asking "what do I have to decide".

So the page groups values by the decision they serve:

| Group | The decision | Keys named |
|---|---|---|
| Capacity | how many agents run at once, and what happens to the rest | `maxActiveConversations`, `maxQueuedConversations`, `runtimeIdleTtlMinutes` |
| Storage | whether conversations keep their context | `persistence.*` |
| The agent's power | what the agent may do in the cluster | `global.agentops.runtime.rbacMode`, `rbac.runtime` |
| The runtime | image, credential, placement | `runtime.*` |
| Access | who can reach the manager and the console | `adapterAuth.*`, `console.auth.*` |
| Housekeeping | what is reclaimed, and when | `retention.*`, `housekeeping.*` |
| Lifecycle | who owns the CRDs | `crds.enabled`, `crds.keep` |

Each group is a short table of the keys that matter with their defaults, plus
one line on the consequence. Anything beyond that is a link.

**The exhaustive list is named, not reproduced:**

```sh
helm show values ./chart
```

*Alternative considered — generating the table from `values.yaml` at build time.*
It would need a plugin GitHub Pages does not enable, which the site's founding
rule forbids.

### The decisions that are expensive to reverse go FIRST

Three choices are cheap now and painful later, so they are a section before the
install rather than a footnote inside it:

1. **Storage.** `persistence.enabled=false` means `ContinuityPossible()` is
   false and every run starts fresh. Changing it later does not recover the
   context that was never kept.
2. **RBAC posture.** `rbacMode` decides what the agent can do. `full` is
   cluster-admin for an LLM-driven process — the page states that plainly and
   does not offer it as a default.
3. **CRD ownership.** `crds.keep` is why an uninstall does not cascade-delete
   every Conversation. The annotation protects nothing retroactively, so the
   decision belongs before the first install.

### Bundles get a section, never their values

A bundle is named, given its enable flag, and linked:

| Bundle | Flag | Page |
|---|---|---|
| Kubernetes events | `k8s-bundle.enabled` (or `global.demo.enabled`) | `k8s-bundle.md` |
| Prometheus alerts | `prometheus-bundle.enabled` | `prometheus-bundle.md` |
| Telegram | `telegram-bundle.enabled` | `telegram-bundle.md` |

The k8s bundle's row carries the one asymmetry worth knowing: it has **no Helm
condition**, because it turns on via `enabled` OR `global.demo.enabled` and a
condition evaluates only the first existing path. Its templates self-gate.

### The page ends by wiring one route, because an install answers nothing without it

A fresh install with a bundle enabled still does nothing: sources no Ready
Pipeline claims DROP their signals. That is the single most likely "I installed
it and nothing happened", and it is a property of the model rather than a bug,
so the page closes with the smallest honest route — one Pipeline naming a
source, a profile and a toolset — and links `concepts.md` for the fields.

### It supersedes Getting started's forward promise

Getting started says "a page for real installs comes later". That sentence
becomes a link, and its Next card points here instead of at `concepts.md`.

## Risks / Trade-offs

- **A values page drifts from the chart** → Only keys an operator must decide
  are named, and each is one an existing document already owns the semantics of.
  The exhaustive list is a command, not a copy.
- **Overlap with `concepts.md`'s substrate section** → That section owns the
  `runtime:` / `global.agentops.runtime.*` semantics. This page states the
  decision and links. The test is the same one the Introduction uses: a sentence
  that a field rename would break belongs in the reference page.
- **Overlap with the demo page** → They answer different questions and say so in
  their first lines. Getting started is a first look, this is an install.
- **The page grows into a hardening guide** → Bounded by the non-goals: ingress,
  TLS and forward-auth stay in `console.md`, which owns the trust boundary.
