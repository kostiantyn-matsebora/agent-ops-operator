## ADDED Requirements

### Requirement: The bundle exposes each health surface as values, with default rules ahead of the log rules
The bundle SHALL render the adapter's surface configuration from values under
the source — one `enabled` switch per surface and that surface's knob (the
config entry states, the repair severities, the sensor device classes) — with
config entries, repairs and sensors enabled and the update digest disabled by
default, and SHALL declare every key in the adapter's config schema so an
unknown key is refused rather than ignored.

The shipped rules SHALL open with one rule per surface, ahead of the log
rules: config entries and sensors dwell, because a Home Assistant restart
passes both through their failed states; repairs and the update digest carry a
zero dwell, because a repair is a standing fact and a digest is a list. A
values file that replaces `rules` replaces these too, and the page SHALL say so.

#### Scenario: Default install
- **WHEN** the bundle renders with default values
- **THEN** the source's config enables config entries, repairs and sensors, disables updates, and its rules open with one rule per surface before the first log rule

#### Scenario: A surface is switched off in values
- **WHEN** `logsAdapter.source.surfaces.sensors.enabled` is `false`
- **THEN** the rendered config disables the sensor surface and nothing else changes

#### Scenario: An unknown surface key is refused
- **WHEN** the rendered config carries a surface key the adapter does not know
- **THEN** the adapter refuses the configuration on the source's Ready condition, naming the key, rather than ignoring it
