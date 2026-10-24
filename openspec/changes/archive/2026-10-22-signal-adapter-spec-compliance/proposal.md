# Signal adapter spec compliance

## Why

An audit of all six shipped signal adapters (`cron`, `alertmanager`,
`k8s-events`, `ha`, `pulse`, `telegram`) against signal-adapter contract
v2.0.0 and each adapter's own spec found four live defects, one shared
undocumented behaviour, and spec pages that contradict themselves or omit
`pulse` entirely.

The live defects:

- `signals/pulse` never pushes a rotated signing secret to Pulse — the
  idempotence comparison strips it, so after a rotation every inbound
  delivery is 401'd until an unrelated field happens to change.
- `signals/alertmanager` and `signals/k8s-events` tell the operator to set
  `SignalAdapter.spec.kubernetesAccess` — a field deleted from the CRD, so
  the one remediation message built to heal the condition never can.
- `signals/k8s-events` stops examining events past the 50th emitting match
  on a relist pass and can advance the cursor past an unexamined event — a
  permanent, unreported loss.
- `signals/ha` drops a still-failing record as quiet churn when the system
  log becomes unreadable mid-window — the spec's fail-open rule, inverted
  exactly during an HA outage.

## What Changes

- Fix each adapter where it violates `signal-adapter-contract`,
  `signal-adapter-lifecycle` or its own spec. The four live defects first.
- Then the rest: unreported shadowed-rule warnings and event-driven mute
  visibility (k8s-events), the silent malformed `register` block, the
  placeholder-URL registration and the `send_resolved` zero value
  (alertmanager), unsigned inbound when registration config is incomplete
  and a 404 that names no source (pulse), the unpruned Ready-report state
  (cron), and the rune-splitting title truncation (telegram).
- Settle the refractory window both event adapters (`ha`, `k8s-events`)
  carry: a post-emit quiet period on an emitted id, which ADP-3's letter
  forbids. The delta blesses it as a narrow named allowance rather than
  removing a deliberate chatter bound. **This touches
  `docs/contracts/signal-adapter.md`, which needs explicit approval**
  (`signal-rules.md`) — approving this proposal is that approval.
- Resolve the spec-internal contradictions so there is one rule to comply
  with: the lifecycle spec's stale `spec.kubernetesAccess` sentence, the
  suppression spec's dead fail-open rung, the HA spec's two id scopes, the
  alertmanager spec's stale `register: {name, namespace}` required claim,
  and the pulse spec's unsatisfiable token-rotation heal.
- Make `pulse` a shipped adapter everywhere the lists still say five: the
  contract's conformance-carry, version-claim and severity-sender
  enumerations, the
  conformance suite (its binary never runs there), the integration page's
  missing version claim, and the repo rules files.
- Purge retired vocabulary stated as current claims: manager "grouping"
  comments (cron, telegram, pulse), current-tense "fingerprint" comments
  (ha, k8s-events, pulse), and the deleted-field error texts.
- Extend the conformance suite to pin the fixed behaviours it can reach.
  Behaviours it cannot reach are pinned by unit tests in each component.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `signal-adapter-contract`: add `signals/pulse` to the shipped-adapter
  enumerations, and state the one narrow post-emit refractory allowance the
  event adapters rely on (an ADP-3 carve-out, named and bounded).
- `signal-adapter-lifecycle`: remove the stale "optional
  `spec.kubernetesAccess`" clause that contradicts the same spec's own
  deletion statement.
- `alertmanager-signal-adapter`: correct the `register` block contract —
  no `name` field exists, `namespace` defaults to the adapter's own — and
  require malformed blocks and placeholder-URL registration to degrade to
  instructions.
- `pulse-signal-adapter`: give signing-secret rotation a heal path the
  redacting Pulse API permits, scope the token-rotation heal to what
  kubelet-resolved `envFrom` can deliver, and pick one retry wording.
- `ha-signal-adapter`: resolve the id-scope contradiction in favour of the
  dwell-path mint, make the surfaces table true of the shipped defaults,
  and state the unreadable-log fail-open so the quiet-drop rung cannot
  swallow it.
- `k8s-event-suppression`: rework the dead fail-open rung against its own
  scenarios, require mute visibility to survive a lane going quiet, and
  make the relist batch clip non-silent.
- `k8s-events-signal-adapter`: bound the per-pod fallback id when a tracked
  kind's workload is momentarily unresolvable.
- `chat-signal-origination`: name the selection (callback-query) id as a
  minting identity beside the update and message ids.
- `contract-conformance-suite`: run the pulse binary, and add assertions
  for the fixed behaviours the fake manager can observe.

## Impact

Code:

- `signals/pulse/` — signing-secret rotation, 404 body, unsigned-inbound
  guard, stale comments.
- `signals/alertmanager/` — remediation text, `send_resolved` omission,
  placeholder-URL guard, malformed-register degradation, stale comments.
- `signals/k8s-events/` — remediation text, shadowed-rule reporting,
  fail-open rung, mute sweep, relist clip, fallback id, stale comments.
- `signals/ha/` — unreadable-log fail-open, payload bound, stale comments.
- `signals/cron/`, `signals/telegram/` — reported-map pruning, rune-safe
  title truncation, stale comments.
- `platform/manager/test/conformance/` — pulse binary plus new assertions.

Reference docs made untrue and updated here:

- `docs/contracts/signal-adapter.md` — the refractory allowance, changelog
  entry, version moves to 2.2.0 (additive — 2.1.0 landed meanwhile with
  the optional `severity` field).
- `docs/integrations/pulse.md` — the missing specification version claim.
- `docs/integrations/home-assistant.md`, `kubernetes.md`, `prometheus.md`,
  `telegram.md` and `docs/guides/signal-adapter.md` (cron's claim) — each
  adapter's claimed version moves with its restored conformance.
- `docs/CHANGELOG.md` — behaviour fixes adopters can observe.
- `.claude/rules/signal-rules.md` and `.claude/rules/structure.md` — the
  five-adapter lists gain `pulse`.

Adopter site: the landing page, introduction, getting-started, installation
and the remaining guides describe none of the defective behaviours —
checked, not skipped. `docs/guides/signal-adapter.md` is the one adopter
page touched, for its version claim.
