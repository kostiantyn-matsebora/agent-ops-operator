{{- /*
The substrate facts, resolved ONCE and readable from every subchart.

Both helpers read `.Values.global` and nothing else, deliberately: a subchart is
rendered with its own context, so a helper reaching for a top-level parent value
would resolve to the subchart's values instead of failing — silently disagreeing
with the parent about the agent's identity or its power. `global.` is the only
scope both contexts share, which is why the canonical keys live there.
*/ -}}

{{- /* The ServiceAccount every runtime pod runs as. Exactly one per release —
its RBAC IS the agent's power, whichever bundle originated the conversation. */ -}}
{{- define "agentops.runtimeServiceAccount" -}}
{{- dig "agentops" "runtime" "serviceAccountName" "" (.Values.global | default dict) | default "agentops-runtime" -}}
{{- end -}}

{{- /* The EFFECTIVE RBAC mode: never empty, always one of none|readonly|full.

Empty means readonly under demo mode and none otherwise. Defaulting it to
readonly outright would silently bind cluster `view` to the runtime SA of every
existing install on upgrade; defaulting it to none would break the demo promise
that one flag yields a working, cluster-reading agent. `full` is never inferred.

The parent's bindings and k8s-bundle's MCP derivation both call this, so they
cannot drift apart. */ -}}
{{- define "agentops.runtimeRbacMode" -}}
{{- $g := .Values.global | default dict -}}
{{- $mode := dig "agentops" "runtime" "rbacMode" "" $g -}}
{{- if not $mode -}}
{{- ternary "readonly" "none" (dig "demo" "enabled" false $g | toString | eq "true") -}}
{{- else if has $mode (list "none" "readonly" "full") -}}
{{- $mode -}}
{{- else -}}
{{- fail (printf "global.agentops.runtime.rbacMode must be \"none\", \"readonly\" or \"full\" (empty = readonly under demo, else none), got %q" $mode) -}}
{{- end -}}
{{- end -}}
