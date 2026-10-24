# adapter-rendered-messages — delta

## MODIFIED Requirements

### Requirement: Adapters declare the contract version they speak

`GET /channel/ops` SHALL require the polling adapter to declare the outbound
contract version it supports, and SHALL reject an absent or unsupported
declaration with 400 naming what is expected, rather than delivering operations
it cannot interpret.

THE VERSION DOES NOT MOVE FOR THE BLOCK GRAMMAR. No field is added and no field
changes meaning: a body that was markdown is now markdown plus a grammar, read
by the same component that already read the markdown.

An adapter that does not implement the grammar renders the tags literally.
That is prevented by `AgentProfile.spec.outputFormat`, a REQUIRED field with
no default.

A profile declaring `none` never emits a tag, and an install sets `blocks`
only once its adapters understand the grammar. The compatibility boundary is
the PROFILE, not the wire.

#### Scenario: Outdated adapter fails loudly

- **WHEN** an adapter built against the string-valued contract polls for
  operations
- **THEN** the manager responds 400 naming the required contract version, and no
  empty messages are posted

#### Scenario: Current adapter polls normally

- **WHEN** an adapter declares the supported version
- **THEN** operations are delivered as before

#### Scenario: An adapter that never learns the grammar keeps working

- **WHEN** an adapter has no parser for block tags
- **THEN** it serves profiles that have not opted in and sees the prose it always
  saw, with no version to negotiate and nothing to refuse
