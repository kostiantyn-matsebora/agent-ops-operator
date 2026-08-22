# Design — adopter guides

## D1. The tiers are LEARNING order, and each page carries its own risk

The ladder orders what a reader must understand, not what they can break.

Risk is **not** monotonic along it. Binding a shell toolset and an admin MCP
server to a chat-addressable Pipeline (tier 2, pure YAML) opens a wider hole
than a badly written cron adapter (tier 3, real code).

So no page may imply that danger grows with the tier. **Each page states what
its own mistake costs**, and tier 2's warning is not smaller than tier 3's.

## D2. Generated, committed, drift-checked

GitHub Pages builds `docs/` from `master` with no workflow, so a generated file
must be committed — the same constraint the screenshots and the recording live
under.

But a template can do what a screenshot cannot: **CI regenerates it and fails on
a non-empty diff.**

That is the whole reason to generate rather than write. A hand-written CR
example is correct on the day it is written and silently wrong after the next
field rename — which is exactly the drift `truthful-specs` exists to clean up
one directory over. Generating without the drift check buys nothing but a
harder-to-edit file.

| Artifact | Source | Kept true by |
|---|---|---|
| CR templates | `chart/files/crds/` | regenerate in CI, fail on diff |
| Worked examples | `helm template` over a bundle's own values | regenerate in CI, fail on diff |

## D3. Minimal inline, full reference behind a link

Field counts, from the CRDs:

```
  MCPToolset       1        Pipeline        14        AgentProfile    39
  MCPConfig        5        SignalSource     8        AgentRuntime    54
```

A tier-1 template is a few lines — only `profileRef` is required on `Pipeline`,
and `AgentProfile` requires nothing at all. A full dump of `AgentRuntime` is 54
fields, on the page where a reader can least afford a wall.

So each page carries the **minimal** CR — required fields plus what that page
teaches — and links the generated full reference for everything else.

## D4. The chart is the single source of example values

Examples are rendered from the chart's own bundle values, never invented.

Invented examples are a second set of values to keep true, and they are how a
real identifier gets pasted in as the "better" example — which has already
happened once in this repository.

**Ordering consequence:** after `scrub-identity` substitutes the documented
placeholder into the chart values, every rendered example inherits it and the
publication guard enforces it. Generating before that bakes a real identifier
into the site instead.

| Tier | Rendered from |
|---|---|
| 1 | k8s-bundle under demo values |
| 2 | ha-bundle — two toolsets split by privilege, and its MCPConfig |
| 3 | telegram-bundle — a Channel, a SignalSource, both adapter CRs |
| 4 | the parent chart's `runtime:` |

## D5. Four parts, the same four, every page

```
   what you are doing   →  the task, in a sentence, and what it costs to get wrong
   fill this in         →  the minimal CR, generated
   the full surface     →  a link to docs/cr-reference.md
   something that works →  the in-repo implementation for this tier
```

The fourth part is why the tiers are cheap to write: `signals/cron` is the
reference signal adapter, `channels/telegram` the reference channel adapter,
`runtimes/claude` the reference runtime. Each page points at one rather than
inventing a toy.

## D6. The CR reference is a repo file, not a site page

It is pure reference — every field of every kind — which the site's own rules
place in `docs/` beside `concepts.md` and `contracts.md` rather than as a site
page with front matter and a nav entry.

The guides link to it on GitHub, exactly as Getting started already links
`concepts.md`.

## D7. Tier 4 opens with what a runtime is trusted with

`--allowedTools` is the sole permission authority, and the runtime is what
applies it. A third-party runtime that ignores it **silently voids every toolset
binding in the install**, and nothing in the manager detects that.

That is the first thing on the page, not a caution at the end. `egress-proxy`
exists because this trust is real, and the page says so and links it.

## Open questions

None. The tier order, the page set, the index approach, the reference's home and
the change's size are all settled.
