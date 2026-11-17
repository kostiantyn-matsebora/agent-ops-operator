## MODIFIED Requirements

### Requirement: Read-only RBAC granted by the chart, not the operator
The chart SHALL grant SA `agentops-adapter-console` a namespaced read-only Role covering `get`, `list`, `watch` on every `agentops.dev` kind **and on `deployments` and `pods`**. The latter are there because versions, image digests, restart counts and manager health exist in no CR.

No write verb SHALL be granted on any resource, and no reconciler SHALL create or bind RBAC.

In a split release the chart SHALL also grant the console a read-only Role on
`pods` in the conversations namespace, so runtime pods stay visible. The
console SHALL read `CONVERSATION_NAMESPACE`, defaulting to `POD_NAMESPACE`.

The trust boundary SHALL be documented. Whoever holds the UI token:

- can read everything this ServiceAccount can read, conversation payloads included
- can instruct any agent the console can reach, when writes are enabled

#### Scenario: Read-only in fact
- **WHEN** the console's Role is inspected
- **THEN** it contains no write verb on any resource

#### Scenario: Console SA can watch, not write
- **WHEN** access for SA `agentops-adapter-console` is checked
- **THEN** `list`/`watch` on pipelines/conversations succeed and any write verb or Secret read is denied

#### Scenario: A broken pod is visible
- **WHEN** a component is in CrashLoopBackOff
- **THEN** the console reports it, having read pod state directly

#### Scenario: Runtime pods stay visible across the split
- **WHEN** a runtime pod runs in the conversations namespace of a split release
- **THEN** the console lists it from that namespace, under a Role that grants no write verb
