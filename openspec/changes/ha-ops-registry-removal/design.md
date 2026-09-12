## Context

The home-assistant bundle enumerates the admin MCP server's tools into one
`MCPToolset`, `ha-admin`, and withholds twenty-six under one comment: "they
restart Home Assistant, delete registry objects, manage backups, or install
software".

The ops profile's prompt lists five describe-and-stop cases. One is "would
remove or re-pair a device, or delete history", and the rule ends "wait to be
told".

Live, the two halves contradicted each other:

1. The agent read the prompt as a permanent refusal.
2. Told to proceed, it tried REST, which has no registry-remove endpoint.
3. The MCP tool that does it was withheld, and egress mediation refuses an
   unlisted tool on the wire, so the shell path was closed too.
4. The only route left was a hand-written websocket client in Node — the
   credential path the spec already calls "advisory", the widest path, reached
   because the narrow one was shut.

## Goals / Non-Goals

**Goals**

- An administrator that can remove a stale registry object through the
  MEDIATED path, gated by the prompt's confirmation.
- A withheld class stated by what it IS, so the next reader can place a new
  ha-mcp tool without re-deriving the line.
- A prompt gate the agent cannot read as a refusal.

**Non-Goals**

- Shipping restart, backups, software installs or preference tools. Those are
  instance-wide and stay a values decision.
- Shipping deletes of authored configuration. The prompt already stops for
  "automations or scripts someone else wrote", and the toolset now matches it.
- Any change to the credential path. It stays documented as the wider one.
- A per-install knob for the confirmation sentence. It is the prompt, and an
  install overriding `profiles.ops.systemPrompt` owns its own text.

## Decisions

**The line is "who wrote it", not "is it a delete".** Registry objects are
discovered or created through the UI and cheap to recreate. Automations,
scripts, scenes and dashboards are somebody's authored work, which the prompt
already protects.

- Sorting by that line puts every `remove_*` registry tool in and every
  `config_remove_*` authored delete out.
- The prefix is not the line: `ha_config_remove_category`, `_label`, `_group`
  and `_calendar_event` are registry objects despite `config_`, and ship.
- Alternative considered: ship `ha_remove_entity` alone. Rejected as the
  narrowest change, because the next stale device hits the same wall one tool
  over.

**The count moves from 52 to 62, and the docs say so in classes.** The adopter
page states "78 registered, 62 ship" and names the three withheld classes.

- It does not list the sixteen tools. The values comment keeps that full
  list, because it is what an adopter copies from.

**The prompt sentence is added, not rewritten.** "Write exactly what you would
do and wait to be told" stays. The new sentence states what "told" means: a
person's reply in the thread.

- It names three example phrases, says the agent does exactly what it
  described, and forbids the two failure modes seen live — asking again, and
  deferring to the person.
- Only the ops profile gains it. The user profile's describe-and-stop is
  "house-wide rather than one thing", a scope rule rather than a
  destructiveness rule. The same sentence there would invite a confirmed
  house-wide action from a control-token agent.

**The chart test re-pins by class.** In
`TestHaAdminToolsetIsEnumeratedAndWithholdsTheDestructive` the `absent` list
names one tool per withheld class — `ha_restart`, `ha_manage_backup`,
`ha_manage_hacs`, `ha_config_remove_automation`. The `want` list gains
`ha_remove_entity` and `ha_remove_device`. A second assertion pins the
confirmation sentence in the rendered `ha-operator` prompt.

**Bundle version 0.1.1 → 0.2.0.** A default toolset gaining reach is a minor
bump, not a patch. The parent chart's version follows at release, not here.

## Risks / Trade-offs

- **An install on the default list gains ten destructive tools on upgrade.**
  Accepted: the admin toolset is opt-in (`adminMcp.enabled`), the ops route
  binding it is opt-in (`pipelines.enabled`), and the credential path already
  reached all of this. The changelog entry states it under Changed.
- **The prompt gate is judgement, not enforcement.** A confirmation in the
  thread is text the model reads. That was already true of every
  describe-and-stop line, and the toolset change does not widen what a person
  can ask for, only what the agent can then do through the mediated path.
- **ha-mcp renames a tool.** The enumerated list is pinned to 8.3.0 and the
  comment says so. Unchanged risk.

## Migration Plan

None. Helm replaces lists, so an install restating `adminMcp.toolset.tools`
keeps its list. An install on the default gains the tools on `helm upgrade`,
and the changelog says which.

## Open Questions

None.
