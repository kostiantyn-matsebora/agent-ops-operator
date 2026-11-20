## [14.0.1] — 2026-10-10

- `signal-cron` 0.2.0.

### Fixed

- **The self-heal reaper's `cronAdapter` still pinned `signal-cron:0.0.1-rc3`**,
  an unreleased candidate rather than a real version — 14.0.0's own fix for
  `signal-cron:0.2.0` (a version that had never shipped) landed on the closest
  tag that DID exist instead of actually cutting one. Off by default
  (`reaper.enabled: false`), so no install pulled it, but an install that turns
  the reaper on now gets a real release: `signal-cron:0.2.0`, built from the
  same unchanged source the `rc3` tag already shipped.
