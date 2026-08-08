## MODIFIED Requirements

### Requirement: Single getUpdates consumer preserved
Exactly one getUpdates consumer per **bot token** SHALL hold at all times. The polling loop SHALL live in the telegram ROUTER component, not in the channel adapter: the router SHALL run one loop per distinct token, its workload SHALL run single-instance (replicas 1, Recreate), and the channel adapter SHALL NOT poll at all — it receives topic messages forwarded by the router and continues posting them to `/channel/inbound`. The documented migration SHALL sequence shutdown of the previously-polling adapter before the router starts, so two consumers of one token are never live simultaneously.

#### Scenario: Two bots poll independently, once each
- **WHEN** the router serves two channels with distinct projected tokens
- **THEN** it runs exactly two getUpdates loops — one per token — inside the single pod

#### Scenario: The channel adapter never polls
- **WHEN** the split stack is running
- **THEN** the channel adapter makes no getUpdates call, and its only Telegram API calls are sends and topic creation

#### Scenario: Upgrade never double-polls
- **WHEN** an install migrates from the polling channel adapter to the router-fronted stack following the documented steps
- **THEN** at no point do two getUpdates consumers use the same bot token

#### Scenario: Offset survives the split
- **WHEN** the migration completes
- **THEN** the router resumes from the offset the previous adapter persisted, rather than re-reading old updates
