## MODIFIED Requirements

### Requirement: Activity is served over replay and stream endpoints

The manager SHALL serve `GET /activity?since=<cursor>&limit=<n>` returning events after a cursor. It SHALL serve `GET /activity/stream` as SSE, delivering events as they occur, each carrying its cursor.

Both, and `POST /activity`, SHALL be authenticated with the adapter bearer scheme. A client reconnecting with a cursor older than the buffer's oldest SHALL receive an explicit resync signal rather than a silent gap.

The activity log is a per-process ring buffer with no cross-replica fan-out. A hop the reconciler emits only ever lands in the leader's own buffer, since reconciliation is leader-elected.

A manager replica that is not the current leader SHALL reverse-proxy `GET /activity`, `GET /activity/stream` and `POST /activity` to whichever replica the leader-election Lease currently names as holder. A client pinned to a non-leader would otherwise silently miss a leader-only hop for the life of its connection.

Unlike a channel op poll, a non-leader SHALL NOT refuse an activity request when the leader cannot be reached. It SHALL serve or record from its own buffer instead. A partial, locally-served view is strictly better than none for telemetry.

#### Scenario: Reconnect after a gap is explicit
- **WHEN** a client reconnects with a cursor that has been evicted
- **THEN** the response tells it to resync, and the client is expected to re-read snapshots rather than assume continuity

#### Scenario: Unauthenticated access is refused
- **WHEN** any of the three endpoints is called without a valid adapter token
- **THEN** the request is rejected with 401 and no events are disclosed

#### Scenario: A non-leader proxies an activity request to the leader
- **WHEN** `GET /activity`, `GET /activity/stream` or `POST /activity` is served by a manager replica that is not the current leader, and the leader is reachable
- **THEN** that replica forwards the request to the leader and relays its response, including a live SSE stream relayed as it arrives rather than batched

#### Scenario: An unreachable leader still answers, from local state
- **WHEN** a non-leader cannot resolve or reach the current leader
- **THEN** the request is served from, or recorded into, that replica's own activity buffer rather than refused
