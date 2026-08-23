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
previously maintained by hand: an operator who grants the agent `full` and
leaves the server read-only has asked for a write-capable agent and given it no
way to write — the runtime image ships no CLI to fall back to. Every mode but
`full` — including none and unset — yields a read-only server. */ -}}
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

{{- /* Whether the bundle renders wiring of its own. Returns "true" or "".

Bundles normally ship none: a route names objects from several components and
only the parent sees them all. This bundle is the exception because it owns its
whole lane — source, profile and both toolsets — so the ONLY foreign name a
route needs is a channel, which is values-supplied and omitted when unset.

It still defaults OFF, and that default is the load-bearing part: enabling this
bundle for its adapter and profile must never silently add a second route beside
the one the install declared. Demo mode is the one path that forces it on,
because "one flag, a working install" is the whole promise, and an install that
answers nothing until you hand-write a CR does not keep it.

`null` means that rule; an explicit boolean is absolute in both directions, so
`pipelines.enabled: false` is the opt-out even under demo mode. That is why this
value is nullable and `enabled` on the other components is not — theirs never
have to disagree with demo mode. */ -}}
{{- define "k8s-bundle.wiringActive" -}}
{{- if include "k8s-bundle.active" . -}}
{{- if kindIs "bool" .Values.pipelines.enabled -}}
{{- if .Values.pipelines.enabled }}true{{ end -}}
{{- else if dig "demo" "enabled" false (.Values.global | default dict) -}}
true
{{- end -}}
{{- end -}}
{{- end -}}

{{- /* Whether the OBSERVING route renders. Returns "true" or "".

Explicit wins; otherwise it derives from the release's single posture value, for
the same reason mcpServers.readOnly does: the two routes differ in one binding,
so rendering both by default would make every event open two conversations, one
of which can mutate the cluster. Deriving picks exactly one and keeps it
consistent with the server and the toolset the same mode already decided. */ -}}
{{- define "k8s-bundle.observePipelineEnabled" -}}
{{- $o := .Values.pipelines.observe -}}
{{- if kindIs "bool" $o.enabled -}}
{{- if $o.enabled }}true{{ end -}}
{{- else if ne (include "agentops.runtimeRbacMode" .) "full" -}}
true
{{- end -}}
{{- end -}}

{{- /* Whether the ACTING route renders. Returns "true" or "". The mirror of the
above: derived on only under `full`, where the server registers mutating tools
and the k8s-admin toolset exists for a route to bind.

Explicit true under a read-only release still renders the route — the value is
absolute in both directions — but it binds no mutating toolset, because none was
created. Degraded and honest beats a ref to an object nobody rendered. */ -}}
{{- define "k8s-bundle.adminPipelineEnabled" -}}
{{- $a := .Values.pipelines.admin -}}
{{- if kindIs "bool" $a.enabled -}}
{{- if $a.enabled }}true{{ end -}}
{{- else if eq (include "agentops.runtimeRbacMode" .) "full" -}}
true
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

{{- /* The runtime identity for one of this bundle's routes, named AFTER the
route so `kubectl get sa` reads as the wiring does. Rendered by
pipeline-identity.yaml unless the route names its own. */ -}}
{{- define "k8s-bundle.routeServiceAccount" -}}
{{- printf "agentops-%s" .route -}}
{{- end -}}
