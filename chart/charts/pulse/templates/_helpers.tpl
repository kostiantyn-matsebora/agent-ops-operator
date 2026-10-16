{{- /*
The bundle is active when it is enabled. There is no demo branch, and that is
deliberate: every component here needs a Pulse instance and an API token, and
a demo cluster has neither. Self-gating (rather than a Helm `condition:`)
keeps the shape identical to the other bundles.
*/ -}}
{{- define "pulse.active" -}}
{{- if .Values.enabled -}}
true
{{- end -}}
{{- end -}}

{{- /* Two sources for one credential is ambiguous, so it fails the render
rather than picking one. Same rule telegram and home-assistant apply to their
own credentials. Called with `(dict "label" <values path, for the message> "c"
<the credential sub-object: existingSecret + token>)`. */ -}}
{{- define "pulse.validateCredentialForm" -}}
{{- $label := .label -}}
{{- $c := .c -}}
{{- if and $c.existingSecret $c.token -}}
{{- fail (printf "pulse: set EITHER %s.existingSecret (%v) OR .token, not both — two sources for one value is ambiguous." $label $c.existingSecret) -}}
{{- end -}}
{{- end -}}

{{- /* Names of the Secrets this bundle CREATES when the raw value itself was
supplied rather than an existing Secret's name. Fixed, because more than one
template references each and a values path would be a second place to
disagree. */ -}}
{{- define "pulse.ingestSecretCreatedName" -}}agentops-pulse-ingest{{- end -}}
{{- define "pulse.observerSecretCreatedName" -}}agentops-pulse-observer{{- end -}}
{{- define "pulse.operatorSecretCreatedName" -}}agentops-pulse-operator{{- end -}}

{{- /* The ingest lane's registration credential: an existing Secret's name,
or the one this bundle creates from a supplied token. Empty when neither form
is set — the source still serves, reporting a degraded Ready reason rather
than failing the render. */ -}}
{{- define "pulse.ingestSecret" -}}
{{- include "pulse.validateCredentialForm" (dict "label" "signal.credentials" "c" .Values.signal.credentials) -}}
{{- $c := .Values.signal.credentials -}}
{{- if $c.existingSecret -}}
{{ $c.existingSecret }}
{{- else if $c.token -}}
{{ include "pulse.ingestSecretCreatedName" . }}
{{- end -}}
{{- end -}}

{{- /* The observer MCP pair's credential — a Pulse API token scoped to
monitoring:read (at least). */ -}}
{{- define "pulse.observerSecret" -}}
{{- include "pulse.validateCredentialForm" (dict "label" "mcp.observer.credentials" "c" .Values.mcp.observer.credentials) -}}
{{- $c := .Values.mcp.observer.credentials -}}
{{- if $c.existingSecret -}}
{{ $c.existingSecret }}
{{- else if $c.token -}}
{{ include "pulse.observerSecretCreatedName" . }}
{{- end -}}
{{- end -}}

{{- /* The operator MCP pair's credential — the wider-scoped Pulse API token.
Independent of the observer one: an install may supply either, both, or
neither. */ -}}
{{- define "pulse.operatorSecret" -}}
{{- include "pulse.validateCredentialForm" (dict "label" "mcp.operator.credentials" "c" .Values.mcp.operator.credentials) -}}
{{- $c := .Values.mcp.operator.credentials -}}
{{- if $c.existingSecret -}}
{{ $c.existingSecret }}
{{- else if $c.token -}}
{{ include "pulse.operatorSecretCreatedName" . }}
{{- end -}}
{{- end -}}

{{- /* Whether the OBSERVER MCP pair renders. Needs the component on AND a
credential: an MCPConfig whose header points at nothing costs the agent its
tools and looks installed doing it. Soft-gated rather than failed, mirroring
home-assistant's own `mcpEnabled` — an install may enable the component ahead
of supplying the token. */ -}}
{{- define "pulse.observerMcpEnabled" -}}
{{- if and .Values.mcp.observer.enabled (include "pulse.observerSecret" .) -}}
true
{{- end -}}
{{- end -}}

{{- define "pulse.operatorMcpEnabled" -}}
{{- if and .Values.mcp.operator.enabled (include "pulse.operatorSecret" .) -}}
true
{{- end -}}
{{- end -}}

