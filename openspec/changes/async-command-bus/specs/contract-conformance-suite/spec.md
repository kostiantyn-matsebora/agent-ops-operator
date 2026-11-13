## REMOVED Requirements

### Requirement: The retry budget is measured against the claim window
**Reason**: There is no claim window. The acknowledgement states a delivery TTL.

**Migration**: Read the replacement requirement below.

## ADDED Requirements

### Requirement: The retry budget is measured against the delivery TTL
The suite SHALL inject retryable backpressure, a `429` carrying a stated wait and transient `5xx` responses, into one operation's transport calls, through the local double the adapter is already pointed at.

It SHALL assert the adapter's total in-process time for that one operation stays strictly below the delivery TTL, reporting success or failure before the TTL ends.

The TTL judged against SHALL be the `deliveryTtlSeconds` the fake manager stated in its acknowledgement reply. `FakeManager` SHALL serve `GET /channel/ops`, `POST /channel/ops/{id}/ack` and `POST /channel/ops/{id}/done`, so the suite exercises the three-step contract.

#### Scenario: Absorbed backpressure reports inside the TTL
- **WHEN** injected `429` and `5xx` responses clear after a few retries
- **THEN** the operation is reported done, and its total elapsed time is below the stated TTL

#### Scenario: Unending backpressure is abandoned in time
- **WHEN** the injected backpressure never clears
- **THEN** the adapter reports the operation failed before the TTL ends, so the manager never has to fail it at the TTL

#### Scenario: An adapter must acknowledge before it acts
- **WHEN** the suite offers an op and the adapter acts without acknowledging
- **THEN** the implementation fails the suite
