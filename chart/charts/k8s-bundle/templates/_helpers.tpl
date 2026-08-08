{{- /*
The bundle is active when it is enabled directly OR by demo mode. Helm's
`condition:` cannot express that — it evaluates only the first existing values
path, so with k8s-bundle.enabled present in defaults, demo mode could never flip
the subchart on. Hence self-gating in every template, and hence the demo flag
living under `global.` (the only scope a subchart can read).
*/ -}}
{{- define "k8s-bundle.active" -}}
{{- if or .Values.enabled (dig "demo" "enabled" false (.Values.global | default dict)) -}}
true
{{- end -}}
{{- end -}}

{{- /* The adapter's deterministic ServiceAccount, created by the SignalAdapter
reconciler. The chart binds permissions to this name; the operator grants none. */ -}}
{{- define "k8s-bundle.adapterServiceAccount" -}}
agentops-signal-{{ .Values.eventsAdapter.name }}
{{- end -}}

{{- /* The runtime ServiceAccount whose RBAC is the agent's power. */ -}}
{{- define "k8s-bundle.runtimeServiceAccount" -}}
{{ .Values.profile.runtime.serviceAccountName }}
{{- end -}}

{{- /* Deployment + Service name of the in-cluster MCP server. Fixed, because
mcp.yaml defaults an empty MCPConfig URL onto this Service. */ -}}
{{- define "k8s-bundle.mcpServerName" -}}
agentops-mcp-k8s
{{- end -}}

{{- /* The endpoint path for the declared transport. containers/kubernetes-mcp-server
serves streamable HTTP on /mcp and SSE on /sse from the same --port. */ -}}
{{- define "k8s-bundle.mcpPath" -}}
{{- if eq .Values.mcp.transport "sse" -}}/sse{{- else -}}/mcp{{- end -}}
{{- end -}}

{{- /* The MCP SERVER's ServiceAccount — deliberately NOT the runtime's. Its RBAC
is what MCP tools can reach, reviewable independently of the agent's own. */ -}}
{{- define "k8s-bundle.mcpServerServiceAccount" -}}
{{ .Values.mcpServers.serviceAccountName }}
{{- end -}}

{{- /* Whether the mutating toolset exists. Explicit values win; otherwise it
follows the deployed server, because granting tool names a --read-only server
never registers is how an allowlist rots into fiction. */ -}}
{{- define "k8s-bundle.mcpAdminEnabled" -}}
{{- $admin := .Values.mcp.toolsets.admin -}}
{{- if kindIs "bool" $admin.enabled -}}
{{- if $admin.enabled }}true{{ end -}}
{{- else if and .Values.mcpServers.enabled (not .Values.mcpServers.readOnly) -}}
true
{{- end -}}
{{- end -}}
