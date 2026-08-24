{{- /*
The bundle is active when it is enabled directly OR by demo mode. Helm's
`condition:` cannot express that — it evaluates only the first existing values
path, so with kubernetes.enabled present in defaults, demo mode could never flip
the subchart on. Hence self-gating in every template, and hence the demo flag
living under `global.` (the only scope a subchart can read).
*/ -}}
{{- define "kubernetes.active" -}}
{{- if or .Values.enabled (dig "demo" "enabled" false (.Values.global | default dict)) -}}
true
{{- end -}}
{{- end -}}

{{- /* The adapter's deterministic ServiceAccount, created by the SignalAdapter
reconciler. The chart binds permissions to this name; the operator grants none. */ -}}
{{- define "kubernetes.adapterServiceAccount" -}}
agentops-signal-{{ .Values.eventsAdapter.name }}
{{- end -}}

{{- /* Deployment + Service name of the in-cluster MCP server. Fixed, because
mcp.yaml defaults an empty MCPConfig URL onto this Service. */ -}}
{{- define "kubernetes.mcpServerName" -}}
agentops-mcp-k8s
{{- end -}}

{{- /* The endpoint path for the declared transport. containers/kubernetes-mcp-server
serves streamable HTTP on /mcp and SSE on /sse from the same --port. */ -}}
{{- define "kubernetes.mcpPath" -}}
{{- if eq .Values.mcp.transport "sse" -}}/sse{{- else -}}/mcp{{- end -}}
{{- end -}}

{{- /* The MCP SERVER's ServiceAccount — deliberately NOT the runtime's. Its RBAC
is what MCP tools can reach, reviewable independently of the agent's own. */ -}}
{{- define "kubernetes.mcpServerServiceAccount" -}}
{{ .Values.mcpServers.serviceAccountName }}
{{- end -}}

{{- /* MAY AGENTS ON THIS LANE CHANGE THE CLUSTER? Returns "true" or "".

THE ONE STATED SETTING THAT MOVES THE GROUP, and it is this bundle's own —
never a release-wide permission value. Four things follow from it, and the name
says what all four are about:

  1. the MCP server drops --read-only, so mutating tools are REGISTERED
  2. that server's ServiceAccount gets the acting grant instead of the reads
  3. the `k8s-admin` mutating toolset renders
  4. the ACTING route ships instead of the observing one

Moving as a group was always deliberate — both walls sit on ONE path, since an
agent reaches the cluster THROUGH this server, so fixing one and not the other
leaves the hole one indirection along. What was wrong was the TRIGGER: a
release-wide `rbacMode` whose name mentioned none of the four, so an install
could not read off its own values what it had granted.

EACH OF THE FOUR REMAINS INDIVIDUALLY OVERRIDABLE, and an explicit value is
absolute in both directions. None of them derives from another, so setting the
toolset does not move the server's flag and setting the server's flag does not
render the toolset. */ -}}
{{- define "kubernetes.allowMutations" -}}
{{- if .Values.allowMutations }}true{{ end -}}
{{- end -}}

{{- /* Whether the deployed server runs --read-only. Returns "true" or "".

An explicit mcpServers.readOnly wins; otherwise a read-only server unless this
bundle states `allowMutations`. `readOnly: true` under `allowMutations: true` is
the useful override: broad grants on the server that nothing can exercise. */ -}}
{{- define "kubernetes.mcpServerReadOnly" -}}
{{- if kindIs "bool" .Values.mcpServers.readOnly -}}
{{- if .Values.mcpServers.readOnly }}true{{ end -}}
{{- else if include "kubernetes.allowMutations" . -}}
{{- else -}}
true
{{- end -}}
{{- end -}}

{{- /* The server SA's RBAC mode. Explicit wins; otherwise `full` under
`allowMutations` and `readonly` without it.

READONLY IS THE FLOOR HERE, NOT `none`, and that is deliberate: an agent that
can read the cluster through MCP and do nothing at all through its own identity
is the useful shape two identities exist to offer. */ -}}
{{- define "kubernetes.mcpServerRbacMode" -}}
{{- with .Values.mcpServers.rbac.mode -}}
{{- . -}}
{{- else -}}
{{- ternary "full" "readonly" (eq (include "kubernetes.allowMutations" $) "true") -}}
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
{{- define "kubernetes.wiringActive" -}}
{{- if include "kubernetes.active" . -}}
{{- if kindIs "bool" .Values.pipelines.enabled -}}
{{- if .Values.pipelines.enabled }}true{{ end -}}
{{- else if dig "demo" "enabled" false (.Values.global | default dict) -}}
true
{{- end -}}
{{- end -}}
{{- end -}}

