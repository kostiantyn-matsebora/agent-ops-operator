{{- /*
The bundle is active when it is enabled. There is no demo branch, and that is
deliberate: every component here needs a Home Assistant endpoint and a
credential, and a demo cluster has neither. Self-gating (rather than a Helm
`condition:`) keeps the shape identical to the other bundles.
*/ -}}
{{- define "home-assistant.active" -}}
{{- if .Values.enabled -}}
true
{{- end -}}
{{- end -}}

{{- /* The Home Assistant base URL, trimmed of a trailing slash. Required by
every component, so it is resolved in one place and fails in one place. */ -}}
{{- define "home-assistant.endpoint" -}}
{{- $ep := trimSuffix "/" (.Values.homeAssistant.endpoint | default "") -}}
{{- if not $ep -}}
{{- fail "home-assistant: homeAssistant.endpoint is required (e.g. https://ha.example.org) — every component in this bundle reaches the same instance, and nothing can discover it" -}}
{{- end -}}
{{ $ep }}
{{- end -}}

{{- /* Two sources for one token is ambiguous, so it fails the render rather than
picking one. Same rule telegram applies to its bot credential. */ -}}
{{- define "home-assistant.validateCredentials" -}}
{{- $c := .Values.homeAssistant.credentials -}}
{{- if and $c.controlSecret $c.controlToken -}}
{{- fail (printf "home-assistant: set EITHER homeAssistant.credentials.controlSecret (%v) OR .controlToken, not both — two sources for one token is ambiguous." $c.controlSecret) -}}
{{- end -}}
{{- if and $c.operatorSecret $c.operatorToken -}}
{{- fail (printf "home-assistant: set EITHER homeAssistant.credentials.operatorSecret (%v) OR .operatorToken, not both — two sources for one token is ambiguous." $c.operatorSecret) -}}
{{- end -}}
{{- end -}}

{{- /* Name of the Secret this bundle CREATES for each credential when the token
itself was supplied. Fixed, because two templates and three consumers reference
it and a values path would be a fourth place to disagree. */ -}}
{{- define "home-assistant.controlSecretCreatedName" -}}agentops-ha-control{{- end -}}
{{- define "home-assistant.operatorSecretCreatedName" -}}agentops-ha-operator{{- end -}}

{{- /* The credential the everyday CONTROL lane uses: an existing Secret's name,
or the one this bundle creates from a supplied token. Empty when neither form is
set. NOT a read-only credential — Home Assistant has no such role. */ -}}
{{- define "home-assistant.controlSecret" -}}
{{- include "home-assistant.validateCredentials" . -}}
{{- $c := .Values.homeAssistant.credentials -}}
{{- if $c.controlSecret -}}
{{ $c.controlSecret }}
{{- else if $c.controlToken -}}
{{ include "home-assistant.controlSecretCreatedName" . }}
{{- end -}}
{{- end -}}

{{- /* The credential the FIXING lane uses. Its presence — in EITHER form — is
what decides whether the ops profile and the ops route render at all. */ -}}
{{- define "home-assistant.operatorSecret" -}}
{{- include "home-assistant.validateCredentials" . -}}
{{- $c := .Values.homeAssistant.credentials -}}
{{- if $c.operatorSecret -}}
{{ $c.operatorSecret }}
{{- else if $c.operatorToken -}}
{{ include "home-assistant.operatorSecretCreatedName" . }}
{{- end -}}
{{- end -}}

{{- /* The credential the INGEST lane authenticates with: its own if set, else
the OPERATOR one, else control.

Operator first, and this one is a REQUIREMENT rather than a preference. Home
Assistant's `subscribe_events` is admin-only, and so are `system_log/list` and
`config_entries/get` — the whole read loop. A control token connects, passes
auth, and is then refused the subscription, which surfaces as an Unreachable
source that looks like a network problem. Falling back to control is kept only
so a control-only install fails loudly at the source rather than at render. */ -}}
{{- define "home-assistant.ingestSecret" -}}
{{- $c := .Values.homeAssistant.credentials -}}
{{- or .Values.logsAdapter.source.credentialsSecret $c.ingestSecret (include "home-assistant.operatorSecret" .) (include "home-assistant.controlSecret" .) | default "" -}}
{{- end -}}

{{- /* The credential the MCP path authenticates with: its own if set, else the
CONTROL token, else the operator one.

Control first, and it costs nothing: Home Assistant's MCP server exposes Assist
intents, every one of them within a control user's rights. The operator token's
extra power is configuration, which no MCP tool reaches — so defaulting to it
would widen the shared path and buy no capability. ONE endpoint, ONE token, and
the toolsets are what separate looking from doing. */ -}}
{{- define "home-assistant.mcpSecret" -}}
{{- or .Values.mcp.credentialsSecret (include "home-assistant.controlSecret" .) (include "home-assistant.operatorSecret" .) | default "" -}}
{{- end -}}

