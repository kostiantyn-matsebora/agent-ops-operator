# Design

## Context

See `proposal.md` — Why. The requirements are in `specs/docs-site/spec.md` and
`specs/documentation-structure/spec.md`.

Four constraints from the existing site and generator shape the whole approach:

- **The generator already renders the chart.** `render_preset()` runs
  `helm template`, parses every document to `{kind, name, text}` and memoises the
  result. `PRESETS` declares the `--set` values per preset, guarded so no
  undeclared placeholder and no credential can reach a published page.
- **CI already fails on a stale generated block.** `--check` is wired. Nothing
  new is needed to make a generated table self-policing.
- **`generate_block()` returns a fenced `yaml` block and only that.** Emitting a
  table is the one structural change to the marker machinery.
- **The pages hold no theme and the theme holds no prose.** An integration page
  is markdown plus kramdown attribute lists, exactly like every other page.

**There is NO prototype for this change, and none is owed.** Nothing here is a
composition — no geometry, no timings, no spacing. The one thing that benefits
from being settled before it is copied is the page SHAPE, and the design settles
that below by writing the Kubernetes page first and treating it as the reference
for the other three.

## Goals / Non-Goals

**Goals:**

- One page shape, used four times, that a fifth integration can be added to
  without inventing a new one.
- A component inventory that cannot be wrong, because nobody types it.
- Delete more than is added: four pages go, and the exhaustive values with them.

**Non-Goals:**

- Changing anything the chart renders. This change reads the chart, it does not
  edit it.
- Documenting cron or MCP as integrations — see the proposal's Out of scope.
- Making the generator understand Helm. It runs `helm template` and diffs object
  sets, and knows nothing about templates, conditionals or values semantics.

## Decisions

### A component's objects are a SET DIFFERENCE against a prerequisite-aware baseline

**Chosen:** for each component, render the bundle twice — once with the
component off and once with it on — and attribute the difference.

The naive form of this does not survive contact with the chart. Toggling one
flag at a time fails outright:

```
k8s-bundle: mcp.url is required when mcp.enabled=true
(or enable the mcpServers component to run the MCP server in-cluster)
```

And a baseline that ignores dependencies attributes a prerequisite's objects to
the component that needed it — `mcpServers` appears to render `MCPConfig` and
`MCPToolset`, which belong to `mcp`.

**So a component declares two things:**

| declared | is |
|---|---|
| `sets` | the values that turn this component on |
| `requires` | the components that must be on for it to render at all |

The baseline is the bundle enabled with every component off **plus every
required component's own `sets`**, and the component's row is what its own
`sets` add to that.

Verified against the current chart — every row is the component's own
contribution and nothing else:

```
eventsAdapter -> SignalAdapter/k8s-events, SignalSource/cluster-events,
                 ClusterRole + ClusterRoleBinding/agentops-signal-k8s-events-events
profile       -> AgentProfile/k8s-engineer
mcp           -> MCPConfig/k8s-api, MCPToolset/k8s-observability
mcpServers    -> Deployment + Service + ServiceAccount + ClusterRole
                 + ClusterRoleBinding/agentops-mcp-k8s
pipelines     -> Pipeline/k8s-observe, ServiceAccount/agentops-k8s-observe
```

Four of those five reproduce `k8s-bundle.md`'s hand-written table word for word,
which is the evidence that the technique describes the same thing a human was
describing. The fifth is the row the human has not written yet.

**A MISSING `requires` FAILS THE RENDER, LOUDLY.** That is the property that
makes a hand-maintained dependency list safe: the chart's own guard refuses, the
generator reports it, and CI stops. It cannot silently produce a wrong row.

Alternatives considered:

- **Parse the chart's templates** and read the `if` conditions. Rejected: it is a
  second implementation of Helm's semantics, wrong the first time a template uses
  anything interesting.
- **Annotate every rendered object with the value that produced it**, in the
  chart. Rejected: it puts documentation machinery into shipped manifests, and
  the annotation would be as hand-written as the table it replaces.
- **Diff against the bundle DISABLED** rather than enabled-with-components-off.
  Rejected: the difference then includes everything the bundle renders
  unconditionally, attributed to whichever component happened to be measured
  first.

### ONE marker per bundle, emitting the whole table