{{- /* Whether the OBSERVING route renders. Returns "true" or "".

Explicit wins; otherwise it follows this bundle's own `allowMutations`, for the
same reason the server's flag does: the two routes differ in one binding, so
rendering both by default would make every event open two conversations, one of
which can mutate the cluster. One stated setting picks exactly one. */ -}}
{{- define "kubernetes.observePipelineEnabled" -}}
{{- $o := .Values.pipelines.observe -}}
{{- if kindIs "bool" $o.enabled -}}
{{- if $o.enabled }}true{{ end -}}
{{- else if not (include "kubernetes.allowMutations" .) -}}
true
{{- end -}}
{{- end -}}

{{- /* Whether the ACTING route renders. Returns "true" or "". The mirror of the
above: on only under `allowMutations`, where the server registers mutating tools
and the k8s-admin toolset exists for a route to bind.

Explicit true under a read-only release still renders the route — the value is
absolute in both directions — but it binds no mutating toolset, because none was
created. Degraded and honest beats a ref to an object nobody rendered. */ -}}
{{- define "kubernetes.adminPipelineEnabled" -}}
{{- $a := .Values.pipelines.admin -}}
{{- if kindIs "bool" $a.enabled -}}
{{- if $a.enabled }}true{{ end -}}
{{- else if include "kubernetes.allowMutations" . -}}
true
{{- end -}}
{{- end -}}

{{- /* Whether the mutating toolset exists. Explicit wins; otherwise it follows
`allowMutations` — a SIBLING of the server's flag, not a consequence of it.

IT USED TO FOLLOW THE DEPLOYED SERVER, on the ground that granting tool names a
--read-only server never registers is how an allowlist rots into fiction. That
reasoning is kept as a DEFAULT — one setting keeps all four consistent — but
chaining them meant flipping the server's flag silently rendered a toolset, so
neither could be read on its own. Granting a tool the deployed server does not
register is now degraded and honest rather than impossible: the route binds a
name that resolves to nothing, and says so. */ -}}
{{- define "kubernetes.mcpAdminEnabled" -}}
{{- $admin := .Values.mcp.toolsets.admin -}}
{{- if kindIs "bool" $admin.enabled -}}
{{- if $admin.enabled }}true{{ end -}}
{{- else if and .Values.mcpServers.enabled (include "kubernetes.allowMutations" .) -}}
true
{{- end -}}
{{- end -}}

{{- /* The runtime identity for one of this bundle's routes, named AFTER the
route so `kubectl get sa` reads as the wiring does. Rendered by
pipeline-identity.yaml unless the route names its own. */ -}}
{{- define "kubernetes.routeServiceAccount" -}}
{{- printf "agentops-%s" .route -}}
{{- end -}}

{{- /* THE ACCOUNT A ROUTE OF THIS kubernetes RUNS AS, resolved ONCE.

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
{{- define "kubernetes.routeAccount" -}}
{{- $cfg := .cfg -}}
{{- $p := .p -}}
{{- $explicit := $cfg.serviceAccountName | default ($p.serviceAccountName | default "") -}}
{{- if $explicit -}}
{{ $explicit }}
{{- else -}}
{{- $rbac := $cfg.rbac | default dict -}}
{{- if or ($rbac.clusterRoles | default list) ($rbac.bindClusterRoles | default list) -}}
{{ include "kubernetes.routeServiceAccount" (dict "ctx" .ctx "route" $cfg.name) }}
{{- end -}}
{{- end -}}
{{- end -}}

{{- /* Does THIS bundle render that account? True only where the route declares a
grant AND named no account of its own — an account the operator names is theirs,
and naming is not creating. */ -}}
{{- define "kubernetes.routeAccountRendered" -}}
{{- $cfg := .cfg -}}
{{- $p := .p -}}
{{- if not ($cfg.serviceAccountName | default ($p.serviceAccountName | default "")) -}}
{{- $rbac := $cfg.rbac | default dict -}}
{{- if or ($rbac.clusterRoles | default list) ($rbac.bindClusterRoles | default list) -}}true{{- end -}}
{{- end -}}
{{- end -}}
