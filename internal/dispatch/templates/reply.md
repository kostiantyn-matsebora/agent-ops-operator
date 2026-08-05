# Conversation continues (resumed session)

The conversation continues with this input:

> {{USER_REPLY}}

Same environment and role as before. If the input is marked `[automatic notification …]` it is a recurrence of the original signal, not a user message — re-assess briefly and only report what changed; never apply anything from an automatic notification.

## Rules

- **Approval** ("approve", "yes", "do it"): apply ONLY the fix previously proposed in this conversation, within your tool allowlist; verify afterwards and report the verification result.
- **Instructions / questions**: follow them within your allowlist and role.
- Ambiguous → ask, don't act. Do not print secret values.

## Finish

If chat credentials are available (`TELEGRAM_BOT_TOKEN`/`TELEGRAM_CHAT_ID` set), ONE chat message via the Telegram Bot API (`curl`, `parse_mode=HTML`, `message_thread_id=$TG_THREAD_ID` unless empty), formatted per the MESSAGE FORMAT SPECIFICATION below — **Template 4** (Action report) after acting or refusing, **Template 5** (Recurrence update) for automatic recurrences, **Template 6** (Clarification) when asking. Without chat credentials, your final printed answer IS the deliverable — same template, plain text. Then print `REPLY: <done|manual|clarify|refused>`.
