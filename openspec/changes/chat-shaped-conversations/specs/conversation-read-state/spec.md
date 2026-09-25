## MODIFIED Requirements

### Requirement: The watermark only moves forward and never past the present
A reported watermark at or before the value already stored SHALL be treated as a
no-op: it SHALL NOT be written, SHALL NOT be an error, and SHALL be reported as
skipped.

Where a reader is named, "already stored" SHALL mean that READER's watermark, so
one reader's report SHALL NOT be skipped because another reader is further
ahead.

A reported watermark ahead of the manager's own clock SHALL be clamped to the
manager's current time.

A report MAY instead ask for a REWIND:

- it SHALL name a reader and a time
- it SHALL set that reader's own entry to the time, even where it is earlier than the stored one
- it SHALL never move the channel-wide mark
- naming no reader, it SHALL be refused
- the clamp SHALL still apply

#### Scenario: A stale client cannot un-read a thread
- **WHEN** one client reports a thread read up to T2 and a second client with a stale view then reports the same thread read up to an earlier T1
- **THEN** the stored watermark remains T2 and the second report is skipped

#### Scenario: A skewed clock cannot mark the future read
- **WHEN** a client reports a watermark hours ahead of the manager's clock
- **THEN** the stored watermark is the manager's current time, and activity arriving after it is still unread

#### Scenario: Re-reporting an unchanged watermark writes nothing
- **WHEN** a thread whose watermark already covers its latest activity is reported read again
- **THEN** no status patch is issued and the report is skipped

#### Scenario: A reader rewinds their own mark
- **WHEN** a reader whose entry is at T2 asks for a rewind to T1
- **THEN** that reader's entry is T1, the channel-wide mark is unchanged, and every other reader's entry is unchanged

#### Scenario: A rewind with no reader is refused
- **WHEN** a rewind names no reader
- **THEN** it is refused and nothing is written
