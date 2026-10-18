## [13.0.1] — 2026-08-24

**Every component image is now published BY CI, from a tag.**

### Changed

Ten components moved to versions the release workflow built and pushed:
`channel-telegram` 0.24.3, `console` 0.38.1, `context-sync` 0.2.2,
`egress-proxy` 0.2.3, `gateway-telegram` 0.5.2, `housekeeping` 0.2.2,
`signal-alertmanager` 0.7.2, `signal-ha` 0.2.2, `signal-k8s-events` 0.4.2,
`signal-telegram` 0.6.2.

**No behaviour changes.** The images are built from the same source as the
versions they replace. What is new is HOW they got there.

The previous ones were pushed by hand before this repository was public, so
nothing in the tree recorded which commit produced them. The registry's
immutability rule meant a tag could never be added after the fact — only a
new version could carry one.

Component tags exist for that provenance and deliberately create no GitHub
Release: the chart version is the one an adopter types.
