## MODIFIED Requirements

### Requirement: Manager runtime state is served by GET /status
The manager SHALL serve `GET /status`, adapter-token authenticated. It SHALL report at minimum:

- the build version
- the identity of the replica holding the leader lease
- runtime slots in use against `MAX_ACTIVE_CONVERSATIONS` and, separately, against `MAX_ACTIVE_MEMBERS`, each with its own waiting count
- per-adapter channel op queue depth, with the oldest queued op and the oldest claimed-but-uncompleted op
- active cooldowns

The runtime-slot Prometheus gauges SHALL carry a `pool` label naming the pool.

This state is in-memory by design. `OpQueue` holds pending and claimed ops in no Kubernetes object, so without this endpoint it is unobservable by any client.

#### Scenario: A stalled adapter is diagnosable
- **WHEN** an adapter claims ops and does not complete them
- **THEN** `/status` reports a growing claimed-but-uncompleted count for that adapter with the age of the oldest such op

#### Scenario: The runtime ceiling is visible
- **WHEN** runtime slots in use equal `MAX_ACTIVE_CONVERSATIONS`
- **THEN** `/status` reports the ceiling and the in-use count, so queueing is distinguishable from stalling

#### Scenario: Each pool reports its own ceiling
- **WHEN** the members pool is full while the conversations pool is not
- **THEN** `/status` reports members in use equal to `MAX_ACTIVE_MEMBERS` and conversations in use below `MAX_ACTIVE_CONVERSATIONS`

#### Scenario: Not a CR mirror
- **WHEN** `/status` is called
- **THEN** its payload contains no CR spec or status, only manager-held state

#### Scenario: Authenticated
- **WHEN** `/status` is called without a valid adapter token
- **THEN** the request is rejected with 401
