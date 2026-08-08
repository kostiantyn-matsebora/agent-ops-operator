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

{{- /* The profile the events Pipeline routes to: an explicit override, else the
bundle's own profile. Failing at render time beats shipping a Pipeline that can
never be Ready. */ -}}
{{- define "k8s-bundle.profileRef" -}}
{{- $ref := .Values.eventsAdapter.source.profileRef -}}
{{- if and (not $ref) .Values.profile.enabled -}}
{{- $ref = .Values.profile.name -}}
{{- end -}}
{{- if not $ref -}}
{{- fail "k8s-bundle: eventsAdapter.source.create is on but no profile resolves — set k8s-bundle.eventsAdapter.source.profileRef to an existing AgentProfile, or enable k8s-bundle.profile" -}}
{{- end -}}
{{- $ref -}}
{{- end -}}

{{- /* The runtime ServiceAccount whose RBAC is the agent's power. */ -}}
{{- define "k8s-bundle.runtimeServiceAccount" -}}
{{ .Values.profile.runtime.serviceAccountName }}
{{- end -}}
