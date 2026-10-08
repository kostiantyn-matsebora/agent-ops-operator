## Purpose

How a channel op is claimed exactly once across more than one manager
replica, how an abandoned claim is recovered, and what a replica that
cannot safely answer a poll does instead.

## ADDED Requirements

### Requirement: A channel op's claim is written only by the leader

Claiming an outbound channel op out to a polling adapter, and deciding
that a previously claimed op is abandoned, SHALL be performed only by the
manager replica currently holding the leader-election `Lease`.

The claim SHALL be recorded on the owning Conversation's `status`, keyed
per channel, as `status.threads[].claim` with the subfields `holder` (the
claiming replica's identity) and `claimedAt` (the time it was claimed).

#### Scenario: The leader claims an op

- **WHEN** the leader's reconciler finds a channel binding with no thread
  and no live claim
- **THEN** it writes a claim naming itself and the current time, then
  hands the op to the next matching poll

#### Scenario: A non-leader never writes a claim

- **WHEN** a manager replica that is not the current `Lease` holder would
  otherwise need to claim or revoke a claim
- **THEN** it performs neither write, regardless of what its own cached
  view of the Conversation shows

### Requirement: A claim expires if its holder goes silent

A claim SHALL carry a staleness bound. A claim older than that bound, on a
channel that still has no thread, SHALL be treated as abandoned.

The replica that clears an abandoned claim is always the current leader —
the same replica that is the only one ever allowed to write a claim in the
first place. Clearing one is therefore never a second, competing writer.

A leader failover SHALL NOT, on its own, require waiting for that bound.
The claim persists on the Conversation's `status`, so the new leader's
first reconcile pass finds it.

A claim whose holder is not the current leader is treated as abandoned at
once and cleared, without waiting for the staleness bound.

#### Scenario: A legitimately slow claim is left alone

- **WHEN** the current leader claimed an op less than the staleness bound
  ago, and the claiming adapter has not yet completed it
- **THEN** the leader does not reclaim or re-dispatch that op

#### Scenario: A stale claim is retried

- **WHEN** a claim is older than the staleness bound and its channel still
  has no thread
- **THEN** the current leader clears the claim and dispatches the op again

#### Scenario: A dead leader's claim is not waited out

- **WHEN** the leader that wrote a claim stops renewing its `Lease` and a
  new leader takes over
- **THEN** the new leader's reconcile pass finds a claim held by a
  former leader, clears it and dispatches the op immediately, without waiting for the
  staleness bound to elapse

### Requirement: A non-leader rejects rather than proxies a poll it cannot safely answer

A manager replica that is not the current leader, on receiving a poll for
a channel op it would need to claim, SHALL reject the request rather than
answer from its own state or forward the request elsewhere.

The rejection SHALL be distinguishable from "no work is currently
available," so a polling adapter can retry immediately rather than
waiting out its normal idle backoff.

#### Scenario: A non-leader rejects a poll

- **WHEN** an adapter's poll for a channel op is served by a manager
  replica that is not the current leader
- **THEN** that replica rejects the request, and the adapter retries
  rather than treating the rejection as "nothing to do"

#### Scenario: A conforming adapter tolerates the rejection

- **WHEN** a channel adapter built against this contract receives the
  rejection
- **THEN** it retries exactly as it already does on an empty, timed-out
  poll, never surfacing the rejection as an adapter-side error
