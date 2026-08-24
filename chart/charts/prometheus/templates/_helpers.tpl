{{- /* Active only when enabled directly: demo mode never turns this bundle on. */ -}}
{{- define "prometheus.active" -}}
{{- if .Values.enabled -}}
true
{{- end -}}
{{- end -}}

{{- /* The adapter's deterministic SA; the chart binds to it, the operator grants nothing. */ -}}
{{- define "prometheus.adapterServiceAccount" -}}
agentops-signal-{{ .Values.alertmanager.name }}
{{- end -}}

{{- /* Fixed: mcp.yaml defaults an empty MCPConfig URL onto this Service. */ -}}
{{- define "prometheus.mcpServerName" -}}
agentops-mcp-prometheus
{{- end -}}

{{- /* The server selects ONE transport per process, so path and workload must agree. */ -}}
{{- define "prometheus.mcpPath" -}}
{{- if eq .Values.mcp.transport "sse" -}}/sse{{- else -}}/mcp{{- end -}}
{{- end -}}

{{- /* Never the runtime SA — that separation is why this component exists. */ -}}
{{- define "prometheus.mcpServerServiceAccount" -}}
{{ .Values.mcpServers.serviceAccountName }}
{{- end -}}

{{- /* Wiring gate. The demo branch is inert here (active has no demo path) and is
kept only so the rule reads identically in every bundle that ships wiring. */ -}}
{{- define "prometheus.wiringActive" -}}
{{- if include "prometheus.active" . -}}
{{- if kindIs "bool" .Values.pipelines.enabled -}}
{{- if .Values.pipelines.enabled }}true{{ end -}}
{{- else if dig "demo" "enabled" false (.Values.global | default dict) -}}
true
{{- end -}}
{{- end -}}
{{- end -}}

{{- /* The runtime identity for this bundle's route, named after the route. */ -}}
{{- define "prometheus.routeServiceAccount" -}}
{{- printf "agentops-%s" .route -}}
{{- end -}}

{{- /* THE ACCOUNT THIS BUNDLE'S ROUTE RUNS AS, resolved ONCE. Returns the name,
or EMPTY where the route inherits the release default.

ONE RESOLVER, TWO READERS — `pipelines.yaml` writes the name onto the CR and
`pipeline-identity.yaml` decides whether to render the object. Disagreeing means
a Pipeline naming an account nothing created, which fails at POD ADMISSION: no
pod, a conversation with no phase, and nothing in the render to look at.

AN ACCOUNT IS RENDERED ONLY WHERE THE ROUTE IS GRANTED SOMETHING. One bound to
nothing is indistinguishable from the floor every unnamed route already
inherits. */ -}}
{{- define "prometheus.routeAccount" -}}
{{- $p := .p -}}
{{- if ($p.serviceAccountName | default "") -}}
{{ $p.serviceAccountName }}
{{- else -}}
{{- $rbac := $p.rbac | default dict -}}
{{- if or ($rbac.clusterRoles | default list) ($rbac.bindClusterRoles | default list) -}}
{{ include "prometheus.routeServiceAccount" (dict "ctx" .ctx "route" $p.name) }}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "prometheus.routeAccountRendered" -}}
{{- $p := .p -}}
{{- if not ($p.serviceAccountName | default "") -}}
{{- $rbac := $p.rbac | default dict -}}
{{- if or ($rbac.clusterRoles | default list) ($rbac.bindClusterRoles | default list) -}}true{{- end -}}
{{- end -}}
{{- end -}}
