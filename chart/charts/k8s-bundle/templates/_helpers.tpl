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

{{- /* Whether the deployed server runs --read-only. Returns "true" or "".

An explicit mcpServers.readOnly wins; otherwise it DERIVES from the release's
single runtime RBAC mode, because the two are bound by an invariant operators
previously maintained by hand: a read-only server under a `full` agent pushes
every write back onto kubectl, which is the single-wall path this component
exists to replace. Every mode but `full` — including none and unset — yields a
read-only server. */ -}}
{{- define "k8s-bundle.mcpServerReadOnly" -}}
{{- if kindIs "bool" .Values.mcpServers.readOnly -}}
{{- if .Values.mcpServers.readOnly }}true{{ end -}}
{{- else if eq (include "agentops.runtimeRbacMode" .) "full" -}}
{{- else -}}
true
{{- end -}}
{{- end -}}

{{- /* The server SA's RBAC mode. Explicit wins; otherwise `full` follows a
`full` agent and everything else — none and unset included — is readonly.
`none` maps to a READONLY server on purpose: an agent that can read the cluster
through MCP and do nothing at all through its own identity is the useful shape
the two-identity design exists to offer. */ -}}
{{- define "k8s-bundle.mcpServerRbacMode" -}}
{{- with .Values.mcpServers.rbac.mode -}}
{{- . -}}
{{- else -}}
{{- ternary "full" "readonly" (eq (include "agentops.runtimeRbacMode" $) "full") -}}
{{- end -}}
{{- end -}}

{{- /* Whether the mutating toolset exists. Explicit values win; otherwise it
follows the deployed server, because granting tool names a --read-only server
never registers is how an allowlist rots into fiction. With the derivation
above, `rbacMode: full` therefore renders k8s-admin as a consequence rather than
as a fourth thing to remember. */ -}}
{{- define "k8s-bundle.mcpAdminEnabled" -}}
{{- $admin := .Values.mcp.toolsets.admin -}}
{{- if kindIs "bool" $admin.enabled -}}
{{- if $admin.enabled }}true{{ end -}}
{{- else if and .Values.mcpServers.enabled (not (include "k8s-bundle.mcpServerReadOnly" .)) -}}
true
{{- end -}}
{{- end -}}
