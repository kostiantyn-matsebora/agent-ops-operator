{{- /* Active only when enabled directly: demo mode never turns this bundle on. */ -}}
{{- define "prometheus-bundle.active" -}}
{{- if .Values.enabled -}}
true
{{- end -}}
{{- end -}}

{{- /* The adapter's deterministic SA; the chart binds to it, the operator grants nothing. */ -}}
{{- define "prometheus-bundle.adapterServiceAccount" -}}
agentops-signal-{{ .Values.alertmanager.name }}
{{- end -}}

{{- /* Fixed: mcp.yaml defaults an empty MCPConfig URL onto this Service. */ -}}
{{- define "prometheus-bundle.mcpServerName" -}}
agentops-mcp-prometheus
{{- end -}}

{{- /* The server selects ONE transport per process, so path and workload must agree. */ -}}
{{- define "prometheus-bundle.mcpPath" -}}
{{- if eq .Values.mcp.transport "sse" -}}/sse{{- else -}}/mcp{{- end -}}
{{- end -}}

{{- /* Never the runtime SA — that separation is why this component exists. */ -}}
{{- define "prometheus-bundle.mcpServerServiceAccount" -}}
{{ .Values.mcpServers.serviceAccountName }}
{{- end -}}

{{- /* Wiring gate. The demo branch is inert here (active has no demo path) and is
kept only so the rule reads identically in every bundle that ships wiring. */ -}}
{{- define "prometheus-bundle.wiringActive" -}}
{{- if include "prometheus-bundle.active" . -}}
{{- if kindIs "bool" .Values.pipelines.enabled -}}
{{- if .Values.pipelines.enabled }}true{{ end -}}
{{- else if dig "demo" "enabled" false (.Values.global | default dict) -}}
true
{{- end -}}
{{- end -}}
{{- end -}}
