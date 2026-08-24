# runtime-egress-mediation Specification

## Purpose
Enforcement of a conversation's bound tool access at a point the agent does not control: a per-conversation egress proxy inside the runtime pod, which the agent's traffic cannot route around, together with the pod properties that soundness depends on.

## Requirements

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

### Requirement: Egress mediation is on by default and declinable per runtime
Egress mediation SHALL be ENABLED BY DEFAULT. The wall that constrains an
uncooperative agent SHALL NOT be something an operator has to discover.

`--allowedTools` configures a COOPERATING agent. One with a shell can open a
socket to a bound MCP server and call anything that server registers, so the
allowlist is a configuration rather than a boundary. Shipping the boundary off by
default left every install with the weaker of the two until someone read far
enough to find it.

IT SHALL REMAIN DECLARABLE PER RUNTIME, and a runtime SHALL be able to decline
it — a vendor reaching no MCP server has nothing to mediate, and a proxy there is
cost without a boundary.

**THE COST SHALL BE NAMED WHERE THE DEFAULT IS MET.** Mediation adds a privileged
init container requiring `NET_ADMIN`, which a namespace under `restricted` Pod
Security admission REFUSES — so the install fails at POD ADMISSION rather than at
render, far from the setting responsible. The post-install notes SHALL state
that, and SHALL name the value that turns it off.

#### Scenario: A fresh install is mediated
- **WHEN** the chart is installed with no egress values supplied
- **THEN** runtime pods carry the proxy and the redirect, and the tool access their wiring granted is enforced where the agent does not control it

#### Scenario: A runtime that needs no proxy declines it
- **WHEN** a runtime declares mediation disabled
- **THEN** its pods carry the same containers, environment and security context an unmediated pod carries, with nothing added

#### Scenario: Enabling it needs no image change
- **WHEN** mediation applies to a runtime whose image is a derived, adopter-built one
- **THEN** its conversations are mediated without rebuilding that image

#### Scenario: The admission cost is stated, not discovered
- **WHEN** the chart is installed
- **THEN** the notes state that mediation adds a privileged init container which a `restricted` Pod Security namespace refuses, and name the value that disables it

