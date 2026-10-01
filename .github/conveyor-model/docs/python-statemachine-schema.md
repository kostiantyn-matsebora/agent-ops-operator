# python-statemachine's native YAML schema

Reference for the schema `statemachine.io.load(path, format="yaml")` actually
accepts.

- **Source:** reverse-engineered from `statemachine/io/native.py` in the
  installed package (`python-statemachine==3.2.1`). The package ships no
  bundled example. The public docs page was not fetched during this spike.
- **Verified:** a real `load()` call in `spike.py` ran the
  `fix --pr:green--> merge` transition end to end.

**This is the library's OWN vocabulary, not ours — kept deliberately
separate.**

- Our hand-authored files (`station.yaml`, `loop.yaml`, `workflows.yaml`) use
  a flatter, easier-to-diff shape (`transitions: [{from, event, to}, ...]`)
  and are the source of truth.
- A converter (`to_native_schema()` in `spike.py`) turns ours into this one
  only at load time, for the engine and the diagram.
- **Why the two stay apart:** the native shape has no slot for an EXPLICIT
  no-op transition. `conveyor.py`'s `SKIP` means "considered, and defined as
  a no-op" — the totality test depends on that distinction — but the native
  format can only represent "absent," which collapses it.
- This document exists to keep the converter honest, and to answer "does
  this engine even support field X" without re-reading library source every
  time.

## Document shape

```yaml
name: station            # optional; our converter sets it from our own `machine:` field
description: ...         # optional, free text
datamodel: ...           # optional; a context dict for guard/action expressions
states:                  # REQUIRED. A MAPPING keyed by state id -- NOT a list.
  implement:
    initial: true         # exactly one state (at each nesting level) must set this,
                           # or the library picks the first key in the mapping
    transitions:
      - event: fire:archive
        target: archive
      - event: round:start
        target: fix
  fix:
    transitions: [...]
  done:
    final: true           # marks a terminal state
```

Only four top-level document keys are accepted:
`name`, `description`, `datamodel`, `states`. Anything else raises
`InvalidDefinition` — the parser rejects unknown keys rather than silently
ignoring a typo.

## State node keys

| Key | Meaning |
|---|---|
| `initial` | `true`/`false` (also accepts `"yes"`/`"on"`/`"1"` etc. as truthy strings) |
| `final` | marks a terminal state — our `terminal: [...]` list maps onto this |
| `parallel` | SCXML orthogonal regions — not used by anything of ours yet |
| `enter` / `exit` | actions to run on entering/leaving the state (string = callback name on the bound model, or a structured action node) |
| `transitions` | a list of transition nodes, scoped to this state |
| `invoke` | one or more CHILD STATECHARTS to run while in this state — **this is the mechanism for `conveyor.run` invoking `conveyor.implement`/`fix`/`archive` as nested workflows** |
| `states` | nested child states (true hierarchical/compound states) |
| `history` | shallow/deep history pseudo-states |
| `donedata` | only valid alongside `final: true` |

## Transition node keys

```yaml
- event: pr:green       # the event name that fires this transition
  target: merge          # the destination state id
  cond: "some_expr"      # guard: fires only if truthy
  unless: "some_expr"    # guard: fires only if falsy
  internal: false         # SCXML "internal transition" flag
  before: [...]           # actions run before the state change
  on: [...]               # actions run as part of the transition
  after: [...]            # actions run after the state change
```

**No `event:` means an "eventless" transition** — evaluated automatically
when the state is entered, which is not a shape our tables use (every one of
our transitions is event-driven), so the converter always sets it.

## Guards and actions: the restricted-AST boundary

- `cond:` / `unless:` / `expr:` fields are **strings**, compiled by a
  restricted-AST evaluator by default (`load(..., trusted=False)`, the
  default). Comparisons, attribute/index reads, boolean logic — **no
  function calls**. `load(..., trusted=True)` lifts this to arbitrary
  Python, which we do not want: it would let a YAML file execute code,
  defeating the entire point of keeping the declaration inert.