**Chosen:** `<!-- generated: renders bundle=k8s-bundle -->` fills one table with
a row per component.

A marker per component would put four or five markers on a page whose table is
one thing, and each would have to be kept in the right order by hand. One marker
means the generator owns the row order too — declaration order, which is the
order the components are meant to be read in.

### The generated table states OBJECTS. The prose states meaning

The table's columns are the component, the value that enables it, and the objects
it renders. Everything a set difference cannot express stays hand-written prose
around the block:

- that a `SignalSource` appears only under `source.create`
- that the MCP server's ServiceAccount is **its own**, and carries the grant
- that the profile is behaviour only
- why the events lane needs dwell at all

This is the split the spec states as *"the generator owns what is rendered, the
page owns what it means"*, and it is what keeps the block from growing prose that
the next regeneration deletes.

### The dispatcher returns markdown, not a yaml fence

`generate_block()` currently ends `return f"```yaml\n{yaml_text}\n```\n"`. The
`renders` kind returns a markdown table instead. This is the one structural
change to the marker machinery, and it is small — but it is the reason the
`renders` marker cannot simply be bolted on beside `template` and `example`.

### The page shape is four questions, in the Pipeline's own order

```
docs/integrations/<system>.md
  │
  ├─ one paragraph        what this integration is, and what it ships as
  ├─ What starts work     the signal lane
  ├─ What it may reach    the toolsets, and at which privilege
  ├─ Where it answers     — present only where the system carries answers
  ├─ Turn it on           the flag, the credential, the one decision
  ├─ What it renders      the GENERATED block
  └─ Tuning               the deep prose, where the integration has any
```

**Write Kubernetes first and completely.** It is the only one with all of
signals, tools, suppression rules and self-exclusion, so the shape that survives
it survives the rest. The other three are then filled against it rather than
re-derived, which is the role a prototype would otherwise play.

**"Where it answers" is omitted, not left empty.** Only Telegram carries answers,
and a heading reading "not applicable" on three pages teaches that the section is
noise.

### Pages are named for the system, and the URL is the page

`docs/integrations/kubernetes.md` at `/integrations/kubernetes/`. The permalink is
declared on the page, as every site page declares its own.

`home-assistant.md`, not `ha.md` — `ha` is the SUBCHART's abbreviation, and the
whole point of the naming rule is that the page does not inherit the packaging's
vocabulary.

### The deep prose moves, it is not rewritten

The event-suppression explanation, the workload-grouping rule, the
self-exclusion mechanisms, the Home Assistant privilege split and the credential
path are **moved as they stand**, edited only where they address a reader who has
already installed. They are the best writing in the four pages and rewriting them
from memory would lose the reasoning that makes them worth publishing.

## Risks / Trade-offs

- **CI gets slower.** Roughly six renders per bundle across four bundles, call it
  25 `helm template` invocations at 1–2s each → 30–60s added to the generator and
  to every CI run of it. Mitigation: the renders are memoised per set of values
  within a run, and the baseline is shared by every component of a bundle.
- **The `requires` graph is hand-maintained.** → A missing entry fails the render
  rather than producing a wrong row, so the failure mode is a red build with the
  chart's own error message in it.
- **Four pages are deleted and their inbound links break** — six in `README.md`,
  four in `installation.md`. → The link updates are tasks, and a repository-wide
  grep for the old filenames is the check.
- **A generated table is less expressive than the prose it replaces.** → That is
  the trade being made deliberately: the prose stays, above the block, and only
  the inventory is surrendered to the generator.
- **`prometheus-bundle` has never been rendered by the generator**, so its first
  preset may surface a values problem nobody has hit. → Better found by this
  change than by an adopter.

## Migration Plan

Site-only. No chart version, no image, nothing for an adopter to upgrade, so no
`CHANGELOG.md` entry.

**The deletions must land in the same change as the additions.** A release in
which `README.md` still links `docs/k8s-bundle.md` while the file is gone is a
broken link on the project's front page, and a release in which both exist is two
documents wearing one subject — which is the state this change exists to end.

Rollback is `git revert`: the four pages come back with it.

## Open Questions

None. The page set, the naming, the marker's shape and the attribution technique
were each settled against the chart before this was written.