{{- /* The MCP endpoint: the configured URL, or Home Assistant's own built-in
MCP Server integration. There is no server workload to deploy here — the house
serves its own. */ -}}
{{- define "home-assistant.mcpURL" -}}
{{- with .Values.mcp.url -}}
{{ . }}
{{- else -}}
{{ include "home-assistant.endpoint" . }}{{ include "home-assistant.mcpPath" . }}
{{- end -}}
{{- end -}}

{{- /* The path for the declared transport. Home Assistant's integration serves
SSE at /mcp_server/sse; /mcp is the streamable-HTTP convention other servers
use. */ -}}
{{- define "home-assistant.mcpPath" -}}
{{- if eq .Values.mcp.transport "sse" -}}/mcp_server/sse{{- else -}}/mcp{{- end -}}
{{- end -}}

{{- /* The headers the MCP path sends.

Values win outright. Otherwise the Authorization header is sourced from the
`authorization` key of the credential Secret — the key holding "Bearer <token>"
rather than the bare token, because a header value referencing a Secret is
substituted WHOLE and nothing can prepend a scheme to it. Resolved in the
runtime pod; the manager reads no Secrets. */ -}}
{{- define "home-assistant.mcpHeaders" -}}
{{- with .Values.mcp.headers -}}
{{ toYaml . }}
{{- else -}}
- name: Authorization
  valueFrom:
    secretKeyRef:
      name: {{ include "home-assistant.mcpSecret" . }}
      key: authorization
{{- end -}}
{{- end -}}

{{- /* Whether the MCP component renders. It needs a credential as well as an
endpoint: an MCPConfig Home Assistant will answer 401 to costs an agent its
tools and looks installed doing it. */ -}}
{{- define "home-assistant.mcpEnabled" -}}
{{- if and .Values.mcp.enabled (include "home-assistant.mcpSecret" .) -}}
true
{{- end -}}
{{- end -}}

{{- /* Whether the ACTIONS toolset exists. Not gated on a credential: Home
Assistant registers the same Assist intents for any user, so there is no
read-only server mode to detect and no credential fact to follow. Turning it off
is how an install says "look, never touch". */ -}}
{{- define "home-assistant.mcpActionsEnabled" -}}
{{- if .Values.mcp.toolsets.actions.enabled -}}
true
{{- end -}}
{{- end -}}

{{- /* Whether the USER profile renders: it needs SOME way to reach the house —
the MCP path or a read credential for the REST one. */ -}}
{{- define "home-assistant.userProfileEnabled" -}}
{{- if and .Values.profiles.user.enabled (or (include "home-assistant.mcpEnabled" .) (include "home-assistant.controlSecret" .)) -}}
true
{{- end -}}
{{- end -}}

{{- /* Whether the OPS profile renders. The operator credential is the
prerequisite: an agent asked to repair an integration with a token that cannot
reconfigure one fails every task it is given, looking installed while it does. */ -}}
{{- define "home-assistant.opsProfileEnabled" -}}
{{- if and .Values.profiles.ops.enabled (include "home-assistant.operatorSecret" .) -}}
true
{{- end -}}
{{- end -}}

{{- /* Whether the bundle renders wiring of its own. Returns "true" or "".

Bundles normally ship none: a route names objects from several components and
only the parent sees them all. This bundle is the exception because it owns its
whole lane — the source, both profiles and both toolsets — so the only foreign
names a route needs are chat sources and channels, and those are values-supplied
and omitted when unset.

It DEFAULTS OFF, and nothing forces it on: unlike kubernetes there is no demo
path, so a plain boolean says everything a nullable one would. */ -}}
{{- define "home-assistant.wiringActive" -}}
{{- if and (include "home-assistant.active" .) .Values.pipelines.enabled -}}
true
{{- end -}}
{{- end -}}

{{- /* Whether a route binds the built-in SHELL toolset, opening the REST path.
Takes the route name, because the answer differs by route.

Explicit wins for both. Otherwise: ON for the ops route, OFF for the control
one. Repairing an integration means touching configuration, and no MCP tool here
reaches configuration — Home Assistant's MCP server exposes Assist intents only
— so REST IS how the operator works, while the everyday agent needs nothing
beyond the intents and would only gain its own token's whole surface. */ -}}
{{- define "home-assistant.restAccessFor" -}}
{{- $ctx := .ctx -}}
{{- if kindIs "bool" $ctx.Values.pipelines.restAccess -}}
{{- if $ctx.Values.pipelines.restAccess }}true{{ end -}}
{{- else if eq .route "ops" -}}
true
{{- end -}}
{{- end -}}

