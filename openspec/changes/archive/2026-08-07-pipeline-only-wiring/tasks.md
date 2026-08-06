# Tasks: pipeline-only-wiring

## 1. API + routing

- [x] 1.1 Remove `SignalSourceSpec.ChannelRef`/`ProfileRef` and `ChannelSpec.DefaultProfileRef`; regen deepcopy + CRDs; clean samples (Pipeline sample already carries the wiring)
- [x] 1.2 `routeSignals`/`routeSignalGroup`: require the Ready-Pipeline claim; return a not-wired reason surfaced by both `/ingest/alertmanager` and `/signal/inbound` responses
- [x] 1.3 Router `defaultProfile`: pipeline-only (drop the `defaultProfileRef` branch); `SignalSourceReconciler`: add `Wired` condition (watch Pipelines)

## 2. Tests + verification

- [x] 2.1 Update helpers/tests: mkChannel loses defaultProfile param (inbound default-profile tests go through a Pipeline), mkSignalSource loses profile param, signal-routing tests create+reconcile Pipelines; new asserts: not-wired drop reason, `Wired` transitions
- [x] 2.2 `go build/vet` + full envtest green; helm lint; chart 1.4.0 / manager 0.7.0 built + pushed

## 3. Live + docs

- [x] 3.1 Live: apply `home-ops-pipeline` (alertmanager → ha-engineer → home-ops) BEFORE upgrading; upgrade; verify alert routing continuity (test alert via webhook → conversation via pipeline), `Wired` conditions, bare-message default profile via pipeline
- [x] 3.2 README (CRD table rows, contract wording, migration section) + CLAUDE.md (wiring-lives-only-in-Pipeline invariant); commit
