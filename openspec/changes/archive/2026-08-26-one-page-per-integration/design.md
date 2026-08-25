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
- Remove more than is added: four pages of subchart documentation become four
  integration pages, and the exhaustive values go with the old ones.

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
kubernetes: mcp.url is required when mcp.enabled=true
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

Verified by rendering the chart as it stands — every row is the component's own
contribution and nothing else:

```
eventsAdapter -> SignalAdapter/k8s-events, SignalSource/cluster-events,
                 ServiceAccount/agentops-signal-k8s-events,
                 ClusterRole + ClusterRoleBinding/agentops-signal-k8s-events-events-default
profile       -> AgentProfile/k8s-engineer
mcp           -> MCPConfig/k8s-api, MCPToolset/k8s-observability
mcpServers    -> Deployment + Service + ServiceAccount + ClusterRole
                 + ClusterRoleBinding/agentops-mcp-k8s
pipelines     -> Pipeline/k8s-observe
```

`home-assistant` was rendered the same way and behaves the same: its wiring
component contributes `Pipeline/ha-control` and `Pipeline/ha-ops` and nothing
else, and its `logsAdapter` contributes exactly the adapter and the source.

**All five rows reproduce `docs/kubernetes.md`'s hand-written table**, which is
the evidence that the technique describes the same thing a human was describing.

**That the table is right TODAY is not an argument against generating it — it is
the argument for it.** The wiring row named one `Pipeline` while `af5bb49` had
the chart rendering a route `ServiceAccount` beside it, and went back to being
correct when `84a2654` stopped rendering that account. Nobody edited the row in
either direction. A table whose accuracy is restored by an unrelated chart
change is not being maintained, it is being lucky.

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

**Chosen:** `<!-- generated: renders bundle=kubernetes -->` fills one table with
a row per component. The bundle is named by its subchart directory —
`kubernetes`, `prometheus`, `home-assistant`, `telegram` — not by the page.

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
  ├─ one line + a diagram   what this does for you
  ├─ What you get           the outcomes, as a table
  ├─ Turn it on            the command, and where the answers turn up
  ├─ The values you set     what exists · what things are called
  ├─ Tune what reaches you  TASK headings — "Stop telling me about X"
  ├─ Adopt it               worked pipelines binding the bundle's parts
  ├─ What the bundle renders  the GENERATED listing
  └─ Going deeper           one link to concepts.md
```

**IT IS USAGE-LED, AND THE FIRST DRAFT WAS NOT.** That draft opened on the
signal lane and the toolsets, put "turn it on" fourth, and moved the old pages'
reference prose in whole — the verification ladder, owner-reference grouping,
the self-exclusion loop, Alertmanager's rule vocabulary.

- **An adopter is configuring, tuning and adopting**, in that order. A bundle is
  a thing they switch on and then bend to their install, so the page is those
  three acts and the mechanism is a link.
- **"Adopt it" is the act the first draft had no section for**, and it is the
  one that makes a bundle worth documenting: its source, profile, toolsets and
  config are ordinary CRs with stable names, bindable from pipelines of the
  reader's own, with their own agents.
- **That reframes the generated block.** It is not trivia about the chart, it is
  the list of names a reader's own `Pipeline` references — so it is headed *what
  the bundle renders* and sits beside Adopt it.
- **Tuning is TASK-HEADED**, never vocabulary-headed. "Stop telling me about
  probe warnings" and four lines of YAML, not a table of what `for` means.

**Write Kubernetes first and completely.** It is the only one with all of
signals, tools, suppression rules and self-exclusion, so the shape that survives
it survives the rest. The other three are then filled against it rather than
re-derived, which is the role a prototype would otherwise play.

**AND IT IS REVIEWED BEFORE THE OTHER THREE ARE WRITTEN.** The shape was wrong
twice — reference-led, then still carrying developer material — and each miss
would have been three more pages to redo.

**NOTHING GENERIC IS RESTATED PER INTEGRATION.** `serviceAccountName`,
`runtimeRef`, `maxTurns` and how `channels` merge are Pipeline and profile
facts. Four pages restating them is one fact in four places, and the guides
already own it.

**A page states what is TRUE OF ITS OWN LANE**, which is why each one's values
tables are checked against a real `helm template` of that bundle's route rather
than against the page it replaces.

### Pages are named for the system, and the URL is the page

`docs/integrations/kubernetes.md` at `/integrations/kubernetes/`. The permalink is
declared on the page, as every site page declares its own.

`home-assistant.md`, not `ha.md`. That one is already right — `413bbbc` renamed
`ha-bundle.md` past the subchart's old abbreviation — and the rule is what keeps
it right: a page does not inherit the packaging's vocabulary, so a subchart
renamed again leaves the page and every link to it alone.

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
- **Four pages move and their inbound links break** — five in one `README.md`
  table row (the fifth is `docs/claude.md`, which does not move), four in
  `installation.md`, four in `docs/index.md`'s *Run it* list. All of them name
  the CURRENT filenames already, as raw GitHub URLs, because `413bbbc` renamed
  the pages and repointed them. → The link updates are tasks, and a
  repository-wide grep for both the pre-`413bbbc` names and the current ones is
  the check.
- **No published URL is at stake.** None of the four carries front matter, so
  none declares a `permalink` and none is reachable as a site page today. The
  move is a file move plus the front matter that publishes them for the first
  time — which is also why the second rename in one release cycle costs a reader
  nothing.
- **A generated table is less expressive than the prose it replaces.** → That is
  the trade being made deliberately: the prose stays, above the block, and only
  the inventory is surrendered to the generator.
- **`prometheus` has never been rendered by the generator** — `PRESETS` runs
  `tier1` through `tier4` and none enables it — so its first preset may surface a
  values problem nobody has hit. → Better found by this change than by an
  adopter.

## Migration Plan

Site-only. No chart version, no image, nothing for an adopter to upgrade, so no
`CHANGELOG.md` entry.

**The moves must land in the same change as the rewrites.** A release in which
`README.md` still links `docs/kubernetes.md` while the file has moved is a broken
link on the project's front page, and a release in which both copies exist is two
documents wearing one subject — which is the state this change exists to end.

Rollback is `git revert`: the four pages come back with it.

## Open Questions

None. The page set, the naming, the marker's shape and the attribution technique
were each settled against the chart before this was written.
