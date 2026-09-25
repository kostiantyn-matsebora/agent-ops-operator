## Purpose

Gives every adopter-authored kind, every severity value and every adopter-declared type an icon a surface can draw, resolved by one ladder so an object is never drawn without one and the chart's own objects always declare theirs.

## ADDED Requirements

### Requirement: Every adopter-authored kind carries an optional icon
`spec.icon` SHALL exist on Pipeline, SignalSource, Channel, AgentProfile, AgentRuntime, SignalAdapter, ChannelAdapter, MCPToolset and MCPConfig, with one meaning.

- Four forms, told apart by nothing manager-side: `aops:<name>` (the built-in set), `mdi:<name>` (a named public set), a URL, or an emoji.
- It is interface metadata. Nothing routes on it, no condition reads it, and removing it changes where no signal goes.
- Conversation and ConversationInput SHALL carry no icon field. They are materialised state and inherit their Pipeline's.
- A composition kind another change adds SHALL carry the same field with the same meaning.

#### Scenario: A source declares an emoji
- **WHEN** a SignalSource sets `spec.icon: 🔥`
- **THEN** every surface that names the source draws the emoji beside it, and ingest is unchanged

#### Scenario: The field is inert to wiring
- **WHEN** a Pipeline's icon is changed
- **THEN** no conversation is re-wired and no condition changes

### Requirement: Icons resolve by one ladder
Wherever an object is drawn, its icon SHALL resolve in this order and stop at the first hit:

1. the instance's own `spec.icon`
2. the kind's built-in icon, `aops:<kind>`, one per kind above
3. nothing

- The built-in set SHALL hold an entry for every kind in the ladder, so an adopter's object is never drawn bare.
- The manager SHALL publish the resolved reference on the messages and listings it already publishes, so a surface resolves nothing itself.
- A surface that cannot draw a form SHALL omit it and fail nothing, as today.

#### Scenario: An undeclared icon falls back to the kind
- **WHEN** an adopter applies a Channel with no `spec.icon`
- **THEN** the topology draws it with the built-in channel icon

#### Scenario: A declared icon wins
- **WHEN** the same Channel later sets `spec.icon: mdi:slack`
- **THEN** surfaces that can draw a named set draw it, and Telegram omits it

### Requirement: Severity values ship with icons
Each of the four severity values SHALL have a built-in icon, `aops:severity-<value>`, present in every surface's built-in set.

- The signal card SHALL carry the severity's icon reference beside the value.
- A surface that prints only emoji SHALL have an emoji per value.

#### Scenario: A critical card is marked
- **WHEN** a `critical` signal opens a conversation
- **THEN** the console card and the Telegram card both lead with the critical mark

### Requirement: Type values take their icons from the chart
The chart value `global.agentops.signalTypes[]{name, icon}` SHALL declare an icon per type value, reaching the manager as configuration.

- The signal card SHALL carry the type's icon reference when the signal's `type` label names a declared type.
- An undeclared type SHALL render as its bare value, with no icon and no fault.
- The table is reversible by `helm upgrade` and is documented as a configuration value.

#### Scenario: A declared type is drawn
- **WHEN** values declare `{name: security, icon: mdi:shield}` and an alert carries `type: security`
- **THEN** the card carries `mdi:shield` beside `security`

#### Scenario: An undeclared type is bare
- **WHEN** an alert carries `type: billing` and no such type is declared
- **THEN** the card shows `billing` and no icon

### Requirement: Every CR the chart ships declares a built-in icon
Every agentops object the chart renders SHALL set `spec.icon` to an `aops:` reference, and a render test SHALL fail on one that does not.

- The built-in form is required because it needs no network and survives an air-gapped install.
- Adopter-authored objects are under no such obligation. The ladder covers them.

#### Scenario: A shipped object without an icon fails the test
- **WHEN** a bundle template renders a Pipeline with no `spec.icon`
- **THEN** the chart render test fails naming the object

#### Scenario: A shipped object with a URL icon fails the test
- **WHEN** a bundle template renders a Channel with `spec.icon: https://example.com/x.svg`
- **THEN** the chart render test fails naming the object and the required form
