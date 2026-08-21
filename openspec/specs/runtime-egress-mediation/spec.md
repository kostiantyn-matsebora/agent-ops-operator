# runtime-egress-mediation Specification

## Purpose
Enforcement of a conversation's bound tool access at a point the agent does not control: a per-conversation egress proxy inside the runtime pod, which the agent's traffic cannot route around, together with the pod properties that soundness depends on.

## Requirements

### Requirement: Egress mediation is opt-in per AgentRuntime and absent by default
An `AgentRuntime` SHALL declare egress mediation explicitly. When it is not declared, the runtime pod SHALL be exactly the pod built today — no proxy, no interception, no additional containers, and no change to the agent's environment. Enabling it SHALL NOT require any change to a runtime IMAGE, so an adopter's derived image keeps working unchanged.

#### Scenario: An install that does not ask for it is unchanged
- **WHEN** an `AgentRuntime` with no mediation stanza dispatches a conversation
- **THEN** the runtime pod carries the same containers, environment and security context it carries today

#### Scenario: Enabling it needs no image change
- **WHEN** mediation is enabled on an `AgentRuntime` whose `spec.image` is a derived, adopter-built image
- **THEN** conversations on that runtime are mediated without rebuilding that image

### Requirement: The agent's egress cannot bypass the proxy
When mediation is enabled, ALL TCP egress from the agent container SHALL be delivered to the proxy regardless of the destination the agent chooses. Bypass SHALL NOT be possible by addressing a service directly, by rewriting the compiled MCP configuration, or by any destination the agent selects. Traffic originating from the proxy itself SHALL NOT be re-intercepted.

#### Scenario: A direct connection to an MCP server is intercepted
- **WHEN** the agent opens a connection to an MCP server's address without using its compiled configuration
- **THEN** the connection is handled by the proxy and subject to the same enforcement

#### Scenario: Editing the compiled MCP config changes nothing about enforcement
- **WHEN** the agent rewrites its own `mcp.json` to point elsewhere
- **THEN** enforcement is unaffected, because interception does not depend on that file

### Requirement: The proxy enforces the conversation's bound toolset on MCP traffic
The proxy SHALL evaluate MCP requests against the tool access the conversation's wiring granted, and SHALL refuse a call to a tool that access does not include. A refusal SHALL be returned to the agent as an MCP-level error naming the refusal, never as a transport failure, so the agent can report it rather than retry blindly. Tool DISCOVERY SHALL be filtered consistently with tool INVOCATION, so a tool the agent may not call is not advertised to it.

#### Scenario: A tool outside the binding is refused
- **WHEN** a conversation bound only to read toolsets issues a mutating MCP tool call
- **THEN** the call does not reach the MCP server, and the agent receives an MCP error stating the tool is not granted

#### Scenario: A tool inside the binding passes through
- **WHEN** the same conversation issues a tool call its bound toolsets grant
- **THEN** the call reaches the server and the response is returned unmodified

#### Scenario: Discovery matches invocation
- **WHEN** the agent lists available tools through the proxy
- **THEN** the listing contains exactly the tools it would be permitted to call

### Requirement: Non-MCP egress is forwarded untouched
Traffic that is not MCP SHALL be forwarded without inspection or modification, and SHALL NOT be terminated, re-encrypted or otherwise interfered with. The agent's access to the LLM API, source repositories and package registries SHALL keep working with no configuration change and no additional trust anchor.

#### Scenario: LLM API traffic is unaffected
- **WHEN** the agent calls the LLM API over TLS through the proxy
- **THEN** the connection succeeds end to end, with no certificate supplied by the proxy

#### Scenario: Repository access is unaffected
- **WHEN** the runtime clones or fetches its configured repository
- **THEN** the operation succeeds unchanged

### Requirement: The proxy holds no Kubernetes access and learns its policy at pod creation
The tool access the proxy enforces SHALL be supplied to it when the runtime pod is created, by the component that already resolves it. The proxy SHALL NOT read the Kubernetes API, SHALL NOT hold a ServiceAccount token, and SHALL NOT be granted any RBAC. A proxy SHALL serve exactly one conversation, so it SHALL NOT be able to present or enforce another conversation's access.

#### Scenario: No cluster credential is present
- **WHEN** a mediated runtime pod is inspected
- **THEN** the proxy container mounts no ServiceAccount token and no RBAC object exists for it

#### Scenario: Policy arrives with the pod
- **WHEN** the pod is created for a conversation whose pipeline binds a given set of toolsets
- **THEN** the proxy enforces exactly that set without querying anything

### Requirement: Interception soundness rests on pod properties that are pinned
Interception distinguishes the agent from the proxy by process identity, so the runtime pod SHALL guarantee the properties that keep that distinction meaningful: the agent container SHALL run as a non-root identity distinct from the proxy's, SHALL NOT be able to escalate privilege, and SHALL hold no capability that permits changing identity or network configuration. The privilege required to install interception SHALL be confined to pod startup and SHALL NOT be held by the agent container at any point. These properties SHALL be pinned by test rather than left to hold incidentally.

#### Scenario: The agent cannot assume the proxy's identity
- **WHEN** the agent attempts to run as the proxy's identity
- **THEN** it cannot, because privilege escalation is denied and no capability permitting it is held

#### Scenario: The agent never holds interception privilege
- **WHEN** a mediated runtime pod is running
- **THEN** no capability permitting network configuration is present on the agent container

### Requirement: Mediation failure denies rather than exposes
If the proxy is not ready or has failed, the agent's mediated traffic SHALL fail closed. An agent SHALL NOT reach a mediated destination unmediated because the proxy is unavailable, and the resulting failure SHALL be reported on the conversation rather than presented as an empty or successful result.

#### Scenario: Agent starts before the proxy is ready
- **WHEN** the agent container issues a mediated request before the proxy is serving
- **THEN** the request fails rather than reaching the destination directly

#### Scenario: Proxy failure is visible
- **WHEN** the proxy fails during a run
- **THEN** the failure surfaces on the conversation rather than as silently reduced tooling
