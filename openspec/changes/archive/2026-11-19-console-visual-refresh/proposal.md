## Why

The console is built to inspect an install. It is now also where people hold
conversations with agents, and the conversations page does not read like one.

- Every action, event and run detail takes the same visual weight.
- The page is laid out for a wide desktop and is hard to use on a phone.
- Work the manager accepts and finishes later (`async-command-bus`) has no
  visible states. A reply, a close or a delete just waits.

A redesign was explored against the live console and settled in a clickable
mockup.

- It took about sixty rounds of correction to reach a layout that is quiet,
  stable and complete.
- Those corrections are the real product of the exploration.
- Without a written record each one is lost and found again the hard way.

The redesign must not take the current look away. Some operators have
screenshots, runbooks and habits built on it. Presentation therefore has to
become a thing the console can swap, and today it cannot.

## What Changes

- **A new look, `workspace`, is the default.** A conversation workspace in the
  Slack tradition, with a phone layout, applied to every console page.
- **The current look stays, as `classic`.** It keeps its layout and behaviour.
  A viewer switches between the two from the header, and the choice persists.
- **Behaviour and presentation separate.** A headless model layer owns data,
  state and commands. A look owns markup, styling and motion. Adding a third
  look means adding a folder.
- **The new look shows the manager's asynchronous work.** Sending, accepted,
  delivered, working, failed with a retry, and a connection state.
- **An install can name the default look** with the chart value
  `console.defaultLook`, so a conservative install keeps `classic` for everyone
  who has not chosen.
- **Every message can be copied** to the clipboard as its markdown.
- **The guardrails from the exploration become requirements.** Nothing changes
  size while it animates, incoming events notify by appearing in place, and run
  detail stays folded unless asked for.
- **Every look is responsive and proven at six named viewports**, phone, tablet
  in both orientations, laptop, desktop and a wide screen, by the end-to-end tier.
- **The mockup is committed in the change** at `mockup/console.html`, with a
  statement of where it is and is not the target.

No manager, CRD or channel-contract change is part of this. The console service
and the chart change only to carry the default look.

## Capabilities

### New Capabilities

- `console-looks`: the seam between behaviour and presentation. A registry of
  looks, selection, persistence, the default, forcing one by URL, and the parity
  a look must meet before it ships.
- `console-workspace-look`: the new look. Its composition on every page, its
  responsive behaviour, and how it presents conversations, signals, run detail
  and the other pages.
- `console-workspace-feedback`: how the new look shows live and asynchronous
  state. Command states, connection state, arrivals, notification by placement,
  and the rules that keep layout still.

### Modified Capabilities

- `console-chat-layout`: its four-column layout becomes the `classic` look's
  layout, so it no longer claims to be the only one.
- `console-deployment`: the chart gains `console.defaultLook`.

## Impact

Code:

- `platform/console/ui/src/`: a `model/` layer, a `looks/classic/` folder holding
  today's components moved without behaviour change, and a `looks/workspace/`
  folder.
- `platform/console/ui/src/theme/`: the colour-scheme store keeps its name. A new
  look store sits beside it.
- `platform/console/ui/e2e-live/`: specs run once per look.
- `platform/console/`: the session carries the install's default look.
- `chart/`: `console.defaultLook`, its render test and its schema entry.
- No manager or CRD change.

Reference docs that become untrue:

- `docs/console.md`: the page describes one look.
- `docs/console-guide.md`: the tour is of the classic look.
- `docs/testing.md`: the console's browser tier gains a per-look dimension.
- `docs/configuration.md`: `console.defaultLook`.
- `docs/CHANGELOG.md`: the default look changes for every install.
- `.claude/rules/palette-and-mark.md`: states which tokens are shared with the
  docs site and which belong to a look.

Adopter site that becomes untrue:

- the landing page and its screenshots,
- `docs/introduction.md` and `docs/getting-started.md`, where they show the
  console,
- `docs/installation.md`, where it shows the console's values,
- the console screenshots and the demo recording, regenerated for both looks.

Delivery risk:

- Every console end-to-end spec describes today's DOM. Moving it behind a seam is
  the largest cost here, and `design.md` says how it is paid.
- The default look changes for every install on upgrade. That is the breaking
  part and belongs in the changelog.
