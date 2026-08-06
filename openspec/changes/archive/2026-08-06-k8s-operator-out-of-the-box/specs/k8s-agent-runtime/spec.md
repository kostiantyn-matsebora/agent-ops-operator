# k8s-agent-runtime

## ADDED Requirements

### Requirement: Kubernetes runtime image derives from the claude runtime
The `runtime-k8s/` image (`agentops-runtime-k8s`) SHALL be built `FROM` the pinned claude runtime image, inheriting its `/work` contract behavior, kubectl, and entrypoint unchanged, and SHALL add a pinned Kubernetes MCP server installed at build time (available on `PATH` for stdio use) — no package downloads at pod start.

#### Scenario: Work contract unchanged
- **WHEN** an AgentRuntime uses the k8s image
- **THEN** runtime pods long-poll `/work`, execute units, and report `/work/done` exactly as the claude runtime does

#### Scenario: MCP server available offline
- **WHEN** a runtime pod starts in a cluster without internet egress
- **THEN** the Kubernetes MCP server starts from the baked-in install without any download

### Requirement: MCP server supports a non-destructive mode
The baked-in Kubernetes MCP server SHALL support restriction to non-destructive tools via environment (verified against the pinned version), so profiles can pair read-only RBAC with a read-only toolset.

#### Scenario: Non-destructive mode honors the flag
- **WHEN** the server starts with the non-destructive environment flag set
- **THEN** destructive Kubernetes tools are not offered to the agent
