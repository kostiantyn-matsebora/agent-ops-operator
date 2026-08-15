# conversation-read-state Specification

## Purpose
How far a conversation thread has been read: the per-channel watermark on
`Conversation.status.threads[]`, the per-identity overlay beside it, the
`POST /channel/read` contract verb the manager serves for both, and the
backfill rules that keep an upgrade and a new reader from inventing a backlog.

## Requirements
### Requirement: A conversation thread carries how far it has been read
`Conversation.status.threads[]` SHALL carry a read watermark per binding:
`readAt`, the point in the conversation's activity up to which that channel's
thread has been seen, and `readTracked`, marking a binding created after read
reporting existed.

Read state SHALL be per THREAD and therefore per CHANNEL. A conversation bound to
several channels SHALL carry an independent watermark for each, and reading it on
one channel SHALL NOT mark it read on another.

The watermark SHALL be written only by the manager. No adapter and no console
SHALL write it to the Kubernetes API directly.

#### Scenario: Each bound channel reads independently
- **WHEN** a conversation is bound to two channels and one of them reports its thread read
- **THEN** only that channel's binding carries the watermark, and the other binding is unchanged

#### Scenario: The manager owns the write
- **WHEN** a read is reported for a thread
- **THEN** the manager patches `Conversation` status and the reporting component issues no Kubernetes write

### Requirement: A thread records a per-identity watermark where the transport has one
A thread binding SHALL additionally carry `readers[]`, a bounded list of
`{key, readAt}`, recording how far an individual reader has seen that thread.

`readAt` on the binding SHALL remain the CHANNEL-WIDE mark, for transports that
cannot tell one reader from another. The per-identity list is an OVERLAY: a
channel that reports no reader keeps reporting only the channel-wide mark, and
SHALL remain fully conformant.

`key` SHALL be OPAQUE to the manager. The manager SHALL NOT interpret it,
SHALL NOT derive it, and SHALL NOT store any identity from which it was derived.

The list SHALL be bounded at 50 entries per binding, evicting the oldest
`readAt` first.

#### Scenario: Two readers on one thread read independently
- **WHEN** one reader reports a console thread read and a second reader has not
- **THEN** the conversation is read for the first and unread for the second

#### Scenario: A transport with no reader identity is unaffected
- **WHEN** an adapter reports a read without naming a reader
- **THEN** the channel-wide watermark advances, no reader entry is written, and the report succeeds

#### Scenario: The manager stores no identity
- **WHEN** a read is reported for a reader
- **THEN** the stored key is the opaque value supplied by the adapter, and no address or other identity appears anywhere on the object

#### Scenario: The reader list stays bounded
- **WHEN** a fifty-first reader reports a read on one binding
- **THEN** the entry with the oldest watermark is evicted and the new one is recorded

### Requirement: A reader with no watermark falls back to the channel's
A reader with no entry on a binding SHALL be treated as having the binding's
channel-wide `readAt`. This SHALL apply equally to a reader who has never
reported and to one whose entry was evicted.

Eviction SHALL therefore never present a reader with a backlog they cannot act
on — the same rule, for the same reason, as a binding that predates read
tracking being treated as read.

#### Scenario: A new reader inherits the channel's position
- **WHEN** a reader who has never reported a read views a conversation whose channel-wide watermark covers its latest activity
- **THEN** it is read for them, and no backlog is presented

#### Scenario: An evicted reader is not shown a backlog
- **WHEN** a reader's entry has been evicted and they view the conversation list
- **THEN** unreadness is answered from the channel-wide mark rather than from an absent watermark

### Requirement: The watermark only moves forward and never past the present
A reported watermark at or before the value already stored SHALL be treated as a
no-op: it SHALL NOT be written, SHALL NOT be an error, and SHALL be reported as
skipped. Where a reader is named, "already stored" SHALL mean that READER's
watermark, so one reader's report SHALL NOT be skipped because another reader is
further ahead.

A reported watermark ahead of the manager's own clock SHALL be clamped to the
manager's current time.

#### Scenario: A stale client cannot un-read a thread
- **WHEN** one client reports a thread read up to T2 and a second client with a stale view then reports the same thread read up to an earlier T1
- **THEN** the stored watermark remains T2 and the second report is skipped

#### Scenario: A skewed clock cannot mark the future read
- **WHEN** a client reports a watermark hours ahead of the manager's clock
- **THEN** the stored watermark is the manager's current time, and activity arriving after it is still unread

#### Scenario: Re-reporting an unchanged watermark writes nothing
- **WHEN** a thread whose watermark already covers its latest activity is reported read again
- **THEN** no status patch is issued and the report is skipped

### Requirement: A thread is unread when its activity is newer than its watermark
A bound thread SHALL be considered unread when the conversation's
`status.lastActivity` is after that binding's `readAt`, or when the binding
carries `readTracked` with no `readAt` at all.

A channel with no binding on a conversation SHALL NOT be considered to have an
unread conversation there: with no thread there is no watermark and no claim to
make.

#### Scenario: New activity makes a read thread unread again
- **WHEN** a thread is reported read and the agent then posts a further answer
- **THEN** the thread is unread again

#### Scenario: A bound but never-read thread is unread
- **WHEN** a thread binding is created and nothing has reported it read
- **THEN** it is unread

#### Scenario: An unbound channel has nothing unread
- **WHEN** a conversation has no binding on a given channel
- **THEN** that conversation is not unread for that channel

### Requirement: Bindings that predate read tracking are treated as read
A thread binding without `readTracked` SHALL be treated as READ. The manager
SHALL set `readTracked` on every thread binding it creates from that point on,
for every channel.

A binding predating the mechanism cannot be distinguished from one nobody has
read, so it is backfilled quiet — the same rule, for the same reason, as
`status.runs[].deliveryTracked`. Without it, an upgrade presents every
conversation in the namespace as new.

#### Scenario: Upgrading does not invent a backlog
- **WHEN** the manager is upgraded in a namespace holding conversations whose threads were bound before this mechanism existed
- **THEN** those threads are reported as read and no unread backlog appears

#### Scenario: A thread bound after the upgrade starts unread
- **WHEN** a new conversation's thread binding is created after the upgrade
- **THEN** the binding carries `readTracked` and is unread until it is reported read
