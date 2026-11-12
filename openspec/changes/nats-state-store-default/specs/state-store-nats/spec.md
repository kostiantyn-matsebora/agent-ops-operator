## ADDED Requirements

### Requirement: The `nats` backend implements every field of the inventory
The `nats` backend SHALL implement every field of `state.Stores` and SHALL
pass the generic conformance suite and its bench unchanged. No scenario SHALL
be skipped, weakened or added for it alone.

#### Scenario: A kind cannot be left out
- **WHEN** a field of the inventory is not wired by the `nats` backend
- **THEN** the conformance run fails and names that field

#### Scenario: The suite is the same suite
- **WHEN** the `nats` backend runs the conformance suite
- **THEN** it runs the scenarios `kube` runs, against an in-process
  JetStream server, in `go test ./...` with no cluster

### Requirement: A revision is the server's per-key sequence
The `nats` backend SHALL use the KV entry revision as the opaque `Revision`.
`Update` and `DeleteAt` SHALL be conditional on it, and a lost race SHALL
surface as the contract's conflict error, never as the client library's.

#### Scenario: A stale write is refused
- **WHEN** two writers update one record from the same revision
- **THEN** the first succeeds, the second receives the conflict error and
  the record holds the first writer's value

#### Scenario: A create never overwrites
- **WHEN** a record is created under a key that already holds one,
  including one marked for deletion
- **THEN** the already-exists error is returned and nothing is written

### Requirement: Deletion, identity and expiry follow the contract
The record's identity, creation time, finalizers and deletion marker SHALL be
stored in the entry's value, minted by the backend on create and read-only to
a caller. A record past its expiry SHALL be invisible to `Get` and `List` and
SHALL be delivered as a delete on the feed. A sweep SHALL remove it from the
bucket.

#### Scenario: A held record is marked, not removed
- **WHEN** a record with a finalizer is deleted
- **THEN** it stays readable with its deletion time set and is removed by the
  `Update` that leaves its finalizers empty

#### Scenario: An expired record disappears without a sweep having run
- **WHEN** a record is read after its expiry and before the sweep reaches it
- **THEN** `Get` reports not found and `List` omits it

### Requirement: The feed resumes from a revision
`Watch` SHALL deliver an at-least-once feed that resumes strictly after the
revision a consumer names, and SHALL emit a bookmark while idle so a resume
point does not age out of the server's history. The channel SHALL close when
the server connection is lost for good, as the contract says.

#### Scenario: Resume after a restart
- **WHEN** a consumer resumes from the revision of the last event it handled
- **THEN** it receives every later change once or more and none earlier

#### Scenario: A server restart is survived
- **WHEN** a state server restarts and the manager reconnects
- **THEN** feeds resume from their last revision and no record is lost

### Requirement: Indexes are served from a mirror, reads are not
The backend SHALL serve `List` from an in-memory mirror of the bucket kept
current by a watch, evaluating the declared index extractors over it.
`Get`, `Update` and `DeleteAt` SHALL read the server, never the mirror, so a
compare-and-swap precondition is never a guess. Until the mirror has caught up
with the server at construction, `List` SHALL block rather than answer from a
partial set.

#### Scenario: A list sees a write that completed before it
- **WHEN** a record is created and a list for its index is issued after the
  create returned
- **THEN** the list includes the record

#### Scenario: A mirror never answers a precondition
- **WHEN** the mirror is behind the server and a caller updates from a
  revision read through `Get`
- **THEN** the write is judged against the server's revision

### Requirement: Appended items and blobs have their own homes
Runs SHALL be stored as one entry each under the parent's key prefix. `Grow`
SHALL apply the declared merge under a compare-and-swap retry and never
surface a conflict. `GetWithItems`, `ListWithItems` and `WatchWithItems` SHALL
return a record with the items as they stood at one revision of the parent.
Blobs SHALL live in a JetStream Object Store, so a payload larger than a KV
value is stored whole.

#### Scenario: Concurrent grows commute
- **WHEN** two edits grow one run at the same moment
- **THEN** both are applied and the result equals applying them in either
  order

#### Scenario: A large input is stored whole
- **WHEN** a blob larger than the server's maximum message size is put
- **THEN** it reads back byte-identical

### Requirement: A `kube` install's records are imported before the manager serves
When the `nats` backend starts over an empty bucket set and the Kubernetes API
holds conversation, run, input, signal, cooldown, receipt or adapter-state
records, the manager SHALL copy them into the store before it reports ready.
Adapter state is the key-value annotations on `Channel` and `SignalSource`.

- The import SHALL start only on the replica holding the leader Lease.
- Each copied record SHALL carry the source's `resourceVersion` as
  `importedFrom`.
- The import SHALL repeat a pass at an interval. A pass SHALL create each
  missing record and treat already-exists as done.
- A pass SHALL re-copy a record whose source `resourceVersion` differs from
  `importedFrom`, only if the store record has had no write of its own. It
  SHALL never overwrite a record the new version has written.
- The passes SHALL continue until no manager pod of another version exists.
  One final pass SHALL then run, and the marker SHALL be written last.
- The import SHALL NOT run again once the marker exists.
- The import SHALL NOT modify or delete the Kubernetes objects it read.

#### Scenario: An upgrade keeps its conversations
- **WHEN** an install with live `kube` conversations upgrades to the default
  backend
- **THEN** every conversation, its runs, its queued inputs and each source's
  window and counters are readable through the store after the first start

#### Scenario: An interrupted import is repaired
- **WHEN** the manager is killed after importing half the records
- **THEN** the next start completes the import and no record is duplicated