{{- /* Whether the ADMIN MCP path renders. Returns "true" or "".

Off by default and never derived: it is a separate server holding a credential
that can change the whole house, so turning it on is a decision. */ -}}
{{- define "home-assistant.adminMcpEnabled" -}}
{{- if .Values.adminMcp.enabled -}}
true
{{- end -}}
{{- end -}}

{{- /* Whether this bundle DEPLOYS the admin server, as opposed to pointing at
one that already exists (a HACS component inside Home Assistant, or anything
else you run). */ -}}
{{- define "home-assistant.adminMcpServerEnabled" -}}
{{- if and (include "home-assistant.adminMcpEnabled" .) .Values.adminMcpServer.enabled -}}
true
{{- end -}}
{{- end -}}

{{- /* Deployment + Service name of the in-cluster admin MCP server. Fixed,
because the MCPConfig defaults its URL onto this Service. */ -}}
{{- define "home-assistant.adminMcpServerName" -}}
agentops-mcp-ha
{{- end -}}

{{- /* The admin MCP endpoint: the configured URL, or the deployed Service.

An enabled component with NEITHER fails the render rather than rendering an
MCPConfig that points nowhere — which would cost the ops agent its tools and
look installed doing it. */ -}}
{{- define "home-assistant.adminMcpURL" -}}
{{- if .Values.adminMcp.url -}}
{{ .Values.adminMcp.url }}
{{- else if .Values.adminMcpServer.enabled -}}
{{ printf "http://%s.%s.svc:%v%s" (include "home-assistant.adminMcpServerName" .) .Release.Namespace .Values.adminMcpServer.port .Values.adminMcpServer.path }}
{{- else -}}
{{- fail "home-assistant: adminMcp.enabled is true but there is no server to reach — either set adminMcpServer.enabled=true so the bundle deploys one, or set adminMcp.url to a server you already run (for example a HACS MCP integration serving inside Home Assistant). An MCPConfig pointing nowhere silently costs the ops agent its tools" -}}
{{- end -}}
{{- end -}}

{{- /* The credential the deployed admin server authenticates to Home Assistant
with: its own if set, else the OPERATOR token. The operator token is what makes
registry writes possible at all, so a control token here would produce a server
that starts, answers, and refuses every repair. */ -}}
{{- define "home-assistant.adminMcpSecret" -}}
{{- or .Values.adminMcpServer.credentialsSecret (include "home-assistant.operatorSecret" .) | default "" -}}
{{- end -}}

{{- /* The runtime identity for one of this bundle's routes, named after it. */ -}}
{{- define "home-assistant.routeServiceAccount" -}}
{{- printf "agentops-%s" .route -}}
{{- end -}}

{{- /* THE ACCOUNT A ROUTE OF THIS home-assistant RUNS AS, resolved ONCE.

Called with `(dict "ctx" $ "cfg" <route cfg> "p" .Values.pipelines)`. Returns the
name, or EMPTY where the route inherits the release default.

ONE RESOLVER, TWO READERS — `pipelines.yaml` writes the name onto the CR and
`pipeline-identity.yaml` decides whether to render the object. If those two ever
disagreed the failure is a Pipeline naming an account nothing created, which
fails at POD ADMISSION: no pod, a conversation with no phase, and nothing in the
render to look at.

AN ACCOUNT IS RENDERED ONLY WHERE THE ROUTE IS GRANTED SOMETHING. An account
bound to nothing is indistinguishable from the floor every unnamed route already
inherits, so rendering one adds a name to every audit and buys no boundary.
Where a route needs nothing, it names nothing. */ -}}
{{- define "home-assistant.routeAccount" -}}
{{- $cfg := .cfg -}}
{{- $p := .p -}}
{{- $explicit := $cfg.serviceAccountName | default ($p.serviceAccountName | default "") -}}
{{- if $explicit -}}
{{ $explicit }}
{{- else -}}
{{- $rbac := $cfg.rbac | default dict -}}
{{- if or ($rbac.clusterRoles | default list) ($rbac.bindClusterRoles | default list) -}}
{{ include "home-assistant.routeServiceAccount" (dict "ctx" .ctx "route" $cfg.name) }}
{{- end -}}
{{- end -}}
{{- end -}}

{{- /* Does THIS bundle render that account? True only where the route declares a
grant AND named no account of its own — an account the operator names is theirs,
and naming is not creating. */ -}}
{{- define "home-assistant.routeAccountRendered" -}}
{{- $cfg := .cfg -}}
{{- $p := .p -}}
{{- if not ($cfg.serviceAccountName | default ($p.serviceAccountName | default "")) -}}
{{- $rbac := $cfg.rbac | default dict -}}
{{- if or ($rbac.clusterRoles | default list) ($rbac.bindClusterRoles | default list) -}}true{{- end -}}
{{- end -}}
{{- end -}}
