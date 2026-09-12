## 1. Chart

- [ ] 1.1 Carry the uncommitted ops-prompt edit from the main checkout into the worktree (`chart/charts/home-assistant/values.yaml`, the sentence after "wait to be told"), and confirm the main checkout is left clean of it.
- [ ] 1.2 Move the ten registry-removal tools into `adminMcp.toolset.tools`, keeping the list sorted as it is.
- [ ] 1.3 Rewrite the "NOT SHIPPED" comment by class — restart and backups, software and preferences, deletes of authored configuration — listing the sixteen that stay withheld.
- [ ] 1.4 Update the "Verified against ha-mcp 8.3.0" count from 52 to 62.
- [ ] 1.5 Bump `chart/charts/home-assistant/Chart.yaml` to 0.2.0.
- [ ] 1.6 Render the chart with the bundle, the admin MCP and its server enabled and confirm the toolset carries `ha_remove_entity` and the `ha-operator` prompt carries the confirmation sentence.

## 2. Unit tests

- [ ] 2.1 In `TestHaAdminToolsetIsEnumeratedAndWithholdsTheDestructive`, add `ha_remove_entity` and `ha_remove_device` to the `want` list and re-pin `absent` as one tool per withheld class: `ha_restart`, `ha_manage_backup`, `ha_manage_hacs`, `ha_config_remove_automation`.
- [ ] 2.2 Add an assertion that the rendered `ha-operator` profile's prompt contains the confirmation-gate sentence.
- [ ] 2.3 `go test ./internal/integration/ -run 'TestHa'` green in `platform/manager` with helm on the path.

## 3. E2E tests

- [ ] 3.1 Not applicable: nothing here is decided by a cluster. The toolset is a rendered list the runtime reads, and the prompt is rendered text. Both are settled by the chart render test.

## 4. Documentation

- [ ] 4.1 Reference docs: `docs/integrations/home-assistant.md` — the "78 tools, 52 ship" paragraph becomes 62 and names the three withheld classes. `docs/CHANGELOG.md` — an `[Unreleased]` Changed entry naming the ten tools, the bundle version and that a default-list install gains them on upgrade.
- [ ] 4.2 Adopter site: confirm the landing page, `introduction.md`, `getting-started.md`, `installation.md` and `docs/guides/*` state no tool count or withheld class, and `python3 .github/scripts/docs-generate.py --check` passes.
