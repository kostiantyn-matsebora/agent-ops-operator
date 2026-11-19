## ADDED Requirements

### Requirement: CONVO_ID carries the conversation's identity

The `CONVO_ID` environment variable SHALL carry the conversation's `ID` (`conversation-identity`), and SHALL NOT carry its name. The runtime SHALL treat the value as OPAQUE: carried, compared for equality and passed back to `GET $CONTROL_URL/work` as the `convo` parameter, never parsed and never shown to a person as a name.

The manager SHALL resolve the `convo` parameter by `ID`. A value that is the `ID` of no live conversation SHALL be refused with an error naming that reason. It SHALL NOT be retried as a name.

The pod's own name, `POD_NAME`, is unchanged and is not the conversation's identity.

#### Scenario: The runtime polls with the identity

- **WHEN** a runtime pod starts for a conversation named `alert-node-down-ns-2`
- **THEN** `CONVO_ID` holds that conversation's `ID` and its work poll sends it as `convo`

#### Scenario: A pod of a deleted conversation is refused

- **WHEN** a runtime pod survives its conversation's deletion and polls with the old `ID` after a new conversation took the same name
- **THEN** the manager refuses the poll with a reason naming the unknown conversation, and hands it none of the new conversation's work

#### Scenario: A runtime that treats the value as opaque needs no change

- **WHEN** a conformant runtime is run against a manager that sends the `ID`
- **THEN** it conforms unchanged
