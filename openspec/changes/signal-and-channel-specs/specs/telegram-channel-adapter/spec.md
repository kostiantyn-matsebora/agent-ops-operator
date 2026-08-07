# telegram-channel-adapter — delta

## ADDED Requirements

### Requirement: Chart-shipped ChannelAdapter declares the telegram config schema
The chart's gated telegram `ChannelAdapter` CR SHALL declare the config contract on its spec: a JSON Schema for `spec.config` declaring `chatId` (string, required), `feedThreadId` (integer), `approvers` (array of integers), and `pollingEnabled` (boolean), plus `credentialKeys` documenting `botToken` (not required — the `TELEGRAM_BOT_TOKEN` fallback exists). The declaration SHALL live beside the `image` reference in the same template so an image bump and its schema update travel in one diff. The adapter binary SHALL be unchanged — it plays no role in the declaration.

#### Scenario: Declaration matches the parser
- **WHEN** the chart renders with `telegramAdapter.enabled=true`
- **THEN** the `ChannelAdapter` for telegram declares exactly the fields the adapter's config parser accepts, with `chatId` required

#### Scenario: Misconfigured channel flagged before the adapter sees it
- **WHEN** a `type: telegram` Channel is created with `config: {}` while the declaring ChannelAdapter exists
- **THEN** the Channel gains `ConfigValid=False` naming `chatId` from the manager, in addition to whatever Ready condition the adapter later reports

#### Scenario: Adapter binary unchanged
- **WHEN** the telegram adapter image runs against a ChannelAdapter with or without the declaration
- **THEN** its behavior is identical — it neither reads nor publishes any schema
