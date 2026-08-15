## Context

`global.demo.enabled` is meant to be a turnkey install. It renders the console
(adapter, `Channel`, `SignalSource/console`) and the k8s bundle's route
`k8s-observe` — but that route claims `cluster-events` only and binds no
channels. The console therefore installs unusable: `Wired=False` on its own
source, no composer, and answers that land in `status.runs[].result` where only
`kubectl` will find them.

The constraint that shapes every option: **a subchart reads no parent scope but
`global.`**, and **Helm cannot derive one value from another**. The console's
identity lives in top-level `console.*`; the bundle that ships the route cannot
see it.

## Goals / Non-Goals

**Goals:**

- An install that deploys the console can start a conversation in it with no
  further wiring.
- Disabling the console leaves no route claiming a source that does not exist.
- One fact about the console's identity, or — where that is impossible — a
  duplication that cannot silently drift.

**Non-Goals:**

- Not a values migration. `console.*` stays where operators set it.
- Not a new default component; the console is already on by default.
- Not a change to claim semantics. Sources stay shareable.

## Decisions

### The fact goes in `global.agentops.console`, because that is the only scope that works

`global.agentops.runtime.*` already exists for exactly this reason — the
substrate facts a subchart must see. The console's `signalSource` and `channel`
join it.

*Alternatives considered:*

- **Parent renders the claim itself.** The parent is where wiring is declared, so
  this is legitimate — but the route needs a profile and a toolset that the
  bundle renders, so the parent would name three foreign objects to avoid the
  bundle naming one.
- **A `pipelines.extraSignalSources` values key on the bundle.** Works, but the
  operator has to set it themselves, which is the workaround this change exists
  to delete.
- **Derive it.** Not available: values cannot compute from other values, and a
  parent cannot pass a computed value into a subchart.

### The duplication is real, so it is CHECKED

`global.agentops.console.signalSource` genuinely repeats
`console.signalSourceName | default console.name`. Nothing can remove that. What
can be removed is the possibility of it being wrong, so the parent's
`console.yaml` fails the render when the two disagree — in both directions, with
the exact value to set in the message.

The reverse direction is the one that matters most. With
`console.enabled: false` and the globals left in place, the bundle's route would
claim a source nothing renders. That is worse than it sounds: the claim does not
fail loudly, the Pipeline reports `Wired=True` for the sources that do exist, and
signals to the missing one are simply never delivered. This is the same reasoning
as `agentops.generatedSecretGuard` — fail the render rather than trust a note.

*The guard sits BEFORE the `if .Values.console.enabled` gate*, since the case it
most needs to catch is the one where the rest of that template does not render.

### The claim rides the route the bundle already ships

Not a second Pipeline. A second route claiming the console source would make
every unaddressed console message ambiguous — the chat lane refuses a bare
message with more than one claimant — so the turnkey install would ship with its
own composer broken in a new way. One route, two sources.

`channels` merges rather than replaces: an operator naming their own channel
keeps it, and the console is appended if it is not already there.

## Risks / Trade-offs

- **An upgrade that had `console.enabled: false` now fails to render** → That is
  the guard doing its job, and the message names the two values to clear. It is
  a loud, one-line fix at upgrade time instead of a silently dead lane. Called
  out in `CHANGELOG.md`.
- **The demo route now answers chat as well as cluster events** → Intended: it is
  the same agent with the same read-only reach, and the console is the surface
  the demo exists to show. The cost is the conversation the reader starts, which
  they asked for.
- **Two values still describe one thing** → Unavoidable given Helm's model;
  bounded by the guard and by both being documented in one place.
- **A route rendered with the console disabled and globals cleared loses its
  channel binding** → Correct: with no channel the conversation dispatches
  immediately and the answer lands in `status.runs[].result`, which is the
  documented no-channel behavior.
