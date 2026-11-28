## ADDED Requirements

### Requirement: A workspace subdirectory is named by the conversation's ID

Where a workspace claim is used, the subdirectory a conversation's runtime pod mounts SHALL be named by the conversation's `ID` and SHALL NOT be named by its name. A conversation created later under a reused name SHALL therefore mount a different subdirectory from the earlier one and SHALL NOT find its files.

This requirement names the subdirectory only. Where the checkout is mounted, and what is inside the subdirectory, are specified elsewhere and are unaffected. `/data/workspace` and any later workspace path keep their own requirements.

#### Scenario: A reused name gets a fresh directory

- **WHEN** a conversation is deleted and another is created under the same name against the same workspace claim
- **THEN** the second pod mounts a subdirectory named for its own `ID`, and the first conversation's tree is not visible in it

#### Scenario: A restart keeps the directory

- **WHEN** a runtime pod for an open conversation is deleted and recreated
- **THEN** the new pod mounts the subdirectory named for the same `ID`
