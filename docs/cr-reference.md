# Custom resource reference

**Generated from `chart/files/crds/` by `python3 .github/scripts/docs-generate.py`. Do not edit.**

Every field of every kind, with the type the API server enforces. What a field MEANS beyond its own sentence is in [concepts.md](concepts.md), and the contracts the adapter and runtime kinds serve are in [contracts.md](contracts.md).

API group: `agentops.dev/v1alpha1`. Every kind is namespaced.

## Kinds

| Kind | You write it | Fields |
|---|---|---|
| [AgentProfile](#agentprofile) | yes | 40 |
| [Pipeline](#pipeline) | yes | 14 |
| [MCPToolset](#mcptoolset) | yes | 1 |
| [MCPConfig](#mcpconfig) | yes | 5 |
| [SignalSource](#signalsource) | yes | 8 |
| [SignalAdapter](#signaladapter) | yes | 18 |
| [Channel](#channel) | yes | 4 |
| [ChannelAdapter](#channeladapter) | yes | 16 |
| [AgentRuntime](#agentruntime) | yes | 54 |
| [Conversation](#conversation) | no — the operator does | 35 |
| [ConversationInput](#conversationinput) | no — the operator does | 5 |

## AgentProfile

AgentProfileSpec is an addressable agent IDENTITY: repository, agent role, prompts, credentials and limits. It carries NO capabilities — what an agent may DO (its tool allowlist and MCP servers) comes exclusively from the Pipeline routing its conversation, so one profile serves routes with genuinely different capabilities without being cloned or edited.

### spec

| Field | Type | Required | Description |
|---|---|---|---|
| `agent` | `string` |  | Agent is the agent to adopt: name of `.claude/agents/<agent>.md` in the repository. A profile names one agent and a Pipeline names one profile, so the agent comes from the wiring and no message may select another. |
| `env` | `[]object` |  | Env: extra environment for the agent process; values may use valueFrom (secretKeyRef / configMapKeyRef) for credentials the agent needs. This stays on the profile deliberately: these are the AGENT's own credentials (an API token it was built around), not the route's capabilities, and moving them would put secret references into the wiring object. |
| `env[].name` | `string` | **yes** | Name of the environment variable. Must be a C_IDENTIFIER. |
| `env[].value` | `string` |  | Variable references $(VAR_NAME) are expanded using the previously defined environment variables in the container and any service environment variables. If a variable cannot be resolved, the reference in the input string will be unchanged. Double $$ are reduced to a single $, which allows for escaping the $(VAR_NAME) syntax: i.e. "$$(VAR_NAME)" will produce the string literal "$(VAR_NAME)". Escaped references will never be expanded, regardless of whether the variable exists or not. Defaults to "". |
| `env[].valueFrom` | `object` |  | Source for the environment variable's value. Cannot be used if value is not empty. |
| `env[].valueFrom.configMapKeyRef` | `object` |  | Selects a key of a ConfigMap. |
| `env[].valueFrom.configMapKeyRef.key` | `string` | **yes** | The key to select. |
| `env[].valueFrom.configMapKeyRef.name` | `string` |  | Name of the referent. This field is effectively required, but due to backwards compatibility is allowed to be empty. Instances of this type with an empty value here are almost certainly wrong. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names |
| `env[].valueFrom.configMapKeyRef.optional` | `boolean` |  | Specify whether the ConfigMap or its key must be defined |
| `env[].valueFrom.fieldRef` | `object` |  | Selects a field of the pod: supports metadata.name, metadata.namespace, `metadata.labels['<KEY>']`, `metadata.annotations['<KEY>']`, spec.nodeName, spec.serviceAccountName, status.hostIP, status.podIP, status.podIPs. |
| `env[].valueFrom.fieldRef.apiVersion` | `string` |  | Version of the schema the FieldPath is written in terms of, defaults to "v1". |
| `env[].valueFrom.fieldRef.fieldPath` | `string` | **yes** | Path of the field to select in the specified API version. |
| `env[].valueFrom.resourceFieldRef` | `object` |  | Selects a resource of the container: only resources limits and requests (limits.cpu, limits.memory, limits.ephemeral-storage, requests.cpu, requests.memory and requests.ephemeral-storage) are currently supported. |
| `env[].valueFrom.resourceFieldRef.containerName` | `string` |  | Container name: required for volumes, optional for env vars |
| `env[].valueFrom.resourceFieldRef.divisor` | `object` |  | Specifies the output format of the exposed resources, defaults to "1" |
| `env[].valueFrom.resourceFieldRef.resource` | `string` | **yes** | Required: resource to select |
| `env[].valueFrom.secretKeyRef` | `object` |  | Selects a key of a secret in the pod's namespace |
| `env[].valueFrom.secretKeyRef.key` | `string` | **yes** | The key of the secret to select from. Must be a valid secret key. |
| `env[].valueFrom.secretKeyRef.name` | `string` |  | Name of the referent. This field is effectively required, but due to backwards compatibility is allowed to be empty. Instances of this type with an empty value here are almost certainly wrong. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names |
| `env[].valueFrom.secretKeyRef.optional` | `boolean` |  | Specify whether the Secret or its key must be defined |
| `maxTurns` | `integer` |  | MaxTurns bounds the agent's own turns within ONE work unit. It is a runaway bound, not a budget: the conversation is unaffected. |
| `outputFormat` | `string` | **yes** | OutputFormat declares this agent's OUTPUT CONTRACT. REQUIRED, and deliberately without a default, because both candidate defaults are wrong: blocks — the operator's shared output-format specification is appended to the prompt: the block grammar, the fold, the markdown subset and a default section set none — NOTHING is appended, and the profile's own prompt owns formatting entirely NO DEFAULT ON PURPOSE. `none` leaves output unformatted unless the author wrote a format into the prompt; `blocks` shapes output by something the author never asked for. Refusing to guess is the honest resolution, so the author declares it and a profile omitting the field is REFUSED. IDENTITY, NEVER CAPABILITY. It shapes how the agent SPEAKS, not what it may call — the allowlist and the MCP servers remain exclusively the originating Pipeline's. IT GATES THE PROMPT, NEVER THE PARSE. Adapters parse whatever they are given, so a profile declaring `none` whose agent emits tags anyway is still rendered as blocks. Decoupling them is what keeps this safe: a switch that moved the parser too could be configured into a state where the model emits tags nothing is looking for. IT IS ALSO THE COMPATIBILITY BOUNDARY. Nothing on the wire signals the grammar — adapters parse the body, and no contract version changed — so an adapter with no parser renders `<title>` as literal characters. What protects it is a profile declaring `none`. Making the declaration mandatory is what stops that protection being accidental. IT DOES NOT GATE THE OPERATOR'S OWN PROMPT CONTENT. Text stating that the printed answer IS the deliverable is a fact about the system rather than a preference, and is injected whatever this says. |
| `prompt` | `string` |  | Prompt / ReplyPrompt are repo-relative template paths (job-style lanes). When empty, the operator's built-in lane templates wrap the agent. |
| `replyPrompt` | `string` |  | ReplyPrompt wraps a follow-up in an existing conversation, where Prompt wraps the first unit. |
| `repository` | `object` |  | RepositorySpec identifies the git repository an agent runs from. The runtime checks it out as its working directory, so the agent has access to the whole repo (CLAUDE.md, .claude/agents, skills, any assets). Optional: without a repository the agent runs as a pure advisor over its tools. |
| `repository.auth` | `object` |  | RepoAuth references credentials for a (possibly private) git repository. |
| `repository.auth.secretRef` | `object` | **yes** | SecretRef points to a Secret holding either key `sshKey` (type=ssh, private deploy key) or key `token` (type=https, PAT; optional `username`). |
| `repository.auth.secretRef.name` | `string` | **yes** | Name of the referenced object. |
| `repository.auth.type` | `string` | **yes** | RepoAuthType selects the git auth mechanism. |
| `repository.ref` | `string` |  | Branch or ref to check out. |
| `repository.url` | `string` |  | URL of the repository to check out, in any form git understands (ssh://, git@host:path, https://). Empty means no checkout. |
| `resources` | `object` |  | Resources for the runtime pod running this profile. |
| `resources.claims` | `[]object` |  | Claims lists the names of resources, defined in spec.resourceClaims, that are used by this container. This is an alpha field and requires enabling the DynamicResourceAllocation feature gate. This field is immutable. It can only be set for containers. |
| `resources.claims[].name` | `string` | **yes** | Name must match the name of one entry in pod.spec.resourceClaims of the Pod where this field is used. It makes that resource available inside a container. |
| `resources.claims[].request` | `string` |  | Request is the name chosen for a request in the referenced claim. If empty, everything from the claim is made available, otherwise only the result of this request. |
| `resources.limits` | `map[string]object` |  | Limits describes the maximum amount of compute resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| `resources.requests` | `map[string]object` |  | Requests describes the minimum amount of compute resources required. If Requests is omitted for a container, it defaults to Limits if that is explicitly specified, otherwise to an implementation-defined value. Requests cannot exceed Limits. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| `runtimeRef` | `object` |  | RuntimeRef selects the AgentRuntime (execution backend) for this profile. Falls back to the AgentRuntime named "default", then to the manager's bootstrap configuration. |
| `runtimeRef.name` | `string` | **yes** | Name of the referenced object. |
| `systemPrompt` | `string` |  | SystemPrompt is INLINE role text appended to the agent's system prompt, for profiles with no repository — where `agent` can name no `.claude/agents/<agent>.md` because nothing is checked out. It is identity, not capability: it shapes how the agent behaves, never what it may call (that is the Pipeline's toolsets, always). Appended, never replacing: the runtime keeps its own system prompt and adds this. A profile WITH a repository should carry its role in the definition file instead, which is version-controlled and can declare tools; this exists so a repo-less profile is not silently personality-free. |

### status

Written by the operator. Read it, never set it.

| Field | Type | Required | Description |
|---|---|---|---|
| `conditions` | `[]object` |  |  |
| `conditions[].lastTransitionTime` | `string` | **yes** | lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the underlying condition changed. If that is not known, then using the time when the API field changed is acceptable. |
| `conditions[].message` | `string` | **yes** | message is a human readable message indicating details about the transition. This may be an empty string. |
| `conditions[].observedGeneration` | `integer` |  | observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date with respect to the current state of the instance. |
| `conditions[].reason` | `string` | **yes** | reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of specific condition types may define expected values and meanings for this field, and whether the values are considered a guaranteed API. The value should be a CamelCase string. This field may not be empty. |
| `conditions[].status` | `string` | **yes** | status of the condition, one of True, False, Unknown. |
| `conditions[].type` | `string` | **yes** | type of condition in CamelCase or in foo.example.com/CamelCase. |
| `observedGeneration` | `integer` |  | ObservedGeneration of the last processed spec. |

## Pipeline

PipelineSpec declares the wiring between the pipeline elements: every referenced signal source's signals become conversations bound to ALL referenced channels with the pipeline's profile, and conversations started from any referenced channel are bound to all of them (full mirroring). Runtime selection stays profile.runtimeRef — the Pipeline binds no runtime, credentials, or config.

### spec

| Field | Type | Required | Description |
|---|---|---|---|
| `channelRefs` | `[]object` |  | ChannelRefs: every conversation of this pipeline is mirrored on all of these surfaces. Channels may appear in several pipelines. |
| `channelRefs[].name` | `string` | **yes** | Name of the referenced object. |
| `icon` | `string` |  | Icon is how this Pipeline is RECOGNISED in a list of them. Optional, and purely how the name is presented. A REFERENCE, not an image. Four forms, and the manager tells them apart not at all — it publishes the string and interprets it no further: aops:kubernetes the built-in set, shipped inside each surface mdi:kubernetes a named icon from a public set https://example/logo.svg your own, by URL 🔎 an emoji, drawable by anything WHAT A SURFACE CAN DRAW IS THE SURFACE'S BUSINESS. `aops:` and an emoji work everywhere, because every adapter ships the first and every transport can print the second. Telegram can draw neither a URL nor a named set — a command menu takes no image — so it renders what it can and omits the rest. Nothing fails over an icon. Prefer `aops:` for anything shipped: it needs no network, survives an air-gapped install, and is the only form guaranteed on every surface. It is INTERFACE METADATA, not wiring, and it does not weaken the rule that this CR carries the wiring exclusively: nothing routes on it, no condition reads it, and removing it changes where not one signal goes. Same category as `ChannelAdapter.spec.configSchema`. It lives HERE rather than on the profile because a Pipeline is what a message addresses, so a Pipeline is what appears in a menu. |
| `mcpConfigs` | `object` |  | MCPConfigs binds MCPConfig CRs supplying this wiring's MCP servers, overlaid per server key in ref order (later wins). No mode: an agent definition declares no servers, so there is nothing to compose against. |
| `mcpConfigs.refs` | `[]object` | **yes** | Refs are applied in order: MCP server keys are overlaid with the later ref winning a collision. |
| `mcpConfigs.refs[].name` | `string` | **yes** | Name of the referenced object. |
| `profileRef` | `object` | **yes** | ProfileRef: the agent answering the conversations this pipeline originates — those from the signal sources it WATCHES, and those a chat command addresses to it by name. Channels supply no default. |
| `profileRef.name` | `string` | **yes** | Name of the referenced object. |
| `signalSourceRefs` | `[]object` |  | SignalSourceRefs: the sources feeding this pipeline. A source is SHAREABLE exactly as a channel is — any number of pipelines may list one, and a signal admitted there opens a conversation on EVERY Ready pipeline listing it, each with its own profile and capabilities. Listing a source means "I watch this", not "I own this". |
| `signalSourceRefs[].name` | `string` | **yes** | Name of the referenced object. |
| `toolsets` | `object` |  | Toolsets binds MCPToolset CRs contributing to the allowlist of this wiring's conversations, plus the mode composing them with what the AGENT'S OWN DEFINITION declares (merge unions, overwrite replaces). |
| `toolsets.mode` | `string` |  | Mode composes this binding's tools with the agent definition's: merge unions them (the agent keeps what it declared, the wiring adds), overwrite passes the wiring's alone (the agent's declaration does not apply to this route). Built-ins included — name them in the toolset. |
| `toolsets.refs` | `[]object` | **yes** | Refs are applied in order: tool lists concatenate with dedup, the first occurrence keeping its position. |
| `toolsets.refs[].name` | `string` | **yes** | Name of the referenced object. |

### status

Written by the operator. Read it, never set it.

| Field | Type | Required | Description |
|---|---|---|---|
| `conditions` | `[]object` |  | Conditions: Ready (all references resolve). There is no SourceConflict condition — sources are shareable, so listing one another pipeline also lists is a valid configuration, not a conflict. |
| `conditions[].lastTransitionTime` | `string` | **yes** | lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the underlying condition changed. If that is not known, then using the time when the API field changed is acceptable. |
| `conditions[].message` | `string` | **yes** | message is a human readable message indicating details about the transition. This may be an empty string. |
| `conditions[].observedGeneration` | `integer` |  | observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date with respect to the current state of the instance. |
| `conditions[].reason` | `string` | **yes** | reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of specific condition types may define expected values and meanings for this field, and whether the values are considered a guaranteed API. The value should be a CamelCase string. This field may not be empty. |
| `conditions[].status` | `string` | **yes** | status of the condition, one of True, False, Unknown. |
| `conditions[].type` | `string` | **yes** | type of condition in CamelCase or in foo.example.com/CamelCase. |

## MCPToolset

MCPToolsetSpec is a named, reusable list of tool patterns. It carries NO server definitions — those belong exclusively to MCPConfig CRs.

### spec

| Field | Type | Required | Description |
|---|---|---|---|
| `tools` | `[]string` | **yes** | Tools this toolset grants: MCP namespaces ("mcp__victorialogs__*") or built-in tool names ("Bash"). Any allowlist entry the runtime accepts is legal; the patterns are opaque to the manager, which passes them through exactly like the profile's allowedTools. |

## MCPConfig

MCPConfigSpec is a reusable, shareable set of MCP server definitions, or — as an escape hatch — a reference to a hand-written mcp.json.

### spec

| Field | Type | Required | Description |
|---|---|---|---|
| `configMapRef` | `object` |  | ConfigMapRef / SecretRef mount a complete hand-written mcp.json (key mcp.json) instead of compiling one. Such a config is EXCLUSIVE: a document the operator maintains by hand is opaque to us, so binding it alongside any other config is an error rather than a partial result. |
| `configMapRef.name` | `string` | **yes** | Name of the referenced object. |
| `secretRef` | `object` |  | ObjectRef references another object by name (same namespace). |
| `secretRef.name` | `string` | **yes** | Name of the referenced object. |
| `servers` | `map[string]object` |  | Servers defined inline. Mutually exclusive with the raw forms below. |

### status

Written by the operator. Read it, never set it.

| Field | Type | Required | Description |
|---|---|---|---|
| `conditions` | `[]object` |  |  |
| `conditions[].lastTransitionTime` | `string` | **yes** | lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the underlying condition changed. If that is not known, then using the time when the API field changed is acceptable. |
| `conditions[].message` | `string` | **yes** | message is a human readable message indicating details about the transition. This may be an empty string. |
| `conditions[].observedGeneration` | `integer` |  | observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date with respect to the current state of the instance. |
| `conditions[].reason` | `string` | **yes** | reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of specific condition types may define expected values and meanings for this field, and whether the values are considered a guaranteed API. The value should be a CamelCase string. This field may not be empty. |
| `conditions[].status` | `string` | **yes** | status of the condition, one of True, False, Unknown. |
| `conditions[].type` | `string` | **yes** | type of condition in CamelCase or in foo.example.com/CamelCase. |

## SignalSource

SignalSourceSpec maps a signal stream to conversations with a profile: type-agnostic routing metadata plus an opaque per-type config that only the serving signal implementation interprets. The source carries NO wiring — which profile answers and which channels mirror is declared exclusively on a Pipeline that claims this source. Unclaimed sources drop signals (Wired=False condition).

### spec

| Field | Type | Required | Description |
|---|---|---|---|
| `adapter` | `string` | **yes** | Adapter names the SignalAdapter serving this source — a REFERENCE, not an attribute: the named adapter's implementation defines and validates Config's schema. Sibling of Config by design (see Channel.Adapter). |
| `config` | `free-form` |  | Config carries whatever the signal type needs; schema-less by design. Validated by the serving adapter, never by the operator. |
| `credentialsSecretRef` | `object` |  | CredentialsSecretRef names the Secret holding this source's transport credentials (e.g. an API key). The operator only writes the NAME into the serving adapter's pod spec (kubelet-resolved envFrom projection); nothing reads the Secret's values through the API. |
| `credentialsSecretRef.name` | `string` |  | Name of the referent. This field is effectively required, but due to backwards compatibility is allowed to be empty. Instances of this type with an empty value here are almost certainly wrong. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names |
| `grouping` | `object` |  | Grouping decides which signals share one conversation, and how long a repeat of the same fingerprint is suppressed. |
| `grouping.cooldownHours` | `integer` |  | CooldownHours: suppress identical fingerprints within this window. |
| `grouping.signatureLabels` | `[]string` |  | SignatureLabels compose the signature (signal label keys). |
| `grouping.windowDays` | `integer` |  | WindowDays: reuse an existing conversation with the same signature updated within this window. |

### status

Written by the operator. Read it, never set it.

| Field | Type | Required | Description |
|---|---|---|---|
| `conditions` | `[]object` |  |  |
| `conditions[].lastTransitionTime` | `string` | **yes** | lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the underlying condition changed. If that is not known, then using the time when the API field changed is acceptable. |
| `conditions[].message` | `string` | **yes** | message is a human readable message indicating details about the transition. This may be an empty string. |
| `conditions[].observedGeneration` | `integer` |  | observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date with respect to the current state of the instance. |
| `conditions[].reason` | `string` | **yes** | reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of specific condition types may define expected values and meanings for this field, and whether the values are considered a guaranteed API. The value should be a CamelCase string. This field may not be empty. |
| `conditions[].status` | `string` | **yes** | status of the condition, one of True, False, Unknown. |
| `conditions[].type` | `string` | **yes** | type of condition in CamelCase or in foo.example.com/CamelCase. |
| `cooldown` | `[]object` |  | Cooldown records fingerprint suppression for THIS source. The manager keeps an in-memory map as the hot path, but this is the record: it is loaded on first use per source after a restart, so a restart mid-incident no longer re-opens conversations for signals inside an active window. Bounded and pruned past the window. Only a FRESH fingerprint writes here — and a fresh fingerprint already causes a conversation create or patch — so the suppressed high-volume case costs nothing. |
| `cooldown[].at` | `string` | **yes** | At is when the fingerprint was last admitted; the window runs from here. |
| `cooldown[].fingerprint` | `string` | **yes** | Fingerprint identifies the signal being suppressed. |
| `lastReceived` | `string` |  |  |
| `receivedTotal` | `integer` |  |  |

## SignalAdapter

SignalAdapterSpec declares a signal-type IMPLEMENTATION — nothing more. The CR's NAME is the routing key: SignalSources whose spec.type equals it are served by this adapter (one adapter per implementation, by construction). No configuration lives here: per-source settings are on the served SignalSources (config, credentialsSecretRef — projected into the pod by the reconciler, kubelet-resolved, never read through the API).

### spec

| Field | Type | Required | Description |
|---|---|---|---|
| `configSchema` | `free-form` |  | ConfigSchema is a JSON Schema (draft 2020-12) describing spec.config on the Channels/SignalSources this adapter serves. OPTIONAL — declaring nothing behaves exactly as before. This is interface metadata, not configuration: it holds no config values, connectivity, or credentials, so the CR stays pure implementation. Because it lives on the spec it is readable by any cluster client (kubectl, docs tooling) the moment the CR is applied — no registration step, and the adapter binary plays no part. Authoring rule: bump the schema in the same diff as `image`. |
| `credentialKeys` | `[]object` |  | CredentialKeys documents the Secret keys the implementation expects in a served CR's credentialsSecretRef. Documentation ONLY — the manager reads no Secrets, so it can never verify these. |
| `credentialKeys[].description` | `string` |  | Description of what this key holds. Documentation only -- the manager reads no Secrets. |
| `credentialKeys[].key` | `string` | **yes** | Key is the Secret key (projected as env <credentialEnvPrefix><KEY>). |
| `credentialKeys[].required` | `boolean` |  | Required marks a key the implementation cannot work without. |
| `image` | `string` |  | Image implementing the signal adapter contract. Required UNLESS servedBy names the workload that already serves this identity. |
| `kubernetesAccess` | `boolean` |  | KubernetesAccess declares that this implementation talks to the Kubernetes API (e.g. to register itself with a sender). When true the reconciler mounts the SA token and injects POD_NAMESPACE — and grants NOTHING: permissions are bound externally (chart or user) against the deterministic SA name agentops-signal-<name>. |
| `port` | `integer` |  | Port the image's own HTTP surface listens on (webhook-receiving implementations). When set, the reconciler owns a Service agentops-signal-<name> targeting it and injects LISTEN_ADDR — enabling the adapter is a complete appliance. Unset = no inbound surface (e.g. cron). |
| `resources` | `object` |  | ResourceRequirements describes the compute resource requirements. |
| `resources.claims` | `[]object` |  | Claims lists the names of resources, defined in spec.resourceClaims, that are used by this container. This is an alpha field and requires enabling the DynamicResourceAllocation feature gate. This field is immutable. It can only be set for containers. |
| `resources.claims[].name` | `string` | **yes** | Name must match the name of one entry in pod.spec.resourceClaims of the Pod where this field is used. It makes that resource available inside a container. |
| `resources.claims[].request` | `string` |  | Request is the name chosen for a request in the referenced claim. If empty, everything from the claim is made available, otherwise only the result of this request. |
| `resources.limits` | `map[string]object` |  | Limits describes the maximum amount of compute resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| `resources.requests` | `map[string]object` |  | Requests describes the minimum amount of compute resources required. If Requests is omitted for a container, it defaults to Limits if that is explicitly specified, otherwise to an implementation-defined value. Requests cannot exceed Limits. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| `servedBy` | `object` |  | ServedBy declares this adapter EXTERNALLY SERVED: another adapter's pod already holds the process, and this CR exists only to give it a signal identity. When set the reconciler creates NO Deployment, Service or ServiceAccount and reports Ready=True with reason ServedBy; the named ChannelAdapter's reconciler injects SIGNAL_ADAPTER_TOKEN into its pod. Why this exists: a chat transport is inherently a SURFACE and an ORIGINATOR — it carries conversations on threads and starts them from the general surface. Without this, declaring both identities produces two Deployments, one of which is an idle pod existing only to make a source Served. This repo has paid for that shape once already (gateway-telegram was an adapter with a signal-free SignalSource purely to carry a credential, which then sat at Wired=False). The difference here is the one that matters: an externally-served source originates real conversations for a Pipeline that claims it. |
| `servedBy.kind` | `string` | **yes** | Kind of the referenced adapter. |
| `servedBy.name` | `string` | **yes** | Name of the referenced adapter. |
| `singleton` | `boolean` |  | Singleton runs the workload as replicas 1 + strategy Recreate so no rollout ever runs two instances side by side (pollers and schedulers must not double-fire). |

### status

Written by the operator. Read it, never set it.

| Field | Type | Required | Description |
|---|---|---|---|
| `conditions` | `[]object` |  | Conditions: Deployed (workload rendered), Ready (workload available). |
| `conditions[].lastTransitionTime` | `string` | **yes** | lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the underlying condition changed. If that is not known, then using the time when the API field changed is acceptable. |
| `conditions[].message` | `string` | **yes** | message is a human readable message indicating details about the transition. This may be an empty string. |
| `conditions[].observedGeneration` | `integer` |  | observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date with respect to the current state of the instance. |
| `conditions[].reason` | `string` | **yes** | reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of specific condition types may define expected values and meanings for this field, and whether the values are considered a guaranteed API. The value should be a CamelCase string. This field may not be empty. |
| `conditions[].status` | `string` | **yes** | status of the condition, one of True, False, Unknown. |
| `conditions[].type` | `string` | **yes** | type of condition in CamelCase or in foo.example.com/CamelCase. |
| `servedSources` | `integer` |  | ServedSources counts SignalSources naming this adapter in spec.type. |

## Channel

ChannelSpec configures one chat surface: type-agnostic metadata plus an opaque per-type config that only the channel implementation interprets. A Channel describes WHERE output goes, never HOW it is sent: delivery is the operator's job (it hands agent output to the serving adapter), so no agent ever learns a transport and no runtime holds a surface's credentials. The channel carries NO wiring and originates NOTHING — it CARRIES conversations. A message on this surface's general area arrives as a chat signal from a chat SignalSource, and the Pipeline claiming that source declares who answers.

### spec

| Field | Type | Required | Description |
|---|---|---|---|
| `adapter` | `string` | **yes** | Adapter names the ChannelAdapter serving this surface — a REFERENCE, not an attribute: the named adapter's implementation is what defines and validates Config's schema. The operator never interprets it beyond routing. (Sibling of Config by design, as Kubernetes pairs a selector with its implementation-owned config: StorageClass.provisioner + parameters, IngressClass.controller + parameters.) |
| `config` | `free-form` |  | Config carries whatever the channel type needs; schema-less by design. Validated by the serving adapter, never by the operator. |
| `credentialsSecretRef` | `object` |  | CredentialsSecretRef names the Secret holding this surface's transport credentials (e.g. a bot token) — credentials are per-surface usage, never per-implementation. The operator only writes the NAME into the serving adapter's pod spec (kubelet-resolved envFrom projection); nothing reads the Secret's values through the API. |
| `credentialsSecretRef.name` | `string` |  | Name of the referent. This field is effectively required, but due to backwards compatibility is allowed to be empty. Instances of this type with an empty value here are almost certainly wrong. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names |

### status

Written by the operator. Read it, never set it.

| Field | Type | Required | Description |
|---|---|---|---|
| `conditions` | `[]object` |  |  |
| `conditions[].lastTransitionTime` | `string` | **yes** | lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the underlying condition changed. If that is not known, then using the time when the API field changed is acceptable. |
| `conditions[].message` | `string` | **yes** | message is a human readable message indicating details about the transition. This may be an empty string. |
| `conditions[].observedGeneration` | `integer` |  | observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date with respect to the current state of the instance. |
| `conditions[].reason` | `string` | **yes** | reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of specific condition types may define expected values and meanings for this field, and whether the values are considered a guaranteed API. The value should be a CamelCase string. This field may not be empty. |
| `conditions[].status` | `string` | **yes** | status of the condition, one of True, False, Unknown. |
| `conditions[].type` | `string` | **yes** | type of condition in CamelCase or in foo.example.com/CamelCase. |

## ChannelAdapter

ChannelAdapterSpec declares a channel-type IMPLEMENTATION — nothing more. The CR's NAME is the routing key: Channels whose spec.type equals it are served by this adapter (one adapter per implementation, by construction). No configuration lives here: per-surface settings are on the served Channels (config, credentialsSecretRef — projected into the pod by the reconciler, kubelet-resolved, never read through the API).

### spec

| Field | Type | Required | Description |
|---|---|---|---|
| `configSchema` | `free-form` |  | ConfigSchema is a JSON Schema (draft 2020-12) describing spec.config on the Channels/SignalSources this adapter serves. OPTIONAL — declaring nothing behaves exactly as before. This is interface metadata, not configuration: it holds no config values, connectivity, or credentials, so the CR stays pure implementation. Because it lives on the spec it is readable by any cluster client (kubectl, docs tooling) the moment the CR is applied — no registration step, and the adapter binary plays no part. Authoring rule: bump the schema in the same diff as `image`. |
| `credentialKeys` | `[]object` |  | CredentialKeys documents the Secret keys the implementation expects in a served CR's credentialsSecretRef. Documentation ONLY — the manager reads no Secrets, so it can never verify these. |
| `credentialKeys[].description` | `string` |  | Description of what this key holds. Documentation only -- the manager reads no Secrets. |
| `credentialKeys[].key` | `string` | **yes** | Key is the Secret key (projected as env <credentialEnvPrefix><KEY>). |
| `credentialKeys[].required` | `boolean` |  | Required marks a key the implementation cannot work without. |
| `echoesOwnMessages` | `boolean` |  | EchoesOwnMessages says whether this transport shows a person the message they just typed on it. INTERFACE METADATA, like configSchema: it holds no configuration and grants nothing — it states one fact about the implementation that only the implementation knows. It is what makes the delivery rule per DESTINATION. A message is delivered to every bound channel except the surface that DISPLAYED it, and displaying is a property of the transport: a chat app puts your own message in your thread, a viewer that renders only what it is sent does not — so withholding it there is how a console user's own question went missing from the transcript it started. Defaults to TRUE, which is the conservative reading: an adapter that has not been asked keeps today's behaviour, and nobody sees their own message twice. A viewer sets it false. |
| `image` | `string` | **yes** | Image implementing the adapter contract. |
| `kubernetesAccess` | `boolean` |  | KubernetesAccess declares that this implementation talks to the Kubernetes API (e.g. a console rendering agentops CRs). When true the reconciler mounts the SA token and injects POD_NAMESPACE — and grants NOTHING: permissions are bound externally (chart or user) against the deterministic SA name agentops-adapter-<name>. Identical semantics to SignalAdapter.kubernetesAccess. |
| `port` | `integer` |  | Port the image's own HTTP surface listens on (implementations that are PUSHED to rather than polling — e.g. a channel adapter receiving updates forwarded by an ingest router). When set, the reconciler owns a Service agentops-adapter-<name> targeting it and injects LISTEN_ADDR, so enabling the adapter is a complete appliance and the chart ships no connectivity. Unset = no inbound surface. Identical semantics to SignalAdapter.port. |
| `resources` | `object` |  | ResourceRequirements describes the compute resource requirements. |
| `resources.claims` | `[]object` |  | Claims lists the names of resources, defined in spec.resourceClaims, that are used by this container. This is an alpha field and requires enabling the DynamicResourceAllocation feature gate. This field is immutable. It can only be set for containers. |
| `resources.claims[].name` | `string` | **yes** | Name must match the name of one entry in pod.spec.resourceClaims of the Pod where this field is used. It makes that resource available inside a container. |
| `resources.claims[].request` | `string` |  | Request is the name chosen for a request in the referenced claim. If empty, everything from the claim is made available, otherwise only the result of this request. |
| `resources.limits` | `map[string]object` |  | Limits describes the maximum amount of compute resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| `resources.requests` | `map[string]object` |  | Requests describes the minimum amount of compute resources required. If Requests is omitted for a container, it defaults to Limits if that is explicitly specified, otherwise to an implementation-defined value. Requests cannot exceed Limits. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| `singleton` | `boolean` |  | Singleton runs the workload as replicas 1 + strategy Recreate so no rollout ever runs two instances side by side (required for pull-based transports like Telegram getUpdates). |

### status

Written by the operator. Read it, never set it.

| Field | Type | Required | Description |
|---|---|---|---|
| `conditions` | `[]object` |  | Conditions: Deployed (workload rendered), Ready (workload available). |
| `conditions[].lastTransitionTime` | `string` | **yes** | lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the underlying condition changed. If that is not known, then using the time when the API field changed is acceptable. |
| `conditions[].message` | `string` | **yes** | message is a human readable message indicating details about the transition. This may be an empty string. |
| `conditions[].observedGeneration` | `integer` |  | observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date with respect to the current state of the instance. |
| `conditions[].reason` | `string` | **yes** | reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of specific condition types may define expected values and meanings for this field, and whether the values are considered a guaranteed API. The value should be a CamelCase string. This field may not be empty. |
| `conditions[].status` | `string` | **yes** | status of the condition, one of True, False, Unknown. |
| `conditions[].type` | `string` | **yes** | type of condition in CamelCase or in foo.example.com/CamelCase. |
| `servedChannels` | `integer` |  | ServedChannels counts Channels naming this adapter in spec.type. |

## AgentRuntime

AgentRuntimeSpec defines HOW agents execute: the worker image implementing the operator's work contract, and its pod-level defaults. Adopters bring their own agent backend (claude-code, aider, custom) by supplying an image that: 1. long-polls GET $CONTROL_URL/work?convo=$CONVO_ID&pod=$POD_NAME&wait=25 2. executes the returned unit (promptText or promptFile+promptVars against the checked-out repository), streaming progress to STDOUT (pod logs) 3. reports POST $CONTROL_URL/work/done {convo,runId,status,sessionId,result} 4. exits 0 after RUNTIME_IDLE_TTL_M minutes without work

### spec

| Field | Type | Required | Description |
|---|---|---|---|
| `args` | `[]string` |  | Command / Args override the image's entrypoint. Both empty runs it as the image declares it. |
| `command` | `[]string` |  | Command/Args override the image entrypoint. |
| `contextStorage` | `string` |  | ContextStorage declares where this runtime keeps a conversation's context, so the manager can tell whether continuity is possible here BEFORE promising it. A runtime keeping context on its home volume, in a deployment that provides none, can never continue anything — and saying that up front is what stops every follow-up failing for a reason the operator already chose. |
| `contextSync` | `object` |  | ContextSync moves the LIVE context off the durable volume and keeps a snapshot on it instead. ABSENT means today's behaviour, unchanged: the home volume is mounted directly and there is no sidecar. |
| `contextSync.exclude` | `[]string` |  | Exclude drops churn from INSIDE the included paths — lock files, temp files, anything rewritten constantly without being context. Without it the change detector reports a change on nearly every cycle and the skip-when-unchanged rule buys nothing. |
| `contextSync.interval` | `string` |  | Interval is how often the context is checkpointed while a pod is alive, as a Go duration ("2m"). "0" disables the timer and leaves only work-boundary checkpoints, which is the right setting for a low-churn backend. The interval bounds what a SIGKILL can lose: a crash, an OOM or a node reboot takes everything written since the last checkpoint, and no design removes that — only shortens it. |
| `contextSync.paths` | `[]string` | **yes** | Paths are INCLUDE globs, relative to the runtime's HOME, naming what is worth persisting. For the reference runtime that is ".claude/projects/-data-workspace/**". An include list rather than an exclude list, deliberately: caches, tool state and telemetry are then excluded BY CONSTRUCTION, instead of by a list that has to chase every file a vendor decides to add. It is also the difference between copying a few megabytes of transcripts and copying a package cache over NFS every two minutes. |
| `contextSync.retain` | `integer` |  | Retain is how many previous copies to keep. More than one because a checkpoint taken mid-run may hold a partially written file. Keeping the previous generations means such a copy costs a fallback rather than the context itself. |
| `egressMediation` | `object` |  | EgressMediation interposes a proxy in the runtime pod that the agent's traffic cannot route around, so the tool access its wiring granted is enforced somewhere the agent does not control. ABSENT means today's pod exactly: no proxy, no interception, no added containers. The RUNTIME declares it because enabling it changes what the pod may do at startup, and a namespace under `restricted` Pod Security admission cannot run it at all. That is an execution-substrate property, which is what an AgentRuntime is for. |
| `egressMediation.excludePorts` | `[]integer` |  | ExcludePorts are destination ports left unredirected. For destinations that must not pass through a userspace proxy at all — not for tuning. Anything excluded here is reachable by the agent UNMEDIATED, so the list is a hole in the boundary by construction and is reported as one. |
| `egressMediation.port` | `integer` |  | Port the proxy listens on inside the pod, and the port the agent's traffic is redirected to. Overridable only because a runtime image may already use the default. Nothing outside the pod can reach it — the two containers share a network namespace and no Service names it. |
| `egressMediation.resources` | `object` |  | Resources for the proxy container. |
| `egressMediation.resources.claims` | `[]object` |  | Claims lists the names of resources, defined in spec.resourceClaims, that are used by this container. This is an alpha field and requires enabling the DynamicResourceAllocation feature gate. This field is immutable. It can only be set for containers. |
| `egressMediation.resources.claims[].name` | `string` | **yes** | Name must match the name of one entry in pod.spec.resourceClaims of the Pod where this field is used. It makes that resource available inside a container. |
| `egressMediation.resources.claims[].request` | `string` |  | Request is the name chosen for a request in the referenced claim. If empty, everything from the claim is made available, otherwise only the result of this request. |
| `egressMediation.resources.limits` | `map[string]object` |  | Limits describes the maximum amount of compute resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| `egressMediation.resources.requests` | `map[string]object` |  | Requests describes the minimum amount of compute resources required. If Requests is omitted for a container, it defaults to Limits if that is explicitly specified, otherwise to an implementation-defined value. Requests cannot exceed Limits. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| `env` | `[]object` |  | Env: extra environment for every worker of this runtime. |
| `env[].name` | `string` | **yes** | Name of the environment variable. Must be a C_IDENTIFIER. |
| `env[].value` | `string` |  | Variable references $(VAR_NAME) are expanded using the previously defined environment variables in the container and any service environment variables. If a variable cannot be resolved, the reference in the input string will be unchanged. Double $$ are reduced to a single $, which allows for escaping the $(VAR_NAME) syntax: i.e. "$$(VAR_NAME)" will produce the string literal "$(VAR_NAME)". Escaped references will never be expanded, regardless of whether the variable exists or not. Defaults to "". |
| `env[].valueFrom` | `object` |  | Source for the environment variable's value. Cannot be used if value is not empty. |
| `env[].valueFrom.configMapKeyRef` | `object` |  | Selects a key of a ConfigMap. |
| `env[].valueFrom.configMapKeyRef.key` | `string` | **yes** | The key to select. |
| `env[].valueFrom.configMapKeyRef.name` | `string` |  | Name of the referent. This field is effectively required, but due to backwards compatibility is allowed to be empty. Instances of this type with an empty value here are almost certainly wrong. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names |
| `env[].valueFrom.configMapKeyRef.optional` | `boolean` |  | Specify whether the ConfigMap or its key must be defined |
| `env[].valueFrom.fieldRef` | `object` |  | Selects a field of the pod: supports metadata.name, metadata.namespace, `metadata.labels['<KEY>']`, `metadata.annotations['<KEY>']`, spec.nodeName, spec.serviceAccountName, status.hostIP, status.podIP, status.podIPs. |
| `env[].valueFrom.fieldRef.apiVersion` | `string` |  | Version of the schema the FieldPath is written in terms of, defaults to "v1". |
| `env[].valueFrom.fieldRef.fieldPath` | `string` | **yes** | Path of the field to select in the specified API version. |
| `env[].valueFrom.resourceFieldRef` | `object` |  | Selects a resource of the container: only resources limits and requests (limits.cpu, limits.memory, limits.ephemeral-storage, requests.cpu, requests.memory and requests.ephemeral-storage) are currently supported. |
| `env[].valueFrom.resourceFieldRef.containerName` | `string` |  | Container name: required for volumes, optional for env vars |
| `env[].valueFrom.resourceFieldRef.divisor` | `object` |  | Specifies the output format of the exposed resources, defaults to "1" |
| `env[].valueFrom.resourceFieldRef.resource` | `string` | **yes** | Required: resource to select |
| `env[].valueFrom.secretKeyRef` | `object` |  | Selects a key of a secret in the pod's namespace |
| `env[].valueFrom.secretKeyRef.key` | `string` | **yes** | The key of the secret to select from. Must be a valid secret key. |
| `env[].valueFrom.secretKeyRef.name` | `string` |  | Name of the referent. This field is effectively required, but due to backwards compatibility is allowed to be empty. Instances of this type with an empty value here are almost certainly wrong. More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names |
| `env[].valueFrom.secretKeyRef.optional` | `boolean` |  | Specify whether the Secret or its key must be defined |
| `home` | `object` |  | Home volume for durable agent session state. |
| `home.emptyDir` | `boolean` |  | EmptyDir (default when no pvcRef): session state dies with the pod. |
| `home.pvcRef` | `object` |  | PVCRef mounts an existing (usually RWX) PVC at /data/home. |
| `home.pvcRef.name` | `string` | **yes** | Name of the referenced object. |
| `idleTtlMinutes` | `integer` |  | IdleTTLMinutes before an idle worker exits (respawned on demand). |
| `image` | `string` | **yes** | Image implementing the work contract. Derive your own to add tooling: what an agent may REACH is wiring, so an image never grants it. |
| `nodeSelector` | `map[string]string` |  | NodeSelector placing runtime pods, applied with Tolerations and Affinity below. |
| `resources` | `object` |  | Resources default for runtime pods (AgentProfile.resources overrides). |
| `resources.claims` | `[]object` |  | Claims lists the names of resources, defined in spec.resourceClaims, that are used by this container. This is an alpha field and requires enabling the DynamicResourceAllocation feature gate. This field is immutable. It can only be set for containers. |
| `resources.claims[].name` | `string` | **yes** | Name must match the name of one entry in pod.spec.resourceClaims of the Pod where this field is used. It makes that resource available inside a container. |
| `resources.claims[].request` | `string` |  | Request is the name chosen for a request in the referenced claim. If empty, everything from the claim is made available, otherwise only the result of this request. |
| `resources.limits` | `map[string]object` |  | Limits describes the maximum amount of compute resources allowed. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| `resources.requests` | `map[string]object` |  | Requests describes the minimum amount of compute resources required. If Requests is omitted for a container, it defaults to Limits if that is explicitly specified, otherwise to an implementation-defined value. Requests cannot exceed Limits. More info: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| `serviceAccountName` | `string` |  | ServiceAccountName is this runtime's security identity: its RBAC defines exactly what agents executing on this runtime may do in the cluster. Give each runtime with a different trust level its OWN ServiceAccount — runtimes sharing an SA share powers. Falls back to the operator's default runtime SA when empty. |
| `workspace` | `object` |  | Workspace volume for the repository checkout. Absent = ephemeral, which is always correct — persisting it preserves uncommitted work across a pod restart and skips the re-clone. |
| `workspace.emptyDir` | `boolean` |  | EmptyDir (default when no pvcRef): the checkout dies with the pod, which costs a re-clone and nothing else. |
| `workspace.pvcRef` | `object` |  | PVCRef mounts an existing (RWX when conversations run concurrently) PVC, one subdirectory per conversation, at /data/workspace. |
| `workspace.pvcRef.name` | `string` | **yes** | Name of the referenced object. |

### status

Written by the operator. Read it, never set it.

| Field | Type | Required | Description |
|---|---|---|---|
| `conditions` | `[]object` |  |  |
| `conditions[].lastTransitionTime` | `string` | **yes** | lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the underlying condition changed. If that is not known, then using the time when the API field changed is acceptable. |
| `conditions[].message` | `string` | **yes** | message is a human readable message indicating details about the transition. This may be an empty string. |
| `conditions[].observedGeneration` | `integer` |  | observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date with respect to the current state of the instance. |
| `conditions[].reason` | `string` | **yes** | reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of specific condition types may define expected values and meanings for this field, and whether the values are considered a guaranteed API. The value should be a CamelCase string. This field may not be empty. |
| `conditions[].status` | `string` | **yes** | status of the condition, one of True, False, Unknown. |
| `conditions[].type` | `string` | **yes** | type of condition in CamelCase or in foo.example.com/CamelCase. |

## Conversation

ConversationSpec pins a conversation to its chat surfaces and an agent profile, and carries its queue of work units (append-only; pruned once processed).

### spec

| Field | Type | Required | Description |
|---|---|---|---|
| `channelRefs` | `[]object` |  | ChannelRefs — every listed channel mirrors the whole conversation (own thread per channel, replies and acks fanned out). Empty = chat-less (HTTP-only / shadow). |
| `channelRefs[].name` | `string` | **yes** | Name of the referenced object. |
| `inputs` | `[]object` |  |  |
| `inputs[].agent` | `string` |  | Agent is DEPRECATED and no longer written. It carried the per-message agent override of the retired `/<pipeline>:<agent>` addressing form, which let whoever typed it select an agent definition the WIRING never declared. A Pipeline names one profile and a profile names one agent, so the agent is already fully determined by the wiring. Dispatch still READS it for one release, so inputs already queued when the manager restarts dispatch to the agent they were parsed with. Same posture as the retired `sessionId` dual-read; removing the field is a later change. Deprecated: nothing sets this. Do not add a writer. |
| `inputs[].id` | `string` | **yes** |  |
| `inputs[].origin` | `object` |  | Origin is where this input came from. Absent on inputs created before provenance existed — and an absent origin means NOT POSTED to bound channels, so upgrading cannot fill open threads with history. |
| `inputs[].origin.kind` | `string` | **yes** | OriginKind says HOW an input reached the manager. Two values, and there are only two doors: a signal through a claimed SignalSource, or a channel the user is already looking at. (`POST /task` was a third once; it is gone.) |
| `inputs[].origin.name` | `string` |  | Name is the SignalSource or Channel the input came from. |
| `inputs[].origin.sender` | `string` |  | Sender is the transport-side identity that typed this input, when the serving adapter supplied one. Attribution, never authority: nothing is resolved through it and no permission reads it. It is recorded because a person's message is now delivered to every OTHER bound surface, and reconciliation composes that delivery from the conversation alone. An in-memory sender would leave the same message attributed on the fast path and anonymous when re-derived after a restart — the same class of bug the delivery markers on runs fixed. A chat signal carries the same fact in its labels (LabelChatSender); this is where the CHANNEL lane keeps it. |
| `inputs[].origin.signalKind` | `string` |  | SignalKind is the originating signal's lane (alert \| job \| task \| chat) for `signal` origins, empty otherwise. It says whether a PERSON typed this input — which decides how it is rendered on the surfaces that did not show it (somebody's words, or the event that opened the conversation). It does NOT decide whether it is delivered: that is per destination, read off the origin SURFACE. |
| `inputs[].payload` | `string` |  |  |
| `inputs[].payloadRef` | `object` |  | ObjectRef references another object by name (same namespace). |
| `inputs[].payloadRef.name` | `string` | **yes** | Name of the referenced object. |
| `inputs[].receivedAt` | `string` |  |  |
| `inputs[].type` | `string` | **yes** | InputType classifies a work unit. |
| `mcpConfigs` | `object` |  | ToolingBinding binds MCP configs to a wiring: an ordered set of refs. Content lives entirely in the referenced CRs (MCPConfig) — the binding carries refs only. There is deliberately no mode here. MCP SERVERS reach a run only through the compiled mcp.json, and an agent definition has no field that declares one — so there is nothing on the other side for a mode to compose against, and its two values would do the same thing. (Tools are different: see ToolsetBinding.) |
| `mcpConfigs.refs` | `[]object` | **yes** | Refs are applied in order: MCP server keys are overlaid with the later ref winning a collision. |
| `mcpConfigs.refs[].name` | `string` | **yes** | Name of the referenced object. |
| `originReader` | `object` |  | OriginReader records WHO started this conversation, so their own read watermark can be stamped the moment their thread is created — the person who typed the request has by definition seen it, and presenting it back to them as unread before any answer exists is the one case the watermark rule gets plainly wrong. Written once at creation and read EXACTLY ONCE, when the binding on its channel is established. It is provenance in the same sense as PipelineRef: nothing resolves anything through it, and it grants nothing. The key is OPAQUE — the originating surface's own salted hash, in that surface's key space, which is why the channel is recorded beside it. No identity is stored, here or anywhere else on this object. |
| `originReader.channel` | `string` | **yes** |  |
| `originReader.key` | `string` | **yes** |  |
| `pipelineRef` | `object` |  | PipelineRef names the Pipeline that ORIGINATED this conversation. PROVENANCE, NEVER WIRING. It is written once at creation and read for exactly two things: scoping conversation REUSE, and ATTRIBUTION in displays. Nothing resolves a profile, a channel set or a capability through it — those come from the materialized fields beside it, which is what keeps editing or deleting the Pipeline from re-wiring a conversation already running. Resolving anything through this ref would undo that. It exists because a source is SHAREABLE: two Pipelines listing one source both open a conversation per signal, and those conversations carry the same signature. Without the ref, the second Pipeline's next signal would be absorbed by the first Pipeline's conversation and run under the wrong profile with the wrong tools. It also replaces attribution-by-inference, which went blank exactly when two Pipelines wired identically. Absent on conversations predating the field. Nothing backfills it — inference is what it replaces — so an empty ref is read conservatively (see routeSignalGroup: reusable only while ONE Ready Pipeline serves the source). |
| `pipelineRef.name` | `string` | **yes** | Name of the referenced object. |
| `profileRef` | `object` | **yes** | ObjectRef references another object by name (same namespace). |
| `profileRef.name` | `string` | **yes** | Name of the referenced object. |
| `signal` | `object` |  | Signal is what the ORIGINATING SIGNAL was, for attribution. PROVENANCE, exactly like PipelineRef: written once at creation, read only for display, and nothing is ever resolved through it. It is on the CONVERSATION because a conversation has exactly one originating signal — reuse is scoped to one signature and one pipeline, so every signal that lands here came from the same source with the same grouping labels. IT USED TO LIVE ONLY ON THINGS BUILT TO BE PRUNED. The source was on `spec.inputs[].origin.name`, which `pruneProcessed` empties, and the labels were on the `ConversationInput`, which is DELETED with the queue entry. So a finished conversation kept its answer, kept the question, and could not say what started it — a viewer showed the phase, the profile and the pipeline of an alert and not the source that fired it or a single one of its labels. The same loss `status.runs[].inputs[]` was added to fix, two fields over. Absent on conversations created before this field, and on anything a channel started. Render it as absent rather than guessing. |
| `signal.labels` | `map[string]string` |  | Labels are the signal's grouping labels, as the adapter sent them. BOUNDED at MaxSignalLabels. A Conversation is long-lived where the ConversationInput that used to hold these was not, and an adapter's label set is its own business — an unbounded map on a durable object is an etcd cost nobody chose. |
| `signal.sourceRef` | `object` |  | SourceRef is the SignalSource this conversation came from. |
| `signal.sourceRef.name` | `string` | **yes** | Name of the referenced object. |
| `signature` | `string` |  | Signature groups same/similar problems into one conversation (e.g. alertgroup/alertname/namespace, job:<name>). |
| `title` | `string` |  |  |
| `toolsets` | `object` |  | Toolsets / MCPConfigs are the originating Pipeline's tooling bindings, snapshotted at creation like ChannelRefs and ProfileRef: materialized per-conversation state, NOT wiring — nothing sets them by hand, and re-wiring the pipeline affects only new conversations. Only the refs are snapshotted; the referenced CRs' CONTENT is re-read at every use, so editing a toolset or config reaches running conversations. Every origination has a Pipeline behind it — a signal of any kind through a claimed source, or a chat command naming one — and a conversation whose Pipeline declared no bindings carries none, because nothing else supplies them: profiles carry no capabilities at all. |
| `toolsets.mode` | `string` |  | Mode composes this binding's tools with the agent definition's: merge unions them (the agent keeps what it declared, the wiring adds), overwrite passes the wiring's alone (the agent's declaration does not apply to this route). Built-ins included — name them in the toolset. |
| `toolsets.refs` | `[]object` | **yes** | Refs are applied in order: tool lists concatenate with dedup, the first occurrence keeping its position. |
| `toolsets.refs[].name` | `string` | **yes** | Name of the referenced object. |

### status

Written by the operator. Read it, never set it.

| Field | Type | Required | Description |
|---|---|---|---|
| `closedAt` | `string` |  | ClosedAt stamps the transition into phase Closed, and is the ORIGIN of the delete clock — the only thing that reads it. A dedicated timestamp rather than the Closed condition's lastTransitionTime: a condition's transition time is rewritten by any reason change on the same condition, so a clock built on it can be reset by an unrelated status update. This is written once, at the transition, and CLEARED by a reopen — which is what stops the delete clock. |
| `conditions` | `[]object` |  |  |
| `conditions[].lastTransitionTime` | `string` | **yes** | lastTransitionTime is the last time the condition transitioned from one status to another. This should be when the underlying condition changed. If that is not known, then using the time when the API field changed is acceptable. |
| `conditions[].message` | `string` | **yes** | message is a human readable message indicating details about the transition. This may be an empty string. |
| `conditions[].observedGeneration` | `integer` |  | observedGeneration represents the .metadata.generation that the condition was set based upon. For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date with respect to the current state of the instance. |
| `conditions[].reason` | `string` | **yes** | reason contains a programmatic identifier indicating the reason for the condition's last transition. Producers of specific condition types may define expected values and meanings for this field, and whether the values are considered a guaranteed API. The value should be a CamelCase string. This field may not be empty. |
| `conditions[].status` | `string` | **yes** | status of the condition, one of True, False, Unknown. |
| `conditions[].type` | `string` | **yes** | type of condition in CamelCase or in foo.example.com/CamelCase. |
| `contextCheckpoint` | `object` |  | RuntimeStartFailures counts CONSECUTIVE failures to bring a runtime pod up, and is reset to zero the moment one reaches Running. It exists to back off. A pod that cannot start for an environmental reason — a volume that will not attach is the case this was built for — fails again immediately if it is recreated immediately, and the resulting hot loop buys nothing while filling the event stream. The count is what makes the interval grow. Kept on status rather than in memory because the decision must survive a manager restart: a process that forgets is a process that hot-loops from zero every rollout, exactly when a storage incident is most likely. ContextCheckpoint records the most recent SUCCESSFUL durable copy of this conversation's context. Durable state rather than telemetry, and the distinction is load-bearing. The activity log is bounded and lossy by design, but whether a conversation has a usable context after a crash decides whether it can continue at all — so it cannot depend on a record that may have been evicted from a ring buffer. Written ONLY when a checkpoint actually transferred data. A skipped checkpoint writes nothing: recording every skip would patch every conversation on every interval forever, which is precisely the write amplification that suppressed signals already avoid by writing cooldown only on admission. |
| `contextCheckpoint.at` | `string` | **yes** | At is when the copy completed. |
| `contextCheckpoint.bytes` | `integer` |  | Bytes transferred by this checkpoint. Zero is meaningful: it means the copy ran and found nothing changed. |
| `contextCheckpoint.generation` | `string` |  | Generation names the copy on the volume, so an operator recovering by hand knows which directory to look in and a restore can fall back to an earlier one. |
| `contextCheckpoint.quiesced` | `boolean` | **yes** | Quiesced reports whether this copy was taken at a WORK BOUNDARY, with nothing inflight, or during a run. A mid-run copy is still worth taking — a long run is exactly what a crash would otherwise lose in full — but it may contain a partially written file. Labelling it is what lets a restore, and a person, tell a known-consistent copy from a best-effort one instead of guessing. |
| `inflight` | `object` |  | InflightRun tracks the unit currently dispatched to the worker. |
| `inflight.dispatchedAt` | `string` |  |  |
| `inflight.inputIds` | `[]string` |  |  |
| `inflight.runId` | `string` | **yes** |  |
| `lastActivity` | `string` |  |  |
| `lastRuntimeStartFailure` | `string` |  | LastRuntimeStartFailure stamps the most recent reap of a pod that never started. Together with RuntimeStartFailures it is the whole of the backoff state — the delay is derived from the two, never stored, so nothing can disagree about when the next attempt is due. |
| `phase` | `string` |  | ConversationPhase is the coarse conversation state. |
| `processedInputIds` | `[]string` |  | IDs of processed inputs (bounded; used to prune spec.inputs). |
| `reopens` | `integer` |  | Reopens counts how many times this conversation has been reopened. It is not decoration: ensure-topic op ids are STABLE per conversation×channel so reconciliation can re-derive them, which means a reopen's request to re-establish a thread would otherwise dedup against the original topic creation and never reach the adapter. The count makes each reopen's op distinct while keeping every one of them derivable. |
| `runs` | `[]object` |  | Last runs, newest last (bounded). |
| `runs[].delivered` | `[]string` |  | Delivered names the bound channels whose thread has already received this run's reply. It is what makes the reply DERIVABLE: reconciliation enqueues a send for every bound thread missing from this list, so a manager restart between `/work/done` and the adapter claiming the op re-derives the answer instead of losing it. Per-THREAD rather than a per-run boolean on purpose: a fan-out to three channels can be interrupted after one succeeds, and a boolean would either re-post to the delivered thread or abandon the other two. |
| `runs[].deliveryTracked` | `boolean` |  | DeliveryTracked marks a run recorded by a manager that tracks delivery. It is what tells an UNDELIVERED run from a PRE-UPGRADE one — both look like "a completed run with no delivery markers", and both completed before the current process started, so no timestamp can separate them. Absent (an older manager wrote the run): reconciliation records it delivered without sending, so upgrading never re-posts history. Set with no markers: the reply is genuinely owed and is re-enqueued. |
| `runs[].exitCode` | `integer` |  |  |
| `runs[].finishedAt` | `string` |  |  |
| `runs[].inputIds` | `[]string` |  |  |
| `runs[].inputs` | `[]object` |  | Inputs are the messages this run consumed, kept where the run keeps its answer — so a conversation records the questions as well as the answers and its whole timeline reads off status in order. The queue is a QUEUE: spec.inputs[] is pruned once dispatch has consumed an entry, which is what stops answered work running twice, and it took the only copy of what a person said with it. This is the copy that stays. Written in the SAME status write that marks the inputs processed, and therefore strictly before the reconciler prunes them: a record written afterwards would be lost by a crash in between, permanently. Absent on runs written before this existed. A viewer renders what it has and invents nothing. |
| `runs[].inputs[].id` | `string` | **yes** | ID is the input's id, the same one runs[].inputIds names. |
| `runs[].inputs[].payloadRef` | `object` |  | PayloadRef names the ConversationInput the full text was read from, when there was one. A CITATION of where the message lived, not a live pointer: that object is deleted with the queue entry, which is why the beginning of the text is kept here rather than only referenced. |
| `runs[].inputs[].payloadRef.name` | `string` | **yes** | Name of the referenced object. |
| `runs[].inputs[].receivedAt` | `string` |  |  |
| `runs[].inputs[].sender` | `string` |  | Sender is who typed it, when a sender was named. Attribution only. |
| `runs[].inputs[].surface` | `string` |  | Surface is the channel this message was typed on, empty when no surface displayed it (an alert, a job tick, a posted task). |
| `runs[].inputs[].text` | `string` |  | Text is the message, inlined up to MaxRecordedInputText. |
| `runs[].inputs[].truncated` | `boolean` |  | Truncated says Text is NOT the whole message — it was longer than the cap and what is kept here is its beginning. A reader must present it as a fragment rather than as what somebody said. |
| `runs[].inputs[].type` | `string` |  | InputType classifies a work unit. |
| `runs[].jobKind` | `string` |  |  |
| `runs[].result` | `string` |  |  |
| `runs[].runId` | `string` | **yes** |  |
| `runs[].startedAt` | `string` |  |  |
| `runs[].status` | `string` | **yes** |  |
| `runtimeContextId` | `string` |  | RuntimeContextID is the RUNTIME's opaque handle for this conversation's accumulated context — every message, tool call and model response it has built up. The manager stores it and hands it back on the next work unit; it never interprets it and never assumes where the context lives (session files on a volume, a thread id at a vendor API, rows in a database are all valid, and none of them are distinguishable from here). Named for what agent-ops means, not for one backend's word: "session" is claude-code's noun, and a vendor's noun in this API would teach every later reader that the operator knows what is inside the handle. LATEST-WINS. Every completed run's reported handle replaces this one. It was write-once, which was unsound: a run may legitimately end in a different context than it was asked to continue, and keeping the first handle then named something that no longer existed — so every later message repeated the same failed continuation and one recoverable loss became permanent. |
| `runtimePod` | `string` |  |  |
| `runtimeStartFailures` | `integer` |  |  |
| `sessionId` | `string` |  | SessionID is the former name of RuntimeContextID. DEPRECATED, and retained for exactly one release so the rename cannot do the harm this field exists to prevent: it is still DECODED, so a conversation written by an older manager is adopted rather than losing its handle at the moment of upgrade. Readers must prefer RuntimeContextID and fall back to this; writers must only ever set RuntimeContextID. |
| `threads` | `[]object` |  | Threads: one binding per bound channel whose topic has been created. |
| `threads[].channel` | `string` | **yes** | Channel name (same namespace). |
| `threads[].readAt` | `string` |  | ReadAt is how far this thread has been SEEN — reported by the adapter that serves the channel, never inferred. MONOTONIC: it only moves forward, and the manager clamps it to its own clock, so neither a stale client nor a skewed one can un-read a thread or mark future activity read. Per THREAD, therefore per CHANNEL: a conversation bound to Telegram and the console has two audiences reading it in two places, and one shared mark would let a Telegram reader clear the console's. |
| `threads[].readTracked` | `boolean` |  | ReadTracked marks a binding created by a manager that tracks reads. It is what tells a NEVER-READ binding from a PRE-UPGRADE one — both look like "a binding with no readAt", and no timestamp can separate them, exactly as with status.runs[].deliveryTracked. Absent (an older manager bound the thread): treated as READ, so upgrading never presents the whole namespace as new. Set with no ReadAt: genuinely unseen, and unread. |
| `threads[].readers` | `[]object` |  | Readers is the PER-IDENTITY watermark, for transports that can tell one reader from another. ReadAt above stays the CHANNEL-WIDE mark and is not replaced by this: a Telegram topic is read or it is not, and there is nobody to attribute that to — so an adapter reporting no reader keeps reporting only the channel-wide mark and stays fully conformant. BOUNDED at MaxReadersPerThread, oldest watermark evicted first, because this list grows with readers × conversations. Eviction is not a loss: an evicted reader falls back to the channel-wide mark, exactly as a reader who has never reported does. |
| `threads[].readers[].key` | `string` | **yes** | Key identifies the reader and is OPAQUE to the manager — a salted hash the reporting adapter computed. The manager never derives it, never interprets it, and stores no identity it was derived from, so a conversation records THAT someone read it without recording WHO. Same contract as ThreadID and RuntimeContextID. |
| `threads[].readers[].readAt` | `string` |  |  |
| `threads[].threadId` | `string` | **yes** | ThreadID — an opaque string in the channel type's own id space (e.g. a Telegram forum topic id in decimal, a Slack ts). |
| `threadsArchived` | `[]string` |  | ThreadsArchived names the bound channels whose thread has already been archived by a completed close-topic op. This is what retires "close-topic is the ONE op not derivable from CR state". It was the exception only because it was enqueued while the object was disappearing, leaving nothing to record against. Closing no longer deletes, so the object survives — and a Closed conversation whose thread is missing from this list is an archive still owed, re-derivable on the next reconcile exactly as runs[].delivered[] makes a reply derivable. Per-THREAD for the same reason: a fan-out interrupted after one channel must not re-archive that one or abandon the rest. |

## ConversationInput

ConversationInputSpec carries a large out-of-line work-unit payload.

### spec

| Field | Type | Required | Description |
|---|---|---|---|
| `conversationRef` | `object` | **yes** | ObjectRef references another object by name (same namespace). |
| `conversationRef.name` | `string` | **yes** | Name of the referenced object. |
| `labels` | `map[string]string` |  | Labels are the originating signal's labels, kept beside the payload rather than on the Conversation: they are per-input, they can be numerous, and this object exists precisely to hold the bulky half of an input. They travel so an adapter can RENDER them — a label table, a chip row, or nothing — from the same data that named the topic. Grouping already consumed them at ingest; this is presentation, and the manager does not decide how much of it a surface shows. |
| `payload` | `string` | **yes** |  |
| `type` | `string` | **yes** | InputType classifies a work unit. |

### status

Written by the operator. Read it, never set it.

| Field | Type | Required | Description |
|---|---|---|---|
| `consumed` | `boolean` |  |  |
| `consumedAt` | `string` |  |  |
