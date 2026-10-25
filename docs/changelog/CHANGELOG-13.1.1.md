## [13.1.1] — 2026-08-27

**Every image rebuilt on the current toolchain.** The weekly scan of the
published images reported fixable findings in every Go binary: 22 Go
standard-library CVEs, CVE-2025-68121 among them at CRITICAL.

The manager carried eight more from `golang.org/x/net`, `x/oauth2` and
`x/text`, and `runtime-claude` five inside npm's bundled dependencies. The tree
had already moved past all of them (Go 1.25, the manager's `x/*` bumped). The
published images had not.

### Changed

Twelve components moved to versions built from that tree:
`channel-telegram` 0.24.4, `console` 0.38.2, `context-sync` 0.2.3,
`egress-proxy` 0.2.4, `gateway-telegram` 0.5.3, `housekeeping` 0.2.3,
`manager` 0.57.2, `runtime-claude` 0.8.3, `signal-alertmanager` 0.7.3,
`signal-ha` 0.2.3, `signal-k8s-events` 0.4.3, `signal-telegram` 0.6.3.

**No behaviour changes.** `runtime-ollama` 0.1.0 was already built on Go 1.25
and scanned clean, so it is unchanged.