#### Scenario: A non-leader never imports
- **WHEN** a replica that does not hold the Lease starts over empty buckets
- **THEN** it copies nothing and reports not ready until the marker exists

#### Scenario: A late write by an old replica is caught
- **WHEN** an old-version replica creates a Signal and appends an input to a
  Conversation after the first pass
- **THEN** both are present in the store after the final pass

#### Scenario: A new-version write is never overwritten
- **WHEN** the new version has written a record that an old replica also changed
- **THEN** the final pass leaves the new version's value

#### Scenario: A fresh install imports nothing
- **WHEN** the backend starts over empty buckets and an empty API
- **THEN** it writes the marker and no import work happens

#### Scenario: The source is left intact
- **WHEN** the import finishes
- **THEN** the Kubernetes objects are byte-identical to before it ran

### Requirement: The server is a bundle the manager alone reaches
The chart SHALL run the state server as the `nats` bundle: a JetStream
enabled `nats-server` with a PersistentVolume for its store directory, a
liveness and readiness probe, and a credential the manager receives as
`valueFrom`. The manager SHALL be the only client.

#### Scenario: The manager reads no secret
- **WHEN** the manager's pod is rendered for the `nats` backend
- **THEN** the credential reaches it by `valueFrom` and the manager holds no
  `secrets` verb

#### Scenario: Not Ready until the store is reachable
- **WHEN** the state server is unreachable
- **THEN** the manager reports not ready and creates no conversation

### Requirement: The state server has no single point of failure by default
The bundle SHALL run three servers by default, clustered, and every bucket and
object store SHALL be created with three replicas. A bucket's replica count
SHALL be set at creation by the manager from the chart, never changed by it
afterwards.

The bundle SHALL ship a PodDisruptionBudget allowing one server down, and SHALL
spread the servers across nodes by a preference, not a requirement, so a
single-node cluster still schedules all three.

The bundle SHALL accept one replica as an explicit choice, in which case the
render SHALL name the loss of the guarantee in the chart notes.

#### Scenario: Losing one server loses nothing
- **WHEN** one of three servers is killed during a conversation
- **THEN** reads and writes continue, no record is lost and the manager stays ready

#### Scenario: A voluntary disruption is bounded
- **WHEN** a node drain would take a second server down
- **THEN** the PodDisruptionBudget holds it until the first is back

#### Scenario: A single-node cluster still schedules
- **WHEN** the bundle is installed on a one-node cluster
- **THEN** all three servers schedule and the render does not fail

#### Scenario: Changing the replica count after install is warned and left unapplied
- **WHEN** `nats.replicas` is changed after the buckets exist
- **THEN** the manager keeps the existing buckets unchanged, logs a warning
  naming the bucket and both counts, and the docs give the rebuild path

#### Scenario: One replica is explicit
- **WHEN** the adopter sets `nats.replicas` to 1
- **THEN** buckets are created with one replica and the notes say the store is
  a single point of failure

### Requirement: The backend is proven on a real cluster and in a real browser
The `nats` backend SHALL be covered by the system tier and the console browser
tier, not only by the in-process conformance suite. What a cluster decides
(pod loss, quorum, a drain, a rolling image upgrade, a volume, the network
policy, the import against real custom resources) SHALL be a system lane. What a user sees through the
console across those events SHALL be an `e2e-live` spec.

Both tiers SHALL run against the default install, so the default is the thing
tested. The `kube` pin SHALL be exercised by a system run of its own.

#### Scenario: The whole pack runs on the default
- **WHEN** the system pack runs against a default install
- **THEN** every existing lane passes on the `nats` backend without a test
  being edited

#### Scenario: A lost server is survived in a conversation
- **WHEN** one state server pod is deleted while a conversation is working
- **THEN** the conversation completes, no input is lost and the manager never
  reports not ready

#### Scenario: Quorum loss is an outage, not a loss
- **WHEN** two of three state servers are down and then restored
- **THEN** work is held while they are down and `/state/*` reports the outage
  instead of answering stale, and every record written before the loss is
  readable after

#### Scenario: A drain is bounded by the budget
- **WHEN** a node drain would evict a second state server
- **THEN** the eviction waits and the store stays writable

#### Scenario: An upgrade from kube keeps its conversations
- **WHEN** an install on `kube` with a live conversation, a signal and a
  cooldown window is upgraded to the default
- **THEN** all three are readable through `/state/*` and the console after the
  first start, and the custom resources are unchanged

#### Scenario: A rolling server image upgrade keeps writes working
- **WHEN** the state servers are rolled to a new image one at a time
- **THEN** two servers stay up throughout and no write fails

#### Scenario: The console renders across an upgrade from kube
- **WHEN** an install on `kube` is upgraded to the default while the console
  shows a conversation
- **THEN** the page keeps rendering and the thread's unread state survives

#### Scenario: The console renders across a server loss
- **WHEN** a state server is deleted while the console shows a conversation
- **THEN** the page keeps rendering and the thread, its unread state and its
  runs are intact after the server returns

#### Scenario: The kube pin passes the smoke
- **WHEN** the smoke tier runs with `stateStore.backend` set to `kube`
- **THEN** it passes and the install renders no state server

### Requirement: The state servers' volumes outlive an uninstall
The state servers' volumes SHALL outlive an uninstall of the chart, so a reinstall finds its data where it was.

#### Scenario: An uninstall keeps the state servers' volumes
- **WHEN** the chart is uninstalled on the default backend
- **THEN** the state servers' volumes remain, so a reinstall finds its data where it was
