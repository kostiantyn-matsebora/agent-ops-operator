## MODIFIED Requirements

### Requirement: The manager exposes only state that exists nowhere else
The manager SHALL expose its own runtime state and its own resolution results, and SHALL NOT expose snapshots of configuration. Anything readable from a configuration object (a Pipeline, a Channel, a SignalSource, an adapter CR) SHALL be read from the API server by the client under its own RBAC, never proxied through the manager. State is the exception and has its own surface: a conversation or a signal lives wherever the selected state backend keeps it, and only the manager knows where, so the manager serves it through the state read surface.

#### Scenario: No CR proxying
- **WHEN** a client needs the spec or status of a configuration object (any `agentops.dev` object that is not state)
- **THEN** no manager endpoint serves it, and the client reads it from the Kubernetes API under its own ServiceAccount

#### Scenario: State is served by the state read surface
- **WHEN** a client needs a conversation or a signal
- **THEN** the manager's state read surface serves it, whichever backend holds it

#### Scenario: Manager-only state is reachable
- **WHEN** a client needs state the manager holds in memory
- **THEN** an authenticated endpoint serves it, such as `GET /status`, because no Kubernetes object carries it
