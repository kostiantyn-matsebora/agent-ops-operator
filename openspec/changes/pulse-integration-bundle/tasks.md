## 1. Pulse signal adapter — backend-developer

- [ ] 1.1 Scaffold `signals/pulse/` as a dependency-free Go module and
      image, mirroring `signals/alertmanager/`'s structure. Verify
      `go build ./...` succeeds.
- [ ] 1.2 Implement the signal adapter contract's server half:
      `GET /signal/sources?adapter=`, `POST /webhook/{source}`,
      `POST /signal/inbound`, 404 on an unknown or unserved source. Verify
      with a unit test posting to an unserved source and asserting 404.
- [ ] 1.3 Define the registered webhook's Go-template JSON body and its
      matching parser in one package, with a pinned test fixture asserting
      the template's field set and the parser's expected fields agree.
- [ ] 1.4 Implement HMAC signature verification
      (`X-Pulse-Timestamp`/`X-Pulse-Signature`) with a bounded timestamp
      tolerance, gated on an optional signing secret. Verify unit tests
      cover valid, invalid, missing-signature and unsigned-mode cases.
- [ ] 1.5 Implement firing/resolved normalization: a firing event produces
      one fingerprinted signal, a resolved event produces none (200
      response, nothing pushed). Verify with unit tests for both event
      types.
- [ ] 1.6 Implement pod-start registration against
      `POST /api/notifications/webhooks` (create-or-update, idempotent),
      reading the Pulse URL, API token, source name and signing secret from
      env. Verify with a unit test against a fake Pulse server asserting
      create-then-update idempotence.
- [ ] 1.7 Implement the bounded-retry, keep-healing registration loop: no
      crash-loop on failure, a degraded Ready reason reported via
      `/signal/sources/{name}/status`, healing on a later successful retry.
      Verify with a unit test simulating an initial failure then a success.

## 2. Pulse MCP server (`mcp-pulse`) — backend-developer

- [ ] 2.1 Scaffold `platform/mcp-pulse/` as a thin, stateless
      streamable-HTTP MCP server, mirroring `platform/mcp-aops/`'s shape.
      Verify `go build ./...` succeeds.
- [ ] 2.2 Implement token forwarding: every call relays the caller's bearer
      token unchanged to Pulse's REST API, with no local scope decision.
      Verify a unit test asserting the forwarded header matches the inbound
      one.
- [ ] 2.3 Implement the read tool set (inventory/resources, metrics/charts,
      alerts, storage history) as thin forwards to Pulse's REST endpoints.
      Verify unit tests per tool.
- [ ] 2.4 Implement the two Community-tier operator-state write tools
      (maintenance window, criticality tag) as thin forwards to Pulse's
      `scope_only` operator-state endpoints. Verify unit tests per tool.
- [ ] 2.5 Verify a call made with an insufficiently-scoped token is relayed
      and refused by Pulse rather than short-circuited locally, with a unit
      test against a fake Pulse server returning 403.

## 3. Bundle chart `chart/charts/pulse/` — deployment-engineer

- [ ] 3.1 Scaffold the subchart (`Chart.yaml`, `values.yaml`,
      `_helpers.tpl`), self-gated `enabled: false`, never forced on by demo
      mode. Verify `helm template` with default values renders nothing from
      the bundle.
- [ ] 3.2 Render the `signal` component: the Pulse `SignalAdapter` CR plus,
      under `defaultSource.enabled`, a `SignalSource` naming it, with the
      claim left to the wiring component. Verify a render test asserting
      the source reports unclaimed when wiring is off.
- [ ] 3.3 Render the `profiles` component: `pulse-observer` and
      `pulse-operator` AgentProfiles with inline, Community-edition-framed
      roles and no capabilities. Verify a render test asserting neither
      declares `allowedTools` or `mcp`.
- [ ] 3.4 Render the `mcp` component: the `mcp-pulse` Deployment and
      Service under its own ServiceAccount (distinct from the runtime SA,
      failing the render if set equal), plus the two `MCPConfig`/
      `MCPToolset` pairs with enumerated, non-wildcard tool patterns and
      `headers.valueFrom` secret refs. Verify render tests for the
      equal-SA failure and for both pairs rendering independently.
- [ ] 3.5 Render credentials: a raw token value that creates its own
      Secret, or an `existingSecret` reference, doubled per profile and
      configurable independently. Verify a render test for both forms on
      both profiles.
- [ ] 3.6 Render the `pipelines` wiring component (Pipeline mode): two
      Pipelines, one per profile, each claiming the shared source and
      binding its own MCPConfig/MCPToolset pair, with values-supplied
      `channelRefs` omitted when empty. Verify a render test asserting both
      routes claim the same source without conflict.
- [ ] 3.7 Render the `capabilities` wiring component (Coordinator mode),
      mirroring `prometheus-bundle`'s coordinator-contribution-helper
      pattern: two AgentCapabilities contributed to the chart-rendered
      Coordinator's `agents[]`. Verify a render test asserting both appear
      under `wiringMode: coordinator`.
- [ ] 3.8 Confirm `.github/components.sh` auto-discovers both new Go
      modules and images with no manual list edit, and add any multi-arch
      build matrix entries the script's output says are missing. Verify
      `.github/components.sh images` and `.github/components.sh modules`
      list both new components.

## 4. Unit tests

- [ ] 4.1 `go test ./...` in `signals/pulse/` passes.
- [ ] 4.2 `go test ./...` in `platform/mcp-pulse/` passes.
- [ ] 4.3 The chart's render tests covering `chart/charts/pulse/` pass.

## 5. System tests

- [ ] 5.1 Not applicable: this change adds new instances of the existing
      SignalAdapter, MCPConfig, MCPToolset, Pipeline, AgentCapability and
      Coordinator contracts, reconciled by mechanics
      `platform/manager/test/system/` already covers for every other
      bundle. Nothing here changes what a cluster decides — no new RBAC
      posture, no new pod-lifecycle behavior, no new informer path.

## 6. Documentation

Reference docs:

- [ ] 6.1 Write `docs/integrations/pulse.md`, answering the four standard
      integration questions (what starts work, what it may reach, where it
      answers, what it costs to turn on).
- [ ] 6.2 Add a `docs/CHANGELOG.md` entry for the new bundle.
- [ ] 6.3 Add the `<!-- generated: renders bundle=pulse -->` marker to
      `docs/configuration.md` and run
      `python3 .github/scripts/docs-generate.py` to fill it, since this
      change adds new chart values.

Adopter site:

- [ ] 6.4 Add Pulse to `docs/_data/nav.yml`'s Integrations group.
- [ ] 6.5 Add Pulse to `README.md`'s integration logo strip and to the
      integrations/runtimes table.