{{- define "pulse.observerProfileEnabled" -}}
{{- if .Values.profiles.observer.enabled -}}
true
{{- end -}}
{{- end -}}

{{- define "pulse.operatorProfileEnabled" -}}
{{- if .Values.profiles.operator.enabled -}}
true
{{- end -}}
{{- end -}}

{{- /* Whether the bundle renders wiring of its own. Returns "true" or "".
Bundles normally ship none: a route names objects from several components and
only the parent sees them all. This bundle is the exception because it owns
its whole lane — the source, both profiles and both MCP pairs — so the only
foreign name a route needs is a channel, which is values-supplied and omitted
when unset.

DEFAULTS OFF, and nothing forces it on: unlike kubernetes there is no demo
path, so a plain boolean says everything a nullable one would. */ -}}
{{- define "pulse.wiringActive" -}}
{{- if and (include "pulse.active" .) .Values.pipelines.enabled -}}
true
{{- end -}}
{{- end -}}

{{- /* Whether each route actually renders: the wiring component is active,
the route's own toggle is on, and its profile is enabled. Read by
pipelines.yaml, capabilities.yaml, pipeline-identity.yaml and the parent
chart's `agentops.defaultRuntimeGuard` — one answer, four readers, so they
cannot disagree about whether a route rendered. */ -}}
{{- define "pulse.observerRouteEnabled" -}}
{{- if and (include "pulse.wiringActive" .) .Values.pipelines.observer.enabled (include "pulse.observerProfileEnabled" .) -}}
true
{{- end -}}
{{- end -}}

{{- define "pulse.operatorRouteEnabled" -}}
{{- if and (include "pulse.wiringActive" .) .Values.pipelines.operator.enabled (include "pulse.operatorProfileEnabled" .) -}}
true
{{- end -}}
{{- end -}}

{{- /* Fixed: mcp.yaml defaults an empty mcp.url onto this Service, and the
server's RBAC-equal guard references it. */ -}}
{{- define "pulse.mcpServerName" -}}
agentops-mcp-pulse
{{- end -}}

{{- /* The shared MCP endpoint both pairs point at: the configured URL, or the
deployed Service. An enabled MCP pair with NEITHER fails the render rather
than rendering an MCPConfig that points nowhere. */ -}}
{{- define "pulse.mcpServerURL" -}}
{{- if .Values.mcp.url -}}
{{ .Values.mcp.url }}
{{- else if .Values.mcpServer.enabled -}}
{{ printf "http://%s.%s.svc:%v/mcp" (include "pulse.mcpServerName" .) .Release.Namespace .Values.mcpServer.port }}
{{- else -}}
{{- fail "pulse: mcp.url is required when an mcp.observer/mcp.operator pair is enabled (or set mcpServer.enabled=true to deploy mcp-pulse in-cluster) — an MCPConfig pointing nowhere silently costs an agent its tools" -}}
{{- end -}}
{{- end -}}

{{- /* The headers a pair's MCPConfig sends. Values win outright. Otherwise
the Authorization header is sourced from the `authorization` key of that
pair's own credential Secret — the key holding "Bearer <token>" rather than
the bare token, because a header value referencing a Secret is substituted
WHOLE and nothing can prepend a scheme to it. Resolved in the runtime pod; the
manager reads no Secrets. */ -}}
{{- define "pulse.observerHeaders" -}}
{{- with .Values.mcp.observer.headers -}}
{{ toYaml . }}
{{- else -}}
- name: Authorization
  valueFrom:
    secretKeyRef:
      name: {{ include "pulse.observerSecret" . }}
      key: authorization
{{- end -}}
{{- end -}}

{{- define "pulse.operatorHeaders" -}}
{{- with .Values.mcp.operator.headers -}}
{{ toYaml . }}
{{- else -}}
- name: Authorization
  valueFrom:
    secretKeyRef:
      name: {{ include "pulse.operatorSecret" . }}
      key: authorization
{{- end -}}
{{- end -}}

