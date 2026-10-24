## Why

`ha-ops` was asked to delete nine stale `media_player` entities, confirmed in
the thread, and refused twice. First it read its describe-and-stop rule as a
"hard stop, not an approval gate". Told to bypass, it could not.

- Home Assistant removes registry entries over the websocket API only.
- The MCP server registers `ha_remove_entity`, and the bundle withholds it.
- Egress mediation refuses the withheld tool on the wire.

**A confirmation gate with no tool behind it is a refusal wearing a gate's
name.** An administrator profile that cannot remove a stale entity is not an
administrator.

## What Changes

- **The `ha-admin` toolset ships the registry-removal tools.** Ten of the
  twenty-six withheld tools move into the default list: `ha_remove_entity`,
  `ha_remove_device`, `ha_remove_area_or_floor`, `ha_remove_zone`,
  `ha_remove_helpers_integrations`, `ha_config_remove_category`,
  `ha_config_remove_label`, `ha_config_remove_group`, `ha_remove_todo_item`
  and `ha_config_remove_calendar_event`. Sixty-two of seventy-eight ship.
- **Sixteen stay withheld, and the rule for them is restated in three
  classes**: restart (`ha_restart`, `ha_reload_core` stays as it was), backups
  (`ha_manage_backup`), software and preferences (`ha_manage_hacs`,
  `ha_manage_app`, `ha_manage_updates`, `ha_import_blueprint`, the `manage_*`
  preference tools, `ha_report_issue`), and the deletes of things a person
  WROTE (`ha_config_remove_automation`, `_script`, `_scene`,
  `ha_config_delete_dashboard`, `_dashboard_resource`). The last class is the
  prompt's own "edits automations or scripts someone else wrote" line, made
  structural.
- **The ops profile's describe-and-stop rule states it is a CONFIRMATION
  GATE.** One sentence after "wait to be told": a person's reply in the thread
  that they want it done is the authorization, the agent then does exactly
  what it described, and it neither asks twice nor tells them to run it
  themselves.
- **Not breaking.** The admin toolset renders only under
  `adminMcp.enabled`, off by default. An install restating
  `adminMcp.toolset.tools` keeps its own list, since Helm replaces lists.
  An install on the default list gains the ten tools on upgrade, behind the
  same prompt gate that already governs the credential path.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `ha-bundle`: the "tooling is split by risk" requirement gains what the
  withheld class IS — restart, backups, software, and deletes of authored
  configuration — so registry removals are inside the shipped list rather
  than outside it. The "two profiles" requirement gains that the operator's
  role prompt states its describe-and-stop rule as a confirmation gate a
  person's reply passes.

## Impact

- **Chart**: `chart/charts/home-assistant/values.yaml` (the toolset list, its
  comment, the ops profile's system prompt), `Chart.yaml` version bump.
- **Tests**: `platform/manager/internal/integration/charttemplate_test.go`,
  `TestHaAdminToolsetIsEnumeratedAndWithholdsTheDestructive` — the `absent`
  list names `ha_remove_entity`, which now ships, so the assertion inverts for
  it and the withheld list is re-pinned by class.
- **Reference docs**: `docs/integrations/home-assistant.md` (the "78 tools, 52
  ship" paragraph and the withheld classes), `docs/CHANGELOG.md`
  (`[Unreleased]`, Changed).
- **Adopter site**: the same integration page is the adopter's page for this
  bundle. The landing page, `introduction.md`, `getting-started.md`,
  `installation.md` and `docs/guides/*` name no tool count and no withheld
  class, and stay as they are. The `renders bundle=home-assistant` table is
  unaffected — no object is added or removed.
- **No manager, contract or CRD change.** Nothing outside the bundle reads the
  tool list.
