## [13.2.0] — 2026-08-27

**A burst that healed before its dwell closed no longer opens a
conversation.** Both dwell adapters re-check a matched event at the end of
its `for` window.

For a kind with no health predicate — every kind but Pod in the
cluster-events lane, every integration without a config-entry state in the
Home Assistant lane — the re-check asked *did it recur*.

A controller that retried every few seconds for half a minute and then
healed answered yes.

On the reference install that was most of the backlog: twelve conversations
about Longhorn snapshot purges that had healed two minutes before the
catch-all fired, each concluding "self-resolved, no action needed".

### Changed


- `signal-k8s-events` 0.4.4, `signal-ha` 0.2.4: the second verification rung
  asks whether the event was **still recurring as the window closed** — its
  last third, floored at thirty seconds, derived from the window actually
  waited so escalation keeps the proportion. A burst that went quiet before
  that is dropped as churn. One still arriving is reported once, and its
  evidence names how long before the close the last event arrived.
- The Home Assistant re-check applies the same rule to the log's own count: a
  count that rose early in the window and stopped rising is a blip that
  healed, not an integration still failing.

**Trade-off, named.** A controller backing off to a retry period longer than
the closing window — ninety seconds under a three-minute rule — is dropped
at that deadline, and its next retry opens a fresh window.

The report is delayed by one window, not lost. Anyone preferring the
previous behaviour restates the rule with a shorter `for`. There is no
switch.

No configuration, CRD or RBAC change. `NodeNotReady` and the other
`for: "0"` reasons are unaffected. They are never re-checked — a scheduled
reboot is the time axis's job (`route.muteTimeIntervals`, see the
Kubernetes page).
