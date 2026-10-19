## Context

`chart/templates/coordinator.yaml` builds three lists before rendering the
`Coordinator` object: `$sources` (signal claims), `$channels` (escalation
binding), and `$agents` (the outbound roster).

Only `$agents` starts from `agentops.coordinatorContributions`, every
enabled bundle's own capability. `$sources` and `$channels` are derived
independently and stay untouched by this change (proposal.md - Impact).

The reaper fold-in (`$r.enabled`) appends one more entry to `$agents` and
one more name to `$sources`, after the contributions are read. That
ordering — derive, then fold in the reaper — is what this change preserves.

The override point sits where the derivation sits, so the reaper's
`append` runs unchanged whether `$agents` started from contributions or
from `coordinator.agents`.

The final render block only ever emits `capabilityRef` today. Every
existing source of `$agents` entries — contributions, the reaper — is a
plain member.

`Coordinator.spec.agents[]` (`coordinator-model`) already allows
`coordinatorRef` entries. This feature only gives the chart a second way
to produce an entry that satisfies the CRD's existing "exactly one of" CEL
rule.

## Goals / Non-Goals

**Goals:**
- One values key, `coordinator.agents`, that either stays out of the way
  (empty) or fully replaces the derived roster (set).
- Support both ref kinds from values, matching what the CRD already
  accepts.
- Fail the render on a malformed override entry, rather than shipping a
  manifest the API server then rejects.

**Non-Goals:**
- No merge of an override with the derived roster. Proposal.md's
  Non-Goals already excludes a targeting/routing mechanism. A partial
  override is the same problem in miniature: which bundle entries survive
  alongside which stated ones, with no stated use case to settle it.
- No change to how `$sources`, `$channels`, or `$c.limits` are computed.

## Decisions

**Replace, never merge.** When `coordinator.agents` is set, it becomes
`$agents` outright. `$contrib.agents` is not consulted at all.

A merge would need a dedup key (name? capability ref?) and a rule for which
side wins a collision. That is machinery with no named use case: the one
motivating scenario (proposal.md - Why) wants a stated tree, not a patched
one.

An install that still wants the derived bundles alongside stated entries
can list them explicitly. `agentops.coordinatorContributions` is already
read elsewhere in this template, and nothing stops a values file from
reproducing its entries by hand.

**Normalize to the existing internal shape.** Every current `$agents` entry
(contributions, the reaper's `append`) is a dict of `name` / `description`
/ `capability`. A `coordinator.agents` entry carries `name` / `description`
plus `capability` or `coordinator` — already the right field names. No
transform is needed. A values entry is used as-is as one `$agents` element.

The final render block switches on which of the two fields is present:

```
{{- range $agents }}
- name: {{ .name }}
  description: {{ .description | quote }}
  {{- if .coordinator }}
  coordinatorRef:
    name: {{ .coordinator }}
  {{- else }}
  capabilityRef:
    name: {{ .capability }}
  {{- end }}
{{- end }}
```

A contributions-derived or reaper entry has no `coordinator` key, so
`.coordinator` resolves to Helm's zero value (falsy), and that entry takes
the `capabilityRef` branch exactly as it does today.

That fallthrough is what keeps the empty-override path byte-identical. No
existing dict literal needs a new, empty `coordinator` key added to it.

**Validate shape at render time, mirroring the CRD's own CEL rule.** Before
`$agents` is assigned from the override, range over `coordinator.agents`
and `fail` naming the entry if it carries both `capability` and
`coordinator`, or neither.

The CRD would refuse the object anyway. Failing at `helm
template`/`install` gives a clearer error than a `Ready=False` object an
installer has to go find, and it follows the chart's existing render-time
guard style (`defaultRuntimeGuard` and the other guards in
`.claude/rules/chart.md` and `.claude/rules/invariants.md`) rather than a
new one.

**Override point sits above the reaper fold-in, not below.** `$agents` is
reassigned right after the `$contrib` read, before `{{- if $r.enabled }}`.
The existing fold-in code is untouched and still runs against whichever
roster — derived or overridden — is current.

The reaper fold-in also needs no `coordinator` key of its own. It only
ever appends a plain member, exactly as before.

## Risks / Trade-offs

- An override naming a non-existent AgentCapability or Coordinator is
  caught by `coordinator-model`'s existing `Ready=False` handling, not by
  the chart. The chart validates only the values SHAPE — which field is
  set — never that the named object exists. It has no cluster to check
  against at render time, the same reason a `serviceAccountName` is a
  reference, not a creation (`.claude/rules/wiring.md`).
- A reader could expect `coordinator.agents` to add to the roster rather
  than replace it. Mitigation: `docs/configuration.md` states the
  replacement plainly, and the render-test suite asserts the replacement
  directly, so a drift toward "merge" is caught.
