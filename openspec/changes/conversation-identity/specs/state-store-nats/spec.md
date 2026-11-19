## MODIFIED Requirements

### Requirement: Deletion, identity and expiry follow the contract
The record's identity, creation time, finalizers and deletion marker SHALL be
stored in the entry's value, minted by the backend on create, returned from that `Create` in the record as stored, and read-only to
a caller. A record past its expiry SHALL be invisible to `Get` and `List` and
SHALL be delivered as a delete on the feed. A sweep SHALL remove it from the
bucket.

#### Scenario: A held record is marked, not removed
- **WHEN** a record with a finalizer is deleted
- **THEN** it stays readable with its deletion time set and is removed by the
  `Update` that leaves its finalizers empty

#### Scenario: Create returns what the entry holds
- **WHEN** a record is created
- **THEN** the returned `ID` and `CreatedAt` equal those in the entry's value, and the returned revision is the entry's sequence

#### Scenario: An expired record disappears without a sweep having run
- **WHEN** a record is read after its expiry and before the sweep reaches it
- **THEN** `Get` reports not found and `List` omits it