{{- /* A wildcard would span both risk tiers through one shared server key —
exactly what the enumerated lists replace. Called with the list of tool
patterns; fails the render on the first one containing "*". */ -}}
{{- define "pulse.refuseWildcardTools" -}}
{{- range . -}}
{{- if contains "*" . -}}
{{- fail (printf "pulse: tool patterns must be enumerated, not wildcarded (found %q) — mcp__pulse__* would span both the observability and admin tiers, defeating the risk split between them" .) -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- /* The server's own Pulse backend — mcp-pulse's PULSE_URL. Defaults onto
the ingest lane's own base URL, since the common case is one Pulse instance
serving both lanes; never derived further than that. */ -}}
{{- define "pulse.mcpServerBackend" -}}
{{- or .Values.mcpServer.backend .Values.signal.defaultSource.baseUrl | default "" -}}
{{- end -}}

{{- /* The runtime identity for one of this bundle's routes, named after it. */ -}}
{{- define "pulse.routeServiceAccount" -}}
{{- printf "agentops-%s" .route -}}
{{- end -}}

{{- /* THE ACCOUNT A ROUTE OF THIS BUNDLE RUNS AS, resolved ONCE. Called with
`(dict "ctx" $ "cfg" <route cfg> "p" .Values.pipelines)`. Returns the name, or
EMPTY where the route inherits the release default.

ONE RESOLVER, TWO READERS — `pipelines.yaml`/`capabilities.yaml` write the
name onto the CR and `pipeline-identity.yaml` decides whether to render the
object. Disagreeing means a Pipeline naming an account nothing created, which
fails at POD ADMISSION: no pod, a conversation with no phase, and nothing in
the render to look at.

AN ACCOUNT IS RENDERED ONLY WHERE THE ROUTE IS GRANTED SOMETHING — neither
route needs Kubernetes RBAC by default, since Pulse is reached entirely over
HTTP/MCP, so nothing renders unless an install adds a grant. */ -}}
{{- define "pulse.routeAccount" -}}
{{- $cfg := .cfg -}}
{{- $p := .p -}}
{{- $explicit := $cfg.serviceAccountName | default ($p.serviceAccountName | default "") -}}
{{- if $explicit -}}
{{ $explicit }}
{{- else -}}
{{- $rbac := $cfg.rbac | default dict -}}
{{- if or ($rbac.clusterRoles | default list) ($rbac.bindClusterRoles | default list) -}}
{{ include "pulse.routeServiceAccount" (dict "ctx" .ctx "route" $cfg.name) }}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "pulse.routeAccountRendered" -}}
{{- $cfg := .cfg -}}
{{- $p := .p -}}
{{- if not ($cfg.serviceAccountName | default ($p.serviceAccountName | default "")) -}}
{{- $rbac := $cfg.rbac | default dict -}}
{{- if or ($rbac.clusterRoles | default list) ($rbac.bindClusterRoles | default list) -}}true{{- end -}}
{{- end -}}
{{- end -}}

{{- /* COORDINATOR-MODE CONTRIBUTION (coordinator-deployment-mode): see
kubernetes.coordinatorContribution for the calling convention. BOTH routes
keep the name of the route they replace (`pulse-observer`, `pulse-operator`)
— never merged, the Community-edition privilege split carries over
unchanged. Gated on the SAME per-route helpers `pipelines.yaml` uses, so a
disabled route or profile withholds the same agent here. */ -}}
{{- define "pulse.coordinatorContribution" -}}
{{- $sources := list -}}
{{- $agents := list -}}
{{- if include "pulse.wiringActive" . -}}
{{- $p := .Values.pipelines -}}
{{- $sig := .Values.signal -}}
{{- if and $sig.enabled $sig.defaultSource.enabled -}}{{- $sources = append $sources $sig.defaultSource.name -}}{{- end -}}
{{- if include "pulse.observerRouteEnabled" . -}}
{{- $agents = append $agents (dict "name" $p.observer.name "capability" $p.observer.name "description" $p.observer.description) -}}
{{- end -}}
{{- if include "pulse.operatorRouteEnabled" . -}}
{{- $agents = append $agents (dict "name" $p.operator.name "capability" $p.operator.name "description" $p.operator.description) -}}
{{- end -}}
{{- end -}}
sources:
{{ toYaml $sources | indent 2 }}
agents:
{{ toYaml $agents | indent 2 }}
{{- end -}}