- `enter:` / `exit:` / `before:` / `on:` / `after:` accept either:
  - a **bare string** → treated as a callback name looked up on the bound
    model object at run time (this is our `registry.yaml` action-by-name
    hook point)
  - a **structured action mapping** (`assign`, `raise`, `log`, `if`,
    `foreach`, `send`, `cancel`, `script`) — an SCXML-parity mini-language
    for simple in-machine effects. `script:` is rejected unless
    `trusted=True`. Everything else here still runs under the restricted
    evaluator.

**Consequence for our design:**

- Every one of our `guard:` names in `workflows.yaml`
  (`fix.may_place`, `fix.may_carry`, `archive.may_fire`, ...) calls a real
  Python decision function (`conveyor.py`'s `fire`, `carry_fix`,
  `carry_archive`, etc.) that reads GitHub facts no restricted expression
  could reach.
- Those guards CANNOT be declared as `cond:` strings inside this schema.
  They have to be evaluated **before** calling into the loaded chart, by our
  own orchestrator, which then fires only the already-decided event.
- The chart itself only ever sees `sm.send("pr:green")`, never "decide
  whether to send pr:green."
- This matches the split `conveyor.py` already has: decisions upstream, pure
  table downstream. It is why `registry.yaml` keeps guards and actions as
  *our* names, resolved by *our* orchestrator, rather than pushing them into
  this schema's `cond:`/`enter:` fields.

## What this means for the one real nested case

**CORRECTED.** An earlier version of this section described
`conveyor.run`'s `running` state conditionally invoking
implement/fix/archive as children.

- That never matched `conveyor.py` — see `workflows.yaml`'s and
  `triggers.yaml`'s own correction notes.
- `conveyor:run` is a standing fact, re-checked by three separate decision
  functions at three different real GitHub events.
- It is not a parent state that invokes anything.

**The one place composition is real:** `conveyor.fix`'s `running` state
contains the ENTIRE `loop` machine (`loop.yaml`) for as long as the fixing
round-by-round cycle runs.

`invoke:` on a state is SCXML's own mechanism for exactly this — "while in
this state, run another statechart as a child." `conveyor.fix`'s chart
could declare:

```yaml
states:
  running:
    invoke:
      - id: loop_child
        src: loop.yaml   # or an inline `content:` mapping
```

This was not exercised in the spike. The spike only covered the flat
station/loop case.

The Mermaid renderer (`render_diagram.py`) already draws this relationship
as a plain composite state (`state running { ... }` containing the loop
machine's own states), never SCXML's `invoke:`. Worth checking whether that
rendering choice and an eventual `invoke:`-based engine implementation
actually agree on the semantics, before committing to either.

It should be its own follow-up spike before committing to it as the nesting
mechanism: `invoke` in SCXML also carries semantics (autoforwarding events,
`finalize`, done-data) we have not yet decided we need.

## Confirmed by the spike (`spike.py`, 2026-10-01)

| Claim | Result |
|---|---|
| `load()` accepts our table shape after conversion to the native mapping-of-states form | PASS |
| `ChartClass(start_value="fix")` forces the starting state from an external value (our GitHub-label read) | PASS |
| `sm.send(event)` transitions correctly per the declared table (`fix` + `pr:green` → `merge`) | PASS |
| Two independently constructed instances do not share state | PASS |
| `current_state` property | **deprecated** — use `configuration` in real code |

## Open questions for a follow-up spike

1. **Superseded by auditing against the real code.** An earlier draft of
   this question assumed `conveyor.run` "invokes" implement/fix/archive
   through some composition mechanism, which never existed in `conveyor.py`
   — see `workflows.yaml`'s and `triggers.yaml`'s own correction notes.
   `conveyor:run` is a standing fact three separate decision functions
   (`fire`, `carry_fix`, `carry_archive`) independently re-check at three
   different real GitHub events. Whether `invoke:` (SCXML's composition
   primitive) is still useful for the one real nesting case —
   `conveyor.fix`'s `running` state containing the whole `loop` machine —
   is still open, but the broader "run invokes the other three" framing is
   not something to spike further.
2. Whether `[validation]` extra's JSON-Schema `validate=True` path is worth
   turning on in CI once our converter is final, so a malformed generated
   document fails loudly instead of at `load()` time.
