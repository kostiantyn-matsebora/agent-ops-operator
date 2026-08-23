# Design — adopter guides

## D0. THE PIPELINE IS THE FIRST GUIDE, AND IT CREATES NOTHING

`Pipeline` is the only object that carries any wiring, and every object it names
— profile, sources, channels, toolsets — a demo install already ships.

So the fundamental lesson costs the reader **no new resources**, which is what
makes it the right thing to meet first.

- **The profile cannot come first.** On its own it is inert, and its whole
  purpose is a Pipeline the reader has not been shown.
- **The two were ONE page for a while**, and that page taught two objects at
  once while implying an agent needs both to exist before anything works. It
  does not — a second route over installed pieces is the commonest thing an
  adopter wants.

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

**The ordering is now ENFORCED rather than remembered**, because a note in a
design document does not survive the session that wrote it:

| Check | Refuses |
|---|---|
| `assert_placeholders` | a render value the CHART's own values files do not document — so the site and the shipped values cannot come to name different examples |
| `audit_identifiers` | a published page naming a host that is not reserved, or an identifier matching a shape this repository has shipped as real |

Both run in `--check`, so CI fails on either.

- **The credential dummies are exempt from the first, by KEY.** A values file
  carrying an example token teaches someone to ship it, so no chart may
  document one and the check must not demand that it does.
- **The second names no literal it guards against.** It works by SHAPE and by
  allowlist — a guard that spelled out the scrubbed value would re-introduce it.

| Tier | Rendered from |
|---|---|
| 1 | k8s-bundle under demo values |
| 2 | ha-bundle — two toolsets split by privilege, and its MCPConfig |
| 3 | telegram-bundle — a Channel, a SignalSource, both adapter CRs |
| 4 | the parent chart's `runtime:` |

## D5. Five parts, the same five, every page

```
   opening              →  what the thing IS, two or three dense sentences
   Before you start     →  when it applies, when it does NOT, prerequisite links
   The overall shape    →  the parts it is built from, as a numbered list
   task sections        →  named for what you do, each with its code BENEATH it
   What comes next      →  a numbered list of onward links
```

**Sections are named for the task, never "Step 3".** `Compose the tool
allowlist` and `Create the deploy key` say what the reader is doing. A number
says only where they are.

**Explanation sits immediately before the code it explains**, never piled at the
front and never cut. A page still points at the in-repo implementation —
`signals/cron`, `channels/telegram`, `runtimes/claude` — from "What comes next"
rather than inventing a toy.

### D8. The two failure modes, both of which shipped here

| Draft | Failed because |
|---|---|
| Reference material re-headed as a guide | it restated `contracts.md` in full, which the spec forbids and a reader has no reason to read twice |
| Bare numbered steps with the concept cut | the instructions had no subject — a reader was told to poll `/work` before being told what a runtime is |

**"Before you start" carries the WHEN-NOT.** Half the readers of the adapter
pages want a `SignalSource` rather than an adapter, and a page that does not say
so has already cost them the wrong afternoon.

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
