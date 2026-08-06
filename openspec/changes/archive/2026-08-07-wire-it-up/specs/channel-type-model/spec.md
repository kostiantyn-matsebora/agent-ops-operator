# channel-type-model — delta

## MODIFIED Requirements

### Requirement: Delivery instructions selected from channel metadata
Dispatch SHALL contain no channel-type-specific delivery wording. Work-unit delivery instructions SHALL be selected from `spec.delivery`: mode `result` (the default, also used for chat-less conversations) instructs that the printed answer is the deliverable captured via `/work/done`; mode `agent` injects the channel's `delivery.agentInstructions` text verbatim. **Conversations bound to more than one channel SHALL force `result` mode regardless of any bound channel's `delivery.mode` — the manager owns distribution on mirrored conversations; `agent` mode retains its meaning only for single-channel conversations.**

#### Scenario: Default mode needs no channel knowledge
- **WHEN** a work unit is dispatched for a conversation on a channel without `spec.delivery`
- **THEN** its prompt carries the printed-answer delivery instructions and no transport-specific steps

#### Scenario: Agent-direct channel supplies its own wording
- **WHEN** a single-channel conversation's channel sets `delivery.mode: agent` with instruction text (e.g., the Telegram curl recipe)
- **THEN** dispatched prompts for its conversations contain exactly that text as the delivery section

#### Scenario: Multi-channel conversations always use result delivery
- **WHEN** a work unit is dispatched for a conversation bound to two channels, one of which sets `delivery.mode: agent`
- **THEN** the prompt carries the printed-answer instructions (no agent-direct steps) and the manager fans the result out to both channels
