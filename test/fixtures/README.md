# Fixtures

Captured payloads, **scrubbed**, shared by the end-to-end pack and the owning
module's unit tests so one captured payload cannot drift between the two.

| File | Is | Read by |
|---|---|---|
| `alertmanager-webhook.json` | an Alertmanager-format webhook body: two firing alerts and one resolved | `signals/alertmanager` tests, and the pack POSTs it to `/webhook/{source}` |
| `telegram-update-message.json` | a Telegram `Update` on a forum supergroup's GENERAL surface — an origination | `signals/telegram` tests, and the pack feeds it to the fake Bot API |
| `telegram-update-topic.json` | the same shape inside a forum TOPIC — a continuation | `channels/telegram` tests, and the pack feeds it to the fake Bot API |
| `claude-stream-json.jsonl` | claude-code's `--output-format stream-json` for a run of three turns, one MCP and one built-in tool call | `runtimes/claude` tests, for the turns and tool calls the work result reports |
| `copilot-session-events.jsonl` | the Copilot SDK's session events for a run of two turns, one MCP and one built-in tool call | `runtimes/copilot` tests, for the same report |

**Every identifier is a placeholder the publication allowlist permits by
name** (`.github/publication-allowlist.json`): the documented chat id, the
documented user id, a username named for the ROLE, hosts under a reserved
example domain. A new kind of identifier is an allowlist entry FIRST.

Module tests read these by relative path from a `_test.go` or `.test.js` file.
A test-only read adds no dependency, so every module stays self-contained.
