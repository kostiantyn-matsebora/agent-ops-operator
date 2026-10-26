## ADDED Requirements

### Requirement: Runtime conformance is verified black-box, against the built runtime binary
A conformance suite SHALL exist that verifies an `AgentRuntime` implementation
against `docs/contracts/runtime.md` by running its **built binary** and
speaking HTTP to it, with a fake manager standing in for the real one.

It SHALL live beside the adapter conformance set in the manager's Go module
(`platform/manager/test/conformance/`), reusing the same `FakeManager`.

`FakeManager` SHALL serve `GET /work` and `POST /work/done`, mirroring the
existing `/channel/ops` and `/channel/ops/{id}/done` pair.

The suite SHALL cover `runtimes/ollama`. It asserts these requirement IDs
from `docs/contracts/runtime.md` behaviorally: `API-1`, `API-2`,
`API-4`, `API-5`, `API-7`, `API-8`, `API-9`, `LC-1`, `LC-2`, `RT-1`,
`RT-2`, `RT-3` and its sub-items, `COMPAT-1`.

It SHALL NOT assert `API-3`, `API-6`, `LC-3`, `SEC-1`, `SEC-2`, `SEC-3` or
any `MGR-n` requirement, each already covered elsewhere:

| Excluded | Because | Covered by |
|---|---|---|
| `API-3`, `API-6`, `MGR-n` | obligates whoever answers `GET /work` — the manager, not the runtime | envtest, the system tier |
| `LC-3` | `/exit` refused mid-run is manager-side chat-router logic, no HTTP call to the runtime pod | the manager's own chat-router unit tests |
| `SEC-1`, `SEC-2`, `SEC-3` | facts about the manager/operator's own pod-spec and RBAC rendering, not runtime-binary behavior | `internal/runtimepod`'s unit tests, the chart's RBAC templates |

#### Scenario: A new runtime joins by being listed
- **WHEN** a new `AgentRuntime` implementation is added to the repository
- **THEN** it is covered by adding it to the suite's list of implementations
  under test, with no change to its own source

#### Scenario: An allowlist regression is caught before merge
- **WHEN** a change to a covered runtime breaks `toolsMode` composition, a
  bound of `turns[]`/`toolCalls[]`, or the idle-TTL exit
- **THEN** the suite fails that implementation and the failure names the
  broken requirement ID

#### Scenario: The vendor call is a local double, not a real model
A covered runtime's vendor call SHALL be pointed at a local double through
its configured endpoint, never at a real inference provider.

- **WHEN** the suite runs a covered runtime
- **THEN** every vendor call that runtime makes reaches a fake standing in
  for it, scripted deterministically by the task text, so the suite needs no
  network access and no model credential

### Requirement: Runtime conformance needs no cluster, no network and no credentials
The runtime conformance set SHALL run with no Kubernetes cluster, no outbound
network access and no third-party credentials, exactly as the adapter
conformance set does, so it gates every pull request including those from
forks.

#### Scenario: The runtime suite gates a fork pull request
- **WHEN** a pull request originates from a fork with no access to
  repository secrets
- **THEN** the runtime conformance suite runs in full and its result gates
  the pull request
